#!/usr/bin/env bash
set -euo pipefail

die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
note() { printf 'NOTICE: %s\n' "$*"; }

for name in PILOT_MODE PILOT_MODEL OPENAI_WIF_AUDIENCE OPENAI_FEDERATION_RULE_ID PUBLISHER_GITHUB_TOKEN GITHUB_REPOSITORY GITHUB_SHA GITHUB_REF GITHUB_WORKSPACE RUNNER_TEMP ACTIONS_ID_TOKEN_REQUEST_TOKEN ACTIONS_ID_TOKEN_REQUEST_URL; do
  test -n "${!name:-}" || die "$name is required"
done

test "$GITHUB_REPOSITORY" = "jiying2007/engineering-platform" || die "unexpected repository"
test "$GITHUB_REF" = "refs/heads/main" || die "pilot must run from protected main"
case "$PILOT_MODE" in feature|debug) ;; *) die "PILOT_MODE must be feature or debug" ;; esac
test "${PUBLISHER_PR_CREATION_CONFIRMED:-false}" = "true" || die "confirm repository allows GitHub Actions to create pull requests"
case "${CI_WAIT_MINUTES:-90}" in ''|*[!0-9]*) die "CI_WAIT_MINUTES must be an integer" ;; esac
test "$CI_WAIT_MINUTES" -ge 5 -a "$CI_WAIT_MINUTES" -le 180 || die "CI_WAIT_MINUTES must be 5..180"

ROOT="$RUNNER_TEMP/m1-pilot"
OUT="$RUNNER_TEMP/m1-pilot-output"
BIN="$ROOT/bin"
PKI="$ROOT/pki"
PREP_ROOT="$ROOT/preparation"
CONTEXT_SOURCE="$ROOT/context-source"
OPERATOR_REPO="$ROOT/evidence-repository"
TOKEN_FILE="$ROOT/publisher-token"
POLICY_FILE="$ROOT/access-policy.json"
PUBLISHER_CONFIG="$ROOT/github-publisher.json"
CONTROL_LOG="$ROOT/control-plane.log"
CONTROL_PID_FILE="$ROOT/control-plane.pid"
PROFILE_JSON="$OUT/codex-profile.json"
STATE_JSON="$OUT/pilot-state.json"
DB_URL="postgres://postgres:postgres@127.0.0.1:5432/engineering_platform?sslmode=disable"
CONTROL_ENDPOINT="https://127.0.0.1:8443"
WORKER_PROFILE="worker/codex-pilot"
BASE_COMMIT="$GITHUB_SHA"

rm -rf "$ROOT" "$OUT"
install -d -m 0700 "$ROOT" "$OUT" "$BIN" "$PKI" "$PREP_ROOT" "$PREP_ROOT/artifacts" "$CONTEXT_SOURCE"

on_exit() {
  status=$?
  set +e
  if test -n "${CONTROL_PID_FILE:-}" -a -f "$CONTROL_PID_FILE"; then
    docker run --rm --network host -e PGPASSWORD=postgres -v "$OUT:/out" postgres:17-alpine       pg_dump -h 127.0.0.1 -U postgres -d engineering_platform -Fc -f /out/core-state.dump >/dev/null 2>&1
    cp "$CONTROL_LOG" "$OUT/control-plane.log" 2>/dev/null
    kill "$(cat "$CONTROL_PID_FILE")" 2>/dev/null
  fi
  rm -f "$TOKEN_FILE"
  rm -rf "$ROOT/openai-wif" "$PKI"
  exit "$status"
}
trap on_exit EXIT

GIT_EXECUTABLE="$(readlink -f "$(command -v git)")"
CODEX_NATIVE="${CODEX_NATIVE:-}"
test -x "$CODEX_NATIVE" || die "qualified CODEX_NATIVE is required"

go build -trimpath -o "$BIN/control-plane" ./cmd/control-plane
go build -trimpath -o "$BIN/eng" ./cmd/eng
go build -trimpath -o "$BIN/worker" ./cmd/worker

