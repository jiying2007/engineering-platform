#!/usr/bin/env bash
set -euo pipefail
umask 077

: "${PILOT:?feature or debug required}"
: "${ENGINEERING_RUN_ID:?engineering workflow run id required}"
ENGINEERING_RECOVERY_RUN_ID="${ENGINEERING_RECOVERY_RUN_ID:-}"
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
RECOVERY_ROOT="$STATE_ROOT/recovery"
STACK_ROOT="$STATE_ROOT/stack"
CI_ROOT="$STATE_ROOT/ci"
RUNTIME_SRC="$RUNNER_TEMP/retained-pilot-base-src"
BIN_DIR="$RUNNER_TEMP/retained-pilot-base-bin"

install -d -m 0700 "$STATE_ROOT" "$ENGINEERING_ROOT" "$RECOVERY_ROOT" "$CI_ROOT" "$BIN_DIR"

run_json="$(gh api "repos/$GITHUB_REPOSITORY/actions/runs/$ENGINEERING_RUN_ID")"
test "$(printf '%s' "$run_json" | jq -er .name)" = "Retained M1 pilot engineering"
test "$(printf '%s' "$run_json" | jq -er .event)" = "workflow_dispatch"
test "$(printf '%s' "$run_json" | jq -er .status)" = "completed"
test "$(printf '%s' "$run_json" | jq -er .head_branch)" = "main"
test "$(printf '%s' "$run_json" | jq -er .path)" = ".github/workflows/retained-pilot-self-hosted-engineer.yml"
ENGINEERING_CONCLUSION="$(printf '%s' "$run_json" | jq -er .conclusion)"
SOURCE_HEAD_SHA="$(printf '%s' "$run_json" | jq -er .head_sha)"
[[ "$SOURCE_HEAD_SHA" =~ ^[0-9a-f]{40}$ ]]

RECOVERY_RECEIPT=""
case "$ENGINEERING_CONCLUSION" in
  success)
    test -z "$ENGINEERING_RECOVERY_RUN_ID"
    ;;
  failure)
    [[ "$ENGINEERING_RECOVERY_RUN_ID" =~ ^[0-9]+$ ]]
    recovery_run_json="$(gh api "repos/$GITHUB_REPOSITORY/actions/runs/$ENGINEERING_RECOVERY_RUN_ID")"
    test "$(printf '%s' "$recovery_run_json" | jq -er .name)" = "Retained M1 pilot engineering recovery"
    test "$(printf '%s' "$recovery_run_json" | jq -er .event)" = "workflow_dispatch"
    test "$(printf '%s' "$recovery_run_json" | jq -er .status)" = "completed"
    test "$(printf '%s' "$recovery_run_json" | jq -er .conclusion)" = "success"
    test "$(printf '%s' "$recovery_run_json" | jq -er .head_branch)" = "main"
    test "$(printf '%s' "$recovery_run_json" | jq -er .path)" = ".github/workflows/retained-pilot-recover.yml"
    recovery_artifact_name="retained-pilot-engineering-recovery-$PILOT-$ENGINEERING_RUN_ID-$ENGINEERING_RECOVERY_RUN_ID"
    gh run download "$ENGINEERING_RECOVERY_RUN_ID"       --repo "$GITHUB_REPOSITORY"       --name "$recovery_artifact_name"       --dir "$RECOVERY_ROOT"
    RECOVERY_RECEIPT="$RECOVERY_ROOT/recovery-receipt.json"
    test -f "$RECOVERY_RECEIPT"
    ;;
  *)
    echo "engineering run has unsupported conclusion $ENGINEERING_CONCLUSION" >&2
    exit 1
    ;;
esac

