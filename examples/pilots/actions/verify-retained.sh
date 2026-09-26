#!/usr/bin/env bash
set -euo pipefail
umask 077

: "${PILOT:?feature or debug required}"
: "${ENGINEERING_RUN_ID:?engineering workflow run id required}"
: "${GH_TOKEN:?GitHub read token required}"
: "${GITHUB_WORKSPACE:?workspace required}"
: "${RUNNER_TEMP:?runner temp required}"
: "${GITHUB_REPOSITORY:?repository required}"

test "$GITHUB_REPOSITORY" = "jiying2007/engineering-platform"

case "$PILOT" in
  feature)
    RUN_ID="m1-feature-routing-run"
    DELIVERY_ID="m1-feature-routing-delivery"
    VERIFICATION_ID="m1-feature-routing-verification"
    PILOT_DIR_NAME="feature-routing"
    CODEX_REQUIREMENT="feature-codex"
    GIT_REQUIREMENT="feature-git"
    CI_REQUIREMENT="feature-ci"
    CODEX_EVIDENCE="feature-codex-evidence"
    GIT_EVIDENCE="feature-git-evidence"
    CI_EVIDENCE="feature-ci-evidence"
    ;;
  debug)
    RUN_ID="m1-debug-firmware-identity-run"
    DELIVERY_ID="m1-debug-firmware-identity-delivery"
    VERIFICATION_ID="m1-debug-firmware-identity-verification"
    PILOT_DIR_NAME="debug-firmware-identity"
    CODEX_REQUIREMENT="debug-codex"
    GIT_REQUIREMENT="debug-git"
    CI_REQUIREMENT="debug-ci"
    CODEX_EVIDENCE="debug-codex-evidence"
    GIT_EVIDENCE="debug-git-evidence"
    CI_EVIDENCE="debug-ci-evidence"
    ;;
  *)
    echo "unsupported pilot $PILOT" >&2
    exit 2
    ;;
esac

STATE_ROOT="$RUNNER_TEMP/retained-pilot-verification"
ENGINEERING_ROOT="$STATE_ROOT/engineering"
STACK_ROOT="$STATE_ROOT/stack"
CI_ROOT="$STATE_ROOT/ci"
RUNTIME_SRC="$RUNNER_TEMP/retained-pilot-base-src"
BIN_DIR="$RUNNER_TEMP/retained-pilot-base-bin"

install -d -m 0700 "$STATE_ROOT" "$ENGINEERING_ROOT" "$CI_ROOT" "$BIN_DIR"

run_json="$(gh api "repos/$GITHUB_REPOSITORY/actions/runs/$ENGINEERING_RUN_ID")"
test "$(printf '%s' "$run_json" | jq -er .name)" = "Retained M1 pilot engineering"
test "$(printf '%s' "$run_json" | jq -er .event)" = "workflow_dispatch"
test "$(printf '%s' "$run_json" | jq -er .status)" = "completed"
test "$(printf '%s' "$run_json" | jq -er .conclusion)" = "success"

artifact_name="retained-pilot-engineering-$PILOT-$ENGINEERING_RUN_ID"
gh run download "$ENGINEERING_RUN_ID"   --repo "$GITHUB_REPOSITORY"   --name "$artifact_name"   --dir "$ENGINEERING_ROOT"

ENGINEERING_STATE="$ENGINEERING_ROOT/engineering-state.json"
test -f "$ENGINEERING_STATE"
test "$(jq -er .version "$ENGINEERING_STATE")" = 1
test "$(jq -er .pilot "$ENGINEERING_STATE")" = "$PILOT"
test "$(jq -er .run_id "$ENGINEERING_STATE")" = "$RUN_ID"
test "$(jq -er .github_engineering_run_id "$ENGINEERING_STATE")" = "$ENGINEERING_RUN_ID"

BASE_COMMIT="$(jq -er .base_commit "$ENGINEERING_STATE")"
RESULT_COMMIT="$(jq -er .result_commit "$ENGINEERING_STATE")"
RESULT_DIGEST="$(jq -er .codex_result_digest "$ENGINEERING_STATE")"
BUNDLE_DIGEST="$(jq -er .bundle_digest "$ENGINEERING_STATE")"
PROFILE_DIGEST="$(jq -er .profile_digest "$ENGINEERING_STATE")"
PR_NUMBER="$(jq -er .pull_request.number "$ENGINEERING_STATE")"
PR_BRANCH="$(jq -er .pull_request.branch "$ENGINEERING_STATE")"
PR_URL="$(jq -er .pull_request.url "$ENGINEERING_STATE")"

