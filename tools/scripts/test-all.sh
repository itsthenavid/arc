#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}/server/go"

# Use writable caches in restrictive environments.
: "${GOCACHE:=/tmp/arc-gocache}"
mkdir -p "${GOCACHE}"
export GOCACHE

echo "==> Arc comprehensive test suite"
echo

echo "1) Unit tests + race + coverage"
go test -race -coverprofile=coverage.out -covermode=atomic ./...
echo

if [[ -n "${ARC_DATABASE_URL:-}" ]]; then
  # Re-run DB-heavy packages with cache disabled so integration paths are exercised in this run.
  echo "2) Integration-focused re-run (ARC_DATABASE_URL is set)"
  go test -count=1 ./cmd/identity ./cmd/internal/auth/api ./cmd/internal/auth/session ./cmd/internal/invite ./cmd/internal/realtime
else
  echo "2) Integration-focused re-run SKIPPED (ARC_DATABASE_URL not set)"
fi
echo

if [[ "${RUN_BENCHMARKS:-false}" == "true" ]]; then
  echo "3) Benchmarks"
  go test -bench=. -benchmem -run=^$ ./cmd/security/password
else
  echo "3) Benchmarks SKIPPED (set RUN_BENCHMARKS=true)"
fi
echo

echo "4) Coverage summary"
go tool cover -func=coverage.out | tail -n 1

if [[ "${GENERATE_COVERAGE_HTML:-false}" == "true" ]]; then
  go tool cover -html=coverage.out -o coverage.html
  echo "   -> coverage.html"
fi
echo

echo "==> All checks completed."
