#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

echo "fmt: gofmt (server/go)"
(
  cd "$ROOT_DIR/server/go"
  gofmt -w .
)

echo "fmt: gofmt (shared)"
(
  cd "$ROOT_DIR/shared"
  gofmt -w .
)

echo "OK: gofmt"
