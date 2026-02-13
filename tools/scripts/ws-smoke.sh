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
AUTH_BEARER="${AUTH_BEARER:-}"
AUTH_QUERY_PARAM="${AUTH_QUERY_PARAM:-}"
EXPECT_UNAUTHORIZED="${EXPECT_UNAUTHORIZED:-false}"

if [[ "${URL}" != ws://* && "${URL}" != wss://* ]]; then
  echo "FAIL: ws-smoke.sh: URL must start with ws:// or wss:// (got: ${URL})" >&2
  exit 2
fi

if [[ "${EXPECT_UNAUTHORIZED}" == "true" && (-n "${AUTH_BEARER}" || -n "${AUTH_QUERY_PARAM}") ]]; then
  echo "FAIL: ws-smoke.sh: EXPECT_UNAUTHORIZED=true cannot be combined with AUTH_BEARER/AUTH_QUERY_PARAM" >&2
  exit 2
fi

if [[ -n "${AUTH_QUERY_PARAM}" && -z "${AUTH_BEARER}" ]]; then
  echo "FAIL: ws-smoke.sh: AUTH_QUERY_PARAM requires AUTH_BEARER" >&2
  exit 2
fi

auth_mode="none"
if [[ -n "${AUTH_BEARER}" ]]; then
  auth_mode="bearer"
fi
if [[ -n "${AUTH_QUERY_PARAM}" ]]; then
  auth_mode="query"
fi

echo "ws-smoke: url=${URL} origin=${ORIGIN} conv=${CONV} kind=${KIND} timeout=${TIMEOUT} auth=${auth_mode} expect_unauthorized=${EXPECT_UNAUTHORIZED}"

args=(
  -url "${URL}"
  -origin "${ORIGIN}"
  -conv "${CONV}"
  -kind "${KIND}"
  -text "${TEXT}"
  -timeout "${TIMEOUT}"
)
if [[ "${EXPECT_UNAUTHORIZED}" == "true" ]]; then
  args+=(-expect-unauthorized)
fi
if [[ -n "${AUTH_QUERY_PARAM}" ]]; then
  args+=(-auth-query-param "${AUTH_QUERY_PARAM}")
fi

if [[ -n "${AUTH_BEARER}" ]]; then
  WS_SMOKE_AUTH_BEARER="${AUTH_BEARER}" go run ./server/go/tools/scripts/ws-smoke.go "${args[@]}"
else
  go run ./server/go/tools/scripts/ws-smoke.go "${args[@]}"
fi
