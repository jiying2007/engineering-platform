#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  cat >&2 <<'EOF'
usage: qualify.sh AUDIENCE FEDERATION_RULE_ID CODEX_PROFILE_JSON OUTPUT_DIR [MODEL]

Configures repository Actions variables, triggers the protected-main
codex-wif-live workflow, waits for success, downloads the retained receipt and
renders worker-codex.json from the exact local codex-profile JSON.
EOF
  exit 2
}

[ "$#" -ge 4 ] && [ "$#" -le 5 ] || usage
AUDIENCE="$1"
RULE_ID="$2"
PROFILE_FILE="$3"
OUTPUT_DIR="$4"
MODEL="${5:-gpt-5.6-sol}"
REPO="jiying2007/engineering-platform"

[ -n "$AUDIENCE" ] || usage
[ -n "$RULE_ID" ] || usage
[ -r "$PROFILE_FILE" ] || { echo "codex profile is not readable" >&2; exit 2; }
case "$OUTPUT_DIR" in /*) ;; *) echo "OUTPUT_DIR must be absolute" >&2; exit 2;; esac

command -v gh >/dev/null
command -v git >/dev/null
command -v jq >/dev/null

mkdir -p "$OUTPUT_DIR"
chmod 700 "$OUTPUT_DIR"

PROFILE_MODEL="$(jq -er .profile.model "$PROFILE_FILE")"
if [ "$PROFILE_MODEL" != "$MODEL" ]; then
  echo "profile model does not match requested live model" >&2
  exit 2
fi
jq -e '
  .profile_digest | test("^sha256:[0-9a-f]{64}$")
' "$PROFILE_FILE" >/dev/null

MAIN_SHA="$(git rev-parse --verify refs/heads/main^{commit})"
REMOTE_MAIN="$(gh api "repos/$REPO/git/ref/heads/main" --jq .object.sha)"
if [ "$MAIN_SHA" != "$REMOTE_MAIN" ]; then
  echo "local main does not match remote main" >&2
  exit 1
fi

gh variable set OPENAI_WIF_AUDIENCE --repo "$REPO" --body "$AUDIENCE"
gh variable set OPENAI_CODEX_FEDERATION_RULE_ID --repo "$REPO" --body "$RULE_ID"

STARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
gh workflow run codex-wif-live.yml   --repo "$REPO"   --ref main   -f "model=$MODEL"

RUN_ID=""
for _ in $(seq 1 60); do
  RUN_ID="$(
    gh run list       --repo "$REPO"       --workflow codex-wif-live.yml       --event workflow_dispatch       --limit 20       --json databaseId,headSha,createdAt       --jq '.[] | select(.headSha == "'"$MAIN_SHA"'" and .createdAt >= "'"$STARTED_AT"'") | .databaseId' |
      head -n 1
  )"
  if [ -n "$RUN_ID" ]; then
    break
  fi
  sleep 1
done

if [ -z "$RUN_ID" ]; then
  echo "could not identify dispatched codex-wif-live run" >&2
  exit 1
fi

gh run watch "$RUN_ID" --repo "$REPO" --exit-status

ARTIFACT="codex-wif-live-$MAIN_SHA-$RUN_ID"
DOWNLOAD_DIR="$OUTPUT_DIR/run-$RUN_ID"
rm -rf "$DOWNLOAD_DIR"
mkdir -p "$DOWNLOAD_DIR"
chmod 700 "$DOWNLOAD_DIR"

gh run download "$RUN_ID"   --repo "$REPO"   --name "$ARTIFACT"   --dir "$DOWNLOAD_DIR"

RECEIPT_SOURCE="$DOWNLOAD_DIR/codex-wif-live-receipt.json"
[ -f "$RECEIPT_SOURCE" ] || {
  echo "live WIF receipt was not downloaded" >&2
  exit 1
}

jq -e   --arg model "$MODEL"   --arg rule "$RULE_ID"   '
    .schema_version == 1 and
    .cli == "codex-cli" and
    .version == "0.155.0" and
    .credential_mode == "workload_identity" and
    .federation_rule_id == $rule and
    .model == $model and
    .turn_status == "completed" and
    .output == "engineering-platform live qualification" and
    .approval_requests == 0 and
    .unexpected_tool_use == false and
    .assertion_removed_before_turn == true
  ' "$RECEIPT_SOURCE" >/dev/null

RECEIPT="$OUTPUT_DIR/codex-wif-live-receipt.json"
install -m 0600 "$RECEIPT_SOURCE" "$RECEIPT"

WORKER_CODEX="$OUTPUT_DIR/worker-codex.json"
jq --arg rule "$RULE_ID" '
  {
    version: 1,
    codex_executable: .codex_executable,
    federation_rule_id: $rule,
    profile: .profile
  }
' "$PROFILE_FILE" > "$WORKER_CODEX"
chmod 600 "$WORKER_CODEX"

cat <<EOF
codex WIF qualification: READY
repository=$REPO
main_sha=$MAIN_SHA
run_id=$RUN_ID
receipt=$RECEIPT
worker_codex=$WORKER_CODEX
EOF