"$BIN/eng" codex-profile --codex "$CODEX_NATIVE" --model "$PILOT_MODEL" > "$PROFILE_JSON"
PROFILE_DIGEST="$(jq -er .profile_digest "$PROFILE_JSON")"
test "$(jq -er .profile.model "$PROFILE_JSON")" = "$PILOT_MODEL" || die "profile model mismatch"

sed   -e "s#__PROFILE_DIGEST__#$PROFILE_DIGEST#g"   -e "s#__WORKER_PROFILE__#$WORKER_PROFILE#g"   examples/pilots/access-policy.json.tmpl > "$POLICY_FILE"
chmod 0600 "$POLICY_FILE"

openssl genrsa -out "$PKI/ca.key" 2048 >/dev/null 2>&1
chmod 0600 "$PKI/ca.key"
openssl req -x509 -new -sha256 -days 2 -key "$PKI/ca.key"   -subj "/CN=engineering-platform-pilot-ca"   -addext "basicConstraints=critical,CA:TRUE"   -addext "keyUsage=critical,keyCertSign,cRLSign"   -out "$PKI/ca.crt" >/dev/null 2>&1
chmod 0644 "$PKI/ca.crt"

issue_client() {
  name="$1"; uri="$2"
  openssl genrsa -out "$PKI/$name.key" 2048 >/dev/null 2>&1
  chmod 0600 "$PKI/$name.key"
  openssl req -new -key "$PKI/$name.key" -subj "/CN=$name" -out "$PKI/$name.csr" >/dev/null 2>&1
  cat > "$PKI/$name.ext" <<EOF
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature
extendedKeyUsage=clientAuth
subjectAltName=URI:$uri
EOF
  openssl x509 -req -sha256 -days 2 -in "$PKI/$name.csr"     -CA "$PKI/ca.crt" -CAkey "$PKI/ca.key" -CAcreateserial     -extfile "$PKI/$name.ext" -out "$PKI/$name.crt" >/dev/null 2>&1
  chmod 0644 "$PKI/$name.crt"
}

openssl genrsa -out "$PKI/server.key" 2048 >/dev/null 2>&1
chmod 0600 "$PKI/server.key"
openssl req -new -key "$PKI/server.key" -subj "/CN=127.0.0.1" -out "$PKI/server.csr" >/dev/null 2>&1
cat > "$PKI/server.ext" <<'EOF'
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=IP:127.0.0.1
EOF
openssl x509 -req -sha256 -days 2 -in "$PKI/server.csr"   -CA "$PKI/ca.crt" -CAkey "$PKI/ca.key" -CAcreateserial   -extfile "$PKI/server.ext" -out "$PKI/server.crt" >/dev/null 2>&1
chmod 0644 "$PKI/server.crt"

issue_client owner "urn:engineering-platform:operator:pilot-owner"
issue_client worker "urn:engineering-platform:worker:codex-pilot"
issue_client publisher "urn:engineering-platform:operator:pilot-publisher"
issue_client codex-importer "urn:engineering-platform:codex-evidence-importer"
issue_client git-importer "urn:engineering-platform:git-evidence-importer"
issue_client ci-importer "urn:engineering-platform:github-ci-importer"
issue_client verifier "urn:engineering-platform:verifier:pilot"

printf '%s' "$PUBLISHER_GITHUB_TOKEN" > "$TOKEN_FILE"
chmod 0600 "$TOKEN_FILE"
unset PUBLISHER_GITHUB_TOKEN

jq -n   --arg root "$PREP_ROOT/artifacts"   --arg git "$GIT_EXECUTABLE"   --arg token "$TOKEN_FILE"   '{
    version:1,
    artifact_root:$root,
    git_executable:$git,
    token_file:$token,
    targets:[{
      repository:"jiying2007/engineering-platform",
      base_ref:"main",
      branch_prefix:"engineering-platform/"
    }]
  }' > "$PUBLISHER_CONFIG"
chmod 0600 "$PUBLISHER_CONFIG"

env   LISTEN_HOST=127.0.0.1 PORT=8443   DATABASE_URL="$DB_URL" AUTO_MIGRATE=1   CONTROL_TLS_CERT_FILE="$PKI/server.crt"   CONTROL_TLS_KEY_FILE="$PKI/server.key"   CONTROL_CLIENT_CA_FILE="$PKI/ca.crt"   CONTROL_AUTH_POLICY_FILE="$POLICY_FILE"   GITHUB_PUBLISHER_CONFIG_FILE="$PUBLISHER_CONFIG"   "$BIN/control-plane" >"$CONTROL_LOG" 2>&1 &
echo $! > "$CONTROL_PID_FILE"

