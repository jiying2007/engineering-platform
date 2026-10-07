#!/usr/bin/env bash
# Ephemeral hosted-runner proof for the exact canonical named service identities.
# Never run on a persistent/production host.
set -euo pipefail
umask 077

test "${GITHUB_ACTIONS:-}" = true
test "${GITHUB_REPOSITORY:-}" = jiying2007/engineering-platform
test "$(id -u)" -ne 0
test -S /run/systemd/private
sudo -n true
command -v getent >/dev/null
command -v useradd >/dev/null
command -v userdel >/dev/null
command -v groupadd >/dev/null
command -v groupdel >/dev/null

roles=(engineering-control engineering-publisher engineering-admission engineering-preparation)
group=engineering-platform
for name in "${roles[@]}"; do
  if getent passwd "$name" >/dev/null; then
    echo "canonical test user already exists; refusing to mutate host: $name" >&2
    exit 1
  fi
done
if getent group "$group" >/dev/null; then
  echo "canonical test group already exists; refusing to mutate host: $group" >&2
  exit 1
fi

prefix="ep-named-${GITHUB_RUN_ID:?}-${GITHUB_RUN_ATTEMPT:?}"
[[ "$prefix" =~ ^ep-named-[0-9]+-[0-9]+$ ]]
work="/run/$prefix"
created_users=()
units=()

cleanup() {
  local status=$? failed=0
  trap - EXIT
  set +e
  for unit in "${units[@]}"; do
    if [[ ! "$unit" =~ ^ep-named-[0-9]+-[0-9]+-[a-z]+-[a-f0-9]{8}\.service$ ]]; then
      failed=1
      continue
    fi
    sudo -n systemctl stop "$unit" >/dev/null 2>&1 || true
    sudo -n systemctl reset-failed "$unit" >/dev/null 2>&1 || true
  done
  for ((i=${#created_users[@]}-1; i>=0; i--)); do
    sudo -n userdel "${created_users[$i]}" >/dev/null 2>&1 || failed=1
  done
  if getent group "$group" >/dev/null; then
    sudo -n groupdel "$group" >/dev/null 2>&1 || failed=1
  fi
  sudo -n rm -rf -- "$work" || failed=1
  for name in "${roles[@]}"; do
    if getent passwd "$name" >/dev/null; then failed=1; fi
  done
  if getent group "$group" >/dev/null; then failed=1; fi
  if [ "$failed" -ne 0 ]; then
    echo "named service identity cleanup requires reconciliation" >&2
    exit 1
  fi
  exit "$status"
}
trap cleanup EXIT

sudo -n install -d -m 0755 -o root -g root "$work"
sudo -n groupadd --system "$group"
for name in "${roles[@]}"; do
  sudo -n useradd --system --gid "$group" --home-dir /nonexistent --no-create-home --shell /usr/sbin/nologin "$name"
  created_users+=("$name")
done

declare -A uids gids
for name in "${roles[@]}"; do
  uid="$(id -u "$name")"
  gid="$(id -g "$name")"
  [[ "$uid" =~ ^[0-9]+$ && "$gid" =~ ^[0-9]+$ ]]
  test "$uid" -gt 0
  test "$gid" -gt 0
  test "$(getent passwd "$name" | cut -d: -f6)" = /nonexistent
  test "$(getent passwd "$name" | cut -d: -f7)" = /usr/sbin/nologin
  uids["$name"]="$uid"
  gids["$name"]="$gid"
done
test "$(printf '%s\n' "${uids[@]}" | sort -u | wc -l)" -eq 4
test "$(printf '%s\n' "${gids[@]}" | sort -u | wc -l)" -eq 1

for name in "${roles[@]}"; do
  suffix="$(printf '%s' "$name-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}" | sha256sum | cut -c1-8)"
  role="${name#engineering-}"
  unit="$prefix-$role-$suffix.service"
  units+=("$unit")
  role_dir="$work/$role"
  sudo -n install -d -m 0700 -o "$name" -g "$group" "$role_dir"
  sudo -n systemd-run --quiet --wait --collect --unit="$unit" \
    --property=Type=oneshot \
    --property=User="$name" \
    --property=Group="$group" \
    --property=UMask=0077 \
    --property=NoNewPrivileges=yes \
    --property=PrivateTmp=yes \
    --property=PrivateDevices=yes \
    --property=ProtectSystem=strict \
    --property=ProtectHome=yes \
    --property=RestrictSUIDSGID=yes \
    --property=RestrictAddressFamilies=AF_UNIX \
    --property=ReadWritePaths="$role_dir" \
    --property=SystemCallArchitectures=native \
    -- /bin/sh -eu -c 'printf "%s:%s:%s:%s\n" "$(id -u)" "$(id -g)" "$(id -un)" "$(id -gn)" > "$1/identity"' _ "$role_dir"
  observed="$(sudo -n cat "$role_dir/identity")"
  test "$observed" = "${uids[$name]}:${gids[$name]}:$name:$group"
  stat_out="$(sudo -n stat -c '%u:%g:%a:%h' "$role_dir/identity")"
  test "$stat_out" = "${uids[$name]}:${gids[$name]}:600:1"
done
python3 - <<'PY'
import json, os
roles = ["engineering-control","engineering-publisher","engineering-admission","engineering-preparation"]
print(json.dumps({
    "version": 1,
    "status": "CANONICAL_NAMED_SERVICE_IDENTITIES_EPHEMERAL_CI_VERIFIED",
    "roles": roles,
    "group": "engineering-platform",
    "persistent_host_provisioned": False,
    "production_units_installed": False,
    "production_qualified": False,
}, sort_keys=True))
PY
