#!/usr/bin/env bash
# Generic single-host deployment helper. Run behind a TLS reverse proxy.
set -euo pipefail
cd "$(dirname "$0")"

if [ ! -f .env.prod ]; then
  echo "ERROR: .env.prod is missing. Create it from .env.prod.example." >&2
  exit 1
fi

docker compose -f docker-compose.prod.yml --env-file .env.prod up -d --build --remove-orphans
docker compose -f docker-compose.prod.yml --env-file .env.prod ps
