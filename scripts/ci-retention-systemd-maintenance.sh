#!/usr/bin/env bash
# Ephemeral hosted-runner proof for the actual retention maintenance oneshot.
# It creates/removes only the canonical preparation identity/group on a disposable runner.
set -euo pipefail
umask 077

test "${GITHUB_ACTIONS:-}" = true
test "${GITHUB_REPOSITORY:-}" = jiying2007/engineering-platform
test "$(id -u)" -ne 0
test -S /run/systemd/private
sudo -n true
command -v jq >/dev/null
command -v python3 >/dev/null
command -v sha256sum >/dev/null
command -v useradd >/dev/null
command -v userdel >/dev/null
command -v groupadd >/dev/null
command -v groupdel >/dev/null

[[ "$#" -eq 1 ]] || { echo "usage: $0 ABSOLUTE_ENG" >&2; exit 2; }
eng="$1"
[[ "$eng" = /* && -x "$eng" ]]
dist="$(dirname -- "$eng")"
"$eng" distribution-verify --dir "$dist" | jq -e '.status=="BYTES_VERIFIED" and .binary_count==6' >/dev/null

user=engineering-preparation
group=engineering-platform
for name in engineering-control engineering-publisher engineering-admission engineering-preparation; do
  if getent passwd "$name" >/dev/null; then
    echo "canonical service user already exists; refusing disposable-host mutation: $name" >&2
    exit 1
  fi
done
if getent group "$group" >/dev/null; then
  echo "canonical service group already exists; refusing disposable-host mutation: $group" >&2
  exit 1
fi

prefix="ep-retention-${GITHUB_RUN_ID:?}-${GITHUB_RUN_ATTEMPT:?}"
[[ "$prefix" =~ ^ep-retention-[0-9]+-[0-9]+$ ]]
root="/run/$prefix"
unit_ok="$prefix-ok.service"
unit_again="$prefix-again.service"
unit_missing="$prefix-missing.service"
for u in "$unit_ok" "$unit_again" "$unit_missing"; do
  [[ "$u" =~ ^ep-retention-[0-9]+-[0-9]+-(ok|again|missing)\.service$ ]]
done
created_user=0
created_group=0

cleanup() {
  local status=$? failed=0
  trap - EXIT
  set +e
  for u in "$unit_missing" "$unit_again" "$unit_ok"; do
    sudo -n systemctl stop "$u" >/dev/null 2>&1 || true
    sudo -n systemctl reset-failed "$u" >/dev/null 2>&1 || true
  done
  if [ "$created_user" -eq 1 ]; then
    sudo -n userdel "$user" >/dev/null 2>&1 || failed=1
  fi
  if [ "$created_group" -eq 1 ] && getent group "$group" >/dev/null; then
    sudo -n groupdel "$group" >/dev/null 2>&1 || failed=1
  fi
  sudo -n rm -rf -- "$root" || failed=1
  if getent passwd "$user" >/dev/null || getent group "$group" >/dev/null; then failed=1; fi
  if [ "$failed" -ne 0 ]; then
    echo "retention maintenance cleanup requires reconciliation" >&2
    exit 1
  fi
  exit "$status"
}
trap cleanup EXIT

service="examples/production/systemd/maintenance/engineering-artifact-retention.service"
timer="examples/production/systemd/maintenance/engineering-artifact-retention.timer"
for expected in   'Type=oneshot' 'User=engineering-preparation' 'Group=engineering-platform'   'UMask=0077' 'NoNewPrivileges=true' 'PrivateTmp=true' 'PrivateDevices=true'   'ProtectSystem=strict' 'ProtectHome=true' 'RestrictSUIDSGID=true'   'LockPersonality=true' 'RestrictAddressFamilies=AF_UNIX' 'SystemCallArchitectures=native'; do
  grep -Fx -- "$expected" "$service" >/dev/null
done
! grep -q '^ConditionPathExists=' "$service"
grep -Fx 'Persistent=true' "$timer" >/dev/null
grep -Fx 'Unit=engineering-artifact-retention.service' "$timer" >/dev/null
grep -Fx 'WantedBy=timers.target' "$timer" >/dev/null

for production_unit in engineering-artifact-retention.service engineering-artifact-retention.timer; do
  load_state="$(sudo -n systemctl show --property=LoadState --value "$production_unit" 2>/dev/null || true)"
  test "$load_state" = not-found
done

sudo -n install -d -m 0755 -o root -g root "$root"
candidate_eng="$eng"
test_eng="$root/eng"
sudo -n install -m 0555 -o root -g root "$candidate_eng" "$test_eng"
test "$(sha256sum "$candidate_eng" | cut -d' ' -f1)" = "$(sha256sum "$test_eng" | cut -d' ' -f1)"
eng="$test_eng"
sudo -n groupadd --system "$group"
created_group=1
sudo -n useradd --system --gid "$group" --home-dir /nonexistent --no-create-home --shell /usr/sbin/nologin "$user"
created_user=1
uid="$(id -u "$user")"
gid="$(id -g "$user")"
test "$uid" -gt 0
test "$gid" -gt 0

source="$root/source"
primary="$root/primary"
replica="$root/replica"
configdir="$root/config"
for d in "$source" "$primary" "$replica" "$configdir"; do
  sudo -n install -d -m 0700 -o "$user" -g "$group" "$d"
done

payload="$source/fixture.bin"
sudo -n -u "$user" env -i PATH=/usr/bin:/bin HOME=/nonexistent python3 - "$payload" <<'PY'
import pathlib,sys
path=pathlib.Path(sys.argv[1])
path.write_bytes((b"RETENTION-MAINTENANCE-CI\0" + bytes(range(64))) * 2048)
path.chmod(0o600)
PY
payload_size="$(sudo -n -u "$user" stat -c %s "$payload")"
payload_digest="sha256:$(sudo -n -u "$user" sha256sum "$payload" | cut -d' ' -f1)"
plan="$source/plan.json"
sudo -n -u "$user" env -i PATH=/usr/bin:/bin HOME=/nonexistent python3 - "$plan" "$payload" "$payload_size" "$payload_digest" <<'PY'
import json,pathlib,sys
path,payload,size,digest=sys.argv[1:]
value={
 "version":1,
 "subject":{
   "run_id":"retention-maintenance-ci",
   "execution_id":"a"*64,
   "task_contract_digest":"sha256:"+"b"*64,
   "run_input_manifest_digest":"sha256:"+"c"*64,
   "base_commit":"d"*40,
 },
 "members":[{
   "artifact_id":"fixture.bin","kind":"test-evidence",
   "size":int(size),"digest":digest,"source_path":payload,
 }]
}
pathlib.Path(path).write_text(json.dumps(value,separators=(",",":")))
pathlib.Path(path).chmod(0o600)
PY
plan_digest="sha256:$(sudo -n -u "$user" sha256sum "$plan" | cut -d' ' -f1)"
archive="$primary/run-a.tar"
packed="$(sudo -n -u "$user" env -i PATH=/usr/bin:/bin HOME=/nonexistent   "$eng" artifact-set pack --plan "$plan" --plan-digest "$plan_digest" --out "$archive")"
archive_digest="$(jq -er '.archive_digest | select(test("^sha256:[0-9a-f]{64}$"))' <<<"$packed")"
archive_size="$(jq -er '.archive_size | select(type=="number" and .>0)' <<<"$packed")"

config="$configdir/retention.json"
sudo -n -u "$user" env -i PATH=/usr/bin:/bin HOME=/nonexistent python3 - "$config" "$primary" "$replica" "$archive_digest" "$archive_size" <<'PY'
import json,pathlib,sys
path,primary,replica,digest,size=sys.argv[1:]
value={
 "version":1,"primary_root":primary,"replica_root":replica,
 "max_total_bytes":int(size)+4096,
 "items":[{"archive":"run-a.tar","run_id":"retention-maintenance-ci","archive_digest":digest}],
}
pathlib.Path(path).write_text(json.dumps(value,separators=(",",":")))
pathlib.Path(path).chmod(0o600)
PY
config_digest="sha256:$(sudo -n -u "$user" sha256sum "$config" | cut -d' ' -f1)"

props=(
  "--property=Type=oneshot"
  "--property=User=$user"
  "--property=Group=$group"
  "--property=UMask=0077"
  "--property=NoNewPrivileges=yes"
  "--property=PrivateTmp=yes"
  "--property=PrivateDevices=yes"
  "--property=ProtectSystem=strict"
  "--property=ProtectHome=yes"
  "--property=ProtectKernelTunables=yes"
  "--property=ProtectKernelModules=yes"
  "--property=ProtectKernelLogs=yes"
  "--property=ProtectControlGroups=yes"
  "--property=ProtectClock=yes"
  "--property=ProtectHostname=yes"
  "--property=RestrictSUIDSGID=yes"
  "--property=LockPersonality=yes"
  "--property=RestrictAddressFamilies=AF_UNIX"
  "--property=ReadOnlyPaths=$primary"
  "--property=ReadWritePaths=$replica"
  "--property=SystemCallArchitectures=native"
  "--property=TimeoutStartSec=60s"
)

run_unit() {
  local unit="$1" cfg="$2" digest="$3"
  sudo -n systemd-run --quiet --wait --pipe --collect --unit="$unit"     "${props[@]}"     -- /bin/sh -eu -c 'printf "IDENTITY:%s:%s:%s:%s\n" "$(id -u)" "$(id -g)" "$(id -un)" "$(id -gn)"; exec "$1" artifact-retention replicate --config "$2" --config-digest "$3"'     _ "$eng" "$cfg" "$digest"
}

first="$(run_unit "$unit_ok" "$config" "$config_digest")"
identity="$(printf '%s\n' "$first" | grep '^IDENTITY:' | tail -n1)"
test "$identity" = "IDENTITY:$uid:$gid:$user:$group"
report="$(printf '%s\n' "$first" | grep '^{' | tail -n1)"
jq -e --arg digest "$config_digest" --argjson bytes "$archive_size" '
  .status=="RETENTION_REPLICA_BYTES_VERIFIED" and .config_digest==$digest and
  .item_count==1 and .copied_count==1 and .existing_count==0 and
  .verified_bytes==$bytes and .deletion_performed==false and
  .execution_authorized==false and .production_qualified==false and
  .second_site_qualified==false' <<<"$report" >/dev/null

sudo -n -u "$user" env -i PATH=/usr/bin:/bin HOME=/nonexistent   "$eng" artifact-set verify --archive "$replica/run-a.tar" --archive-digest "$archive_digest" --run retention-maintenance-ci   | jq -e '.status=="ARTIFACT_SET_BYTES_VERIFIED" and .execution_authorized==false and .production_qualified==false' >/dev/null

second="$(run_unit "$unit_again" "$config" "$config_digest")"
report2="$(printf '%s\n' "$second" | grep '^{' | tail -n1)"
jq -e --arg digest "$config_digest" --argjson bytes "$archive_size" '
  .status=="RETENTION_REPLICA_BYTES_VERIFIED" and .config_digest==$digest and
  .item_count==1 and .copied_count==0 and .existing_count==1 and .verified_bytes==$bytes' <<<"$report2" >/dev/null

before="$(sudo -n -u "$user" sha256sum "$replica/run-a.tar")"
if run_unit "$unit_missing" "$configdir/missing.json" "$config_digest" >/dev/null 2>&1; then
  echo "missing retention config became successful systemd maintenance" >&2
  exit 1
fi
after="$(sudo -n -u "$user" sha256sum "$replica/run-a.tar")"
test "$before" = "$after"

entries="$(sudo -n -u "$user" find "$replica" -mindepth 1 -maxdepth 1 -printf '%f\n' | sort)"
test "$entries" = run-a.tar
for production_unit in engineering-artifact-retention.service engineering-artifact-retention.timer; do
  load_state="$(sudo -n systemctl show --property=LoadState --value "$production_unit" 2>/dev/null || true)"
  test "$load_state" = not-found
done

python3 - "$archive_size" <<'PY'
import json,sys
print(json.dumps({
 "version":1,
 "status":"CANONICAL_RETENTION_SYSTEMD_MAINTENANCE_VERIFIED",
 "service_identity":"engineering-preparation",
 "shared_group":"engineering-platform",
 "item_count":1,
 "verified_bytes":int(sys.argv[1]),
 "first_copy_verified":True,
 "idempotent_existing_readback_verified":True,
 "missing_config_failed":True,
 "timer_enabled":False,
 "persistent_host_provisioned":False,
 "production_unit_installed":False,
 "network_action_executed":False,
 "deletion_performed":False,
 "second_site_qualified":False,
 "production_qualified":False,
},sort_keys=True))
PY
