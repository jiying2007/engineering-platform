#!/usr/bin/env bash
set -euo pipefail

[ "$#" -eq 1 ] || { echo "usage: postgres.sh ABSOLUTE_ROOT" >&2; exit 2; }
ROOT="$1"
case "$ROOT" in /*) ;; *) echo "ROOT must be absolute" >&2; exit 2;; esac
command -v docker >/dev/null

NAME="${EP_PILOT_POSTGRES_NAME:-engineering-platform-pilot-postgres}"
IMAGE="${EP_PILOT_POSTGRES_IMAGE:-postgres:17-alpine}"

if docker inspect "$NAME" >/dev/null 2>&1; then
  running="$(docker inspect -f '{{.State.Running}}' "$NAME")"
  if [ "$running" != true ]; then
    docker start "$NAME" >/dev/null
  fi
else
  docker run -d     --name "$NAME"     -e POSTGRES_USER=postgres     -e POSTGRES_PASSWORD=postgres     -e POSTGRES_DB=engineering_platform     -p 127.0.0.1:55432:5432     --health-cmd "pg_isready -U postgres -d engineering_platform"     --health-interval 2s     --health-timeout 5s     --health-retries 30     "$IMAGE" >/dev/null
fi

for _ in $(seq 1 60); do
  status="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$NAME")"
  if [ "$status" = healthy ]; then
    echo "pilot postgres: READY"
    exit 0
  fi
  sleep 1
done

echo "pilot postgres did not become healthy" >&2
docker logs "$NAME" >&2 || true
exit 1