[[ "$BASE_COMMIT" =~ ^[0-9a-f]{40}$ ]]
[[ "$RESULT_COMMIT" =~ ^[0-9a-f]{40}$ ]]
[[ "$RESULT_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]]
[[ "$BUNDLE_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]]
[[ "$PROFILE_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]]
test "$BASE_COMMIT" != "$RESULT_COMMIT"
test "$PR_URL" = "https://github.com/$GITHUB_REPOSITORY/pull/$PR_NUMBER"

git -C "$GITHUB_WORKSPACE" cat-file -e "$BASE_COMMIT^{commit}"
rm -rf "$RUNTIME_SRC"
git -C "$GITHUB_WORKSPACE" worktree add --detach "$RUNTIME_SRC" "$BASE_COMMIT"

cleanup() {
  if [ -n "${CONTROL_PID:-}" ] && kill -0 "$CONTROL_PID" 2>/dev/null; then
    kill "$CONTROL_PID" 2>/dev/null || true
    wait "$CONTROL_PID" 2>/dev/null || true
  fi
  rm -f "$STATE_ROOT/github-token"
  git -C "$GITHUB_WORKSPACE" worktree remove --force "$RUNTIME_SRC" >/dev/null 2>&1 || true
}
trap cleanup EXIT

(
  cd "$RUNTIME_SRC"
  go build -trimpath -o "$BIN_DIR/control-plane" ./cmd/control-plane
  go build -trimpath -o "$BIN_DIR/eng" ./cmd/eng
)

ENG="$BIN_DIR/eng"
CONTROL="$BIN_DIR/control-plane"

bash "$GITHUB_WORKSPACE/examples/pilots/local-stack/bootstrap.sh"   "$STACK_ROOT" "$PROFILE_DIGEST" worker/codex-pilot

cmp "$ENGINEERING_ROOT/access-policy.json" "$STACK_ROOT/operator/access-policy.json"

docker run --rm --network host   -e PGPASSWORD=postgres   -v "$ENGINEERING_ROOT:/engineering:ro"   postgres:17-alpine   pg_restore -h 127.0.0.1 -p 55432 -U postgres -d engineering_platform     --clean --if-exists --no-owner /engineering/core.dump

set -a
. "$STACK_ROOT/operator/control-plane.env"
set +a
export AUTO_MIGRATE=0

"$CONTROL" >"$STATE_ROOT/control-plane.log" 2>&1 &
CONTROL_PID=$!

for _ in $(seq 1 60); do
  if curl -fsS --cacert "$STACK_ROOT/pki/ca.crt" https://127.0.0.1:18443/healthz >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$CONTROL_PID" 2>/dev/null; then
    cat "$STATE_ROOT/control-plane.log" >&2 || true
    exit 1
  fi
  sleep 1
done
curl -fsS --cacert "$STACK_ROOT/pki/ca.crt" https://127.0.0.1:18443/healthz >/dev/null

git -C "$GITHUB_WORKSPACE" fetch --no-tags "https://github.com/$GITHUB_REPOSITORY.git"   "refs/heads/$PR_BRANCH:refs/ep/retained-result"
test "$(git -C "$GITHUB_WORKSPACE" rev-parse refs/ep/retained-result^{commit})" = "$RESULT_COMMIT"
git -C "$GITHUB_WORKSPACE" merge-base --is-ancestor "$BASE_COMMIT" "$RESULT_COMMIT"

CI_RUN_ID="$(
  gh run list     --repo "$GITHUB_REPOSITORY"     --workflow ci.yml     --event pull_request     --branch "$PR_BRANCH"     --limit 30     --json databaseId,headSha,status,conclusion,createdAt     --jq '.[] | select(.headSha=="'"$RESULT_COMMIT"'" and .status=="completed" and .conclusion=="success") | .databaseId' |
    head -n 1
)"
test -n "$CI_RUN_ID"
printf '%s
' "$CI_RUN_ID" > "$STATE_ROOT/ci-run-id.txt"

artifacts_json="$(gh api "repos/$GITHUB_REPOSITORY/actions/runs/$CI_RUN_ID/artifacts?per_page=100")"

download_artifact_zip() {
  local name="$1"
  local output="$2"
  local id digest size expired
  id="$(printf '%s' "$artifacts_json" | jq -er --arg n "$name" '[.artifacts[]|select(.name==$n)] | if length==1 then .[0].id else error("artifact identity missing/ambiguous") end')"
  digest="$(printf '%s' "$artifacts_json" | jq -er --arg n "$name" '[.artifacts[]|select(.name==$n)] | if length==1 then .[0].digest else error("artifact identity missing/ambiguous") end')"
  size="$(printf '%s' "$artifacts_json" | jq -er --arg n "$name" '[.artifacts[]|select(.name==$n)] | if length==1 then .[0].size_in_bytes else error("artifact identity missing/ambiguous") end')"
  expired="$(printf '%s' "$artifacts_json" | jq -er --arg n "$name" '[.artifacts[]|select(.name==$n)] | if length==1 then .[0].expired else error("artifact identity missing/ambiguous") end')"
  test "$expired" = false
  gh api "repos/$GITHUB_REPOSITORY/actions/artifacts/$id/zip" > "$output"
  test "$(stat -c%s "$output")" = "$size"
  test "sha256:$(sha256sum "$output" | awk '{print $1}')" = "$digest"
  printf '%s
' "$digest"
}

BINARIES_ZIP="$CI_ROOT/engineering-binaries.zip"
CODEX_ZIP="$CI_ROOT/codex-qualification.zip"
ENVELOPE_ZIP="$CI_ROOT/trusted-ci-evidence.zip"

download_artifact_zip "engineering-binaries-$RESULT_COMMIT" "$BINARIES_ZIP" > "$CI_ROOT/binaries.digest"
download_artifact_zip "codex-0.155.0-qualification-$RESULT_COMMIT" "$CODEX_ZIP" > "$CI_ROOT/codex.digest"
CI_ENVELOPE_DIGEST="$(download_artifact_zip "trusted-ci-evidence-$RESULT_COMMIT" "$ENVELOPE_ZIP")"

. "$STACK_ROOT/clients/owner.env"
"$ENG" api GET "/api/v1/runs/$RUN_ID" > "$STATE_ROOT/run-before-complete.json"
EXECUTION_EPOCH="$(jq -er .run.current_epoch "$STATE_ROOT/run-before-complete.json")"

PILOT_DIR="$RUNTIME_SRC/examples/pilots/$PILOT_DIR_NAME"

sed -e "s/__EXECUTION_EPOCH__/$EXECUTION_EPOCH/g"   "$PILOT_DIR/complete.json.tmpl" > "$STATE_ROOT/complete.json"

"$ENG" api POST "/api/v1/runs/$RUN_ID/complete" "$STATE_ROOT/complete.json" > "$STATE_ROOT/run-completed.json"

"$ENG" git-change-manifest   --repository "$GITHUB_WORKSPACE"   --base "$BASE_COMMIT"   --result "$RESULT_COMMIT"   --output "$STATE_ROOT/git-change-manifest.json"   > "$STATE_ROOT/git-change-manifest-response.json"

GIT_MANIFEST_DIGEST="$(jq -er .artifact_digest "$STATE_ROOT/git-change-manifest-response.json")"

"$ENG" codex-receipt-digest --run "$RUN_ID" > "$STATE_ROOT/codex-receipt-digest.json"
CODEX_RECEIPT_DIGEST="$(jq -er .artifact_digest "$STATE_ROOT/codex-receipt-digest.json")"
test "$(jq -er .bundle_digest "$STATE_ROOT/codex-receipt-digest.json")" = "$BUNDLE_DIGEST"
test "sha256:$(sha256sum "$ENGINEERING_ROOT/result.bundle" | awk '{print $1}')" = "$BUNDLE_DIGEST"

sed   -e "s/__RESULT_COMMIT__/$RESULT_COMMIT/g"   -e "s#__CODEX_RECEIPT_DIGEST__#$CODEX_RECEIPT_DIGEST#g"   -e "s#__BUNDLE_DIGEST__#$BUNDLE_DIGEST#g"   -e "s#__GIT_MANIFEST_DIGEST__#$GIT_MANIFEST_DIGEST#g"   -e "s#__CI_ENVELOPE_DIGEST__#$CI_ENVELOPE_DIGEST#g"   "$PILOT_DIR/delivery.json.tmpl" > "$STATE_ROOT/delivery.json"

"$ENG" api POST /api/v1/deliveries "$STATE_ROOT/delivery.json" > "$STATE_ROOT/delivery-response.json"
test "$(jq -er .delivery_receipt_id "$STATE_ROOT/delivery-response.json")" = "$DELIVERY_ID"

. "$STACK_ROOT/clients/codex-evidence.env"
"$ENG" import-codex-evidence   --delivery "$DELIVERY_ID"   --requirement "$CODEX_REQUIREMENT"   --evidence "$CODEX_EVIDENCE"   --receipt-artifact codex-execution-receipt   --bundle-artifact codex-result-bundle   --bundle "$ENGINEERING_ROOT/result.bundle"   > "$STATE_ROOT/codex-evidence.json"

. "$STACK_ROOT/clients/git-evidence.env"
"$ENG" import-git-change-evidence   --repository "$GITHUB_WORKSPACE"   --manifest "$STATE_ROOT/git-change-manifest.json"   --delivery "$DELIVERY_ID"   --requirement "$GIT_REQUIREMENT"   --evidence "$GIT_EVIDENCE"   --artifact git-change-manifest   > "$STATE_ROOT/git-evidence.json"

printf '%s' "$GH_TOKEN" > "$STATE_ROOT/github-token"
chmod 0600 "$STATE_ROOT/github-token"
unset GH_TOKEN

. "$STACK_ROOT/clients/ci-evidence.env"
GITHUB_TOKEN_FILE="$STATE_ROOT/github-token" "$ENG" import-ci-evidence   --delivery "$DELIVERY_ID"   --requirement "$CI_REQUIREMENT"   --evidence "$CI_EVIDENCE"   --artifact ci-provenance   --envelope-zip "$ENVELOPE_ZIP"   --binaries-zip "$BINARIES_ZIP"   --codex-zip "$CODEX_ZIP"   > "$STATE_ROOT/ci-evidence.json"

rm -f "$STATE_ROOT/github-token"

. "$STACK_ROOT/clients/verifier.env"
"$ENG" api POST /api/v1/verifications "$PILOT_DIR/verification.json" > "$STATE_ROOT/verification-response.json"
test "$(jq -er .verification_report_id "$STATE_ROOT/verification-response.json")" = "$VERIFICATION_ID"
test "$(jq -er .result "$STATE_ROOT/verification-response.json")" = PASS

kill "$CONTROL_PID"
wait "$CONTROL_PID" || true
CONTROL_PID=""

docker run --rm --network host   -e PGPASSWORD=postgres   -v "$STATE_ROOT:/state"   postgres:17-alpine   pg_dump -h 127.0.0.1 -p 55432 -U postgres -d engineering_platform     -Fc -f /state/core-verification.dump

test -s "$STATE_ROOT/core-verification.dump"

jq -n   --arg pilot "$PILOT"   --arg engineering_run_id "$ENGINEERING_RUN_ID"   --arg ci_run_id "$CI_RUN_ID"   --arg base_commit "$BASE_COMMIT"   --arg result_commit "$RESULT_COMMIT"   --arg delivery_id "$DELIVERY_ID"   --arg verification_id "$VERIFICATION_ID"   --arg pr_url "$PR_URL"   '{
    version:1,
    pilot:$pilot,
    engineering_run_id:($engineering_run_id|tonumber),
    ci_run_id:($ci_run_id|tonumber),
    base_commit:$base_commit,
    result_commit:$result_commit,
    delivery_receipt_id:$delivery_id,
    verification_report_id:$verification_id,
    pull_request_url:$pr_url,
    verification:"PASS",
    next_gate:"INDEPENDENT_REVIEW"
  }' > "$STATE_ROOT/verification-state.json"

echo "retained pilot verification: PASS"
echo "pr_url=$PR_URL"
echo "next=independent review"
