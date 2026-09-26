#!/usr/bin/env bash
set -euo pipefail
umask 077

: "${PILOT:?feature or debug required}"
: "${BIN_DIR:?built binaries required}"
: "${RUNNER_TEMP:?runner temp required}"
: "${GITHUB_WORKSPACE:?workspace required}"
: "${PUBLISH_TOKEN:?job-scoped publisher token required}"

case "$PILOT" in
  feature)
    PILOT_DIR="$GITHUB_WORKSPACE/examples/pilots/feature-routing"
    RUN_ID="m1-feature-routing-run"
    ;;
  debug)
    PILOT_DIR="$GITHUB_WORKSPACE/examples/pilots/debug-firmware-identity"
    RUN_ID="m1-debug-firmware-identity-run"
    ;;
  *) echo "unsupported pilot $PILOT" >&2; exit 2 ;;
esac

ENG="$BIN_DIR/eng"
CONTROL="$BIN_DIR/control-plane"
STACK_ROOT="$RUNNER_TEMP/retained-pilot-stack"
STATE_ROOT="$RUNNER_TEMP/retained-pilot-state"
PROFILE_FILE="$STATE_ROOT/codex-profile.json"
PREPARATION_FILE="$STATE_ROOT/worker-preparation.json"
WORKER_CODEX_FILE="$STATE_ROOT/worker-codex.json"
LIVE_RECEIPT="$STATE_ROOT/codex-wif-live-receipt.json"
PUBLISHER_FILE="$STACK_ROOT/operator/github-publisher.json"
TOKEN_FILE="$STACK_ROOT/secrets/github-token"
WORKER_PROFILE="worker/codex-pilot"

test -f "$STATE_ROOT/model-phase.json"
test -f "$STATE_ROOT/codex-status.json"
test -f "$LIVE_RECEIPT"

printf '%s' "$PUBLISH_TOKEN" > "$TOKEN_FILE"
chmod 0600 "$TOKEN_FILE"
unset PUBLISH_TOKEN

set -a
# shellcheck disable=SC1090
. "$STACK_ROOT/operator/control-plane-with-publisher.env"
set +a
"$CONTROL" >"$STATE_ROOT/control-plane-publisher.log" 2>&1 &
CONTROL_PID=$!
trap 'kill "$CONTROL_PID" 2>/dev/null || true; wait "$CONTROL_PID" 2>/dev/null || true; rm -f "$TOKEN_FILE"' EXIT
for _ in $(seq 1 60); do
  if curl -fsS --cacert "$STACK_ROOT/pki/ca.crt" https://127.0.0.1:18443/healthz >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$CONTROL_PID" 2>/dev/null; then
    cat "$STATE_ROOT/control-plane-publisher.log" >&2 || true
    exit 1
  fi
  sleep 1
done
curl -fsS --cacert "$STACK_ROOT/pki/ca.crt" https://127.0.0.1:18443/healthz >/dev/null

BASE_COMMIT="$(jq -er .base_commit "$STATE_ROOT/model-phase.json")"
PROFILE_DIGEST="$(jq -er .profile_digest "$STATE_ROOT/model-phase.json")"

# Full readiness: after the model exits, publication may now consume the
# separately scoped job token.
"$ENG" pilot-preflight   --repository "$GITHUB_WORKSPACE"   --base "$BASE_COMMIT"   --codex-profile "$PROFILE_FILE"   --access-policy "$STACK_ROOT/operator/access-policy.json"   --preparation "$PREPARATION_FILE"   --worker-profile "$WORKER_PROFILE"   --worker-codex "$WORKER_CODEX_FILE"   --wif-receipt "$LIVE_RECEIPT"   --publisher "$PUBLISHER_FILE" > "$STATE_ROOT/preflight-before-publication.json"
jq -e '.internal=="READY" and .model_execution=="READY" and .publication=="READY" and ((.external_blockers // [])|length==0)'   "$STATE_ROOT/preflight-before-publication.json" >/dev/null

# Publisher requester is certificate-separated from Worker/owner.
# shellcheck disable=SC1090
. "$STACK_ROOT/clients/publisher.env"
"$ENG" api GET "/api/v1/runs/$RUN_ID" > "$STATE_ROOT/run-before-publication.json"
"$ENG" api GET /api/v1/recovery > "$STATE_ROOT/recovery-before-publication.json"
"$ENG" api GET "/api/v1/runs/$RUN_ID/codex" > "$STATE_ROOT/codex-status.json"

EXECUTION_EPOCH="$(jq -er .run.current_epoch "$STATE_ROOT/run-before-publication.json")"
RECOVERY_EPOCH="$(jq -er .recovery_epoch "$STATE_ROOT/recovery-before-publication.json")"
CODEX_RESULT_DIGEST="$(jq -er .receipt.result_digest "$STATE_ROOT/codex-status.json")"
RESULT_COMMIT="$(jq -er .receipt.result.change.result_commit "$STATE_ROOT/codex-status.json")"
BUNDLE_DIGEST="$(jq -er .receipt.result.change.bundle_digest "$STATE_ROOT/codex-status.json")"
BUNDLE_SIZE="$(jq -er .receipt.result.change.bundle_size "$STATE_ROOT/codex-status.json")"
EXECUTION_ID="$(jq -er .token.execution_id "$STATE_ROOT/codex-status.json")"

