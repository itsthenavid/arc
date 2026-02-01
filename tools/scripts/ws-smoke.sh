#!/usr/bin/env bash
set -euo pipefail

# PR-001 smoke runner (root).
# Runs the two-client websocket smoke test against a running server.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

URL="${URL:-ws://localhost:8080/ws}"
CONV="${CONV:-dev-room-1}"
KIND="${KIND:-direct}"
TEXT="${TEXT:-hello arc 👋}"
TIMEOUT="${TIMEOUT:-5s}"

cd "$ROOT_DIR"

go run ./server/go/tools/scripts/ws-smoke.go \
  -url "$URL" \
  -conv "$CONV" \
  -kind "$KIND" \
  -text "$TEXT" \
  -timeout "$TIMEOUT"
