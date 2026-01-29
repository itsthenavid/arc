#!/usr/bin/env bash
set -euo pipefail

docker compose --env-file infra/.env.example -f infra/compose.yml up -d
echo "Infra is up."
