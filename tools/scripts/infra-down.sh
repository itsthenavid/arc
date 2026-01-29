#!/usr/bin/env bash
set -euo pipefail

docker compose -f infra/compose.yml down
echo "Infra is down."
