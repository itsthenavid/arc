#!/usr/bin/env bash
set -euo pipefail

# CI-friendly WebSocket smoke runner (PR-002).
# Runs the two-client websocket smoke test against a running server.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# Default server endpoint.
URL="${URL:-ws://127.0.0.1:8080/ws}"

# Browsers always send Origin in WS handshakes; CLIs often don't.
# If server enforces origin policy, smoke tests must emulate browsers.
ORIGIN="${ORIGIN:-http://localhost}"

# Scenario params.
CONV="${CONV:-dev-room-1}"
KIND="${KIND:-direct}"
TEXT="${TEXT:-hello arc 👋}"

# Per-step timeout for the smoke test (Go's time.Duration format).
TIMEOUT="${TIMEOUT:-7s}"

cd "$ROOT_DIR"

echo "ws-smoke: url=${URL} origin=${ORIGIN} conv=${CONV} kind=${KIND} timeout=${TIMEOUT}"

go run ./server/go/tools/scripts/ws-smoke.go \
  -url "$URL" \
  -origin "$ORIGIN" \
  -conv "$CONV" \
  -kind "$KIND" \
  -text "$TEXT" \
  -timeout "$TIMEOUT"
