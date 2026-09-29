#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  cat >&2 <<'EOF'
usage: engineer-model.sh PILOT OUTPUT_ROOT CODEX_NATIVE SAVED_LOGIN_FILE [MODEL]

Runs one retained engineering-to-PR chain on a trusted GitHub self-hosted Linux
runner. PILOT is feature or debug. OUTPUT_ROOT, CODEX_NATIVE and SAVED_LOGIN_FILE
must be absolute paths. No publisher credential may be present during the model
phase; the script reads the trusted host's gh credential only after the Codex
process has exited and FINISHED state has been retained.
EOF
  exit 2
}

[ "$#" -ge 4 ] && [ "$#" -le 5 ] || usage
PILOT="$1"
OUTPUT_ROOT="$2"
CODEX_NATIVE="$3"
SAVED_LOGIN_FILE="$4"
MODEL="${5:-gpt-5.6-sol}"
DEBUG_REPRODUCTION_RUN_ID="${DEBUG_REPRODUCTION_RUN_ID:-}"
DEBUG_REPRODUCTION_RECEIPT_DIGEST=""

: "${GITHUB_RUN_ID:?GitHub Actions run id required}"
: "${GITHUB_REPOSITORY:?GitHub repository required}"
: "${GITHUB_SHA:?GitHub source SHA required}"
: "${GITHUB_REF:?GitHub ref required}"
: "${GITHUB_REF_PROTECTED:?GitHub protected-ref fact required}"
test "$GITHUB_REPOSITORY" = "jiying2007/engineering-platform"
test "$GITHUB_REF" = "refs/heads/main"
test "$GITHUB_REF_PROTECTED" = "true"

case "$PILOT" in
  feature)
    test -z "$DEBUG_REPRODUCTION_RUN_ID"
    ;;
  debug)
    [[ "$DEBUG_REPRODUCTION_RUN_ID" =~ ^[0-9]+$ ]]
    ;;
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

install -d -m 0700 "$OUTPUT_ROOT"
STATE_ROOT="$OUTPUT_ROOT/retained-pilot-state"
STACK_ROOT="$OUTPUT_ROOT/retained-pilot-stack"
BIN_DIR="$OUTPUT_ROOT/bin"
rm -rf "$STATE_ROOT" "$STACK_ROOT" "$BIN_DIR"
install -d -m 0700 "$STATE_ROOT" "$BIN_DIR"
PREMODEL_LOG="$STATE_ROOT/preflight-checks.log"
: > "$PREMODEL_LOG"
chmod 0600 "$PREMODEL_LOG"

checkpoint() {
  printf 'PASS %s\n' "$1" | tee -a "$PREMODEL_LOG"
}
starting() {
  printf 'START %s\n' "$1" | tee -a "$PREMODEL_LOG"
}

starting toolchain
command -v git >/dev/null
command -v curl >/dev/null
command -v jq >/dev/null
command -v go >/dev/null
command -v docker >/dev/null
command -v sha256sum >/dev/null
command -v gh >/dev/null
command -v unzip >/dev/null
checkpoint toolchain

starting repository
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)"
test -f "$ROOT/go.mod"
test -z "$(git -C "$ROOT" status --porcelain=v1 --untracked-files=all)"
BASE_COMMIT="$(git -C "$ROOT" rev-parse HEAD)"
test "$BASE_COMMIT" = "$GITHUB_SHA"
REMOTE_MAIN="$(git ls-remote https://github.com/jiying2007/engineering-platform.git refs/heads/main | awk '{print $1}')"
test "$REMOTE_MAIN" = "$BASE_COMMIT"
checkpoint repository

starting repository-source-security
bash "$ROOT/examples/pilots/local-stack/secure-repository-source.sh" "$ROOT" | tee -a "$PREMODEL_LOG"
checkpoint repository-source-security

# Prove the independent publisher credential exists before consuming the
# non-replayable model turn, without exporting or copying it into model state.
# The token is not materialized here; gh only proves that the host credential is
# retrievable. Use the authenticated API rather than anonymous api.github.com
# so retained execution does not depend on the shared anonymous rate limit.
starting github-publisher-readiness
gh auth status --hostname github.com >/dev/null 2>&1
gh api user --jq '.login == "jiying2007"' | grep -qx true
if gh auth token --help >/dev/null 2>&1; then
  :
elif gh auth status --help 2>&1 | grep -q -- '--show-token'; then
  :
