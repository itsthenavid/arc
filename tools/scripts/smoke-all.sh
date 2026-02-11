#!/usr/bin/env bash
set -Eeuo pipefail

# smoke-all.sh - Full local smoke run:
# 1) Quality gates (fmt/lint/script checks/tests)
# 2) Memory-mode server smoke
# 3) Postgres-mode smoke (infra-up + server + ws-smoke)

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

# shellcheck source=tools/scripts/ports.sh
source tools/scripts/ports.sh
# shellcheck source=tools/scripts/smoke-report.sh
source tools/scripts/smoke-report.sh

log() { echo "$@"; }

INFRA_UP=0
MEM_SERVER_PID=""
PG_SERVER_PID=""

cleanup() {
  # Stop postgres-mode server
  if [[ -n "${PG_SERVER_PID}" ]] && kill -0 "${PG_SERVER_PID}" > /dev/null 2>&1; then
    kill "${PG_SERVER_PID}" > /dev/null 2>&1 || true
    wait "${PG_SERVER_PID}" > /dev/null 2>&1 || true
  fi

  # Stop memory-mode server
  if [[ -n "${MEM_SERVER_PID}" ]] && kill -0 "${MEM_SERVER_PID}" > /dev/null 2>&1; then
    kill "${MEM_SERVER_PID}" > /dev/null 2>&1 || true
    wait "${MEM_SERVER_PID}" > /dev/null 2>&1 || true
  fi

  if [[ "${INFRA_UP}" -eq 1 ]]; then
    bash "${ROOT_DIR}/tools/scripts/infra-down.sh" || true
  fi
}

on_err() {
  local code=$?
  report_end "fail"
  cleanup
  echo "FAIL: smoke-all.sh (exit=${code})" >&2
  exit "${code}"
}

on_int() {
  report_end "fail"
  cleanup
  echo "FAIL: smoke-all.sh interrupted" >&2
  exit 130
}

trap on_err ERR
trap on_int INT TERM
trap cleanup EXIT

report_begin
report_kv "cwd" "${ROOT_DIR}"

log
log "==> Smoke: begin"

# 1) Quality gates
step_begin "1) Quality gates (fmt/lint/script/test)"
t0="$(now_ms)"

bash "${ROOT_DIR}/tools/scripts/doctor.sh"
bash "${ROOT_DIR}/tools/scripts/fmt.sh"
bash "${ROOT_DIR}/tools/scripts/shfmt.sh"
bash "${ROOT_DIR}/tools/scripts/shellcheck.sh"
bash "${ROOT_DIR}/tools/scripts/lint.sh"
bash "${ROOT_DIR}/tools/scripts/test.sh"

t1="$(now_ms)"
step_kv "duration_ms" "$((t1 - t0))"
step_end "ok"

# 2) Memory-mode smoke
step_begin "2) Memory-mode smoke (no DB)"
MEM_PORT="$(pick_free_port 8080)"
MEM_ADDR="127.0.0.1:${MEM_PORT}"
step_kv "http_addr" "${MEM_ADDR}"

ARC_HTTP_ADDR="${MEM_ADDR}" ARC_DATABASE_URL="" bash "${ROOT_DIR}/tools/scripts/run-server.sh" &
MEM_SERVER_PID="$!"

bash "${ROOT_DIR}/tools/scripts/wait-http.sh" "http://${MEM_ADDR}/healthz" "25"
URL="ws://${MEM_ADDR}/ws" bash "${ROOT_DIR}/tools/scripts/ws-smoke.sh"

kill "${MEM_SERVER_PID}" > /dev/null 2>&1 || true
wait "${MEM_SERVER_PID}" > /dev/null 2>&1 || true
MEM_SERVER_PID=""

step_end "ok"

# 3) Postgres-mode smoke
step_begin "3) Postgres-mode smoke (infra-up + DB server + ws-smoke)"

bash "${ROOT_DIR}/tools/scripts/infra-up.sh"
INFRA_UP=1

# shellcheck source=/dev/null
source "${ROOT_DIR}/tools/.state/infra.env"
step_kv "postgres_port" "${POSTGRES_PORT}"
step_kv "redis_port" "${REDIS_PORT}"

PG_HTTP_PORT="$(pick_free_port 8080)"
PG_HTTP_ADDR="127.0.0.1:${PG_HTTP_PORT}"
step_kv "http_addr" "${PG_HTTP_ADDR}"

ARC_HTTP_ADDR="${PG_HTTP_ADDR}" \
  ARC_DATABASE_URL="postgres://arc:arc_dev_password@127.0.0.1:${POSTGRES_PORT}/arc?sslmode=disable" \
  bash "${ROOT_DIR}/tools/scripts/run-server.sh" &
PG_SERVER_PID="$!"

bash "${ROOT_DIR}/tools/scripts/wait-http.sh" "http://${PG_HTTP_ADDR}/healthz" "30"
URL="ws://${PG_HTTP_ADDR}/ws" bash "${ROOT_DIR}/tools/scripts/ws-smoke.sh"

kill "${PG_SERVER_PID}" > /dev/null 2>&1 || true
wait "${PG_SERVER_PID}" > /dev/null 2>&1 || true
PG_SERVER_PID=""

step_end "ok"

report_end "ok"

log
log "OK: smoke-all"
