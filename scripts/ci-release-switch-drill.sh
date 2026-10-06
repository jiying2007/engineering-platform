#!/usr/bin/env bash
set -euo pipefail
umask 077

[[ "${GITHUB_ACTIONS:-}" == "true" ]]
[[ "$(id -u)" -ne 0 ]]
command -v sudo >/dev/null
sudo -n true
command -v jq >/dev/null
command -v git >/dev/null
command -v go >/dev/null

[[ "$#" -eq 5 ]] || { echo "usage: $0 BASELINE_SOURCE BASELINE_SHA CURRENT_DIST CURRENT_SHA ROOT" >&2; exit 2; }
baseline_source="$1"
baseline_sha="$2"
current_dist="$3"
current_sha="$4"
root="$5"

[[ "$baseline_source" = /* && "$current_dist" = /* && "$root" = /* ]]
[[ "$baseline_sha" =~ ^[0-9a-f]{40}$ && "$current_sha" =~ ^[0-9a-f]{40}$ ]]
[[ "$baseline_sha" != "$current_sha" ]]
[[ "$(cd "$baseline_source" && git rev-parse HEAD)" == "$baseline_sha" ]]
[[ "$(cd "$baseline_source" && git status --porcelain --untracked-files=no)" == "" ]]
[[ ! -e "$root" ]]

"$current_dist/eng" distribution-verify --dir "$current_dist" |
  jq -e --arg sha "$current_sha" '.status=="BYTES_VERIFIED" and .source_commit==$sha and .binary_count==6' >/dev/null

old_dist="${RUNNER_TEMP:?}/ep-release-baseline-${GITHUB_RUN_ID:?}-${GITHUB_RUN_ATTEMPT:?}"
[[ ! -e "$old_dist" ]]
(
  cd "$baseline_source"
  for attempt in 1 2 3; do
    if go mod download; then
      break
    fi
    [[ "$attempt" -lt 3 ]]
    sleep $((attempt * 2))
  done
  bash scripts/build-distribution.sh "$old_dist"
)
"$old_dist/eng" distribution-verify --dir "$old_dist" |
  jq -e --arg sha "$baseline_sha" '.status=="BYTES_VERIFIED" and .source_commit==$sha and .binary_count==6' >/dev/null

sudo install -d -m 0700 -o root -g root "$root"
cleanup() {
  sudo rm -rf -- "$root"
  rm -rf -- "$old_dist"
}
trap cleanup EXIT

old_install="$(sudo "$old_dist/eng" distribution-install   --from "$old_dist" --into "$root/active" --source-commit "$baseline_sha")"
new_install="$(sudo "$current_dist/eng" distribution-install   --from "$current_dist" --into "$root/candidate" --source-commit "$current_sha")"

old_manifest="$(jq -er --arg sha "$baseline_sha" '
  select(.status=="INSTALLED_BYTES_VERIFIED" and .source_commit==$sha and
         .services_started==false and .configuration_applied==false and
         .execution_authorized==false and .production_qualified==false) |
  .manifest_digest' <<<"$old_install")"
new_manifest="$(jq -er --arg sha "$current_sha" '
  select(.status=="INSTALLED_BYTES_VERIFIED" and .source_commit==$sha and
         .services_started==false and .configuration_applied==false and
         .execution_authorized==false and .production_qualified==false) |
  .manifest_digest' <<<"$new_install")"
[[ "$old_manifest" =~ ^sha256:[0-9a-f]{64}$ ]]
[[ "$new_manifest" =~ ^sha256:[0-9a-f]{64}$ ]]
[[ "$old_manifest" != "$new_manifest" ]]

sudo "$current_dist/eng" installation-readback   --dir "$root/active" --source-commit "$baseline_sha" --manifest-digest "$old_manifest" >/dev/null
sudo "$current_dist/eng" installation-readback   --dir "$root/candidate" --source-commit "$current_sha" --manifest-digest "$new_manifest" >/dev/null

upgrade="$(sudo "$current_dist/eng" installation-switch   --active "$root/active" --candidate "$root/candidate" --previous "$root/previous"   --active-source "$baseline_sha" --active-manifest-digest "$old_manifest"   --candidate-source "$current_sha" --candidate-manifest-digest "$new_manifest")"
jq -e --arg new "$current_sha" --arg old "$baseline_sha" '
  .status=="INSTALLATION_SWITCH_BYTES_VERIFIED" and
  .active_identity.source_commit==$new and .previous_identity.source_commit==$old and
  .services_started==false and .database_changed==false and
  .execution_authorized==false and .production_qualified==false' <<<"$upgrade" >/dev/null

sudo "$current_dist/eng" installation-readback   --dir "$root/active" --source-commit "$current_sha" --manifest-digest "$new_manifest" >/dev/null
sudo "$current_dist/eng" installation-readback   --dir "$root/previous" --source-commit "$baseline_sha" --manifest-digest "$old_manifest" >/dev/null

rollback="$(sudo "$current_dist/eng" installation-switch   --active "$root/active" --candidate "$root/previous" --previous "$root/replaced-current"   --active-source "$current_sha" --active-manifest-digest "$new_manifest"   --candidate-source "$baseline_sha" --candidate-manifest-digest "$old_manifest")"
jq -e --arg old "$baseline_sha" --arg new "$current_sha" '
  .status=="INSTALLATION_SWITCH_BYTES_VERIFIED" and
  .active_identity.source_commit==$old and .previous_identity.source_commit==$new and
  .services_started==false and .database_changed==false and
  .execution_authorized==false and .production_qualified==false' <<<"$rollback" >/dev/null

sudo "$current_dist/eng" installation-readback   --dir "$root/active" --source-commit "$baseline_sha" --manifest-digest "$old_manifest" >/dev/null
sudo "$current_dist/eng" installation-readback   --dir "$root/replaced-current" --source-commit "$current_sha" --manifest-digest "$new_manifest" >/dev/null

jq -n --arg baseline "$baseline_sha" --arg candidate "$current_sha"   --arg old_manifest "$old_manifest" --arg new_manifest "$new_manifest"   '{version:1,status:"CROSS_VERSION_SWITCH_ROLLBACK_VERIFIED",
    baseline_source:$baseline,candidate_source:$candidate,
    baseline_manifest:$old_manifest,candidate_manifest:$new_manifest,
    services_started:false,database_changed:false,production_qualified:false}'
