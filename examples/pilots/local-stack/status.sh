#!/usr/bin/env bash
set -euo pipefail
[ "$#" -eq 1 ] || { echo "usage: status.sh ABSOLUTE_ROOT" >&2; exit 2; }
ROOT="$1"
CA="$ROOT/pki/ca.crt"
test -r "$CA"
curl --fail --silent --show-error --cacert "$CA" https://127.0.0.1:18443/healthz
echo
