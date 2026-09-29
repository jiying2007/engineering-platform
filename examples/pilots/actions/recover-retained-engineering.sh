#!/usr/bin/env bash
set -euo pipefail
umask 077

: "${PILOT:?feature or debug required}"
: "${ENGINEERING_RUN_ID:?source engineering workflow run id required}"
: "${GH_TOKEN:?GitHub read token required}"
: "${GITHUB_REPOSITORY:?repository required}"
: "${GITHUB_RUN_ID:?recovery workflow run id required}"
: "${RUNNER_TEMP:?runner temp required}"

test "$GITHUB_REPOSITORY" = "jiying2007/engineering-platform"
case "$PILOT" in
  feature|debug) ;;
  *) echo "unsupported pilot $PILOT" >&2; exit 2 ;;
esac
[[ "$ENGINEERING_RUN_ID" =~ ^[0-9]+$ ]]

STATE_ROOT="$RUNNER_TEMP/retained-pilot-engineering-recovery"
ENGINEERING_ROOT="$STATE_ROOT/engineering"
install -d -m 0700 "$STATE_ROOT" "$ENGINEERING_ROOT"

run_json="$(gh api "repos/$GITHUB_REPOSITORY/actions/runs/$ENGINEERING_RUN_ID")"
test "$(printf '%s' "$run_json" | jq -er .name)" = "Retained M1 pilot engineering"
test "$(printf '%s' "$run_json" | jq -er .event)" = "workflow_dispatch"
test "$(printf '%s' "$run_json" | jq -er .status)" = "completed"
test "$(printf '%s' "$run_json" | jq -er .conclusion)" = "failure"
test "$(printf '%s' "$run_json" | jq -er .head_branch)" = "main"
test "$(printf '%s' "$run_json" | jq -er .path)" = ".github/workflows/retained-pilot-self-hosted-engineer.yml"
SOURCE_HEAD_SHA="$(printf '%s' "$run_json" | jq -er .head_sha)"
[[ "$SOURCE_HEAD_SHA" =~ ^[0-9a-f]{40}$ ]]

