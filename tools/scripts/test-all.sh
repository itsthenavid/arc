#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}/server/go"

echo "==> Running comprehensive test suite"
echo

# 1. Unit tests with coverage
echo "1. Unit tests with coverage..."
go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
echo

# 2. Integration tests (requires DB)
if [[ -n "${ARC_DATABASE_URL:-}" ]]; then
  echo "2. Integration tests..."
  go test -v -race -tags=integration ./...
  echo
else
  echo "2. Integration tests SKIPPED (ARC_DATABASE_URL not set)"
  echo
fi

# 3. Benchmark tests
echo "3. Benchmark tests..."
go test -bench=. -benchmem -run=^$ ./tools/benchmarks/...
echo

# 4. Race detector
echo "4. Race detector..."
go test -race ./...
echo

# 5. Coverage report
echo "5. Coverage summary:"
go tool cover -func=coverage.out | tail -n 1
echo

# 6. Generate HTML coverage report
echo "6. Generating HTML coverage report..."
go tool cover -html=coverage.out -o coverage.html
echo "   -> coverage.html"
echo

echo "==> All tests passed! ✅"
