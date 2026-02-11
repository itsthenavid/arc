#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

HTTP_ADDR="${ARC_HTTP_ADDR:-127.0.0.1:8080}"
DB_URL="${ARC_DATABASE_URL:-}"

echo "run-server: ARC_HTTP_ADDR=${HTTP_ADDR}"
if [[ -z "${DB_URL}" ]]; then
  echo "run-server: ARC_DATABASE_URL is empty (memory mode)"
else
  echo "run-server: ARC_DATABASE_URL is set (postgres mode)"
fi

export ARC_HTTP_ADDR="${HTTP_ADDR}"
export ARC_DATABASE_URL="${DB_URL}"

cd "${ROOT_DIR}/server/go"
exec go run ./cmd/arc
