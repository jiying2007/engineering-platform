#!/usr/bin/env bash
set -euo pipefail

[ "$#" -ge 2 ] && [ "$#" -le 3 ] || {
  echo "usage: run-control.sh ABSOLUTE_ENV_FILE ABSOLUTE_CONTROL_BINARY [AUTO_MIGRATE]" >&2
  exit 2
}

ENV_FILE="$1"
CONTROL="$2"
AUTO_MIGRATE_OVERRIDE="${3:-}"

case "$ENV_FILE" in
  /*) ;;
  *) echo "ENV_FILE must be absolute" >&2; exit 2 ;;
esac
case "$CONTROL" in
  /*) ;;
  *) echo "CONTROL binary must be absolute" >&2; exit 2 ;;
esac

test -r "$ENV_FILE"
test -x "$CONTROL"

set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

if [ -n "$AUTO_MIGRATE_OVERRIDE" ]; then
  case "$AUTO_MIGRATE_OVERRIDE" in
    0|1) export AUTO_MIGRATE="$AUTO_MIGRATE_OVERRIDE" ;;
    *) echo "AUTO_MIGRATE must be 0 or 1" >&2; exit 2 ;;
  esac
fi

exec "$CONTROL"