for _ in $(seq 1 40); do
  if curl -fsS --cacert "$PKI/ca.crt" --cert "$PKI/owner.crt" --key "$PKI/owner.key" "$CONTROL_ENDPOINT/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
curl -fsS --cacert "$PKI/ca.crt" --cert "$PKI/owner.crt" --key "$PKI/owner.key" "$CONTROL_ENDPOINT/healthz" >/dev/null || {
  cat "$CONTROL_LOG" >&2
  die "control plane did not become ready"
}

eng_as() {
  role="$1"; shift
  env     CONTROL_ENDPOINT="$CONTROL_ENDPOINT"     CONTROL_CLIENT_CERT_FILE="$PKI/$role.crt"     CONTROL_CLIENT_KEY_FILE="$PKI/$role.key"     CONTROL_SERVER_CA_FILE="$PKI/ca.crt"     "$BIN/eng" "$@"
}

worker_env() {
  env     CONTROL_ENDPOINT="$CONTROL_ENDPOINT"     CONTROL_CLIENT_CERT_FILE="$PKI/worker.crt"     CONTROL_CLIENT_KEY_FILE="$PKI/worker.key"     CONTROL_SERVER_CA_FILE="$PKI/ca.crt"     WORKER_PREPARATION_CONFIG="$ROOT/worker-preparation.json"     "$@"
}

case "$PILOT_MODE" in
  feature)
    PILOT_DIR="examples/pilots/feature-routing"
    STEM="m1-feature-routing"
    ;;
  debug)
    PILOT_DIR="examples/pilots/debug-firmware-identity"
    STEM="m1-debug-firmware-identity"
    ;;
esac
RUN_ID="$STEM-run"

sed -e 's#__HUMAN_OWNER__#urn:engineering-platform:operator:pilot-owner#g'   "$PILOT_DIR/work.json.tmpl" > "$ROOT/work.json"
sed -e "s#__BASE_COMMIT__#$BASE_COMMIT#g"   "$PILOT_DIR/task.json.tmpl" > "$ROOT/task.json"

eng_as owner api POST /api/v1/work-items "$ROOT/work.json" > "$OUT/work-response.json"
eng_as owner api POST /api/v1/task-contracts "$ROOT/task.json" > "$OUT/task-response.json"
TASK_DIGEST="$(jq -er .digest "$OUT/task-response.json")"
test "$(jq -er .readiness.status "$OUT/task-response.json")" = READY || die "pilot Task is not READY"

sed   -e "s#__TASK_DIGEST__#$TASK_DIGEST#g"   -e "s#__PROFILE_DIGEST__#$PROFILE_DIGEST#g"   -e "s#__WORKER_PROFILE__#$WORKER_PROFILE#g"   "$PILOT_DIR/run.json.tmpl" > "$ROOT/run.json"
eng_as owner api POST /api/v1/runs "$ROOT/run.json" > "$OUT/run-response.json"
RUN_INPUT_DIGEST="$(jq -er .run.run_input_manifest_digest "$OUT/run-response.json")"
EXECUTION_EPOCH="$(jq -er .run.current_epoch "$OUT/run-response.json")"
test "$EXECUTION_EPOCH" = 1 || die "unexpected initial execution epoch"

sed   -e "s#__PREPARATION_ROOT__#$PREP_ROOT#g"   -e "s#__GIT_EXECUTABLE__#$GIT_EXECUTABLE#g"   -e "s#__CONTEXT_SOURCE__#$CONTEXT_SOURCE#g"   -e "s#__RUN_ID__#$RUN_ID#g"   -e "s#__TASK_DIGEST__#$TASK_DIGEST#g"   -e "s#__RUN_INPUT_DIGEST__#$RUN_INPUT_DIGEST#g"   -e "s#__REPOSITORY_PATH__#$GITHUB_WORKSPACE#g"   examples/pilots/worker-preparation.json.tmpl > "$ROOT/worker-preparation.json"
chmod 0600 "$ROOT/worker-preparation.json"

