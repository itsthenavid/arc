#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

HTTP_ADDR="${ARC_HTTP_ADDR:-127.0.0.1:8080}"
DB_URL="${ARC_DATABASE_URL:-}"
LOG_LEVEL="${ARC_LOG_LEVEL:-info}"
LOG_FORMAT="${ARC_LOG_FORMAT:-auto}"
REQUIRE_HMAC="${ARC_REQUIRE_TOKEN_HMAC:-false}"
TOKEN_HMAC_KEY="${ARC_TOKEN_HMAC_KEY:-}"
PASETO_KEY="${ARC_PASETO_V4_SECRET_KEY_HEX:-}"

color_enabled=0
if [[ -t 1 && "${TERM:-}" != "dumb" ]]; then
  color_enabled=1
fi

if [[ "${color_enabled}" -eq 1 ]]; then
  c_reset=$'\033[0m'
  c_dim=$'\033[2m'
  c_info=$'\033[34m'
  c_warn=$'\033[33m'
  c_error=$'\033[31m'
  c_ok=$'\033[32m'
else
  c_reset=""
  c_dim=""
  c_info=""
  c_warn=""
  c_error=""
  c_ok=""
fi

label() {
  local name="${1}"
  case "${name}" in
    INFO) printf "%s[ℹ INFO]%s" "${c_info}" "${c_reset}" ;;
    WARN) printf "%s[⚠ WARN]%s" "${c_warn}" "${c_reset}" ;;
    ERROR) printf "%s[✖ ERROR]%s" "${c_error}" "${c_reset}" ;;
    OK) printf "%s[✔ OK]%s" "${c_ok}" "${c_reset}" ;;
    *) printf "[%s]" "${name}" ;;
  esac
}

line() {
  local lvl="${1}"
  local msg="${2}"
  printf "%s %s\n" "$(label "${lvl}")" "${msg}"
}

row() {
  local lvl="${1}"
  local key="${2}"
  local val="${3}"
  printf "%s│%s %-12s %s%-14s%s %s\n" \
    "${c_dim}" "${c_reset}" "$(label "${lvl}")" \
    "${c_dim}" "${key}" "${c_reset}" "${val}"
}

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

printf "%s╭───────────────────────────── ✨ Arc Server Runtime ✨ ─────────────────────────────╮%s\n" "${c_dim}" "${c_reset}"
row "INFO" "http_addr" "${HTTP_ADDR}"
row "INFO" "mode" "${mode}"
row "INFO" "log_level" "${LOG_LEVEL}"
row "INFO" "log_format" "${LOG_FORMAT}"
row "INFO" "healthz" "${base_url}/healthz"
row "INFO" "readyz" "${base_url}/readyz"
row "INFO" "ws" "ws://${host}:${port}/ws"
row "OK" "note" "auto pretty logs are enabled when ARC_LOG_FORMAT=auto on a TTY"

if [[ "${mode}" == "postgres" && -z "${PASETO_KEY}" ]]; then
  row "WARN" "paseto_key" "ARC_PASETO_V4_SECRET_KEY_HEX is empty; auth login/refresh endpoints will fail"
fi
if [[ "${REQUIRE_HMAC}" == "true" && ${#TOKEN_HMAC_KEY} -lt 32 ]]; then
  row "WARN" "token_hmac" "ARC_REQUIRE_TOKEN_HMAC=true but ARC_TOKEN_HMAC_KEY is missing/short (min 32 bytes)"
fi
if [[ "${REQUIRE_HMAC}" == "true" && ${#TOKEN_HMAC_KEY} -ge 32 ]]; then
  row "OK" "token_hmac" "token HMAC policy enabled"
fi
printf "%s╰─────────────────────────────────────────────────────────────────────────────────────╯%s\n" "${c_dim}" "${c_reset}"
line "OK" "🚀 launching server"

export ARC_HTTP_ADDR="${HTTP_ADDR}"
export ARC_DATABASE_URL="${DB_URL}"
export ARC_LOG_LEVEL="${LOG_LEVEL}"
export ARC_LOG_FORMAT="${LOG_FORMAT}"

cd "${ROOT_DIR}/server/go"
exec go run ./cmd/arc