artifact_name="retained-pilot-engineering-$PILOT-$ENGINEERING_RUN_ID"
engineering_artifacts_json="$(gh api "repos/$GITHUB_REPOSITORY/actions/runs/$ENGINEERING_RUN_ID/artifacts?per_page=100")"
SOURCE_ARTIFACT_ID="$(printf '%s' "$engineering_artifacts_json" | jq -er --arg n "$artifact_name" '[.artifacts[]|select(.name==$n)] | if length==1 then .[0].id else error("engineering artifact identity missing/ambiguous") end')"
SOURCE_ARTIFACT_DIGEST="$(printf '%s' "$engineering_artifacts_json" | jq -er --arg n "$artifact_name" '[.artifacts[]|select(.name==$n)] | if length==1 then .[0].digest else error("engineering artifact digest missing/ambiguous") end')"
SOURCE_ARTIFACT_EXPIRED="$(printf '%s' "$engineering_artifacts_json" | jq -er --arg n "$artifact_name" '[.artifacts[]|select(.name==$n)] | if length==1 then (. [0].expired|tostring) else error("engineering artifact expiry missing/ambiguous") end')"
test "$SOURCE_ARTIFACT_EXPIRED" = false
[[ "$SOURCE_ARTIFACT_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]]

gh run download "$ENGINEERING_RUN_ID"   --repo "$GITHUB_REPOSITORY"   --name "$artifact_name"   --dir "$ENGINEERING_ROOT"

ENGINEERING_STATE="$ENGINEERING_ROOT/engineering-state.json"
test -f "$ENGINEERING_STATE"
test "$(jq -er .version "$ENGINEERING_STATE")" = 2
test "$(jq -er .pilot "$ENGINEERING_STATE")" = "$PILOT"
PROVIDER_JSON="$(jq -cS .provider "$ENGINEERING_STATE")"
printf '%s' "$PROVIDER_JSON" | jq -e '
  .version==1 and
  .provider_id=="openai-codex" and
  ((.credential_mode=="chatgpt-session" and .execution_mode=="trusted-self-hosted") or
   (.credential_mode=="workload-identity" and .execution_mode=="unattended"))
' >/dev/null
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
test "$SOURCE_HEAD_SHA" = "$BASE_COMMIT"

if [ "$PILOT" = debug ]; then
  DEBUG_REPRODUCTION_RUN_ID="$(jq -er .debug_reproduction_run_id "$ENGINEERING_STATE")"
  DEBUG_REPRODUCTION_RECEIPT_DIGEST="$(jq -er .debug_reproduction_receipt_digest "$ENGINEERING_STATE")"
  [[ "$DEBUG_REPRODUCTION_RUN_ID" =~ ^[0-9]+$ ]]
  [[ "$DEBUG_REPRODUCTION_RECEIPT_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]]

  DEBUG_REPRO_BINDING="$ENGINEERING_ROOT/debug-reproduction-binding.json"
  DEBUG_REPRO_RECEIPT="$ENGINEERING_ROOT/debug-reproduction/reproduction-receipt.json"
  DEBUG_REPRO_TEST="$ENGINEERING_ROOT/debug-reproduction/reproduction-test.go"
  DEBUG_REPRO_OUTPUT="$ENGINEERING_ROOT/debug-reproduction/go-test-output.txt"
  DEBUG_TASK="$ENGINEERING_ROOT/task.json"
  for required in "$DEBUG_REPRO_BINDING" "$DEBUG_REPRO_RECEIPT" "$DEBUG_REPRO_TEST" "$DEBUG_REPRO_OUTPUT" "$DEBUG_TASK"; do
    test -f "$required"
  done

  jq -e \
    --argjson run_id "$DEBUG_REPRODUCTION_RUN_ID" \
    --arg base "$BASE_COMMIT" \
    --arg receipt_digest "$DEBUG_REPRODUCTION_RECEIPT_DIGEST" \
    '.reproduction_run_id==$run_id and
     .base_commit==$base and
     .receipt_digest==$receipt_digest and
     .reproduction_confirmed==true' \
    "$DEBUG_REPRO_BINDING" >/dev/null

  test "sha256:$(sha256sum "$DEBUG_REPRO_RECEIPT" | awk '{print $1}')" = "$DEBUG_REPRODUCTION_RECEIPT_DIGEST"
  jq -e \
    --arg base "$BASE_COMMIT" \
    '.version==1 and
     .reproduction=="m1-debug-firmware-identity" and
     .base_commit==$base and
     .reproduction_confirmed==true and
     .test_exit_code==1 and
     .expected_status=="BLOCKED" and
     .observed_status=="READY" and
     .firmware_identity=="not-a-digest"' \
    "$DEBUG_REPRO_RECEIPT" >/dev/null
  test "sha256:$(sha256sum "$DEBUG_REPRO_TEST" | awk '{print $1}')" = "$(jq -er .test_digest "$DEBUG_REPRO_RECEIPT")"
  test "sha256:$(sha256sum "$DEBUG_REPRO_OUTPUT" | awk '{print $1}')" = "$(jq -er .output_digest "$DEBUG_REPRO_RECEIPT")"
  jq -e '.contract.task_type=="DEBUG" and .material.has_reproduction==true' "$DEBUG_TASK" >/dev/null

  debug_run_json="$(gh api "repos/$GITHUB_REPOSITORY/actions/runs/$DEBUG_REPRODUCTION_RUN_ID")"
  test "$(printf '%s' "$debug_run_json" | jq -er .name)" = "Retained Debug firmware identity reproduction"
  test "$(printf '%s' "$debug_run_json" | jq -er .status)" = completed
  test "$(printf '%s' "$debug_run_json" | jq -er .conclusion)" = success
  test "$(printf '%s' "$debug_run_json" | jq -er .head_sha)" = "$BASE_COMMIT"
fi

CODEX_STATUS="$ENGINEERING_ROOT/codex-status.json"
PUBLICATION_RECEIPT="$ENGINEERING_ROOT/publication-receipt.json"
PUBLICATION_PREFLIGHT="$ENGINEERING_ROOT/preflight-before-publication.json"
for required in "$CODEX_STATUS" "$PUBLICATION_RECEIPT" "$PUBLICATION_PREFLIGHT" "$ENGINEERING_ROOT/result.bundle" "$ENGINEERING_ROOT/core.dump"; do
  test -f "$required"
done
test -s "$ENGINEERING_ROOT/result.bundle"
test -s "$ENGINEERING_ROOT/core.dump"
test "sha256:$(sha256sum "$ENGINEERING_ROOT/result.bundle" | awk '{print $1}')" = "$BUNDLE_DIGEST"

jq -e   --arg base "$BASE_COMMIT"   --arg result "$RESULT_COMMIT"   --arg result_digest "$RESULT_DIGEST"   --arg bundle "$BUNDLE_DIGEST"   --argjson provider "$PROVIDER_JSON"   '.state=="FINISHED" and
   .receipt.kind=="WORKER_ATTESTED_CODEX_EXECUTION" and
   .receipt.result_digest==$result_digest and
   .receipt.result.change.base_commit==$base and
   .receipt.result.change.result_commit==$result and
   .receipt.result.change.bundle_digest==$bundle and
   .receipt.result.codex.provider==$provider and
   .receipt.result.codex.turn_status=="completed" and
   .receipt.result.codex.approval_requests==0 and
   (if $provider.credential_mode=="chatgpt-session"
      then (.receipt.result.codex.credential_bootstrap_removed_before_turn==true and .receipt.result.codex.assertion_removed_before_turn==false)
      else (.receipt.result.codex.assertion_removed_before_turn==true and .receipt.result.codex.credential_bootstrap_removed_before_turn==false)
    end)'   "$CODEX_STATUS" >/dev/null

test "$(jq -er .publication "$PUBLICATION_PREFLIGHT")" = READY
test "$(jq -er .result "$PUBLICATION_RECEIPT")" = CONFIRMED
test "$(jq -er .external_ref "$PUBLICATION_RECEIPT")" = "$PR_URL"
OBSERVED_STATE="$(jq -er .observed_state "$PUBLICATION_RECEIPT")"
printf '%s' "$OBSERVED_STATE" | jq -e   --arg base "$BASE_COMMIT"   --arg result "$RESULT_COMMIT"   --arg branch "$PR_BRANCH"   --argjson number "$PR_NUMBER"   '.base_ref=="main" and
   .base_commit==$base and
   .result_commit==$result and
   .branch==$branch and
   .pull_request_number==$number and
   .pull_request_state=="open"' >/dev/null

if [ -n "$RECOVERY_RECEIPT" ]; then
  jq -e     --arg pilot "$PILOT"     --arg source_name "$artifact_name"     --arg source_digest "$SOURCE_ARTIFACT_DIGEST"     --arg base "$BASE_COMMIT"     --arg result "$RESULT_COMMIT"     --arg result_digest "$RESULT_DIGEST"     --arg bundle "$BUNDLE_DIGEST"     --arg pr_url "$PR_URL"     --arg pr_branch "$PR_BRANCH"     --argjson provider "$PROVIDER_JSON"     --argjson source_run "$ENGINEERING_RUN_ID"     --argjson source_artifact_id "$SOURCE_ARTIFACT_ID"     --argjson recovery_run "$ENGINEERING_RECOVERY_RUN_ID"     --argjson pr_number "$PR_NUMBER"     '.version==2 and
     .provider==$provider and
     .pilot==$pilot and
     .source_engineering_run_id==$source_run and
     .source_artifact.name==$source_name and
     .source_artifact.id==$source_artifact_id and
     .source_artifact.digest==$source_digest and
     .model_phase=="FINISHED" and
     .publication=="CONFIRMED" and
     .base_commit==$base and
     .result_commit==$result and
     .codex_result_digest==$result_digest and
     .bundle_digest==$bundle and
     .pull_request.number==$pr_number and
     .pull_request.url==$pr_url and
     .pull_request.branch==$pr_branch and
     .recovery_run_id==$recovery_run and
     .model_replay==false'     "$RECOVERY_RECEIPT" >/dev/null
fi

pr_json="$(gh api "repos/$GITHUB_REPOSITORY/pulls/$PR_NUMBER")"
printf '%s' "$pr_json" | jq -e   --arg base "$BASE_COMMIT"   --arg result "$RESULT_COMMIT"   --arg branch "$PR_BRANCH"   '.state=="open" and
   .base.ref=="main" and
   .base.sha==$base and
   .head.sha==$result and
   .head.ref==$branch and
   .head.repo.id==.base.repo.id' >/dev/null
test "$(printf '%s' "$pr_json" | jq -er .html_url)" = "$PR_URL"

git -C "$GITHUB_WORKSPACE" cat-file -e "$BASE_COMMIT^{commit}"
rm -rf "$RUNTIME_SRC"
git -C "$GITHUB_WORKSPACE" worktree add --detach "$RUNTIME_SRC" "$BASE_COMMIT"

cleanup() {
  if [ -n "${CONTROL_PID:-}" ] && kill -0 "$CONTROL_PID" 2>/dev/null; then
    kill "$CONTROL_PID" 2>/dev/null || true
    wait "$CONTROL_PID" 2>/dev/null || true
  fi
  rm -f "$STATE_ROOT/github-token"
  # Review bootstraps fresh PKI; disposable keys/config must not enter transport.
  rm -rf -- "$STACK_ROOT"
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

bash "$GITHUB_WORKSPACE/examples/pilots/local-stack/run-control.sh" \
  "$STACK_ROOT/operator/control-plane.env" \
  "$CONTROL" \
  0 \
  >"$STATE_ROOT/control-plane.log" 2>&1 &
CONTROL_PID=$!

for _ in $(seq 1 60); do
  if bash "$GITHUB_WORKSPACE/examples/pilots/local-stack/health.sh" "$STACK_ROOT" verifier >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$CONTROL_PID" 2>/dev/null; then
    cat "$STATE_ROOT/control-plane.log" >&2 || true
    exit 1
  fi
  sleep 1
done
bash "$GITHUB_WORKSPACE/examples/pilots/local-stack/health.sh" "$STACK_ROOT" verifier >/dev/null

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
  expired="$(printf '%s' "$artifacts_json" | jq -er --arg n "$name" '[.artifacts[]|select(.name==$n)] | if length==1 then (.[0].expired|tostring) else error("artifact identity missing/ambiguous") end')"
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
download_artifact_zip "codex-compatibility-qualification-$RESULT_COMMIT" "$CODEX_ZIP" > "$CI_ROOT/codex.digest"
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

jq -n   --arg pilot "$PILOT"   --arg engineering_run_id "$ENGINEERING_RUN_ID"   --arg engineering_recovery_run_id "$ENGINEERING_RECOVERY_RUN_ID"   --arg ci_run_id "$CI_RUN_ID"   --arg base_commit "$BASE_COMMIT"   --arg result_commit "$RESULT_COMMIT"   --arg delivery_id "$DELIVERY_ID"   --arg verification_id "$VERIFICATION_ID"   --arg pr_url "$PR_URL"   --argjson provider "$PROVIDER_JSON"   '{
    version:2,
    provider:$provider,
    pilot:$pilot,
    engineering_run_id:($engineering_run_id|tonumber),
    engineering_recovery_run_id:(if $engineering_recovery_run_id=="" then null else ($engineering_recovery_run_id|tonumber) end),
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
