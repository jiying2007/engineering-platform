#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  cat >&2 <<'EOF'
usage: configure-main-protection.sh OUTPUT_DIR

Configures the minimum GitHub branch-protection policy required by the retained
M1 pilot. The authenticated gh principal must have repository Administration
write permission.

This script only creates the policy when main is currently unprotected. If main
is already protected, it validates the existing policy and fails instead of
silently weakening or replacing a stricter/different operator policy.
EOF
  exit 2
}

[ "$#" -eq 1 ] || usage
OUTPUT_DIR="$1"
case "$OUTPUT_DIR" in
  /*) ;;
  *) echo "OUTPUT_DIR must be absolute" >&2; exit 2 ;;
esac

command -v gh >/dev/null
command -v jq >/dev/null
command -v sha256sum >/dev/null

REPOSITORY="jiying2007/engineering-platform"
BRANCH="main"
API_VERSION="2026-03-10"
EXPECTED_CHECKS_JSON='[
  "go",
  "offline-container-integration",
  "postgres-authority-restore-drill",
  "codex-app-server-0.155.0-qualification",
  "trusted-ci-artifact-evidence"
]'

install -d -m 0700 "$OUTPUT_DIR"

api_get() {
  gh api     -H "Accept: application/vnd.github+json"     -H "X-GitHub-Api-Version: $API_VERSION"     "$1"
}

branch_json="$(api_get "/repos/$REPOSITORY/branches/$BRANCH")"
printf '%s' "$branch_json" |
  jq -e --arg branch "$BRANCH" '.name == $branch and (.commit.sha | test("^[0-9a-f]{40}$"))' >/dev/null

if ! printf '%s' "$branch_json" | jq -e '.protected == true' >/dev/null; then
  payload="$OUTPUT_DIR/main-protection-request.json"
  jq -n --argjson checks "$EXPECTED_CHECKS_JSON" '{
    required_status_checks: {
      strict: true,
      contexts: $checks
    },
    enforce_admins: true,
    required_pull_request_reviews: {
      dismiss_stale_reviews: true,
      require_code_owner_reviews: false,
      required_approving_review_count: 0,
      require_last_push_approval: false
    },
    restrictions: null,
    required_linear_history: true,
    allow_force_pushes: false,
    allow_deletions: false,
    block_creations: false,
    required_conversation_resolution: true,
    lock_branch: false,
    allow_fork_syncing: false
  }' > "$payload"
  chmod 0600 "$payload"

  gh api     --method PUT     -H "Accept: application/vnd.github+json"     -H "X-GitHub-Api-Version: $API_VERSION"     "/repos/$REPOSITORY/branches/$BRANCH/protection"     --input "$payload" >/dev/null

  branch_json="$(api_get "/repos/$REPOSITORY/branches/$BRANCH")"
fi

printf '%s' "$branch_json" | jq -e '.protected == true' >/dev/null || {
  echo "GitHub main is still not protected" >&2
  exit 1
}

protection_json="$(api_get "/repos/$REPOSITORY/branches/$BRANCH/protection")"

printf '%s' "$protection_json" |
  jq -e --argjson expected "$EXPECTED_CHECKS_JSON" '
    .enforce_admins.enabled == true and
    .required_status_checks.strict == true and
    ($expected | all(. as $name | (.required_status_checks.contexts // []) | index($name) != null)) and
    (.required_pull_request_reviews != null) and
    ((.required_pull_request_reviews.required_approving_review_count // 0) >= 0) and
    .required_linear_history.enabled == true and
    ((.allow_force_pushes.enabled // false) == false) and
    ((.allow_deletions.enabled // false) == false) and
    .required_conversation_resolution.enabled == true and
    ((.lock_branch.enabled // false) == false)
  ' >/dev/null || {
    echo "existing GitHub main protection does not satisfy retained-pilot minimum policy" >&2
    exit 1
  }

HEAD_SHA="$(printf '%s' "$branch_json" | jq -er '.commit.sha')"
NORMALIZED_PROTECTION="$OUTPUT_DIR/main-protection.json"
printf '%s' "$protection_json" | jq -S . > "$NORMALIZED_PROTECTION"
chmod 0600 "$NORMALIZED_PROTECTION"
PROTECTION_DIGEST="sha256:$(sha256sum "$NORMALIZED_PROTECTION" | awk '{print $1}')"

RECEIPT="$OUTPUT_DIR/github-main-protection-receipt.json"
jq -n   --arg repository "$REPOSITORY"   --arg branch "$BRANCH"   --arg head_sha "$HEAD_SHA"   --arg protection_digest "$PROTECTION_DIGEST"   --argjson required_checks "$EXPECTED_CHECKS_JSON"   '{
    version: 1,
    repository: $repository,
    branch: $branch,
    protected: true,
    head_sha: $head_sha,
    required_checks: $required_checks,
    enforce_admins: true,
    require_pull_request: true,
    required_linear_history: true,
    allow_force_pushes: false,
    allow_deletions: false,
    required_conversation_resolution: true,
    protection_digest: $protection_digest
  }' > "$RECEIPT"
chmod 0600 "$RECEIPT"

cat <<EOF
github protected main: READY
repository=$REPOSITORY
branch=$BRANCH
head_sha=$HEAD_SHA
protection_digest=$PROTECTION_DIGEST
receipt=$RECEIPT
next=bash examples/pilots/wif/configure-admin-api.sh /operator/wif-admin
EOF