prepared=false
for _ in $(seq 1 30); do
  output="$(worker_env "$BIN/worker" --profile "$WORKER_PROFILE" --prepare-only --once)"
  if test -n "$output"; then
    printf '%s\n' "$output" > "$OUT/preparation-receipt.json"
    prepared=true
    break
  fi
  sleep 1
done
$prepared || die "worker preparation did not claim the Run"

jq --arg rule "$OPENAI_FEDERATION_RULE_ID"   '{
    version:1,
    codex_executable:.codex_executable,
    federation_rule_id:$rule,
    profile:.profile
  }' "$PROFILE_JSON" > "$ROOT/worker-codex.json"
chmod 0600 "$ROOT/worker-codex.json"

install -d -m 0700 "$ROOT/openai-wif"
encoded="$(jq -rn --arg audience "$OPENAI_WIF_AUDIENCE" '$audience|@uri')"
identity_token="$(curl -fsSL -H "Authorization: bearer $ACTIONS_ID_TOKEN_REQUEST_TOKEN" "${ACTIONS_ID_TOKEN_REQUEST_URL}&audience=${encoded}" | jq -er .value)"
printf '%s' "$identity_token" > "$ROOT/openai-wif/identity-token"
unset identity_token
chmod 0600 "$ROOT/openai-wif/identity-token"

worker_env   WORKER_CODEX_CONFIG="$ROOT/worker-codex.json"   OPENAI_IDENTITY_TOKEN_FILE="$ROOT/openai-wif/identity-token"   "$BIN/worker" --profile "$WORKER_PROFILE" --execute-codex --run "$RUN_ID" --once   > "$OUT/codex-worker-receipt.json"

test ! -e "$ROOT/openai-wif/identity-token" || die "upstream WIF assertion survived the model turn"

eng_as owner api GET "/api/v1/runs/$RUN_ID/codex" > "$OUT/codex-status.json"
test "$(jq -er .state "$OUT/codex-status.json")" = FINISHED || die "Codex execution is not FINISHED"
RESULT_DIGEST="$(jq -er .receipt.result_digest "$OUT/codex-status.json")"
RESULT_COMMIT="$(jq -er .receipt.result.change.result_commit "$OUT/codex-status.json")"
BUNDLE_DIGEST="$(jq -er .receipt.result.change.bundle_digest "$OUT/codex-status.json")"
BUNDLE_SIZE="$(jq -er .receipt.result.change.bundle_size "$OUT/codex-status.json")"
EXECUTION_ID="$(jq -er .token.execution_id "$OUT/codex-status.json")"
BUNDLE_PATH="$PREP_ROOT/artifacts/$EXECUTION_ID.bundle"
test -f "$BUNDLE_PATH" || die "retained result bundle missing"
test "$(stat -c%s "$BUNDLE_PATH")" = "$BUNDLE_SIZE" || die "bundle size mismatch"
test "sha256:$(sha256sum "$BUNDLE_PATH" | awk '{print $1}')" = "$BUNDLE_DIGEST" || die "bundle digest mismatch"
cp "$BUNDLE_PATH" "$OUT/result.bundle"

git clone -q --no-hardlinks "$GITHUB_WORKSPACE" "$OPERATOR_REPO"
git -C "$OPERATOR_REPO" fetch -q "$BUNDLE_PATH" HEAD:refs/heads/pilot-result
test "$(git -C "$OPERATOR_REPO" rev-parse refs/heads/pilot-result)" = "$RESULT_COMMIT" || die "bundle result commit mismatch"
test -z "$(git -C "$OPERATOR_REPO" diff --name-only "$BASE_COMMIT" "$RESULT_COMMIT" -- .github/workflows)" || die "pilot result modifies trusted workflow files"

eng_as owner api GET /api/v1/recovery > "$OUT/recovery.json"
RECOVERY_EPOCH="$(jq -er .recovery_epoch "$OUT/recovery.json")"
sed   -e "s/__EXECUTION_EPOCH__/$EXECUTION_EPOCH/g"   -e "s/__RECOVERY_EPOCH__/$RECOVERY_EPOCH/g"   -e "s#__CODEX_RESULT_DIGEST__#$RESULT_DIGEST#g"   "$PILOT_DIR/publish.json.tmpl" > "$ROOT/publish.json"

