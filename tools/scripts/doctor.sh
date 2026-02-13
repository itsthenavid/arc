#!/usr/bin/env bash
set -Eeuo pipefail

# doctor.sh - Environment sanity checks (non-invasive).
# Exits non-zero on missing required tooling for Go server workflows.

need_cmd() {
  local cmd="${1}"
  if ! command -v "${cmd}" > /dev/null 2>&1; then
    echo "MISSING: ${cmd}" >&2
    return 1
  fi
  return 0
}

echo "Arc doctor:"
echo "- git: $(git --version 2> /dev/null || true)"
echo "- docker: $(docker --version 2> /dev/null || true)"
echo "- docker compose: $(docker compose version 2> /dev/null || true)"
echo "- go: $(go version 2> /dev/null || true)"
echo "- golangci-lint: $(golangci-lint --version 2> /dev/null | head -n 1 || true)"
echo "- curl: $(curl --version 2> /dev/null | head -n 1 || true)"
echo "- python3: $(python3 --version 2> /dev/null || true)"
echo "- psql: $(psql --version 2> /dev/null || true)"
echo "- atlas: $(atlas version 2> /dev/null || true)"
echo "- shfmt: $(shfmt --version 2> /dev/null || true)"
echo "- shellcheck: $(shellcheck --version 2> /dev/null | head -n 1 || true)"
echo "- flutter: $(flutter --version 2> /dev/null | head -n 1 || true)"
echo "- mise: $(mise --version 2> /dev/null || true)"
echo "- lefthook: $(lefthook version 2> /dev/null || true)"

# Required for Go/backend workflows
errs=0
need_cmd git || errs=$((errs + 1))
need_cmd go || errs=$((errs + 1))
need_cmd golangci-lint || errs=$((errs + 1))
need_cmd curl || errs=$((errs + 1))
need_cmd python3 || errs=$((errs + 1))
need_cmd shfmt || errs=$((errs + 1))
need_cmd shellcheck || errs=$((errs + 1))

# Docker is required for infra smoke; don't hard-fail if user only runs unit tests,
# but for ci-local and smoke-all we WANT it installed.
need_cmd docker || errs=$((errs + 1))
(docker compose version > /dev/null 2>&1) || errs=$((errs + 1))

if [[ "${errs}" -ne 0 ]]; then
  echo "doctor: FAIL (${errs} missing/invalid dependencies)" >&2
  exit 1
fi

if ! command -v psql > /dev/null 2>&1; then
  echo "doctor: WARN psql not found (needed for apply-schema/smoke postgres path)" >&2
fi
if ! command -v atlas > /dev/null 2>&1; then
  echo "doctor: WARN atlas not found (optional for schema inspection workflows)" >&2
fi

echo "doctor: OK"
