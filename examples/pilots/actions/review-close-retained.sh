#!/usr/bin/env bash
set -euo pipefail
umask 077

: "${PILOT:?feature or debug required}"
: "${VERIFICATION_RUN_ID:?verification workflow run id required}"
: "${REVIEW_RESULT:?PASS or FAIL required}"
: "${REVIEW_FINDINGS_JSON:?findings JSON required}"
: "${REVIEW_KNOWN_LIMITS_JSON:?known limits JSON required}"
: "${GH_TOKEN:?GitHub read token required}"
: "${GITHUB_ACTOR:?GitHub reviewer actor required}"
: "${GITHUB_WORKSPACE:?workspace required}"
: "${RUNNER_TEMP:?runner temp required}"
: "${GITHUB_REPOSITORY:?repository required}"

test "$GITHUB_REPOSITORY" = "jiying2007/engineering-platform"
case "$REVIEW_RESULT" in
  PASS|FAIL) ;;
  *) echo "review result must be PASS or FAIL" >&2; exit 2 ;;
esac

case "$PILOT" in
  feature)
    WORK_ID="m1-feature-routing-work"
    DELIVERY_ID="m1-feature-routing-delivery"
    VERIFICATION_ID="m1-feature-routing-verification"
    REVIEW_ID="m1-feature-routing-review"
    PILOT_DIR_NAME="feature-routing"
    ;;
  debug)
    WORK_ID="m1-debug-firmware-identity-work"
    DELIVERY_ID="m1-debug-firmware-identity-delivery"
    VERIFICATION_ID="m1-debug-firmware-identity-verification"
    REVIEW_ID="m1-debug-firmware-identity-review"
    PILOT_DIR_NAME="debug-firmware-identity"
    ;;
  *) echo "unsupported pilot $PILOT" >&2; exit 2 ;;
esac

printf '%s' "$REVIEW_FINDINGS_JSON" | jq -e 'type=="array"' >/dev/null
printf '%s' "$REVIEW_KNOWN_LIMITS_JSON" | jq -e 'type=="array"' >/dev/null

STATE_ROOT="$RUNNER_TEMP/retained-pilot-review"
VERIFICATION_ROOT="$STATE_ROOT/verification"
STACK_ROOT="$STATE_ROOT/stack"
RUNTIME_SRC="$RUNNER_TEMP/retained-pilot-review-base-src"
BIN_DIR="$RUNNER_TEMP/retained-pilot-review-base-bin"

install -d -m 0700 "$STATE_ROOT" "$VERIFICATION_ROOT" "$BIN_DIR"

run_json="$(gh api "repos/$GITHUB_REPOSITORY/actions/runs/$VERIFICATION_RUN_ID")"
test "$(printf '%s' "$run_json" | jq -er .name)" = "Retained M1 pilot verification"
test "$(printf '%s' "$run_json" | jq -er .event)" = "workflow_dispatch"
test "$(printf '%s' "$run_json" | jq -er .status)" = completed
test "$(printf '%s' "$run_json" | jq -er .conclusion)" = success

artifacts_json="$(gh api "repos/$GITHUB_REPOSITORY/actions/runs/$VERIFICATION_RUN_ID/artifacts?per_page=100")"
artifact_name="$(
  printf '%s' "$artifacts_json" |
  jq -er     --arg prefix "retained-pilot-verification-$PILOT-"     --arg suffix "-$VERIFICATION_RUN_ID"     '[.artifacts[] | select((.name|startswith($prefix)) and (.name|endswith($suffix)) and (.expired==false))] |
     if length==1 then .[0].name else error("verification artifact missing or ambiguous") end'
)"

gh run download "$VERIFICATION_RUN_ID"   --repo "$GITHUB_REPOSITORY"   --name "$artifact_name"   --dir "$VERIFICATION_ROOT"

VERIFICATION_STATE="$VERIFICATION_ROOT/verification-state.json"
test -f "$VERIFICATION_STATE"
test "$(jq -er .version "$VERIFICATION_STATE")" = 1
test "$(jq -er .pilot "$VERIFICATION_STATE")" = "$PILOT"
test "$(jq -er .verification "$VERIFICATION_STATE")" = PASS
test "$(jq -er .delivery_receipt_id "$VERIFICATION_STATE")" = "$DELIVERY_ID"
test "$(jq -er .verification_report_id "$VERIFICATION_STATE")" = "$VERIFICATION_ID"

ENGINEERING_RUN_ID="$(jq -er .engineering_run_id "$VERIFICATION_STATE")"
BASE_COMMIT="$(jq -er .base_commit "$VERIFICATION_STATE")"
RESULT_COMMIT="$(jq -er .result_commit "$VERIFICATION_STATE")"
ENGINEERING_STATE="$VERIFICATION_ROOT/engineering/engineering-state.json"
test -f "$ENGINEERING_STATE"
test "$(jq -er .github_engineering_run_id "$ENGINEERING_STATE")" = "$ENGINEERING_RUN_ID"
PROFILE_DIGEST="$(jq -er .profile_digest "$ENGINEERING_STATE")"

ENGINEERING_ACTOR="$(gh api "repos/$GITHUB_REPOSITORY/actions/runs/$ENGINEERING_RUN_ID" --jq .actor.login)"
test -n "$ENGINEERING_ACTOR"
if [ "$GITHUB_ACTOR" = "$ENGINEERING_ACTOR" ]; then
  echo "independent review requires a different GitHub actor from engineering execution" >&2
  exit 1