eng_as publisher api POST "/api/v1/runs/$RUN_ID/actions" "$ROOT/publish.json" > "$OUT/publication-receipt.json"
ACTION_ID="$(jq -er .action_request_id "$ROOT/publish.json")"
eng_as publisher api GET "/api/v1/actions/$ACTION_ID" > "$OUT/publication-operation.json"
ACTION_STATE="$(jq -er .state "$OUT/publication-operation.json")"
if test "$ACTION_STATE" = UNKNOWN; then
  eng_as publisher api POST "/api/v1/actions/$ACTION_ID/reconcile" <(printf '{}') >/dev/null 2>&1 || true
  eng_as publisher api GET "/api/v1/actions/$ACTION_ID" > "$OUT/publication-operation.json"
  ACTION_STATE="$(jq -er .state "$OUT/publication-operation.json")"
fi
test "$ACTION_STATE" = CONFIRMED || die "publication did not reach CONFIRMED (state=$ACTION_STATE)"
PR_URL="$(jq -er .external_ref "$OUT/publication-operation.json")"
PR_NUMBER="${PR_URL##*/}"

{
  echo "### M1 retained pilot publication"
  echo
  echo "- Pilot: $PILOT_MODE"
  echo "- Result commit: `$RESULT_COMMIT`"
  echo "- Pull request: $PR_URL"
  echo
  echo "**GitHub-token-created PR CI requires a repository write user to choose _Approve workflows to run_.**"
} >> "$GITHUB_STEP_SUMMARY"
printf '::notice title=M1 pilot PR created::Approve the CI workflow on %s\n' "$PR_URL"

github_api() {
  path="$1"
  curl -fsSL     -H "Authorization: Bearer $(cat "$TOKEN_FILE")"     -H "Accept: application/vnd.github+json"     -H "X-GitHub-Api-Version: 2022-11-28"     "https://api.github.com$path"
}

deadline=$(( $(date +%s) + CI_WAIT_MINUTES * 60 ))
CI_RUN_ID=""
while test "$(date +%s)" -lt "$deadline"; do
  github_api "/repos/$GITHUB_REPOSITORY/actions/runs?head_sha=$RESULT_COMMIT&event=pull_request&per_page=100" > "$OUT/ci-runs.json"
  CI_RUN_ID="$(jq -r '[.workflow_runs[] | select(.name=="CI" and .path==".github/workflows/ci.yml")] | sort_by(.created_at) | last | .id // empty' "$OUT/ci-runs.json")"
  if test -n "$CI_RUN_ID"; then
    status="$(jq -r --argjson id "$CI_RUN_ID" '.workflow_runs[] | select(.id==$id) | .status' "$OUT/ci-runs.json")"
    conclusion="$(jq -r --argjson id "$CI_RUN_ID" '.workflow_runs[] | select(.id==$id) | .conclusion // ""' "$OUT/ci-runs.json")"
    note "CI run $CI_RUN_ID status=$status conclusion=$conclusion"
    if test "$status" = completed -a "$conclusion" = success; then
      break
    fi
    case "$conclusion" in
      failure|cancelled|timed_out|stale) die "exact PR-head CI failed: $conclusion" ;;
    esac
  else
    note "waiting for exact PR-head CI run for $RESULT_COMMIT"
  fi
  sleep 15
done
test -n "$CI_RUN_ID" || die "no exact PR-head CI run appeared"
github_api "/repos/$GITHUB_REPOSITORY/actions/runs/$CI_RUN_ID" > "$OUT/ci-run.json"
test "$(jq -er .status "$OUT/ci-run.json")" = completed || die "CI approval/completion timed out"
test "$(jq -er .conclusion "$OUT/ci-run.json")" = success || die "CI is not successful"
test "$(jq -er .head_sha "$OUT/ci-run.json")" = "$RESULT_COMMIT" || die "CI head SHA mismatch"
test "$(jq -er .event "$OUT/ci-run.json")" = pull_request || die "CI event is not pull_request"

