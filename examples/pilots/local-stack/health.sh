#!/usr/bin/env bash
set -euo pipefail

[ "$#" -eq 2 ] || {
  echo "usage: health.sh ABSOLUTE_ROOT CLIENT_SLUG" >&2
  exit 2
}

ROOT="$1"
CLIENT_SLUG="$2"

case "$ROOT" in
  /*) ;;
  *) echo "ROOT must be absolute" >&2; exit 2 ;;
esac

[[ "$CLIENT_SLUG" =~ ^[a-z0-9-]+$ ]] || {
  echo "CLIENT_SLUG must be lowercase alnum/hyphen" >&2
  exit 2
}

CA="$ROOT/pki/ca.crt"
CERT="$ROOT/pki/$CLIENT_SLUG.crt"
KEY="$ROOT/pki/$CLIENT_SLUG.key"

test -r "$CA"
test -r "$CERT"
test -r "$KEY"

curl --fail --silent --show-error \
  --cacert "$CA" \
  --cert "$CERT" \
  --key "$KEY" \
  https://127.0.0.1:18443/healthz
