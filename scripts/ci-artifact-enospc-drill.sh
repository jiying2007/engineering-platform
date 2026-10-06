#!/usr/bin/env bash
set -euo pipefail
umask 077

[[ "${GITHUB_ACTIONS:-}" == "true" ]]
[[ "$(id -u)" -ne 0 ]]
command -v sudo >/dev/null
sudo -n true
command -v python3 >/dev/null
command -v jq >/dev/null

[[ "$#" -eq 3 ]] || { echo "usage: $0 ENG INPUT_ROOT MOUNT_ROOT" >&2; exit 2; }
eng="$1"
input="$2"
mount_root="$3"
[[ "$eng" = /* && "$input" = /* && "$mount_root" = /* && -x "$eng" ]]
[[ ! -e "$input" && ! -e "$mount_root" ]]

mkdir -m 0700 "$input"
sudo install -d -m 0700 "$mount_root"
mounted=0
cleanup() {
  set +e
  if [[ "$mounted" -eq 1 ]]; then
    sudo umount "$mount_root"
  fi
  sudo rm -rf -- "$mount_root"
  rm -rf -- "$input"
}
trap cleanup EXIT

sudo mount -t tmpfs -o size=1m,nodev,nosuid,noexec,mode=0700 tmpfs "$mount_root"
mounted=1
sudo chown "$(id -u):$(id -g)" "$mount_root"
chmod 0700 "$mount_root"
mountpoint -q "$mount_root"
[[ "$(df --output=size -B1 "$mount_root" | tail -1 | tr -d ' ')" -le 2097152 ]]

python3 - "$input" <<'PY'
import hashlib,json,pathlib,sys
root=pathlib.Path(sys.argv[1])
payload=(b"DISK-EXHAUSTION-TEST\x00" * 100000)[:2*1024*1024]
member=root/"payload.bin"
member.write_bytes(payload)
def d(raw): return "sha256:"+hashlib.sha256(raw).hexdigest()
plan={
  "version":1,
  "subject":{
    "run_id":"ci-disk-exhaustion",
    "execution_id":"a"*64,
    "task_contract_digest":d(b"task"),
    "run_input_manifest_digest":d(b"input"),
    "base_commit":"b"*40,
  },
  "members":[{
    "artifact_id":"payload.bin",
    "kind":"build-output",
    "size":len(payload),
    "digest":d(payload),
    "source_path":str(member),
  }],
}
raw=json.dumps(plan,separators=(",",":")).encode()
(root/"plan.json").write_bytes(raw)
(root/"plan.digest").write_text(d(raw)+"\n")
PY

plan="$input/plan.json"
plan_digest="$(cat "$input/plan.digest")"
pack_out="$mount_root/exhausted.tar"
pack_stdout="$input/pack.stdout"
pack_stderr="$input/pack.stderr"
if "$eng" artifact-set pack --plan "$plan" --plan-digest "$plan_digest" --out "$pack_out" >"$pack_stdout" 2>"$pack_stderr"; then
  echo "artifact pack unexpectedly succeeded on undersized tmpfs" >&2
  exit 1
fi
[[ ! -s "$pack_stdout" ]]
[[ ! -e "$pack_out" ]]
if find "$mount_root" -mindepth 1 -maxdepth 1 -print -quit | grep -q .; then
  echo "failed pack left published/temp artifact in tmpfs" >&2
  find "$mount_root" -mindepth 1 -maxdepth 1 -ls >&2
  exit 1
fi

normal="$input/normal"
mkdir -m 0700 "$normal"
packed="$("$eng" artifact-set pack --plan "$plan" --plan-digest "$plan_digest" --out "$normal/set.tar")"
archive_digest="$(jq -er 'select(.status=="ARTIFACT_SET_PACKED_BYTES_VERIFIED" and .execution_authorized==false and .production_qualified==false) | .archive_digest' <<<"$packed")"
[[ "$archive_digest" =~ ^sha256:[0-9a-f]{64}$ ]]

restore_stdout="$input/restore.stdout"
restore_stderr="$input/restore.stderr"
if "$eng" artifact-set restore --archive "$normal/set.tar" --archive-digest "$archive_digest"   --run ci-disk-exhaustion --into "$mount_root/restored" >"$restore_stdout" 2>"$restore_stderr"; then
  echo "artifact restore unexpectedly succeeded on undersized tmpfs" >&2
  exit 1
fi
[[ ! -s "$restore_stdout" ]]
[[ -d "$mount_root/restored" ]]
[[ ! -e "$mount_root/restored/manifest.json" ]]
if find "$mount_root/restored" -type f -perm /0111 -print -quit | grep -q .; then
  echo "failed restore created executable content" >&2
  exit 1
fi

# The full archive remains intact outside the exhausted filesystem.
"$eng" artifact-set verify --archive "$normal/set.tar" --archive-digest "$archive_digest"   --run ci-disk-exhaustion |
  jq -e '.status=="ARTIFACT_SET_BYTES_VERIFIED" and .execution_authorized==false and .production_qualified==false' >/dev/null

printf '%s\n' '{"version":1,"status":"DISK_EXHAUSTION_FAIL_CLOSED_VERIFIED","pack_published":false,"restore_verified":false,"automatic_retry":false,"execution_authorized":false,"production_qualified":false}'
