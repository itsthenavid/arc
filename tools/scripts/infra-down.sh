#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

# shellcheck source=tools/scripts/state.sh
source tools/scripts/state.sh
# shellcheck source=tools/scripts/compose.sh
source tools/scripts/compose.sh

local_compose_file="$(compose_file)"

docker compose -f "$local_compose_file" down

INFRA_ENV_FILE="$(infra_env_path)"
if [[ -f "$INFRA_ENV_FILE" ]]; then
  rm -f "$INFRA_ENV_FILE"
fi
