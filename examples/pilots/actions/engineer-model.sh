#!/usr/bin/env bash
set -euo pipefail
umask 077

: "${PILOT:?feature or debug required}"
: "${MODEL:?model required}"
: "${CODEX_NATIVE:?qualified Codex binary required}"
: "${BIN_DIR:?built engineering-platform binaries required}"
: "${OPENAI_WIF_AUDIENCE:?managed-workspace WIF audience required}"
: "${OPENAI_CODEX_FEDERATION_RULE_ID:?managed-workspace WIF rule required}"
: "${GITHUB_WORKSPACE:?GitHub workspace required}"
: "${RUNNER_TEMP:?runner temp required}"
: "${GITHUB_SHA:?protected main SHA required}"
: "${GITHUB_REPOSITORY:?repository required}"
: "${GITHUB_RUN_ID:?run id required}"

test "$GITHUB_REPOSITORY" = "jiying2007/engineering-platform"
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
WORKER="$BIN_DIR/worker"
CONTROL="$BIN_DIR/control-plane"
STACK_ROOT="$RUNNER_TEMP/retained-pilot-stack"
STATE_ROOT="$RUNNER_TEMP/retained-pilot-state"
PROFILE_FILE="$STATE_ROOT/codex-profile.json"
PREPARATION_FILE="$STATE_ROOT/worker-preparation.json"
WORKER_CODEX_FILE="$STATE_ROOT/worker-codex.json"
LIVE_RECEIPT="$STATE_ROOT/codex-wif-live-receipt.json"
WORKER_PROFILE="worker/codex-pilot"

mkdir -p "$STATE_ROOT"
chmod 700 "$STATE_ROOT"

remote_main="$(git ls-remote https://github.com/jiying2007/engineering-platform.git refs/heads/main | awk '{print $1}')"
local_main="$(git -C "$GITHUB_WORKSPACE" rev-parse HEAD)"
test "$local_main" = "$GITHUB_SHA"
test "$remote_main" = "$GITHUB_SHA"

"$ENG" codex-profile --codex "$CODEX_NATIVE" --model "$MODEL" > "$PROFILE_FILE"
PROFILE_DIGEST="$(jq -er .profile_digest "$PROFILE_FILE")"
BINARY_DIGEST="$(jq -er .profile.binary_digest "$PROFILE_FILE")"

bash "$GITHUB_WORKSPACE/examples/pilots/local-stack/bootstrap.sh"   "$STACK_ROOT" "$PROFILE_DIGEST" "$WORKER_PROFILE"

start_control() {
  local env_file="$1"
  set -a
  # shellcheck disable=SC1090
  . "$env_file"
  set +a
  "$CONTROL" >"$STATE_ROOT/control-plane.log" 2>&1 &
  CONTROL_PID=$!
  echo "$CONTROL_PID" > "$STATE_ROOT/control-plane.pid"
  for _ in $(seq 1 60); do
    if curl -fsS --cacert "$STACK_ROOT/pki/ca.crt" https://127.0.0.1:18443/healthz >/dev/null 2>&1; then
      return 0
    fi
    if ! kill -0 "$CONTROL_PID" 2>/dev/null; then
      cat "$STATE_ROOT/control-plane.log" >&2 || true
      return 1
    fi
    sleep 1
  done
  echo "control-plane did not become ready" >&2
  cat "$STATE_ROOT/control-plane.log" >&2 || true
  return 1
}

stop_control() {
  if [ -n "${CONTROL_PID:-}" ] && kill -0 "$CONTROL_PID" 2>/dev/null; then
    kill "$CONTROL_PID"
    wait "$CONTROL_PID" || true
  fi
  unset CONTROL_PID
}
trap stop_control EXIT

start_control "$STACK_ROOT/operator/control-plane.env"

# Create the immutable Work/Task/Run subject under the owner identity.
# shellcheck disable=SC1090
. "$STACK_ROOT/clients/owner.env"

BASE_COMMIT="$GITHUB_SHA"
HUMAN_OWNER="urn:engineering-platform:operator:pilot-owner"
CONTEXT_DIGEST="sha256:$(sha256sum "$PILOT_DIR/requirement.md" | awk '{print $1}')"
EXPECTED_CONTEXT_DIGEST="$(jq -er '.run_input.context_refs[0].digest' "$PILOT_DIR/run.json.tmpl")"
test "$CONTEXT_DIGEST" = "$EXPECTED_CONTEXT_DIGEST"

sed -e "s/__HUMAN_OWNER__/$HUMAN_OWNER/g"   "$PILOT_DIR/work.json.tmpl" > "$STATE_ROOT/work.json"
sed -e "s/__BASE_COMMIT__/$BASE_COMMIT/g"   "$PILOT_DIR/task.json.tmpl" > "$STATE_ROOT/task.json"