fi
printf '%s
' "$ENGINEERING_ACTOR" > "$STATE_ROOT/engineering-actor.txt"
printf '%s
' "$GITHUB_ACTOR" > "$STATE_ROOT/reviewer-actor.txt"

[[ "$BASE_COMMIT" =~ ^[0-9a-f]{40}$ ]]
[[ "$RESULT_COMMIT" =~ ^[0-9a-f]{40}$ ]]
[[ "$PROFILE_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]]

git -C "$GITHUB_WORKSPACE" cat-file -e "$BASE_COMMIT^{commit}"
rm -rf "$RUNTIME_SRC"
git -C "$GITHUB_WORKSPACE" worktree add --detach "$RUNTIME_SRC" "$BASE_COMMIT"

cleanup() {
  if [ -n "${CONTROL_PID:-}" ] && kill -0 "$CONTROL_PID" 2>/dev/null; then
    kill "$CONTROL_PID" 2>/dev/null || true
    wait "$CONTROL_PID" 2>/dev/null || true
  fi
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

bash "$RUNTIME_SRC/examples/pilots/local-stack/bootstrap.sh"   "$STACK_ROOT" "$PROFILE_DIGEST" worker/codex-pilot

cmp "$VERIFICATION_ROOT/engineering/access-policy.json" "$STACK_ROOT/operator/access-policy.json"

docker run --rm --network host   -e PGPASSWORD=postgres   -v "$VERIFICATION_ROOT:/verification:ro"   postgres:17-alpine   pg_restore -h 127.0.0.1 -p 55432 -U postgres -d engineering_platform     --clean --if-exists --no-owner /verification/core-verification.dump

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

PILOT_DIR="$RUNTIME_SRC/examples/pilots/$PILOT_DIR_NAME"

jq -n   --arg review_id "$REVIEW_ID"   --arg delivery_id "$DELIVERY_ID"   --arg verification_id "$VERIFICATION_ID"   --arg reviewer "urn:engineering-platform:reviewer:pilot"   --arg result "$REVIEW_RESULT"   --argjson findings "$REVIEW_FINDINGS_JSON"   --argjson known_limits "$REVIEW_KNOWN_LIMITS_JSON"   '{
    review_report_id:$review_id,
    delivery_receipt_id:$delivery_id,
    verification_report_id:$verification_id,
    reviewer:$reviewer,
    result:$result,
    findings:$findings,
    known_limits:$known_limits
  }' > "$STATE_ROOT/review.json"

. "$STACK_ROOT/clients/reviewer.env"
"$ENG" api POST /api/v1/reviews "$STATE_ROOT/review.json" > "$STATE_ROOT/review-response.json"
test "$(jq -er .review_report_id "$STATE_ROOT/review-response.json")" = "$REVIEW_ID"
test "$(jq -er .result "$STATE_ROOT/review-response.json")" = "$REVIEW_RESULT"

CLOSURE_STATE="BLOCKED"
if [ "$REVIEW_RESULT" = PASS ]; then
  . "$STACK_ROOT/clients/closure.env"
  "$ENG" api POST /api/v1/closures "$PILOT_DIR/closure.json" > "$STATE_ROOT/closure-response.json"
  test "$(jq -er .closure_receipt_id "$STATE_ROOT/closure-response.json")" != ""
  "$ENG" api GET "/api/v1/work-items/$WORK_ID" > "$STATE_ROOT/work-closed.json"
  test "$(jq -er .state "$STATE_ROOT/work-closed.json")" = CLOSED
  CLOSURE_STATE="CLOSED"
else
  . "$STACK_ROOT/clients/reviewer.env"
  "$ENG" api GET "/api/v1/work-items/$WORK_ID" > "$STATE_ROOT/work-after-fail-review.json"
  test "$(jq -er .state "$STATE_ROOT/work-after-fail-review.json")" != CLOSED
fi

kill "$CONTROL_PID"
wait "$CONTROL_PID" || true
CONTROL_PID=""

docker run --rm --network host   -e PGPASSWORD=postgres   -v "$STATE_ROOT:/state"   postgres:17-alpine   pg_dump -h 127.0.0.1 -p 55432 -U postgres -d engineering_platform     -Fc -f /state/core-review.dump

test -s "$STATE_ROOT/core-review.dump"

jq -n   --arg pilot "$PILOT"   --arg verification_run_id "$VERIFICATION_RUN_ID"   --arg engineering_run_id "$ENGINEERING_RUN_ID"   --arg engineering_actor "$ENGINEERING_ACTOR"   --arg reviewer_actor "$GITHUB_ACTOR"   --arg review_result "$REVIEW_RESULT"   --arg closure "$CLOSURE_STATE"   --arg base_commit "$BASE_COMMIT"   --arg result_commit "$RESULT_COMMIT"   '{
    version:1,
    pilot:$pilot,
    verification_run_id:($verification_run_id|tonumber),
    engineering_run_id:($engineering_run_id|tonumber),
    engineering_actor:$engineering_actor,
    reviewer_actor:$reviewer_actor,
    review_result:$review_result,
    closure:$closure,
    base_commit:$base_commit,
    result_commit:$result_commit
  }' > "$STATE_ROOT/review-state.json"

echo "independent review: $REVIEW_RESULT"
echo "closure=$CLOSURE_STATE"