else
  echo "installed gh cannot export the authenticated token for the post-model publisher" >&2
  exit 1
fi
gh api repos/jiying2007/engineering-platform/branches/main |
  jq -e --arg sha "$BASE_COMMIT" '.name=="main" and .protected==true and .commit.sha==$sha' >/dev/null
checkpoint github-publisher-readiness

starting codex-login
CODEX_NATIVE="$(readlink -f "$CODEX_NATIVE")"
SAVED_LOGIN_FILE="$(readlink -f "$SAVED_LOGIN_FILE")"
CODEX_VERSION_OUTPUT="$("$CODEX_NATIVE" --version)"
[[ "$CODEX_VERSION_OUTPUT" =~ ^codex-cli[[:space:]][0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]
login_status="$(
  env -u OPENAI_API_KEY -u OPENAI_BASE_URL -u OPENAI_FEDERATION_RULE_ID     -u OPENAI_IDENTITY_TOKEN_FILE -u OPENAI_WORKLOAD_IDENTITY_CONTEXT     -u CODEX_API_KEY -u CODEX_ACCESS_TOKEN     "$CODEX_NATIVE" login status 2>&1
)"
test "$(printf '%s' "$login_status" | tr -d '\r' | xargs)" = "Logged in using ChatGPT"
checkpoint codex-login

if [ "$PILOT" = debug ]; then
  starting debug-reproduction-binding
  REPRO_RUN_JSON="$(gh api "repos/$GITHUB_REPOSITORY/actions/runs/$DEBUG_REPRODUCTION_RUN_ID")"
  test "$(printf '%s' "$REPRO_RUN_JSON" | jq -er .name)" = "Retained Debug firmware identity reproduction"
  test "$(printf '%s' "$REPRO_RUN_JSON" | jq -er .event)" = "workflow_dispatch"
  test "$(printf '%s' "$REPRO_RUN_JSON" | jq -er .status)" = "completed"
  test "$(printf '%s' "$REPRO_RUN_JSON" | jq -er .conclusion)" = "success"
  test "$(printf '%s' "$REPRO_RUN_JSON" | jq -er .head_branch)" = "main"
  test "$(printf '%s' "$REPRO_RUN_JSON" | jq -er .head_sha)" = "$BASE_COMMIT"

  REPRO_ARTIFACT_NAME="retained-debug-firmware-identity-reproduction-$BASE_COMMIT-$DEBUG_REPRODUCTION_RUN_ID"
  REPRO_ARTIFACTS_JSON="$(gh api "repos/$GITHUB_REPOSITORY/actions/runs/$DEBUG_REPRODUCTION_RUN_ID/artifacts?per_page=100")"
  REPRO_ARTIFACT_ID="$(printf '%s' "$REPRO_ARTIFACTS_JSON" | jq -er --arg n "$REPRO_ARTIFACT_NAME" '[.artifacts[]|select(.name==$n)] | if length==1 then .[0].id else error("Debug reproduction artifact missing/ambiguous") end')"
  REPRO_ARTIFACT_DIGEST="$(printf '%s' "$REPRO_ARTIFACTS_JSON" | jq -er --arg n "$REPRO_ARTIFACT_NAME" '[.artifacts[]|select(.name==$n)] | if length==1 then .[0].digest else error("Debug reproduction artifact digest missing/ambiguous") end')"
  REPRO_ARTIFACT_SIZE="$(printf '%s' "$REPRO_ARTIFACTS_JSON" | jq -er --arg n "$REPRO_ARTIFACT_NAME" '[.artifacts[]|select(.name==$n)] | if length==1 then .[0].size_in_bytes else error("Debug reproduction artifact size missing/ambiguous") end')"
  REPRO_ARTIFACT_EXPIRED="$(printf '%s' "$REPRO_ARTIFACTS_JSON" | jq -r --arg n "$REPRO_ARTIFACT_NAME" '[.artifacts[]|select(.name==$n)] | if length==1 then .[0].expired else error("Debug reproduction artifact expiry missing/ambiguous") end')"
  test "$REPRO_ARTIFACT_EXPIRED" = false
  [[ "$REPRO_ARTIFACT_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]]

  REPRO_ZIP="$STATE_ROOT/debug-reproduction.zip"
  REPRO_ROOT="$STATE_ROOT/debug-reproduction"
  install -d -m 0700 "$REPRO_ROOT"
  gh api "repos/$GITHUB_REPOSITORY/actions/artifacts/$REPRO_ARTIFACT_ID/zip" > "$REPRO_ZIP"
  chmod 0600 "$REPRO_ZIP"
  test "$(stat -c%s "$REPRO_ZIP")" = "$REPRO_ARTIFACT_SIZE"
  test "sha256:$(sha256sum "$REPRO_ZIP" | awk '{print $1}')" = "$REPRO_ARTIFACT_DIGEST"
  unzip -q "$REPRO_ZIP" -d "$REPRO_ROOT"

  REPRO_RECEIPT="$REPRO_ROOT/reproduction-receipt.json"
  REPRO_TEST="$REPRO_ROOT/reproduction-test.go"
  REPRO_OUTPUT="$REPRO_ROOT/go-test-output.txt"
  test -f "$REPRO_RECEIPT"
  test -f "$REPRO_TEST"
  test -f "$REPRO_OUTPUT"
  jq -e \
    --arg base "$BASE_COMMIT" \
    '.version==1 and
     .reproduction=="m1-debug-firmware-identity" and
     .base_commit==$base and
     .reproduction_confirmed==true and
     .expected_status=="BLOCKED" and
     .observed_status=="READY" and
     .firmware_identity=="not-a-digest" and
     .test_exit_code!=0' "$REPRO_RECEIPT" >/dev/null
  test "sha256:$(sha256sum "$REPRO_TEST" | awk '{print $1}')" = "$(jq -er .test_digest "$REPRO_RECEIPT")"
  test "sha256:$(sha256sum "$REPRO_OUTPUT" | awk '{print $1}')" = "$(jq -er .output_digest "$REPRO_RECEIPT")"
  DEBUG_REPRODUCTION_RECEIPT_DIGEST="sha256:$(sha256sum "$REPRO_RECEIPT" | awk '{print $1}')"

  jq -n \
    --arg run_id "$DEBUG_REPRODUCTION_RUN_ID" \
    --arg artifact_name "$REPRO_ARTIFACT_NAME" \
    --arg artifact_id "$REPRO_ARTIFACT_ID" \
    --arg artifact_digest "$REPRO_ARTIFACT_DIGEST" \
    --arg receipt_digest "$DEBUG_REPRODUCTION_RECEIPT_DIGEST" \
    --arg base_commit "$BASE_COMMIT" \
    '{
      version:1,
      reproduction_run_id:($run_id|tonumber),
      artifact_name:$artifact_name,
      artifact_id:($artifact_id|tonumber),
      artifact_digest:$artifact_digest,
      receipt_digest:$receipt_digest,
      base_commit:$base_commit,
      expected_status:"BLOCKED",
      observed_status:"READY",
      reproduction_confirmed:true
    }' > "$STATE_ROOT/debug-reproduction-binding.json"
  chmod 0600 "$STATE_ROOT/debug-reproduction-binding.json"
  checkpoint debug-reproduction-binding
fi

(
  cd "$ROOT"
  go build -trimpath -o "$BIN_DIR/control-plane" ./cmd/control-plane
  go build -trimpath -o "$BIN_DIR/eng" ./cmd/eng
  go build -trimpath -o "$BIN_DIR/worker" ./cmd/worker
  go build -trimpath -o "$BIN_DIR/codex-saved-login-live" ./cmd/codex-saved-login-live
  go build -trimpath -o "$BIN_DIR/codex-qualifier" ./cmd/codex-qualifier
)

"$BIN_DIR/codex-qualifier"   --codex "$CODEX_NATIVE"   --model "$MODEL"   > "$STATE_ROOT/codex-qualification.json"

"$BIN_DIR/eng" codex-profile \
  --codex "$CODEX_NATIVE" \
  --qualification "$STATE_ROOT/codex-qualification.json" \
  --model "$MODEL" \
  --provider openai-codex \
  --credential chatgpt-session \
  --execution trusted-self-hosted \
  > "$STATE_ROOT/codex-profile.json"
PROFILE_FILE="$STATE_ROOT/codex-profile.json"
PROFILE_DIGEST="$(jq -er .profile_digest "$PROFILE_FILE")"
QUALIFICATION_DIGEST="$(jq -er .qualification_digest "$PROFILE_FILE")"
CODEX_VERSION="$(jq -er .profile.codex_version "$PROFILE_FILE")"
PROVIDER_JSON="$(jq -cS .profile.provider "$PROFILE_FILE")"

bash "$ROOT/examples/pilots/local-stack/bootstrap.sh" "$STACK_ROOT" "$PROFILE_DIGEST" worker/codex-pilot

PG_NAME="engineering-platform-retained-${GITHUB_RUN_ID}-${PPID}"
CONTROL_PID=""
LOGIN_STAGE_ROOT="$OUTPUT_ROOT/saved-login-source"
cleanup() {
  if [ -n "$CONTROL_PID" ] && kill -0 "$CONTROL_PID" 2>/dev/null; then
    kill "$CONTROL_PID" 2>/dev/null || true
    wait "$CONTROL_PID" 2>/dev/null || true
  fi
  docker rm -f "$PG_NAME" >/dev/null 2>&1 || true
  rm -rf -- "$LOGIN_STAGE_ROOT"
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

bash "$ROOT/examples/pilots/local-stack/run-control.sh" \
  "$STACK_ROOT/operator/control-plane.env" \
  "$BIN_DIR/control-plane" \
  >"$STATE_ROOT/control-plane.log" 2>&1 &
CONTROL_PID=$!
for _ in $(seq 1 60); do
  if bash "$ROOT/examples/pilots/local-stack/health.sh" "$STACK_ROOT" owner >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$CONTROL_PID" 2>/dev/null; then
    cat "$STATE_ROOT/control-plane.log" >&2 || true
    exit 1
  fi
  sleep 1
done
bash "$ROOT/examples/pilots/local-stack/health.sh" "$STACK_ROOT" owner >/dev/null

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

starting saved-login-staging
STAGED_LOGIN_FILE="$(
  bash "$ROOT/examples/pilots/self-hosted/stage-saved-login.sh" \
    "$SAVED_LOGIN_FILE" \
    "$LOGIN_STAGE_ROOT"
)"
checkpoint saved-login-staging

WORKER_CODEX_FILE="$STATE_ROOT/worker-codex.json"
jq --arg login "$STAGED_LOGIN_FILE" '{
  version:2,
  codex_executable:.codex_executable,
  saved_login_file:$login,
  qualification:.qualification,
  profile:.profile
}' "$PROFILE_FILE" > "$WORKER_CODEX_FILE"
chmod 0600 "$WORKER_CODEX_FILE"

"$BIN_DIR/eng" pilot-preflight   --repository "$ROOT"   --base "$BASE_COMMIT"   --codex-profile "$PROFILE_FILE"   --access-policy "$STACK_ROOT/operator/access-policy.json"   --preparation "$PREPARATION_FILE"   --worker-profile worker/codex-pilot   --worker-codex "$WORKER_CODEX_FILE"   > "$STATE_ROOT/preflight-before-engineering.json"
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
  .receipt.result.codex.provider.provider_id=="openai-codex" and
  .receipt.result.codex.provider.credential_mode=="chatgpt-session" and
  .receipt.result.codex.provider.execution_mode=="trusted-self-hosted" and
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

jq -n   --arg pilot "$PILOT"   --arg run_id "$RUN_ID"   --arg base_commit "$BASE_COMMIT"   --arg profile_digest "$PROFILE_DIGEST"   --arg qualification_digest "$QUALIFICATION_DIGEST"   --arg codex_version "$CODEX_VERSION"   --argjson provider "$PROVIDER_JSON"   --arg execution_epoch "$EXECUTION_EPOCH"   --arg debug_reproduction_run_id "$DEBUG_REPRODUCTION_RUN_ID"   --arg debug_reproduction_receipt_digest "$DEBUG_REPRODUCTION_RECEIPT_DIGEST"   '{
    version:2,
    execution_origin:"trusted_self_hosted",
    provider:$provider,
    pilot:$pilot,
    run_id:$run_id,
    base_commit:$base_commit,
    profile_digest:$profile_digest,
    qualification_digest:$qualification_digest,
    codex_version:$codex_version,
    execution_epoch:($execution_epoch|tonumber),
    debug_reproduction_run_id:(if $debug_reproduction_run_id=="" then null else ($debug_reproduction_run_id|tonumber) end),
    debug_reproduction_receipt_digest:(if $debug_reproduction_receipt_digest=="" then null else $debug_reproduction_receipt_digest end),
    model_phase:"FINISHED",
    publication:"NOT_STARTED"
  }' > "$STATE_ROOT/model-phase.json"