sed   -e "s/__EXECUTION_EPOCH__/$EXECUTION_EPOCH/g"   -e "s/__RECOVERY_EPOCH__/$RECOVERY_EPOCH/g"   -e "s#__CODEX_RESULT_DIGEST__#$CODEX_RESULT_DIGEST#g"   "$PILOT_DIR/publish.json.tmpl" > "$STATE_ROOT/publish.json"

"$ENG" api POST "/api/v1/runs/$RUN_ID/actions" "$STATE_ROOT/publish.json" > "$STATE_ROOT/publication-receipt.json"
RESULT="$(jq -er .result "$STATE_ROOT/publication-receipt.json")"
OPERATION_ID="$(jq -er .operation_id "$STATE_ROOT/publication-receipt.json")"
if [ "$RESULT" = "UNKNOWN" ]; then
  "$ENG" api POST "/api/v1/actions/$OPERATION_ID/reconcile" <(printf '{}') > "$STATE_ROOT/publication-reconcile.json"
  RESULT="$(jq -er .result "$STATE_ROOT/publication-reconcile.json")"
  if [ "$RESULT" = "CONFIRMED" ]; then
    cp "$STATE_ROOT/publication-reconcile.json" "$STATE_ROOT/publication-receipt.json"
  fi
fi
test "$RESULT" = "CONFIRMED"

PR_URL="$(jq -er .external_ref "$STATE_ROOT/publication-receipt.json")"
OBSERVED_STATE="$(jq -er .observed_state "$STATE_ROOT/publication-receipt.json")"
PR_NUMBER="$(printf '%s' "$PR_URL" | sed -n 's#^.*/pull/\([0-9][0-9]*\)$#\1#p')"
test -n "$PR_NUMBER"
PUBLISHED_BRANCH="$(printf '%s' "$OBSERVED_STATE" | jq -er .branch)"
OBSERVED_RESULT="$(printf '%s' "$OBSERVED_STATE" | jq -er .result_commit)"
test "$OBSERVED_RESULT" = "$RESULT_COMMIT"

BUNDLE_SOURCE="$STACK_ROOT/preparation-root/artifacts/$EXECUTION_ID.bundle"
test -f "$BUNDLE_SOURCE"
test "sha256:$(sha256sum "$BUNDLE_SOURCE" | awk '{print $1}')" = "$BUNDLE_DIGEST"
test "$(stat -c%s "$BUNDLE_SOURCE")" = "$BUNDLE_SIZE"
install -m 0600 "$BUNDLE_SOURCE" "$STATE_ROOT/result.bundle"

# Stop Core before snapshot handoff. PostgreSQL service remains alive for pg_dump.
kill "$CONTROL_PID"
wait "$CONTROL_PID" || true
unset CONTROL_PID
rm -f "$TOKEN_FILE"

docker run --rm --network host   -e PGPASSWORD=postgres   -v "$STATE_ROOT:/state"   postgres:17-alpine   pg_dump -h 127.0.0.1 -p 55432 -U postgres -d engineering_platform     -Fc -f /state/core.dump
test -s "$STATE_ROOT/core.dump"

jq -n   --arg pilot "$PILOT"   --arg run_id "$RUN_ID"   --arg base_commit "$BASE_COMMIT"   --arg result_commit "$RESULT_COMMIT"   --arg result_digest "$CODEX_RESULT_DIGEST"   --arg bundle_digest "$BUNDLE_DIGEST"   --arg profile_digest "$PROFILE_DIGEST"   --arg pr_url "$PR_URL"   --arg pr_branch "$PUBLISHED_BRANCH"   --argjson pr_number "$PR_NUMBER"   --argjson github_engineering_run_id "$GITHUB_RUN_ID"   '{
    version:1,
    pilot:$pilot,
    run_id:$run_id,
    base_commit:$base_commit,
    result_commit:$result_commit,
    codex_result_digest:$result_digest,
    bundle_digest:$bundle_digest,
    profile_digest:$profile_digest,
    pull_request:{number:$pr_number,url:$pr_url,branch:$pr_branch},
    github_engineering_run_id:$github_engineering_run_id,
    next_gate:"APPROVE_AND_PASS_EXACT_PR_HEAD_CI"
  }' > "$STATE_ROOT/engineering-state.json"

# Retain frozen inputs needed to reconstruct the verification environment.
cp "$STACK_ROOT/operator/access-policy.json" "$STATE_ROOT/access-policy.json"
cp "$PREPARATION_FILE" "$STATE_ROOT/worker-preparation.json"
cp "$WORKER_CODEX_FILE" "$STATE_ROOT/worker-codex.json"
cp "$PILOT_DIR/requirement.md" "$STATE_ROOT/requirement.md"
chmod 600 "$STATE_ROOT"/*.json "$STATE_ROOT/core.dump" "$STATE_ROOT/result.bundle"

echo "retained pilot engineering publication: CONFIRMED"
echo "pr_url=$PR_URL"
echo "pr_branch=$PUBLISHED_BRANCH"
echo "next=approve exact PR-head CI, then run retained-pilot-verify"
