#!/usr/bin/env bash
set -euo pipefail

[ "$#" -eq 1 ] || {
  echo "usage: secure-repository-source.sh ABSOLUTE_REPOSITORY" >&2
  exit 2
}

REPOSITORY="$1"
case "$REPOSITORY" in
  /*) ;;
  *) echo "repository path must be absolute" >&2; exit 2 ;;
esac

test -d "$REPOSITORY"
resolved="$(readlink -f -- "$REPOSITORY")"
test "$resolved" = "$REPOSITORY" || {
  echo "repository source must be canonical and symlink-free" >&2
  exit 1
}

chmod go-w "$REPOSITORY"

mode="$(stat -c '%a' "$REPOSITORY")"
perm=$((8#$mode))
if (( perm & 8#022 )); then
  echo "repository source remains group/world writable" >&2
  exit 1
fi

echo "repository source: READY"
echo "repository=$REPOSITORY"
echo "mode=$mode"
