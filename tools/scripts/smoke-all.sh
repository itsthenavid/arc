#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=tools/scripts/ports.sh
source tools/scripts/ports.sh
# shellcheck source=tools/scripts/smoke-report.sh
source tools/scripts/smoke-report.sh

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

log() { echo "$@"; }

INFRA_UP=0
MEM_SERVER_PID=""
PG_SERVER_PID=""

now_ms() {
  # macOS compatible
  python3 - <<'PY'
import time
print(int(time.time()*1000))
PY
}

cleanup() {
  # Stop postgres server if running
  if [[ -n "${PG_SERVER_PID}" ]] && kill -0 "${PG_SERVER_PID}" >/dev/null 2>&1; then
    kill "${PG_SERVER_PID}" >/dev/null 2>&1 || true
    wait "${PG_SERVER_PID}" >/dev/null 2>&1 || true
  fi

  # Stop memory server if running
  if [[ -n "${MEM_SERVER_PID}" ]] && kill -0 "${MEM_SERVER_PID}" >/dev/null 2>&1; then
    kill "${MEM_SERVER_PID}" >/dev/null 2>&1 || true
    wait "${MEM_SERVER_PID}" >/dev/null 2>&1 || true
  fi

  if [[ "${INFRA_UP}" -eq 1 ]]; then
    bash "${ROOT_DIR}/tools/scripts/infra-down.sh" || true
  fi
}
trap cleanup EXIT

smoke_begin
smoke_kv "started_at_ms" "$(now_ms)"
smoke_kv "status" "running"

step() {
  local name="$1"
  log
  log "==> ${name}"
  smoke_kv "step" "${name}"
}

step "1) Quality gates (fmt/lint/test)"
t0="$(now_ms)"
bash "${ROOT_DIR}/tools/scripts/fmt.sh"
bash "${ROOT_DIR}/tools/scripts/lint.sh"
bash "${ROOT_DIR}/tools/scripts/test.sh"
t1="$(now_ms)"
smoke_kv "quality_gates_ms" "$((t1 - t0))"

step "2) Memory-mode smoke (no DB)"
MEM_PORT="$(pick_free_port 8080)"
MEM_ADDR="127.0.0.1:${MEM_PORT}"
smoke_kv "memory_http_addr" "${MEM_ADDR}"

ARC_HTTP_ADDR="${MEM_ADDR}" ARC_DATABASE_URL="" bash "${ROOT_DIR}/tools/scripts/run-server.sh" &
MEM_SERVER_PID="$!"

bash "${ROOT_DIR}/tools/scripts/wait-http.sh" "http://${MEM_ADDR}/healthz"
URL="ws://${MEM_ADDR}/ws" bash "${ROOT_DIR}/tools/scripts/ws-smoke.sh"

# Stop memory server explicitly before proceeding.
kill "${MEM_SERVER_PID}" >/dev/null 2>&1 || true
wait "${MEM_SERVER_PID}" >/dev/null 2>&1 || true
MEM_SERVER_PID=""

step "3) Postgres-mode smoke (infra-up)"
bash "${ROOT_DIR}/tools/scripts/infra-up.sh"
INFRA_UP=1

# shellcheck source=/dev/null
source "${ROOT_DIR}/tools/.state/infra.env"
smoke_kv "postgres_port" "${POSTGRES_PORT}"
smoke_kv "redis_port" "${REDIS_PORT}"

PG_HTTP_PORT="$(pick_free_port 8080)"
PG_HTTP_ADDR="127.0.0.1:${PG_HTTP_PORT}"
smoke_kv "postgres_http_addr" "${PG_HTTP_ADDR}"

ARC_HTTP_ADDR="${PG_HTTP_ADDR}" \
ARC_DATABASE_URL="postgres://arc:arc_dev_password@127.0.0.1:${POSTGRES_PORT}/arc?sslmode=disable" \
bash "${ROOT_DIR}/tools/scripts/run-server.sh" &
PG_SERVER_PID="$!"

bash "${ROOT_DIR}/tools/scripts/wait-http.sh" "http://${PG_HTTP_ADDR}/healthz"
URL="ws://${PG_HTTP_ADDR}/ws" bash "${ROOT_DIR}/tools/scripts/ws-smoke.sh"

# Stop postgres server
kill "${PG_SERVER_PID}" >/dev/null 2>&1 || true
wait "${PG_SERVER_PID}" >/dev/null 2>&1 || true
PG_SERVER_PID=""

smoke_kv "status" "ok"
smoke_kv "finished_at_ms" "$(now_ms)"
smoke_end

log
log "OK: smoke-all"
