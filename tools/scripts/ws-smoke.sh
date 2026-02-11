#!/usr/bin/env bash
# ws-smoke.sh - CI-friendly WebSocket smoke runner for Arc (PR-002).
# Runs the two-client websocket smoke test against a running server.

set -Eeuo pipefail

on_err() {
  local code=$?
  echo "FAIL: ws-smoke.sh: unexpected error (exit=${code})" >&2
  exit "${code}"
}
trap on_err ERR

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

URL="${URL:-ws://127.0.0.1:8080/ws}"
ORIGIN="${ORIGIN:-http://localhost}"
CONV="${CONV:-dev-room-1}"
KIND="${KIND:-direct}"
TEXT="${TEXT:-hello arc 👋}"
TIMEOUT="${TIMEOUT:-7s}"

if [[ "${URL}" != ws://* && "${URL}" != wss://* ]]; then
  echo "FAIL: ws-smoke.sh: URL must start with ws:// or wss:// (got: ${URL})" >&2
  exit 2
fi

echo "ws-smoke: url=${URL} origin=${ORIGIN} conv=${CONV} kind=${KIND} timeout=${TIMEOUT}"

# Run the Go smoke test from the repo root.
go run ./server/go/tools/scripts/ws-smoke.go \
  -url "${URL}" \
  -origin "${ORIGIN}" \
  -conv "${CONV}" \
  -kind "${KIND}" \
  -text "${TEXT}" \
  -timeout "${TIMEOUT}"
