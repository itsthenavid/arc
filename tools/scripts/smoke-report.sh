#!/usr/bin/env bash
set -euo pipefail

# Simple JSON writer without external deps.
# Usage:
#   smoke_kv key value
#   smoke_begin
#   smoke_end
#
# Values are JSON-string escaped minimally (quotes/backslashes/newlines).

STATE_DIR="${STATE_DIR:-tools/.state}"
REPORT_PATH="${REPORT_PATH:-${STATE_DIR}/smoke.json}"

mkdir -p "${STATE_DIR}"

json_escape() {
  local s="${1}"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  s="${s//$'\n'/\\n}"
  s="${s//$'\r'/\\r}"
  s="${s//$'\t'/\\t}"
  echo -n "${s}"
}

SMOKE_JSON=""

smoke_begin() {
  SMOKE_JSON="{"
}

smoke_kv() {
  local k="${1}"
  local v="${2}"
  local ek ev
  ek="$(json_escape "${k}")"
  ev="$(json_escape "${v}")"
  if [[ "${SMOKE_JSON}" != "{" ]]; then
    SMOKE_JSON+=","
  fi
  SMOKE_JSON+="\"${ek}\":\"${ev}\""
}

smoke_end() {
  SMOKE_JSON+="}"
  echo "${SMOKE_JSON}" > "${REPORT_PATH}"
  echo "smoke-report: wrote ${REPORT_PATH}"
}
