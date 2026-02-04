#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

echo "test: go (server/go)"
(
  cd "$ROOT_DIR/server/go"
  go test ./...
)

echo "test: go (shared)"
(
  cd "$ROOT_DIR/shared"
  go test ./...
)

echo "test: flutter (client/flutter)"
(
  cd "$ROOT_DIR/client/flutter"
  flutter test
)

echo "OK: tests"
