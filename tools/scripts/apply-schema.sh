#!/usr/bin/env bash
set -euo pipefail

# Applies infra/db/atlas/schema.sql to the configured database.
# Default local dev DB is port 5433 (from infra/compose.yml).

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

export ARC_DATABASE_URL="${ARC_DATABASE_URL:-postgres://arc:arc_dev_password@127.0.0.1:5433/arc?sslmode=disable}"

echo "apply-schema: url=${ARC_DATABASE_URL}"

if ! command -v psql > /dev/null 2>&1; then
  echo "apply-schema: psql is required (install postgresql client)"
  exit 1
fi

psql "$ARC_DATABASE_URL" -v ON_ERROR_STOP=1 -f infra/db/atlas/schema.sql
echo "apply-schema: OK"