"$ENG" api POST /api/v1/work-items "$STATE_ROOT/work.json" > "$STATE_ROOT/work-response.json"
"$ENG" api POST /api/v1/task-contracts "$STATE_ROOT/task.json" > "$STATE_ROOT/task-response.json"
TASK_DIGEST="$(jq -er .digest "$STATE_ROOT/task-response.json")"

sed   -e "s#__TASK_DIGEST__#$TASK_DIGEST#g"   -e "s#__PROFILE_DIGEST__#$PROFILE_DIGEST#g"   -e "s#__WORKER_PROFILE__#$WORKER_PROFILE#g"   "$PILOT_DIR/run.json.tmpl" > "$STATE_ROOT/run.json"

"$ENG" api POST /api/v1/runs "$STATE_ROOT/run.json" > "$STATE_ROOT/run-response.json"
RUN_INPUT_DIGEST="$(jq -er .run.run_input_manifest_digest "$STATE_ROOT/run-response.json")"
EXECUTION_EPOCH="$(jq -er .run.current_epoch "$STATE_ROOT/run-response.json")"
test "$EXECUTION_EPOCH" = 1

context_hex="${CONTEXT_DIGEST#sha256:}"
install -m 0400 "$PILOT_DIR/requirement.md" "$STACK_ROOT/context-source/$context_hex.bin"

GIT_EXECUTABLE="$(readlink -f "$(command -v git)")"
sed   -e "s#__PREPARATION_ROOT__#$STACK_ROOT/preparation-root#g"   -e "s#__GIT_EXECUTABLE__#$GIT_EXECUTABLE#g"   -e "s#__CONTEXT_SOURCE__#$STACK_ROOT/context-source#g"   -e "s#__RUN_ID__#$RUN_ID#g"   -e "s#__TASK_DIGEST__#$TASK_DIGEST#g"   -e "s#__RUN_INPUT_DIGEST__#$RUN_INPUT_DIGEST#g"   -e "s#__REPOSITORY_PATH__#$GITHUB_WORKSPACE#g"   -e "s#__CONTEXT_DIGEST__#$CONTEXT_DIGEST#g"   "$GITHUB_WORKSPACE/examples/pilots/worker-preparation.json.tmpl"   > "$PREPARATION_FILE"
chmod 600 "$PREPARATION_FILE"

# Preparation is trusted host work only; retry claim polling, never model work.
prepare_ok=0
for _ in $(seq 1 30); do
  set +e
  prepare_output="$(
    (
      # shellcheck disable=SC1090
      . "$STACK_ROOT/clients/worker.env"
      WORKER_PREPARATION_CONFIG="$PREPARATION_FILE"         "$WORKER" --prepare-only --profile "$WORKER_PROFILE" --once
    ) 2>"$STATE_ROOT/prepare.stderr"
  )"
  rc=$?
  set -e
  if [ "$rc" -ne 0 ]; then
    cat "$STATE_ROOT/prepare.stderr" >&2
    exit "$rc"
  fi
  if [ -n "$prepare_output" ]; then
    printf '%s
' "$prepare_output" > "$STATE_ROOT/preparation-receipt.json"
    prepare_ok=1
    break
  fi
  sleep 1
done
test "$prepare_ok" = 1

mint_oidc() {
  local output="$1"
  local expected_workflow_ref="$GITHUB_REPOSITORY/.github/workflows/retained-pilot-engineer.yml@refs/heads/main"
  local encoded token
  encoded="$(jq -rn --arg audience "$OPENAI_WIF_AUDIENCE" '$audience|@uri')"
  token="$(curl -fsSL -H "Authorization: bearer $ACTIONS_ID_TOKEN_REQUEST_TOKEN"     "${ACTIONS_ID_TOKEN_REQUEST_URL}&audience=${encoded}" | jq -er .value)"
  printf '%s' "$token" > "$output"
  unset token
  chmod 0600 "$output"
  python3 - "$output" "$OPENAI_WIF_AUDIENCE" "$GITHUB_REPOSITORY" "$GITHUB_REF" "$expected_workflow_ref" <<'PY'
import base64, json, pathlib, sys, time
token = pathlib.Path(sys.argv[1]).read_text()
parts = token.split(".")
if len(parts) != 3:
    raise SystemExit("OIDC token is not a compact JWT")
payload = parts[1] + "=" * (-len(parts[1]) % 4)
claims = json.loads(base64.urlsafe_b64decode(payload))
audience, repository, ref, workflow_ref = sys.argv[2:]
if claims.get("iss") != "https://token.actions.githubusercontent.com":
    raise SystemExit("unexpected OIDC issuer")
aud = claims.get("aud")
if not (aud == audience or isinstance(aud, list) and audience in aud):
    raise SystemExit("unexpected OIDC audience")
if claims.get("repository") != repository or claims.get("ref") != ref:
    raise SystemExit("unexpected repository/ref claims")
if claims.get("workflow_ref") != workflow_ref:
    raise SystemExit("unexpected workflow_ref")
if int(claims.get("exp", 0)) <= int(time.time()):
    raise SystemExit("expired OIDC assertion")
PY
}