chmod 0600 "$STATE_ROOT"/*.json "$STATE_ROOT/core-pre-publication.dump" "$STATE_ROOT/result.bundle"

echo "trusted self-hosted retained pilot model phase: FINISHED"
echo "pilot=$PILOT"
echo "base_commit=$BASE_COMMIT"
echo "provider_id=$(jq -r .provider_id <<<"$PROVIDER_JSON")"
echo "credential_mode=$(jq -r .credential_mode <<<"$PROVIDER_JSON")"
echo "execution_mode=$(jq -r .execution_mode <<<"$PROVIDER_JSON")"
echo "retained_policy=no model replay after FINISHED; publication failures must reconcile the retained result"

# Only after the model process has exited and the FINISHED Core state/result
# bundle have been retained may the trusted host materialize a publisher token.
TOKEN_FILE="$STACK_ROOT/secrets/github-token"
if gh auth token --help >/dev/null 2>&1; then
  gh auth token --hostname github.com > "$TOKEN_FILE"
else
  legacy_status="$(
    NO_COLOR=1 GH_FORCE_TTY=0 gh auth status --hostname github.com --show-token 2>&1
  )"
  legacy_token="$(
    printf '%s\n' "$legacy_status" |
      sed -n 's/.*Token:[[:space:]]*//p' |
      tail -n 1
  )"
  test -n "$legacy_token"
  case "$legacy_token" in
    *'*'*) echo "legacy gh returned a masked token" >&2; exit 1 ;;
  esac
  printf '%s\n' "$legacy_token" > "$TOKEN_FILE"
  unset legacy_status legacy_token
