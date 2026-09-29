#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  cat >&2 <<'EOF'
usage: qualify-login.sh OUTPUT_DIR CODEX_NATIVE SAVED_LOGIN_FILE [MODEL]

Qualifies one trusted self-hosted Codex ChatGPT saved login without WIF.
The saved login is copied into a fresh isolated Codex HOME only for auth
prewarm, then the bootstrap auth.json is deleted before the read-only model turn.
EOF
  exit 2
}

[ "$#" -ge 3 ] && [ "$#" -le 4 ] || usage
OUTPUT_DIR="$1"
CODEX_NATIVE="$2"
SAVED_LOGIN_FILE="$3"
MODEL="${4:-gpt-5.6-sol}"

for path in "$OUTPUT_DIR" "$CODEX_NATIVE" "$SAVED_LOGIN_FILE"; do
  case "$path" in
    /*) ;;
    *) echo "absolute paths required" >&2; exit 2 ;;
  esac
done

for key in OPENAI_API_KEY OPENAI_BASE_URL OPENAI_FEDERATION_RULE_ID OPENAI_IDENTITY_TOKEN_FILE OPENAI_WORKLOAD_IDENTITY_CONTEXT CODEX_API_KEY CODEX_ACCESS_TOKEN; do
  if [ -n "${!key:-}" ]; then
    echo "$key must be unset for saved ChatGPT login qualification" >&2
    exit 1
  fi
done

command -v jq >/dev/null
command -v sha256sum >/dev/null
command -v go >/dev/null

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)"
test -f "$ROOT/go.mod"

CODEX_NATIVE="$(readlink -f "$CODEX_NATIVE")"
SAVED_LOGIN_FILE="$(readlink -f "$SAVED_LOGIN_FILE")"
test -x "$CODEX_NATIVE"
test -f "$SAVED_LOGIN_FILE"

version="$("$CODEX_NATIVE" --version)"
[[ "$version" =~ ^codex-cli[[:space:]][0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]

login_status="$(
  env     -u OPENAI_API_KEY     -u OPENAI_BASE_URL     -u OPENAI_FEDERATION_RULE_ID     -u OPENAI_IDENTITY_TOKEN_FILE     -u OPENAI_WORKLOAD_IDENTITY_CONTEXT     -u CODEX_API_KEY     -u CODEX_ACCESS_TOKEN     "$CODEX_NATIVE" login status 2>&1
)"
test "$(printf '%s' "$login_status" | tr -d '\r' | xargs)" = "Logged in using ChatGPT"

install -d -m 0700 "$OUTPUT_DIR"
LOGIN_STAGE_ROOT="$OUTPUT_DIR/saved-login-source"
rm -rf -- "$LOGIN_STAGE_ROOT"
trap 'rm -rf -- "$LOGIN_STAGE_ROOT"' EXIT
STAGED_LOGIN_FILE="$(
  bash "$ROOT/examples/pilots/self-hosted/stage-saved-login.sh" \
    "$SAVED_LOGIN_FILE" \
    "$LOGIN_STAGE_ROOT"
)"
WORK="$OUTPUT_DIR/work"
HOME_DIR="$OUTPUT_DIR/codex-home"
BIN="$OUTPUT_DIR/bin"
rm -rf "$WORK" "$HOME_DIR" "$BIN"
install -d -m 0700 "$WORK" "$HOME_DIR" "$BIN"

BINARY_DIGEST="sha256:$(sha256sum "$CODEX_NATIVE" | awk '{print $1}')"
(
  cd "$ROOT"
  go build -trimpath -o "$BIN/codex-saved-login-live" ./cmd/codex-saved-login-live
)

RECEIPT="$OUTPUT_DIR/saved-login-live-receipt.json"
"$BIN/codex-saved-login-live"   --codex "$CODEX_NATIVE"   --digest "$BINARY_DIGEST"   --work "$WORK"   --home "$HOME_DIR"   --saved-login-file "$STAGED_LOGIN_FILE"   --model "$MODEL" > "$RECEIPT"
chmod 0600 "$RECEIPT"

jq -e   --arg digest "$BINARY_DIGEST"   --arg model "$MODEL"   '
   .schema_version==2 and
   .provider.provider_id=="openai-codex" and
   .provider.credential_mode=="chatgpt-session" and
   .provider.execution_mode=="trusted-self-hosted" and
   .binary_digest==$digest and
   .model==$model and
   .turn_status=="completed" and
   .output=="engineering-platform live qualification" and
   .assertion_removed_before_turn==false and
   .credential_bootstrap_removed_before_turn==true'   "$RECEIPT" >/dev/null

test ! -e "$HOME_DIR/.codex/auth.json"

META="$OUTPUT_DIR/self-hosted-login-qualification.json"
PROVIDER_JSON="$(jq -cS .provider "$RECEIPT")"
jq -n   --argjson provider "$PROVIDER_JSON"   --arg codex_version "$version"   --arg binary_digest "$BINARY_DIGEST"   --arg model "$MODEL"   --arg receipt "$RECEIPT"   '{
    version:2,
    provider:$provider,
    login_status:"Logged in using ChatGPT",
    codex_version:$codex_version,
    binary_digest:$binary_digest,
    model:$model,
    bootstrap_removed_before_model:true,
    receipt:$receipt
  }' > "$META"
chmod 0600 "$META"

echo "trusted self-hosted Codex login qualification: READY"
echo "provider_id=$(jq -r .provider_id <<<"$PROVIDER_JSON")"
echo "credential_mode=$(jq -r .credential_mode <<<"$PROVIDER_JSON")"
echo "execution_mode=$(jq -r .execution_mode <<<"$PROVIDER_JSON")"
echo "codex_version=$version"
echo "binary_digest=$BINARY_DIGEST"
echo "model=$MODEL"
echo "receipt=$RECEIPT"
echo "metadata=$META"