# First real WIF turn: read-only qualification. This replaces waiting for a
# separate codex-wif-live workflow as long as the admin rule permits this exact
# protected-main workflow_ref.
live_secret="$RUNNER_TEMP/wif-live"
install -d -m 0700 "$live_secret"
mint_oidc "$live_secret/identity-token"
live_work="$RUNNER_TEMP/wif-live-work"
live_home="$RUNNER_TEMP/wif-live-home"
install -d -m 0700 "$live_work" "$live_home"
audit_context="$(jq -cn --arg run_id "$GITHUB_RUN_ID" --arg pilot "$PILOT" --arg repository "$GITHUB_REPOSITORY" --arg source_sha "$GITHUB_SHA" '{github_run_id:$run_id,pilot:$pilot,repository:$repository,source_sha:$source_sha}')"
"$BIN_DIR/codex-wif-live"   --codex "$CODEX_NATIVE"   --digest "$BINARY_DIGEST"   --work "$live_work"   --home "$live_home"   --federation-rule "$OPENAI_CODEX_FEDERATION_RULE_ID"   --identity-token-file "$live_secret/identity-token"   --audit-context "$audit_context"   --model "$MODEL" > "$LIVE_RECEIPT"
test ! -e "$live_secret/identity-token"

jq --arg rule "$OPENAI_CODEX_FEDERATION_RULE_ID"   '{
    version:1,
    codex_executable:.codex_executable,
    federation_rule_id:$rule,
    profile:.profile
  }' "$PROFILE_FILE" > "$WORKER_CODEX_FILE"
chmod 600 "$WORKER_CODEX_FILE"

# Local/deployment readiness must now be fully ready except publication, whose
# credential is intentionally withheld until after the model process exits.
"$ENG" pilot-preflight   --repository "$GITHUB_WORKSPACE"   --base "$BASE_COMMIT"   --codex-profile "$PROFILE_FILE"   --access-policy "$STACK_ROOT/operator/access-policy.json"   --preparation "$PREPARATION_FILE"   --worker-profile "$WORKER_PROFILE"   --worker-codex "$WORKER_CODEX_FILE"   --wif-receipt "$LIVE_RECEIPT" > "$STATE_ROOT/preflight-before-engineering.json"
jq -e '.internal=="READY" and .model_execution=="READY" and .publication=="BLOCKED_EXTERNAL_PUBLISHER"'   "$STATE_ROOT/preflight-before-engineering.json" >/dev/null

# Second assertion is consumed by the actual retained engineering turn.
engineering_secret="$RUNNER_TEMP/wif-engineering"
install -d -m 0700 "$engineering_secret"
mint_oidc "$engineering_secret/identity-token"

(
  # shellcheck disable=SC1090
  . "$STACK_ROOT/clients/worker.env"
  WORKER_PREPARATION_CONFIG="$PREPARATION_FILE"   WORKER_CODEX_CONFIG="$WORKER_CODEX_FILE"   OPENAI_IDENTITY_TOKEN_FILE="$engineering_secret/identity-token"     "$WORKER" --profile "$WORKER_PROFILE" --execute-codex --run "$RUN_ID" --once
) > "$STATE_ROOT/codex-worker-receipt.json"
test ! -e "$engineering_secret/identity-token"

# Read immutable retained status before publication handoff.
# shellcheck disable=SC1090
. "$STACK_ROOT/clients/owner.env"
"$ENG" api GET "/api/v1/runs/$RUN_ID/codex" > "$STATE_ROOT/codex-status.json"
jq -e '.state=="FINISHED" and .receipt.kind=="WORKER_ATTESTED_CODEX_EXECUTION"' "$STATE_ROOT/codex-status.json" >/dev/null

# The model process is completely gone before any GitHub write credential can be
# introduced in the next workflow step.
stop_control
trap - EXIT

jq -n   --arg pilot "$PILOT"   --arg run_id "$RUN_ID"   --arg base_commit "$BASE_COMMIT"   --arg profile_digest "$PROFILE_DIGEST"   --arg execution_epoch "$EXECUTION_EPOCH"   '{
    pilot:$pilot,
    run_id:$run_id,
    base_commit:$base_commit,
    profile_digest:$profile_digest,
    execution_epoch:($execution_epoch|tonumber),
    model_phase:"FINISHED"
  }' > "$STATE_ROOT/model-phase.json"

echo "retained pilot model phase: FINISHED"
