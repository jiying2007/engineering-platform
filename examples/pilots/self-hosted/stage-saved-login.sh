#!/usr/bin/env bash
set -euo pipefail

[ "$#" -eq 2 ] || {
  echo "usage: stage-saved-login.sh ABSOLUTE_AUTH_JSON ABSOLUTE_STAGE_ROOT" >&2
  exit 2
}

SOURCE="$1"
STAGE_ROOT="$2"

for path in "$SOURCE" "$STAGE_ROOT"; do
  case "$path" in
    /*) ;;
    *) echo "saved-login paths must be absolute" >&2; exit 2 ;;
  esac
done

test "$STAGE_ROOT" != "/"

resolved_source="$(readlink -f -- "$SOURCE")"
test "$resolved_source" = "$SOURCE" || {
  echo "saved ChatGPT login source must be canonical and symlink-free" >&2
  exit 1
}

source_uid="$(stat -c '%u' "$SOURCE")"
source_mode="$(stat -c '%a' "$SOURCE")"
source_size="$(stat -c '%s' "$SOURCE")"
test "$source_uid" = "$(id -u)" || {
  echo "saved ChatGPT login source must be owned by the current user" >&2
  exit 1
}
source_perm=$((8#$source_mode))
if (( source_perm & 8#077 )); then
  echo "saved ChatGPT login source file must be owner-private" >&2
  exit 1
fi
if (( source_size < 16 || source_size > 1048576 )); then
  echo "saved ChatGPT login source size is outside the accepted bound" >&2
  exit 1
fi

SOURCE_PARENT="$(dirname -- "$SOURCE")"
resolved_parent="$(readlink -f -- "$SOURCE_PARENT")"
test "$resolved_parent" = "$SOURCE_PARENT" || {
  echo "saved ChatGPT login source parent must be canonical" >&2
  exit 1
}
parent_uid="$(stat -c '%u' "$SOURCE_PARENT")"
test "$parent_uid" = "$(id -u)" || {
  echo "saved ChatGPT login source parent must be owned by the current user" >&2
  exit 1
}
chmod go-w "$SOURCE_PARENT"
parent_mode="$(stat -c '%a' "$SOURCE_PARENT")"
parent_perm=$((8#$parent_mode))
if (( parent_perm & 8#022 )); then
  echo "saved ChatGPT login source parent remains group/world writable" >&2
  exit 1
fi

STAGE_PARENT="$(dirname -- "$STAGE_ROOT")"
test -d "$STAGE_PARENT"
resolved_stage_parent="$(readlink -f -- "$STAGE_PARENT")"
test "$resolved_stage_parent" = "$STAGE_PARENT" || {
  echo "saved-login stage parent must be canonical" >&2
  exit 1
}

rm -rf -- "$STAGE_ROOT"
install -d -m 0700 "$STAGE_ROOT"
STAGED="$STAGE_ROOT/auth.json"
install -m 0600 "$SOURCE" "$STAGED"

test "$(readlink -f -- "$STAGED")" = "$STAGED"
test "$(stat -c '%u' "$STAGED")" = "$(id -u)"
staged_perm=$((8#$(stat -c '%a' "$STAGED")))
(( (staged_perm & 8#077) == 0 ))
stage_parent_perm=$((8#$(stat -c '%a' "$STAGE_ROOT")))
(( (stage_parent_perm & 8#077) == 0 ))

printf '%s\n' "$STAGED"
