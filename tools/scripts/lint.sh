#!/usr/bin/env bash
set -euo pipefail

# Lint Go modules in this monorepo using the root golangci config.
# This avoids config path ambiguity and keeps results consistent in CI/devcontainer/local.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CONFIG_FILE="$ROOT_DIR/.golangci.yml"

if [[ ! -f "$CONFIG_FILE" ]]; then
  echo "ERROR: missing golangci config: $CONFIG_FILE" >&2
  exit 1
fi

echo "lint: golangci-lint (server/go)"
(
  cd "$ROOT_DIR/server/go"
  golangci-lint run --config "$CONFIG_FILE"
)

echo "lint: golangci-lint (shared)"
(
  cd "$ROOT_DIR/shared"
  golangci-lint run --config "$CONFIG_FILE"
)

echo "OK: lint"