fi
chmod 0600 "$TOKEN_FILE"
test -s "$TOKEN_FILE"
GH_TOKEN="$(cat "$TOKEN_FILE")" gh api user --jq '.login == "jiying2007"' | grep -qx true

bash "$ROOT/examples/pilots/local-stack/run-control.sh" \
  "$STACK_ROOT/operator/control-plane-with-publisher.env" \
  "$BIN_DIR/control-plane" \
  >"$STATE_ROOT/control-plane-publisher.log" 2>&1 &
CONTROL_PID=$!
for _ in $(seq 1 60); do
  if bash "$ROOT/examples/pilots/local-stack/health.sh" "$STACK_ROOT" publisher >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$CONTROL_PID" 2>/dev/null; then
    cat "$STATE_ROOT/control-plane-publisher.log" >&2 || true
    exit 1
  fi
  sleep 1
done
bash "$ROOT/examples/pilots/local-stack/health.sh" "$STACK_ROOT" publisher >/dev/null

"$BIN_DIR/eng" pilot-preflight   --repository "$ROOT"   --base "$BASE_COMMIT"   --codex-profile "$PROFILE_FILE"   --access-policy "$STACK_ROOT/operator/access-policy.json"   --preparation "$PREPARATION_FILE"   --worker-profile worker/codex-pilot   --worker-codex "$WORKER_CODEX_FILE"   --publisher "$STACK_ROOT/operator/github-publisher.json"   > "$STATE_ROOT/preflight-before-publication.json"
jq -e '.internal=="READY" and .model_execution=="READY" and .publication=="READY" and ((.external_blockers // [])|length==0)'   "$STATE_ROOT/preflight-before-publication.json" >/dev/null

