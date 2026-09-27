#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  cat >&2 <<'EOF'
usage: engineer-model.sh PILOT OUTPUT_ROOT CODEX_NATIVE SAVED_LOGIN_FILE [MODEL]

Runs only the retained model phase on a trusted self-hosted Linux host.
PILOT is feature or debug. OUTPUT_ROOT, CODEX_NATIVE and SAVED_LOGIN_FILE must
be absolute paths. No GitHub publisher credential is accepted by this command.
EOF
  exit 2
}

[ "$#" -ge 4 ] && [ "$#" -le 5 ] || usage
PILOT="$1"
OUTPUT_ROOT="$2"
CODEX_NATIVE="$3"
SAVED_LOGIN_FILE="$4"
MODEL="${5:-gpt-5.6-sol}"

case "$PILOT" in
  feature|debug) ;;
  *) usage ;;
esac
for path in "$OUTPUT_ROOT" "$CODEX_NATIVE" "$SAVED_LOGIN_FILE"; do
  case "$path" in
    /*) ;;
    *) echo "absolute paths required" >&2; exit 2 ;;
  esac
done

for key in OPENAI_API_KEY OPENAI_BASE_URL OPENAI_FEDERATION_RULE_ID OPENAI_IDENTITY_TOKEN_FILE OPENAI_WORKLOAD_IDENTITY_CONTEXT CODEX_API_KEY CODEX_ACCESS_TOKEN GITHUB_TOKEN GH_TOKEN PUBLISH_TOKEN; do
  if [ -n "${!key:-}" ]; then
    echo "$key must be unset during the trusted self-hosted model phase" >&2
    exit 1
  fi
done

command -v git >/dev/null
command -v curl >/dev/null
command -v jq >/dev/null
command -v go >/dev/null
command -v docker >/dev/null
command -v sha256sum >/dev/null

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)"
test -f "$ROOT/go.mod"
test -z "$(git -C "$ROOT" status --porcelain=v1 --untracked-files=all)"
test "$(git -C "$ROOT" branch --show-current)" = main
BASE_COMMIT="$(git -C "$ROOT" rev-parse HEAD)"
test "$BASE_COMMIT" = "$(git -C "$ROOT" rev-parse refs/heads/main)"
REMOTE_MAIN="$(git ls-remote https://github.com/jiying2007/engineering-platform.git refs/heads/main | awk '{print $1}')"
test "$REMOTE_MAIN" = "$BASE_COMMIT"
curl -fsSL https://api.github.com/repos/jiying2007/engineering-platform/branches/main |
  jq -e --arg sha "$BASE_COMMIT" '.name=="main" and .protected==true and .commit.sha==$sha' >/dev/null

CODEX_NATIVE="$(readlink -f "$CODEX_NATIVE")"
SAVED_LOGIN_FILE="$(readlink -f "$SAVED_LOGIN_FILE")"
test "$("$CODEX_NATIVE" --version)" = "codex-cli 0.155.0"

install -d -m 0700 "$OUTPUT_ROOT"
STATE_ROOT="$OUTPUT_ROOT/retained-pilot-state"
STACK_ROOT="$OUTPUT_ROOT/retained-pilot-stack"
BIN_DIR="$OUTPUT_ROOT/bin"
QUAL_ROOT="$OUTPUT_ROOT/login-qualification"
rm -rf "$STATE_ROOT" "$STACK_ROOT" "$BIN_DIR" "$QUAL_ROOT"
install -d -m 0700 "$STATE_ROOT" "$BIN_DIR"

(
  cd "$ROOT"
  go build -trimpath -o "$BIN_DIR/control-plane" ./cmd/control-plane
  go build -trimpath -o "$BIN_DIR/eng" ./cmd/eng
  go build -trimpath -o "$BIN_DIR/worker" ./cmd/worker
  go build -trimpath -o "$BIN_DIR/codex-saved-login-live" ./cmd/codex-saved-login-live
)

"$ROOT/examples/pilots/self-hosted/qualify-login.sh"   "$QUAL_ROOT" "$CODEX_NATIVE" "$SAVED_LOGIN_FILE" "$MODEL"
cp "$QUAL_ROOT/saved-login-live-receipt.json" "$STATE_ROOT/saved-login-live-receipt.json"
chmod 0600 "$STATE_ROOT/saved-login-live-receipt.json"

"$BIN_DIR/eng" codex-profile --codex "$CODEX_NATIVE" --model "$MODEL" > "$STATE_ROOT/codex-profile.json"
PROFILE_FILE="$STATE_ROOT/codex-profile.json"
PROFILE_DIGEST="$(jq -er .profile_digest "$PROFILE_FILE")"

"$ROOT/examples/pilots/local-stack/bootstrap.sh" "$STACK_ROOT" "$PROFILE_DIGEST" worker/codex-pilot

PG_NAME="engineering-platform-retained-$PPID-$$"
CONTROL_PID=""
cleanup() {
  if [ -n "$CONTROL_PID" ] && kill -0 "$CONTROL_PID" 2>/dev/null; then
    kill "$CONTROL_PID" 2>/dev/null || true
    wait "$CONTROL_PID" 2>/dev/null || true
  fi
  docker rm -f "$PG_NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if ss -ltn 2>/dev/null | awk '{print $4}' | grep -Eq '(^|:)55432$'; then
  echo "TCP port 55432 is already in use" >&2
  exit 1
fi

docker run -d --rm --name "$PG_NAME"   -e POSTGRES_USER=postgres   -e POSTGRES_PASSWORD=postgres   -e POSTGRES_DB=engineering_platform   -p 127.0.0.1:55432:5432   postgres:17-alpine >/dev/null
for _ in $(seq 1 60); do
  if docker exec "$PG_NAME" pg_isready -U postgres -d engineering_platform >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
docker exec "$PG_NAME" pg_isready -U postgres -d engineering_platform >/dev/null

set -a
# shellcheck disable=SC1090
. "$STACK_ROOT/operator/control-plane.env"
set +a
"$BIN_DIR/control-plane" >"$STATE_ROOT/control-plane.log" 2>&1 &
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

case "$PILOT" in
  feature)
    PILOT_DIR="$ROOT/examples/pilots/feature-routing"
    RUN_ID="m1-feature-routing-run"
    ;;
  debug)
    PILOT_DIR="$ROOT/examples/pilots/debug-firmware-identity"
    RUN_ID="m1-debug-firmware-identity-run"
    ;;
esac

# shellcheck disable=SC1090
. "$STACK_ROOT/clients/owner.env"
HUMAN_OWNER="urn:engineering-platform:operator:pilot-owner"
CONTEXT_DIGEST="sha256:$(sha256sum "$PILOT_DIR/requirement.md" | awk '{print $1}')"
EXPECTED_CONTEXT_DIGEST="$(jq -er '.run_input.context_refs[0].digest' "$PILOT_DIR/run.json.tmpl")"
test "$CONTEXT_DIGEST" = "$EXPECTED_CONTEXT_DIGEST"

sed -e "s/__HUMAN_OWNER__/$HUMAN_OWNER/g" "$PILOT_DIR/work.json.tmpl" > "$STATE_ROOT/work.json"
sed -e "s/__BASE_COMMIT__/$BASE_COMMIT/g" "$PILOT_DIR/task.json.tmpl" > "$STATE_ROOT/task.json"

"$BIN_DIR/eng" api POST /api/v1/work-items "$STATE_ROOT/work.json" > "$STATE_ROOT/work-response.json"
"$BIN_DIR/eng" api POST /api/v1/task-contracts "$STATE_ROOT/task.json" > "$STATE_ROOT/task-response.json"
TASK_DIGEST="$(jq -er .digest "$STATE_ROOT/task-response.json")"
sed   -e "s#__TASK_DIGEST__#$TASK_DIGEST#g"   -e "s#__PROFILE_DIGEST__#$PROFILE_DIGEST#g"   -e "s#__WORKER_PROFILE__#worker/codex-pilot#g"   "$PILOT_DIR/run.json.tmpl" > "$STATE_ROOT/run.json"

"$BIN_DIR/eng" api POST /api/v1/runs "$STATE_ROOT/run.json" > "$STATE_ROOT/run-response.json"
RUN_INPUT_DIGEST="$(jq -er .run.run_input_manifest_digest "$STATE_ROOT/run-response.json")"
EXECUTION_EPOCH="$(jq -er .run.current_epoch "$STATE_ROOT/run-response.json")"
test "$EXECUTION_EPOCH" = 1

context_hex="${CONTEXT_DIGEST#sha256:}"
install -m 0400 "$PILOT_DIR/requirement.md" "$STACK_ROOT/context-source/$context_hex.bin"
GIT_EXECUTABLE="$(readlink -f "$(command -v git)")"
PREPARATION_FILE="$STATE_ROOT/worker-preparation.json"
sed   -e "s#__PREPARATION_ROOT__#$STACK_ROOT/preparation-root#g"   -e "s#__GIT_EXECUTABLE__#$GIT_EXECUTABLE#g"   -e "s#__CONTEXT_SOURCE__#$STACK_ROOT/context-source#g"   -e "s#__RUN_ID__#$RUN_ID#g"   -e "s#__TASK_DIGEST__#$TASK_DIGEST#g"   -e "s#__RUN_INPUT_DIGEST__#$RUN_INPUT_DIGEST#g"   -e "s#__REPOSITORY_PATH__#$ROOT#g"   -e "s#__CONTEXT_DIGEST__#$CONTEXT_DIGEST#g"   "$ROOT/examples/pilots/worker-preparation.json.tmpl" > "$PREPARATION_FILE"
chmod 0600 "$PREPARATION_FILE"

prepare_ok=0
for _ in $(seq 1 30); do
  set +e
  prepare_output="$(
    (
      # shellcheck disable=SC1090
      . "$STACK_ROOT/clients/worker.env"
      WORKER_PREPARATION_CONFIG="$PREPARATION_FILE"         "$BIN_DIR/worker" --prepare-only --profile worker/codex-pilot --once
    ) 2>"$STATE_ROOT/prepare.stderr"
  )"
  rc=$?
  set -e
  if [ "$rc" -ne 0 ]; then
    cat "$STATE_ROOT/prepare.stderr" >&2
    exit "$rc"
  fi
  if [ -n "$prepare_output" ]; then
    printf '%s\n' "$prepare_output" > "$STATE_ROOT/preparation-receipt.json"
    prepare_ok=1
    break
  fi
  sleep 1
done
test "$prepare_ok" = 1

WORKER_CODEX_FILE="$STATE_ROOT/worker-codex.json"
jq --arg login "$SAVED_LOGIN_FILE" '{
  version:1,
  codex_executable:.codex_executable,
  credential_mode:"saved_chatgpt_login",
  saved_login_file:$login,
  profile:.profile
}' "$PROFILE_FILE" > "$WORKER_CODEX_FILE"
chmod 0600 "$WORKER_CODEX_FILE"

"$BIN_DIR/eng" pilot-preflight   --repository "$ROOT"   --base "$BASE_COMMIT"   --codex-profile "$PROFILE_FILE"   --access-policy "$STACK_ROOT/operator/access-policy.json"   --preparation "$PREPARATION_FILE"   --worker-profile worker/codex-pilot   --worker-codex "$WORKER_CODEX_FILE"   --saved-login-receipt "$STATE_ROOT/saved-login-live-receipt.json"   > "$STATE_ROOT/preflight-before-engineering.json"
jq -e '.internal=="READY" and .model_execution=="READY" and .publication=="BLOCKED_EXTERNAL_PUBLISHER"'   "$STATE_ROOT/preflight-before-engineering.json" >/dev/null

(
  # shellcheck disable=SC1090
  . "$STACK_ROOT/clients/worker.env"
  WORKER_PREPARATION_CONFIG="$PREPARATION_FILE"   WORKER_CODEX_CONFIG="$WORKER_CODEX_FILE"     "$BIN_DIR/worker" --profile worker/codex-pilot --execute-codex --run "$RUN_ID" --once
) > "$STATE_ROOT/codex-worker-receipt.json"

# shellcheck disable=SC1090
. "$STACK_ROOT/clients/owner.env"
"$BIN_DIR/eng" api GET "/api/v1/runs/$RUN_ID/codex" > "$STATE_ROOT/codex-status.json"
jq -e '
  .state=="FINISHED" and
  .receipt.kind=="WORKER_ATTESTED_CODEX_EXECUTION" and
  .receipt.result.codex.credential_mode=="saved_chatgpt_login" and
  .receipt.result.codex.credential_bootstrap_removed_before_turn==true and
  .receipt.result.codex.assertion_removed_before_turn==false
' "$STATE_ROOT/codex-status.json" >/dev/null

kill "$CONTROL_PID"
wait "$CONTROL_PID" || true
CONTROL_PID=""

EXECUTION_ID="$(jq -er .token.execution_id "$STATE_ROOT/codex-status.json")"
BUNDLE_DIGEST="$(jq -er .receipt.result.change.bundle_digest "$STATE_ROOT/codex-status.json")"
BUNDLE_SIZE="$(jq -er .receipt.result.change.bundle_size "$STATE_ROOT/codex-status.json")"
BUNDLE_SOURCE="$STACK_ROOT/preparation-root/artifacts/$EXECUTION_ID.bundle"
test -f "$BUNDLE_SOURCE"
test "sha256:$(sha256sum "$BUNDLE_SOURCE" | awk '{print $1}')" = "$BUNDLE_DIGEST"
test "$(stat -c%s "$BUNDLE_SOURCE")" = "$BUNDLE_SIZE"
install -m 0600 "$BUNDLE_SOURCE" "$STATE_ROOT/result.bundle"

docker exec "$PG_NAME" pg_dump -U postgres -d engineering_platform -Fc > "$STATE_ROOT/core-pre-publication.dump"
test -s "$STATE_ROOT/core-pre-publication.dump"

jq -n   --arg pilot "$PILOT"   --arg run_id "$RUN_ID"   --arg base_commit "$BASE_COMMIT"   --arg profile_digest "$PROFILE_DIGEST"   --arg credential_mode "saved_chatgpt_login"   --arg execution_epoch "$EXECUTION_EPOCH"   '{
    version:1,
    execution_origin:"trusted_self_hosted",
    credential_mode:$credential_mode,
    pilot:$pilot,
    run_id:$run_id,
    base_commit:$base_commit,
    profile_digest:$profile_digest,
    execution_epoch:($execution_epoch|tonumber),
    model_phase:"FINISHED",
    publication:"NOT_STARTED"
  }' > "$STATE_ROOT/model-phase.json"
chmod 0600 "$STATE_ROOT"/*.json "$STATE_ROOT/core-pre-publication.dump" "$STATE_ROOT/result.bundle"

echo "trusted self-hosted retained pilot model phase: FINISHED"
echo "pilot=$PILOT"
echo "base_commit=$BASE_COMMIT"
echo "credential_mode=saved_chatgpt_login"
echo "state_root=$STATE_ROOT"
echo "next=independent publication; no model replay"
