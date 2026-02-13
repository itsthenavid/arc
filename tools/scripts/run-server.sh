#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

HTTP_ADDR="${ARC_HTTP_ADDR:-127.0.0.1:8080}"
DB_URL="${ARC_DATABASE_URL:-}"
LOG_LEVEL="${ARC_LOG_LEVEL:-info}"
LOG_FORMAT="${ARC_LOG_FORMAT:-auto}"

color_enabled=0
if [[ -t 1 && "${TERM:-}" != "dumb" ]]; then
  color_enabled=1
fi

if [[ "${color_enabled}" -eq 1 ]]; then
  c_reset=$'\033[0m'
  c_dim=$'\033[2m'
  c_accent=$'\033[36m'
  c_ok=$'\033[32m'
else
  c_reset=""
  c_dim=""
  c_accent=""
  c_ok=""
fi

host="${HTTP_ADDR%:*}"
port="${HTTP_ADDR##*:}"
if [[ "${host}" == "${HTTP_ADDR}" ]]; then
  host="127.0.0.1"
  port="8080"
fi
if [[ "${host}" == "0.0.0.0" || "${host}" == "::" || -z "${host}" ]]; then
  host="127.0.0.1"
fi
base_url="http://${host}:${port}"

if [[ -z "${DB_URL}" ]]; then
  mode="memory"
else
  mode="postgres"
fi

printf "%sArc Server Runtime%s\n" "${c_accent}" "${c_reset}"
printf "%s%-14s%s %s\n" "${c_dim}" "http_addr" "${c_reset}" "${HTTP_ADDR}"
printf "%s%-14s%s %s\n" "${c_dim}" "mode" "${c_reset}" "${mode}"
printf "%s%-14s%s %s\n" "${c_dim}" "log_level" "${c_reset}" "${LOG_LEVEL}"
printf "%s%-14s%s %s\n" "${c_dim}" "log_format" "${c_reset}" "${LOG_FORMAT}"
printf "%s%-14s%s %s\n" "${c_dim}" "healthz" "${c_reset}" "${base_url}/healthz"
printf "%s%-14s%s %s\n" "${c_dim}" "readyz" "${c_reset}" "${base_url}/readyz"
printf "%s%-14s%s %s\n" "${c_dim}" "ws" "${c_reset}" "ws://${host}:${port}/ws"
printf "%sready%s\n" "${c_ok}" "${c_reset}"

export ARC_HTTP_ADDR="${HTTP_ADDR}"
export ARC_DATABASE_URL="${DB_URL}"
export ARC_LOG_LEVEL="${LOG_LEVEL}"
export ARC_LOG_FORMAT="${LOG_FORMAT}"

cd "${ROOT_DIR}/server/go"
exec go run ./cmd/arc
