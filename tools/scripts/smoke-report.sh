#!/usr/bin/env bash
set -Eeuo pipefail

# smoke-report.sh - Structured JSON reporting for smoke runs.
# Produces a stable schema with NO duplicate keys (unlike naive KV appends).
#
# Output schema (example):
# {
#   "started_at_ms":"...",
#   "finished_at_ms":"...",
#   "status":"ok|running|fail",
#   "steps":[
#     {"name":"1) ...","started_at_ms":"...","finished_at_ms":"...","data":{"k":"v"}},
#     ...
#   ],
#   "data":{"k":"v"}
# }

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
  printf "%s" "${s}"
}

# Global report state
REPORT_STARTED_AT_MS=""
REPORT_FINISHED_AT_MS=""
REPORT_STATUS="running"

REPORT_DATA_JSON="{}"
STEPS_JSON="[]"

# Current step state
STEP_OPEN=0
STEP_NAME=""
STEP_STARTED_AT_MS=""
STEP_FINISHED_AT_MS=""
STEP_DATA_JSON="{}"

now_ms() {
  python3 - << 'PY'
import time
print(int(time.time() * 1000))
PY
}

# Minimal "merge" into a JSON object (string-only values).
# We keep this deterministic and dependency-free; values are always JSON strings.
json_obj_add_kv() {
  local obj="${1}"
  local k="${2}"
  local v="${3}"

  local ek ev
  ek="$(json_escape "${k}")"
  ev="$(json_escape "${v}")"

  if [[ "${obj}" == "{}" ]]; then
    printf "{\"%s\":\"%s\"}" "${ek}" "${ev}"
  else
    # Insert before trailing }
    printf "%s" "${obj}" | sed -e "s/}$/,\"${ek}\":\"${ev}\"}/"
  fi
}

json_array_append_obj() {
  local arr="${1}"
  local obj="${2}"

  if [[ "${arr}" == "[]" ]]; then
    printf "[%s]" "${obj}"
  else
    # Insert before trailing ]
    printf "%s" "${arr}" | sed -e "s/]$/,${obj}]/"
  fi
}

report_begin() {
  REPORT_STARTED_AT_MS="$(now_ms)"
  REPORT_STATUS="running"
  REPORT_DATA_JSON="{}"
  STEPS_JSON="[]"

  STEP_OPEN=0
  STEP_NAME=""
  STEP_STARTED_AT_MS=""
  STEP_FINISHED_AT_MS=""
  STEP_DATA_JSON="{}"

  report_write
}

report_kv() {
  local k="${1}"
  local v="${2}"
  REPORT_DATA_JSON="$(json_obj_add_kv "${REPORT_DATA_JSON}" "${k}" "${v}")"
  report_write
}

step_begin() {
  local name="${1:?missing step name}"

  # Close any previous step.
  if [[ "${STEP_OPEN}" -eq 1 ]]; then
    step_end "ok"
  fi

  STEP_OPEN=1
  STEP_NAME="${name}"
  STEP_STARTED_AT_MS="$(now_ms)"
  STEP_FINISHED_AT_MS=""
  STEP_DATA_JSON="{}"

  report_write
}

step_kv() {
  local k="${1}"
  local v="${2}"
  if [[ "${STEP_OPEN}" -ne 1 ]]; then
    echo "smoke-report: step_kv called without an open step" >&2
    exit 2
  fi
  STEP_DATA_JSON="$(json_obj_add_kv "${STEP_DATA_JSON}" "${k}" "${v}")"
  report_write
}

step_end() {
  local status="${1:-ok}"

  if [[ "${STEP_OPEN}" -ne 1 ]]; then
    return 0
  fi

  STEP_FINISHED_AT_MS="$(now_ms)"

  local step_obj
  step_obj="$(
    printf "{%s}" \
      "\"name\":\"$(json_escape "${STEP_NAME}")\",""\
\"status\":\"$(json_escape "${status}")\",""\
\"started_at_ms\":\"$(json_escape "${STEP_STARTED_AT_MS}")\",""\
\"finished_at_ms\":\"$(json_escape "${STEP_FINISHED_AT_MS}")\",""\
\"data\":${STEP_DATA_JSON}"
  )"

  STEPS_JSON="$(json_array_append_obj "${STEPS_JSON}" "${step_obj}")"

  STEP_OPEN=0
  STEP_NAME=""
  STEP_STARTED_AT_MS=""
  STEP_FINISHED_AT_MS=""
  STEP_DATA_JSON="{}"

  report_write
}

report_end() {
  local status="${1:-ok}"

  # Close any open step.
  if [[ "${STEP_OPEN}" -eq 1 ]]; then
    step_end "${status}"
  fi

  REPORT_STATUS="${status}"
  REPORT_FINISHED_AT_MS="$(now_ms)"
  report_write
}

report_write() {
  local started="${REPORT_STARTED_AT_MS}"
  local finished="${REPORT_FINISHED_AT_MS}"
  local status="${REPORT_STATUS}"

  if [[ -z "${started}" ]]; then
    started="$(now_ms)"
  fi
  if [[ -z "${status}" ]]; then
    status="running"
  fi

  local json
  json="$(
    printf "{%s}" \
      "\"started_at_ms\":\"$(json_escape "${started}")\",""\
\"finished_at_ms\":\"$(json_escape "${finished}")\",""\
\"status\":\"$(json_escape "${status}")\",""\
\"steps\":${STEPS_JSON},""\
\"data\":${REPORT_DATA_JSON}"
  )"

  printf "%s\n" "${json}" > "${REPORT_PATH}"
  echo "smoke-report: wrote ${REPORT_PATH}" > /dev/null
}