# Publisher requester is certificate-separated from Worker/owner.
# shellcheck disable=SC1090
. "$STACK_ROOT/clients/publisher.env"
"$BIN_DIR/eng" api GET "/api/v1/runs/$RUN_ID" > "$STATE_ROOT/run-before-publication.json"
"$BIN_DIR/eng" api GET /api/v1/recovery > "$STATE_ROOT/recovery-before-publication.json"
"$BIN_DIR/eng" api GET "/api/v1/runs/$RUN_ID/codex" > "$STATE_ROOT/codex-status.json"

EXECUTION_EPOCH="$(jq -er .run.current_epoch "$STATE_ROOT/run-before-publication.json")"
RECOVERY_EPOCH="$(jq -er .recovery_epoch "$STATE_ROOT/recovery-before-publication.json")"
CODEX_RESULT_DIGEST="$(jq -er .receipt.result_digest "$STATE_ROOT/codex-status.json")"
RESULT_COMMIT="$(jq -er .receipt.result.change.result_commit "$STATE_ROOT/codex-status.json")"
EXECUTION_ID="$(jq -er .token.execution_id "$STATE_ROOT/codex-status.json")"

sed   -e "s/__EXECUTION_EPOCH__/$EXECUTION_EPOCH/g"   -e "s/__RECOVERY_EPOCH__/$RECOVERY_EPOCH/g"   -e "s#__CODEX_RESULT_DIGEST__#$CODEX_RESULT_DIGEST#g"   "$PILOT_DIR/publish.json.tmpl" > "$STATE_ROOT/publish.json"

