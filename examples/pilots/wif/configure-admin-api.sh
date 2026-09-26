#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  cat >&2 <<'EOF'
usage: configure-admin-api.sh OUTPUT_DIR

Required environment:
  OPENAI_ADMIN_KEY   Organization Admin API key with workload-identity permission
  WORKSPACE_ID       Existing managed ChatGPT workspace ID
  PRINCIPAL_ID       Existing active ChatGPT user/service-account OpenAI user ID
  OPENAI_WIF_AUDIENCE Dedicated GitHub OIDC audience

Optional environment:
  SET_GITHUB_VARIABLES=1  Also write the two non-secret repository Actions variables
EOF
  exit 2
}

[ "$#" -eq 1 ] || usage
OUTPUT_DIR="$1"
case "$OUTPUT_DIR" in
  /*) ;;
  *) echo "OUTPUT_DIR must be absolute" >&2; exit 2 ;;
esac

: "${OPENAI_ADMIN_KEY:?OPENAI_ADMIN_KEY required}"
: "${WORKSPACE_ID:?WORKSPACE_ID required}"
: "${PRINCIPAL_ID:?PRINCIPAL_ID required}"
: "${OPENAI_WIF_AUDIENCE:?OPENAI_WIF_AUDIENCE required}"

if [[ "$OPENAI_ADMIN_KEY" == *$'\n'* || "$OPENAI_ADMIN_KEY" == *$'\r'* ]]; then
  echo "OPENAI_ADMIN_KEY must not contain line breaks" >&2
  exit 2
fi
for value in "$WORKSPACE_ID" "$PRINCIPAL_ID" "$OPENAI_WIF_AUDIENCE"; do
  [ -n "$value" ] || usage
  test "$value" = "${value#"${value%%[![:space:]]*}"}"
  test "$value" = "${value%"${value##*[![:space:]]}"}"
done

command -v curl >/dev/null
command -v jq >/dev/null

install -d -m 0700 "$OUTPUT_DIR"

ADMIN_HEADER_FILE="$OUTPUT_DIR/.openai-admin-header"
printf 'Authorization: Bearer %s\n' "$OPENAI_ADMIN_KEY" > "$ADMIN_HEADER_FILE"
chmod 600 "$ADMIN_HEADER_FILE"
trap 'rm -f "$ADMIN_HEADER_FILE"' EXIT

API_BASE="https://api.openai.com"
PROVIDER_NAME="engineering-platform-github-actions-codex"
RULE_NAME="engineering-platform-retained-pilot-main"
ISSUER="https://token.actions.githubusercontent.com"
REPOSITORY="jiying2007/engineering-platform"
REF="refs/heads/main"
LIVE_WORKFLOW_REF="jiying2007/engineering-platform/.github/workflows/codex-wif-live.yml@refs/heads/main"
ENGINEER_WORKFLOW_REF="jiying2007/engineering-platform/.github/workflows/retained-pilot-engineer.yml@refs/heads/main"
CONDITION="assertion.workflow_ref in [\"$LIVE_WORKFLOW_REF\", \"$ENGINEER_WORKFLOW_REF\"]"

api_get() {
  local path="$1"
  curl --fail-with-body --silent --show-error     "$API_BASE$path"     -H @"$ADMIN_HEADER_FILE"
}

api_post() {
  local path="$1"
  local file="$2"
  curl --fail-with-body --silent --show-error     "$API_BASE$path"     -H @"$ADMIN_HEADER_FILE"     -H "Content-Type: application/json"     --data @"$file"
}

providers="$(api_get /v1/organization/workload_identity/providers)"
provider_count="$(
  printf '%s' "$providers" |
    jq -er --arg name "$PROVIDER_NAME" '[.data[] | select(.name==$name)] | length'
)"
if [ "$provider_count" -gt 1 ]; then
  echo "multiple providers share $PROVIDER_NAME" >&2
  exit 1
fi

PROVIDER_REQUEST="$OUTPUT_DIR/provider-request.json"
jq -n   --arg name "$PROVIDER_NAME"   --arg issuer "$ISSUER"   --arg audience "$OPENAI_WIF_AUDIENCE"   '{
    name:$name,
    type:"oidc",
    issuer:$issuer,
    audience:$audience,
    description:"engineering-platform protected-main Codex retained pilots",
    max_assertion_lifetime_seconds:600,
    check_jti:true,
    enabled:true
  }' > "$PROVIDER_REQUEST"
chmod 600 "$PROVIDER_REQUEST"

if [ "$provider_count" -eq 0 ]; then
  provider="$(api_post /v1/organization/workload_identity/providers "$PROVIDER_REQUEST")"
else
  provider="$(
    printf '%s' "$providers" |
      jq -ec --arg name "$PROVIDER_NAME" '.data[] | select(.name==$name)'
  )"
fi

printf '%s' "$provider" |
  jq -e     --arg name "$PROVIDER_NAME"     --arg issuer "$ISSUER"     --arg audience "$OPENAI_WIF_AUDIENCE"     '
      .name==$name and
      .type=="oidc" and
      .issuer==$issuer and
      .audience==$audience and
      .enabled==true and
      .check_jti==true
    ' >/dev/null || {
      echo "existing/created provider does not match required trust policy" >&2
      exit 1
    }

PROVIDER_ID="$(printf '%s' "$provider" | jq -er '.id | select(test("^idp_[A-Za-z0-9_-]+$"))')"

rules="$(api_get "/v1/organization/workload_identity/providers/$PROVIDER_ID/mappings")"
rule_count="$(
  printf '%s' "$rules" |
    jq -er --arg name "$RULE_NAME" '[.data[] | select(.name==$name)] | length'
)"
if [ "$rule_count" -gt 1 ]; then
  echo "multiple federation rules share $RULE_NAME" >&2
  exit 1
fi

RULE_REQUEST="$OUTPUT_DIR/rule-request.json"
jq -n   --arg name "$RULE_NAME"   --arg workspace_id "$WORKSPACE_ID"   --arg principal_id "$PRINCIPAL_ID"   --arg repository "$REPOSITORY"   --arg ref "$REF"   --arg audience "$OPENAI_WIF_AUDIENCE"   --arg condition "$CONDITION"   '{
    name:$name,
    description:"engineering-platform protected-main live qualification and retained engineering",
    workspace_id:$workspace_id,
    principal_id:$principal_id,
    claims:{
      repository:$repository,
      ref:$ref
    },
    audiences:[$audience],
    condition:$condition,
    access_token_lifetime_seconds:600,
    enabled:true
  }' > "$RULE_REQUEST"
chmod 600 "$RULE_REQUEST"

if [ "$rule_count" -eq 0 ]; then
  rule="$(api_post "/v1/organization/workload_identity/providers/$PROVIDER_ID/mappings" "$RULE_REQUEST")"
else
  rule="$(
    printf '%s' "$rules" |
      jq -ec --arg name "$RULE_NAME" '.data[] | select(.name==$name)'
  )"
fi

printf '%s' "$rule" |
  jq -e     --arg name "$RULE_NAME"     --arg workspace_id "$WORKSPACE_ID"     --arg principal_id "$PRINCIPAL_ID"     --arg repository "$REPOSITORY"     --arg ref "$REF"     --arg audience "$OPENAI_WIF_AUDIENCE"     --arg condition "$CONDITION"     '
      .name==$name and
      .workspace_id==$workspace_id and
      .principal_id==$principal_id and
      .enabled==true and
      .claims.repository==$repository and
      .claims.ref==$ref and
      .audiences==[$audience] and
      .condition==$condition and
      .access_token_lifetime_seconds==600
    ' >/dev/null || {
      echo "existing/created federation rule does not match required pilot policy" >&2
      exit 1
    }

FEDERATION_RULE_ID="$(printf '%s' "$rule" | jq -er '.id | select(test("^idpm_[A-Za-z0-9_-]+$"))')"

RECEIPT="$OUTPUT_DIR/wif-admin-receipt.json"
jq -n   --arg provider_id "$PROVIDER_ID"   --arg federation_rule_id "$FEDERATION_RULE_ID"   --arg workspace_id "$WORKSPACE_ID"   --arg principal_id "$PRINCIPAL_ID"   --arg audience "$OPENAI_WIF_AUDIENCE"   --arg repository "$REPOSITORY"   --arg ref "$REF"   --arg condition "$CONDITION"   '{
    version:1,
    provider_id:$provider_id,
    federation_rule_id:$federation_rule_id,
    workspace_id:$workspace_id,
    principal_id:$principal_id,
    audience:$audience,
    repository:$repository,
    ref:$ref,
    condition:$condition
  }' > "$RECEIPT"
chmod 600 "$RECEIPT"

if [ "${SET_GITHUB_VARIABLES:-0}" = 1 ]; then
  command -v gh >/dev/null
  gh variable set OPENAI_WIF_AUDIENCE     --repo "$REPOSITORY"     --body "$OPENAI_WIF_AUDIENCE"
  gh variable set OPENAI_CODEX_FEDERATION_RULE_ID     --repo "$REPOSITORY"     --body "$FEDERATION_RULE_ID"
fi

cat <<EOF
managed-workspace Codex WIF admin configuration: READY
provider_id=$PROVIDER_ID
federation_rule_id=$FEDERATION_RULE_ID
audience=$OPENAI_WIF_AUDIENCE
receipt=$RECEIPT
next=bash examples/pilots/wif/qualify.sh '$OPENAI_WIF_AUDIENCE' '$FEDERATION_RULE_ID' /operator/codex-profile.json /operator/wif
EOF