github_api "/repos/$GITHUB_REPOSITORY/actions/runs/$CI_RUN_ID/artifacts?per_page=100" > "$OUT/ci-artifacts.json"
download_artifact() {
  name="$1"; output="$2"
  id="$(jq -er --arg name "$name" '.artifacts[] | select(.name==$name) | .id' "$OUT/ci-artifacts.json")"
  digest="$(jq -er --arg name "$name" '.artifacts[] | select(.name==$name) | .digest' "$OUT/ci-artifacts.json")"
  size="$(jq -er --arg name "$name" '.artifacts[] | select(.name==$name) | .size_in_bytes' "$OUT/ci-artifacts.json")"
  curl -fsSL -L     -H "Authorization: Bearer $(cat "$TOKEN_FILE")"     -H "Accept: application/vnd.github+json"     -H "X-GitHub-Api-Version: 2022-11-28"     "https://api.github.com/repos/$GITHUB_REPOSITORY/actions/artifacts/$id/zip"     -o "$output"
  test "$(stat -c%s "$output")" = "$size" || die "artifact size mismatch: $name"
  test "sha256:$(sha256sum "$output" | awk '{print $1}')" = "$digest" || die "artifact digest mismatch: $name"
  printf '%s\n' "$digest"
}

CI_ZIP="$OUT/trusted-ci-evidence.zip"
BINARIES_ZIP="$OUT/engineering-binaries.zip"
CODEX_ZIP="$OUT/codex-qualification.zip"
CI_ENVELOPE_DIGEST="$(download_artifact "trusted-ci-evidence-$RESULT_COMMIT" "$CI_ZIP")"
download_artifact "engineering-binaries-$RESULT_COMMIT" "$BINARIES_ZIP" >/dev/null
download_artifact "codex-0.155.0-qualification-$RESULT_COMMIT" "$CODEX_ZIP" >/dev/null

"$BIN/eng" git-change-manifest   --repository "$OPERATOR_REPO"   --base "$BASE_COMMIT"   --result "$RESULT_COMMIT"   --output "$OUT/git-change.json" > "$OUT/git-change-command.json"
GIT_MANIFEST_DIGEST="$(jq -er .artifact_digest "$OUT/git-change-command.json")"

eng_as owner codex-receipt-digest --run "$RUN_ID" > "$OUT/codex-receipt-digest.json"
CODEX_RECEIPT_DIGEST="$(jq -er .artifact_digest "$OUT/codex-receipt-digest.json")"
test "$(jq -er .bundle_digest "$OUT/codex-receipt-digest.json")" = "$BUNDLE_DIGEST" || die "Codex digest command disagrees on bundle"

sed -e "s/__EXECUTION_EPOCH__/$EXECUTION_EPOCH/g"   "$PILOT_DIR/complete.json.tmpl" > "$ROOT/complete.json"
eng_as owner api POST "/api/v1/runs/$RUN_ID/complete" "$ROOT/complete.json" > "$OUT/complete-response.json"

sed   -e "s#__RESULT_COMMIT__#$RESULT_COMMIT#g"   -e "s#__CODEX_RECEIPT_DIGEST__#$CODEX_RECEIPT_DIGEST#g"   -e "s#__BUNDLE_DIGEST__#$BUNDLE_DIGEST#g"   -e "s#__GIT_MANIFEST_DIGEST__#$GIT_MANIFEST_DIGEST#g"   -e "s#__CI_ENVELOPE_DIGEST__#$CI_ENVELOPE_DIGEST#g"   "$PILOT_DIR/delivery.json.tmpl" > "$ROOT/delivery.json"
eng_as owner api POST /api/v1/deliveries "$ROOT/delivery.json" > "$OUT/delivery.json"
DELIVERY_ID="$(jq -er .delivery_receipt_id "$OUT/delivery.json")"

if test "$PILOT_MODE" = feature; then
  CODEX_REQ=feature-codex; CODEX_EVID=feature-codex-evidence
  GIT_REQ=feature-git; GIT_EVID=feature-git-evidence
  CI_REQ=feature-ci; CI_EVID=feature-ci-evidence
