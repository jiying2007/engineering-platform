#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  echo "usage: bootstrap.sh ABSOLUTE_ROOT PROFILE_DIGEST [WORKER_PROFILE]" >&2
  exit 2
}

[ "$#" -ge 2 ] && [ "$#" -le 3 ] || usage
ROOT="$1"
PROFILE_DIGEST="$2"
WORKER_PROFILE="${3:-worker/codex-pilot}"

case "$ROOT" in
  /*) ;;
  *) echo "ROOT must be absolute" >&2; exit 2 ;;
esac
if [[ ! "$PROFILE_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]]; then
  echo "PROFILE_DIGEST must be sha256:<64 lowercase hex>" >&2
  exit 2
fi

command -v openssl >/dev/null
command -v git >/dev/null
command -v sed >/dev/null
command -v readlink >/dev/null

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
PILOT_DIR="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
POLICY_TEMPLATE="$PILOT_DIR/access-policy.json.tmpl"
GIT_EXECUTABLE="$(readlink -f "$(command -v git)")"

mkdir -p "$ROOT"/{pki,clients,operator,secrets,preparation-root/artifacts,context-source}
chmod 700 "$ROOT" "$ROOT"/{pki,clients,operator,secrets,preparation-root,preparation-root/artifacts,context-source}

sed   -e "s#__PROFILE_DIGEST__#$PROFILE_DIGEST#g"   -e "s#__WORKER_PROFILE__#$WORKER_PROFILE#g"   "$POLICY_TEMPLATE" > "$ROOT/operator/access-policy.json"
chmod 600 "$ROOT/operator/access-policy.json"

cat > "$ROOT/operator/github-publisher.json" <<EOF
{
  "version": 1,
  "artifact_root": "$ROOT/preparation-root/artifacts",
  "git_executable": "$GIT_EXECUTABLE",
  "token_file": "$ROOT/secrets/github-token",
  "targets": [
    {
      "repository": "jiying2007/engineering-platform",
      "base_ref": "main",
      "branch_prefix": "engineering-platform/"
    }
  ]
}
EOF
chmod 600 "$ROOT/operator/github-publisher.json"

CA_KEY="$ROOT/pki/ca.key"
CA_CERT="$ROOT/pki/ca.crt"
SERVER_KEY="$ROOT/pki/server.key"
SERVER_CSR="$ROOT/pki/server.csr"
SERVER_CERT="$ROOT/pki/server.crt"

openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 7   -keyout "$CA_KEY" -out "$CA_CERT"   -subj "/CN=engineering-platform-pilot-ca"   -addext "basicConstraints=critical,CA:TRUE"   -addext "keyUsage=critical,keyCertSign,cRLSign" >/dev/null 2>&1
chmod 600 "$CA_KEY"
chmod 644 "$CA_CERT"

openssl req -newkey rsa:2048 -nodes -sha256   -keyout "$SERVER_KEY" -out "$SERVER_CSR"   -subj "/CN=engineering-platform-pilot-control" >/dev/null 2>&1
cat > "$ROOT/pki/server.ext" <<'EOF'
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=DNS:localhost,IP:127.0.0.1
EOF
openssl x509 -req -sha256 -days 7   -in "$SERVER_CSR" -CA "$CA_CERT" -CAkey "$CA_KEY"   -set_serial 1001 -out "$SERVER_CERT" -extfile "$ROOT/pki/server.ext" >/dev/null 2>&1
chmod 600 "$SERVER_KEY"
chmod 644 "$SERVER_CERT"

make_client() {
  local slug="$1"
  local subject="$2"
  local serial="$3"
  local key="$ROOT/pki/$slug.key"
  local csr="$ROOT/pki/$slug.csr"
  local cert="$ROOT/pki/$slug.crt"
  local ext="$ROOT/pki/$slug.ext"

  openssl req -newkey rsa:2048 -nodes -sha256     -keyout "$key" -out "$csr" -subj "/CN=$slug" >/dev/null 2>&1
  cat > "$ext" <<EOF
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature
extendedKeyUsage=clientAuth
subjectAltName=URI:$subject
EOF
  openssl x509 -req -sha256 -days 7     -in "$csr" -CA "$CA_CERT" -CAkey "$CA_KEY"     -set_serial "$serial" -out "$cert" -extfile "$ext" >/dev/null 2>&1
  chmod 600 "$key"
  chmod 644 "$cert"
  cat > "$ROOT/clients/$slug.env" <<EOF
export CONTROL_ENDPOINT=https://127.0.0.1:18443
export CONTROL_CLIENT_CERT_FILE=$cert
export CONTROL_CLIENT_KEY_FILE=$key
export CONTROL_SERVER_CA_FILE=$CA_CERT
EOF
  chmod 600 "$ROOT/clients/$slug.env"
}

make_client owner "urn:engineering-platform:operator:pilot-owner" 1101
make_client worker "urn:engineering-platform:worker:codex-pilot" 1102
make_client publisher "urn:engineering-platform:operator:pilot-publisher" 1103
make_client codex-evidence "urn:engineering-platform:codex-evidence-importer" 1104
make_client git-evidence "urn:engineering-platform:git-evidence-importer" 1105
make_client ci-evidence "urn:engineering-platform:github-ci-importer" 1106
make_client verifier "urn:engineering-platform:verifier:pilot" 1107
make_client reviewer "urn:engineering-platform:reviewer:pilot" 1108
make_client closure "urn:engineering-platform:closure:pilot" 1109

cat > "$ROOT/operator/control-plane.env" <<EOF
export LISTEN_HOST=127.0.0.1
export PORT=18443
export DATABASE_URL=postgres://postgres:postgres@127.0.0.1:55432/engineering_platform?sslmode=disable
export AUTO_MIGRATE=1
export CONTROL_TLS_CERT_FILE=$SERVER_CERT
export CONTROL_TLS_KEY_FILE=$SERVER_KEY
export CONTROL_CLIENT_CA_FILE=$CA_CERT
export CONTROL_AUTH_POLICY_FILE=$ROOT/operator/access-policy.json
EOF
chmod 600 "$ROOT/operator/control-plane.env"

cat > "$ROOT/operator/control-plane-with-publisher.env" <<EOF
. "$ROOT/operator/control-plane.env"
export GITHUB_PUBLISHER_CONFIG_FILE=$ROOT/operator/github-publisher.json
EOF
chmod 600 "$ROOT/operator/control-plane-with-publisher.env"

rm -f "$ROOT/pki"/*.csr "$ROOT/pki"/*.ext
echo "pilot local stack bootstrap: READY"
echo "root=$ROOT"
echo "worker_profile=$WORKER_PROFILE"
echo "profile_digest=$PROFILE_DIGEST"
echo "publisher_token_file=$ROOT/secrets/github-token"
