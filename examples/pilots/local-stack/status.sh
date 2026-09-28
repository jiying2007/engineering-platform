#!/usr/bin/env bash
set -euo pipefail
[ "$#" -eq 1 ] || { echo "usage: status.sh ABSOLUTE_ROOT" >&2; exit 2; }
ROOT="$1"
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
bash "$SCRIPT_DIR/health.sh" "$ROOT" owner
echo