else
  CODEX_REQ=debug-codex; CODEX_EVID=debug-codex-evidence
  GIT_REQ=debug-git; GIT_EVID=debug-git-evidence
  CI_REQ=debug-ci; CI_EVID=debug-ci-evidence
fi

eng_as codex-importer import-codex-evidence   --delivery "$DELIVERY_ID" --requirement "$CODEX_REQ" --evidence "$CODEX_EVID"   --receipt-artifact codex-execution-receipt --bundle-artifact codex-result-bundle   --bundle "$BUNDLE_PATH" > "$OUT/codex-evidence.json"

eng_as git-importer import-git-change-evidence   --repository "$OPERATOR_REPO" --manifest "$OUT/git-change.json"   --delivery "$DELIVERY_ID" --requirement "$GIT_REQ" --evidence "$GIT_EVID"   --artifact git-change-manifest > "$OUT/git-evidence.json"

env   CONTROL_ENDPOINT="$CONTROL_ENDPOINT"   CONTROL_CLIENT_CERT_FILE="$PKI/ci-importer.crt"   CONTROL_CLIENT_KEY_FILE="$PKI/ci-importer.key"   CONTROL_SERVER_CA_FILE="$PKI/ca.crt"   GITHUB_TOKEN_FILE="$TOKEN_FILE"   "$BIN/eng" import-ci-evidence   --delivery "$DELIVERY_ID" --requirement "$CI_REQ" --evidence "$CI_EVID"   --artifact ci-provenance   --envelope-zip "$CI_ZIP" --binaries-zip "$BINARIES_ZIP" --codex-zip "$CODEX_ZIP"   > "$OUT/ci-evidence.json"

eng_as verifier api POST /api/v1/verifications "$PILOT_DIR/verification.json" > "$OUT/verification.json"
test "$(jq -er .result "$OUT/verification.json")" = PASS || die "Verification is not PASS"

jq -n   --arg pilot "$PILOT_MODE"   --arg base "$BASE_COMMIT"   --arg result "$RESULT_COMMIT"   --arg run "$RUN_ID"   --arg task_digest "$TASK_DIGEST"   --arg profile_digest "$PROFILE_DIGEST"   --arg codex_result_digest "$RESULT_DIGEST"   --arg codex_receipt_digest "$CODEX_RECEIPT_DIGEST"   --arg bundle_digest "$BUNDLE_DIGEST"   --arg git_manifest_digest "$GIT_MANIFEST_DIGEST"   --arg ci_envelope_digest "$CI_ENVELOPE_DIGEST"   --arg pr_url "$PR_URL"   --argjson ci_run_id "$CI_RUN_ID"   --arg delivery_id "$DELIVERY_ID"   --arg verification_id "$(jq -er .verification_report_id "$OUT/verification.json")"   '{
    schema_version:1,
    pilot:$pilot,
    base_commit:$base,
    result_commit:$result,
    run_id:$run,
    task_digest:$task_digest,
    profile_digest:$profile_digest,
    codex_result_digest:$codex_result_digest,
    codex_receipt_digest:$codex_receipt_digest,
    bundle_digest:$bundle_digest,
    git_manifest_digest:$git_manifest_digest,
    ci_envelope_digest:$ci_envelope_digest,
    pull_request_url:$pr_url,
    ci_run_id:$ci_run_id,
    delivery_id:$delivery_id,
    verification_report_id:$verification_id,
    state:"VERIFIED_AWAITING_INDEPENDENT_REVIEW"
  }' > "$STATE_JSON"

docker run --rm --network host -e PGPASSWORD=postgres -v "$OUT:/out" postgres:17-alpine   pg_dump -h 127.0.0.1 -U postgres -d engineering_platform -Fc -f /out/core-state.dump
cp "$POLICY_FILE" "$OUT/access-policy.json"

{
  echo
  echo "### Verification complete"
  echo
  echo "- Delivery: `$DELIVERY_ID`"
  echo "- Verification: `$(jq -er .verification_report_id "$OUT/verification.json")` PASS"
  echo "- State artifact is ready for an independent Review/Closure continuation."
} >> "$GITHUB_STEP_SUMMARY"

note "retained $PILOT_MODE pilot reached PASS Verification; independent Review remains pending"
