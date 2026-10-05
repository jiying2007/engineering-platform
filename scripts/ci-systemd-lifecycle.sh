#!/usr/bin/env bash
# Ephemeral hosted-runner acceptance only; never install production-named units.
set -euo pipefail
umask 077
test "${GITHUB_ACTIONS:-}" = true
test "${GITHUB_REPOSITORY:-}" = jiying2007/engineering-platform
test "$(id -u)" -ne 0
test "$(id -g)" -ne 0
test -d /run/systemd/system
test -S /run/systemd/private
sudo -n true
systemctl --system show --property=Version >/dev/null
[[ "$GITHUB_RUN_ID" =~ ^[0-9]+$ && "$GITHUB_RUN_ATTEMPT" =~ ^[0-9]+$ ]]
source="$(git rev-parse HEAD)"
test "$source" = "$EXPECTED_SOURCE"
prefix="ep-sysd-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT"
root="/run/$prefix-$(python3 -c 'import secrets; print(secrets.token_hex(6))')"
dist="$RUNNER_TEMP/$(basename "$root")-dist"
# Only the generated current-run names are eligible for this cleanup. No
# boot-time units/accounts/configuration are written, enabled or modified.
cleanup() {
  local status=$? failed=0 unit inventory
  trap - EXIT
  set +e
  inventory="$(systemctl list-units --all --plain --no-legend "$prefix-*.service")"
  if [ $? -ne 0 ]; then failed=1; fi
  while read -r unit _; do
    [ -n "$unit" ] || continue
    if [[ ! "$unit" =~ ^${prefix}-[a-f0-9]{12}\.service$ ]]; then failed=1; continue; fi
    sudo -n systemctl stop "$unit" || failed=1
    sudo -n systemctl reset-failed "$unit" >/dev/null 2>&1 || true
  done <<< "$inventory"
  if [ "$failed" -eq 0 ]; then
    sudo -n rm -rf -- "$root" || failed=1
    rm -rf -- "$dist" || failed=1
  fi
  if [ "$failed" -ne 0 ]; then echo 'test service cleanup requires reconciliation' >&2; exit 1; fi
  exit "$status"
}
trap cleanup EXIT
sudo -n install -d -m 700 -o "$(id -u)" -g "$(id -g)" "$root"
bash scripts/build-distribution.sh "$dist"
EP_SYSTEMD_INTEGRATION=1 python3 -B scripts/systemd_lifecycle.py \
  --distribution "$dist" --source "$source" --work-root "$root" --prefix "$prefix"
