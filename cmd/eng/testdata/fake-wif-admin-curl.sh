#!/usr/bin/env bash
set -euo pipefail

url=""
data=""
header=""
args=("$@")
while [ "$#" -gt 0 ]; do
  case "$1" in
    https://*) url="$1" ;;
    -H)
      shift
      case "$1" in @*) header="${1#@}" ;; esac
      ;;
    --data)
      shift
      case "$1" in @*) data="${1#@}" ;; esac
      ;;
  esac
  shift
done

printf '%s\n' "${args[*]}" >> "$FAKE_CURL_LOG"
test -n "$header"
grep -qx 'Authorization: Bearer test-admin-key' "$header"

count=0
if [ -f "$FAKE_CURL_COUNT" ]; then
  count="$(cat "$FAKE_CURL_COUNT")"
fi
count=$((count + 1))
printf '%s\n' "$count" > "$FAKE_CURL_COUNT"

mode="${FAKE_WIF_MODE:-create}"
provider_url="https://api.openai.com/v1/organization/workload_identity/providers"
mapping_url="https://api.openai.com/v1/organization/workload_identity/providers/idp_test/mappings"

provider_exact='{"id":"idp_test","name":"engineering-platform-github-actions-codex","type":"oidc","issuer":"https://token.actions.githubusercontent.com","audience":"aud-pilot","description":"engineering-platform protected-main Codex retained pilots","max_assertion_lifetime_seconds":600,"check_jti":true,"enabled":true}'
provider_drift='{"id":"idp_test","name":"engineering-platform-github-actions-codex","type":"oidc","issuer":"https://token.actions.githubusercontent.com","audience":"wrong-audience","description":"drift","max_assertion_lifetime_seconds":600,"check_jti":true,"enabled":true}'
rule_exact='{"id":"idpm_test","name":"engineering-platform-retained-pilot-main","description":"engineering-platform protected-main live qualification and retained engineering","workspace_id":"ws_test","principal_id":"usr_test","claims":{"repository":"jiying2007/engineering-platform","ref":"refs/heads/main"},"audiences":["aud-pilot"],"condition":"assertion.workflow_ref in [\"jiying2007/engineering-platform/.github/workflows/codex-wif-live.yml@refs/heads/main\", \"jiying2007/engineering-platform/.github/workflows/retained-pilot-engineer.yml@refs/heads/main\"]","access_token_lifetime_seconds":600,"enabled":true}'

case "$mode:$count" in
  create:1)
    test "$url" = "$provider_url"
    test -z "$data"
    printf '%s\n' '{"object":"list","data":[]}'
    ;;
  create:2)
    test "$url" = "$provider_url"
    test -n "$data"
    jq -e '
      .name=="engineering-platform-github-actions-codex" and
      .type=="oidc" and
      .issuer=="https://token.actions.githubusercontent.com" and
      .audience=="aud-pilot" and
      .max_assertion_lifetime_seconds==600 and
      .check_jti==true and
      .enabled==true
    ' "$data" >/dev/null
    printf '%s\n' "$provider_exact"
    ;;
  create:3)
    test "$url" = "$mapping_url"
    test -z "$data"
    printf '%s\n' '{"object":"list","data":[]}'
    ;;
  create:4)
    test "$url" = "$mapping_url"
    test -n "$data"
    jq -e '
      .name=="engineering-platform-retained-pilot-main" and
      .workspace_id=="ws_test" and
      .principal_id=="usr_test" and
      .claims.repository=="jiying2007/engineering-platform" and
      .claims.ref=="refs/heads/main" and
      .audiences==["aud-pilot"] and
      .condition=="assertion.workflow_ref in [\"jiying2007/engineering-platform/.github/workflows/codex-wif-live.yml@refs/heads/main\", \"jiying2007/engineering-platform/.github/workflows/retained-pilot-engineer.yml@refs/heads/main\"]" and
      .access_token_lifetime_seconds==600 and
      .enabled==true
    ' "$data" >/dev/null
    printf '%s\n' "$rule_exact"
    ;;
  existing:1)
    printf '{"object":"list","data":[%s]}\n' "$provider_exact"
    ;;
  existing:2)
    test "$url" = "$mapping_url"
    printf '{"object":"list","data":[%s]}\n' "$rule_exact"
    ;;
  drift:1)
    printf '{"object":"list","data":[%s]}\n' "$provider_drift"
    ;;
  *)
    echo "unexpected fake curl call mode=$mode count=$count url=$url data=$data" >&2
    exit 1
    ;;
esac