"$BIN_DIR/eng" api POST "/api/v1/runs/$RUN_ID/actions" "$STATE_ROOT/publish.json" > "$STATE_ROOT/publication-receipt.json"
RESULT="$(jq -er .result "$STATE_ROOT/publication-receipt.json")"
OPERATION_ID="$(jq -er .operation_id "$STATE_ROOT/publication-receipt.json")"
if [ "$RESULT" = "UNKNOWN" ]; then
  "$BIN_DIR/eng" api POST "/api/v1/actions/$OPERATION_ID/reconcile" <(printf '{}') > "$STATE_ROOT/publication-reconcile.json"
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

kill "$CONTROL_PID"
wait "$CONTROL_PID" || true
CONTROL_PID=""
rm -f "$TOKEN_FILE"

docker exec "$PG_NAME" pg_dump -U postgres -d engineering_platform -Fc > "$STATE_ROOT/core.dump"
test -s "$STATE_ROOT/core.dump"

jq -n   --arg pilot "$PILOT"   --arg run_id "$RUN_ID"   --arg base_commit "$BASE_COMMIT"   --arg result_commit "$RESULT_COMMIT"   --arg result_digest "$CODEX_RESULT_DIGEST"   --arg bundle_digest "$BUNDLE_DIGEST"   --arg profile_digest "$PROFILE_DIGEST"   --arg qualification_digest "$QUALIFICATION_DIGEST"   --arg codex_version "$CODEX_VERSION"   --argjson provider "$PROVIDER_JSON"   --arg pr_url "$PR_URL"   --arg pr_branch "$PUBLISHED_BRANCH"   --argjson pr_number "$PR_NUMBER"   --argjson github_engineering_run_id "$GITHUB_RUN_ID"   --arg debug_reproduction_run_id "$DEBUG_REPRODUCTION_RUN_ID"   --arg debug_reproduction_receipt_digest "$DEBUG_REPRODUCTION_RECEIPT_DIGEST"   '{
    version:2,
    execution_origin:"trusted_self_hosted",
    provider:$provider,
    pilot:$pilot,
    run_id:$run_id,
    base_commit:$base_commit,
    result_commit:$result_commit,
    codex_result_digest:$result_digest,
    bundle_digest:$bundle_digest,
    profile_digest:$profile_digest,
    qualification_digest:$qualification_digest,
    codex_version:$codex_version,
    pull_request:{number:$pr_number,url:$pr_url,branch:$pr_branch},
    github_engineering_run_id:$github_engineering_run_id,
    debug_reproduction_run_id:(if $debug_reproduction_run_id=="" then null else ($debug_reproduction_run_id|tonumber) end),
    debug_reproduction_receipt_digest:(if $debug_reproduction_receipt_digest=="" then null else $debug_reproduction_receipt_digest end),
    next_gate:"PASS_EXACT_PR_HEAD_CI"
  }' > "$STATE_ROOT/engineering-state.json"

cp "$STACK_ROOT/operator/access-policy.json" "$STATE_ROOT/access-policy.json"
# PREPARATION_FILE and WORKER_CODEX_FILE are created directly in STATE_ROOT.
# Do not copy them onto themselves: GNU cp treats that as an error and would
# incorrectly fail an otherwise CONFIRMED retained publication.
test "$PREPARATION_FILE" = "$STATE_ROOT/worker-preparation.json"
test "$WORKER_CODEX_FILE" = "$STATE_ROOT/worker-codex.json"
cp "$PILOT_DIR/requirement.md" "$STATE_ROOT/requirement.md"
chmod 0600 "$STATE_ROOT"/*.json "$STATE_ROOT/core.dump" "$STATE_ROOT/core-pre-publication.dump" "$STATE_ROOT/result.bundle"

echo "trusted self-hosted retained pilot engineering publication: CONFIRMED"
echo "pilot=$PILOT"
echo "base_commit=$BASE_COMMIT"
echo "result_commit=$RESULT_COMMIT"
echo "credential_mode=saved_chatgpt_login"
echo "pr_url=$PR_URL"
echo "state_root=$STATE_ROOT"
echo "next=exact PR-head CI, then retained-pilot-verify"