artifact_name="retained-pilot-engineering-$PILOT-$ENGINEERING_RUN_ID"
artifacts_json="$(gh api "repos/$GITHUB_REPOSITORY/actions/runs/$ENGINEERING_RUN_ID/artifacts?per_page=100")"
SOURCE_ARTIFACT_ID="$(printf '%s' "$artifacts_json" | jq -er --arg n "$artifact_name" '[.artifacts[]|select(.name==$n)] | if length==1 then .[0].id else error("source artifact identity missing/ambiguous") end')"
SOURCE_ARTIFACT_DIGEST="$(printf '%s' "$artifacts_json" | jq -er --arg n "$artifact_name" '[.artifacts[]|select(.name==$n)] | if length==1 then .[0].digest else error("source artifact digest missing/ambiguous") end')"
SOURCE_ARTIFACT_EXPIRED="$(printf '%s' "$artifacts_json" | jq -er --arg n "$artifact_name" '[.artifacts[]|select(.name==$n)] | if length==1 then (. [0].expired|tostring) else error("source artifact expiry missing/ambiguous") end')"
test "$SOURCE_ARTIFACT_EXPIRED" = false
[[ "$SOURCE_ARTIFACT_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]]

gh run download "$ENGINEERING_RUN_ID"   --repo "$GITHUB_REPOSITORY"   --name "$artifact_name"   --dir "$ENGINEERING_ROOT"

for required in   engineering-state.json   model-phase.json   codex-status.json   publication-receipt.json   preflight-before-publication.json   result.bundle   core.dump; do
  test -f "$ENGINEERING_ROOT/$required"
done
test -s "$ENGINEERING_ROOT/result.bundle"
test -s "$ENGINEERING_ROOT/core.dump"

ENGINEERING_STATE="$ENGINEERING_ROOT/engineering-state.json"
MODEL_PHASE="$ENGINEERING_ROOT/model-phase.json"
CODEX_STATUS="$ENGINEERING_ROOT/codex-status.json"
PUBLICATION_RECEIPT="$ENGINEERING_ROOT/publication-receipt.json"
PUBLICATION_PREFLIGHT="$ENGINEERING_ROOT/preflight-before-publication.json"

test "$(jq -er .version "$ENGINEERING_STATE")" = 2
test "$(jq -er .version "$MODEL_PHASE")" = 2
test "$(jq -er .pilot "$ENGINEERING_STATE")" = "$PILOT"
test "$(jq -er .github_engineering_run_id "$ENGINEERING_STATE")" = "$ENGINEERING_RUN_ID"
PROVIDER_JSON="$(jq -cS .provider "$ENGINEERING_STATE")"
test "$PROVIDER_JSON" = "$(jq -cS .provider "$MODEL_PHASE")"
printf '%s' "$PROVIDER_JSON" | jq -e '
  .provider_id=="openai-codex" and
  .credential_mode=="chatgpt-session" and
  .execution_mode=="trusted-self-hosted"
' >/dev/null
test "$(jq -er .model_phase "$MODEL_PHASE")" = FINISHED
test "$(jq -er .publication "$MODEL_PHASE")" = NOT_STARTED
test "$(jq -er .publication "$PUBLICATION_PREFLIGHT")" = READY
test "$(jq -er .result "$PUBLICATION_RECEIPT")" = CONFIRMED

BASE_COMMIT="$(jq -er .base_commit "$ENGINEERING_STATE")"
RESULT_COMMIT="$(jq -er .result_commit "$ENGINEERING_STATE")"
RESULT_DIGEST="$(jq -er .codex_result_digest "$ENGINEERING_STATE")"
BUNDLE_DIGEST="$(jq -er .bundle_digest "$ENGINEERING_STATE")"
PR_NUMBER="$(jq -er .pull_request.number "$ENGINEERING_STATE")"
PR_URL="$(jq -er .pull_request.url "$ENGINEERING_STATE")"
PR_BRANCH="$(jq -er .pull_request.branch "$ENGINEERING_STATE")"

[[ "$BASE_COMMIT" =~ ^[0-9a-f]{40}$ ]]
[[ "$RESULT_COMMIT" =~ ^[0-9a-f]{40}$ ]]
[[ "$RESULT_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]]
[[ "$BUNDLE_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]]
test "$BASE_COMMIT" = "$SOURCE_HEAD_SHA"
test "$BASE_COMMIT" != "$RESULT_COMMIT"
test "sha256:$(sha256sum "$ENGINEERING_ROOT/result.bundle" | awk '{print $1}')" = "$BUNDLE_DIGEST"

jq -e   --arg base "$BASE_COMMIT"   --arg result "$RESULT_COMMIT"   --arg result_digest "$RESULT_DIGEST"   --arg bundle "$BUNDLE_DIGEST"   --argjson provider "$PROVIDER_JSON"   '.state=="FINISHED" and
   .receipt.kind=="WORKER_ATTESTED_CODEX_EXECUTION" and
   .receipt.result_digest==$result_digest and
   .receipt.result.change.base_commit==$base and
   .receipt.result.change.result_commit==$result and
   .receipt.result.change.bundle_digest==$bundle and
   .receipt.result.codex.provider==$provider and
   .receipt.result.codex.turn_status=="completed" and
   .receipt.result.codex.approval_requests==0 and
   .receipt.result.codex.credential_bootstrap_removed_before_turn==true'   "$CODEX_STATUS" >/dev/null

test "$(jq -er .external_ref "$PUBLICATION_RECEIPT")" = "$PR_URL"
OBSERVED_STATE="$(jq -er .observed_state "$PUBLICATION_RECEIPT")"
printf '%s' "$OBSERVED_STATE" | jq -e   --arg base "$BASE_COMMIT"   --arg result "$RESULT_COMMIT"   --arg branch "$PR_BRANCH"   --argjson number "$PR_NUMBER"   '.base_ref=="main" and
   .base_commit==$base and
   .result_commit==$result and
   .branch==$branch and
   .pull_request_number==$number and
   .pull_request_state=="open" and
   .publication_outcome=="CREATED"' >/dev/null

pr_json="$(gh api "repos/$GITHUB_REPOSITORY/pulls/$PR_NUMBER")"
printf '%s' "$pr_json" | jq -e   --arg base "$BASE_COMMIT"   --arg result "$RESULT_COMMIT"   --arg branch "$PR_BRANCH"   '.state=="open" and
   .base.ref=="main" and
   .base.sha==$base and
   .head.sha==$result and
   .head.ref==$branch and
   .head.repo.id==.base.repo.id' >/dev/null
test "$(printf '%s' "$pr_json" | jq -er .html_url)" = "$PR_URL"

ACTION_RECEIPT_ID="$(jq -er .action_receipt_id "$PUBLICATION_RECEIPT")"
OPERATION_ID="$(jq -er .operation_id "$PUBLICATION_RECEIPT")"
test -n "$ACTION_RECEIPT_ID"
test -n "$OPERATION_ID"

jq -n   --arg pilot "$PILOT"   --arg source_engineering_run_id "$ENGINEERING_RUN_ID"   --arg source_head_sha "$SOURCE_HEAD_SHA"   --arg source_artifact_name "$artifact_name"   --arg source_artifact_id "$SOURCE_ARTIFACT_ID"   --arg source_artifact_digest "$SOURCE_ARTIFACT_DIGEST"   --arg base_commit "$BASE_COMMIT"   --arg result_commit "$RESULT_COMMIT"   --arg codex_result_digest "$RESULT_DIGEST"   --arg bundle_digest "$BUNDLE_DIGEST"   --argjson provider "$PROVIDER_JSON"   --arg action_receipt_id "$ACTION_RECEIPT_ID"   --arg operation_id "$OPERATION_ID"   --arg pr_url "$PR_URL"   --arg pr_branch "$PR_BRANCH"   --argjson pr_number "$PR_NUMBER"   --arg recovery_run_id "$GITHUB_RUN_ID"   '{
    version:2,
    provider:$provider,
    pilot:$pilot,
    source_engineering_run_id:($source_engineering_run_id|tonumber),
    source_engineering_head_sha:$source_head_sha,
    source_artifact:{
      name:$source_artifact_name,
      id:($source_artifact_id|tonumber),
      digest:$source_artifact_digest
    },
    model_phase:"FINISHED",
    publication:"CONFIRMED",
    base_commit:$base_commit,
    result_commit:$result_commit,
    codex_result_digest:$codex_result_digest,
    bundle_digest:$bundle_digest,
    publication_receipt:{
      action_receipt_id:$action_receipt_id,
      operation_id:$operation_id
    },
    pull_request:{
      number:$pr_number,
      url:$pr_url,
      branch:$pr_branch
    },
    recovery_run_id:($recovery_run_id|tonumber),
    model_replay:false,
    next_gate:"PASS_EXACT_PR_HEAD_CI"
  }' > "$STATE_ROOT/recovery-receipt.json"
chmod 0600 "$STATE_ROOT/recovery-receipt.json"

echo "retained engineering recovery: READY"
echo "source_engineering_run_id=$ENGINEERING_RUN_ID"
echo "model_phase=FINISHED"
echo "publication=CONFIRMED"
echo "model_replay=false"
echo "pr_url=$PR_URL"
