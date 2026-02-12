#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# Use writable caches in restrictive environments (e.g. sandboxed CI/local).
: "${GOCACHE:=/tmp/arc-gocache}"
mkdir -p "${GOCACHE}"
export GOCACHE

echo "test: go (server/go)"
(
  cd "$ROOT_DIR/server/go"
  go test ./...
)

echo "test: go (shared)"
(
  cd "$ROOT_DIR/shared"
  go test ./...
)

# Flutter tests are optional in environments where Flutter SDK is not installed.
# The dedicated CI job "Flutter (test)" is responsible for guaranteeing Flutter tests run in CI.
if command -v flutter > /dev/null 2>&1; then
  if [[ -d "$ROOT_DIR/client/flutter" ]]; then
    if flutter --version --machine > /dev/null 2>&1; then
      echo "test: flutter (client/flutter)"
      (
        cd "$ROOT_DIR/client/flutter"
        flutter test
      )
    else
      echo "SKIP: flutter (client/flutter) - flutter not usable (check permissions/update)"
    fi
  else
    echo "SKIP: flutter (client/flutter) directory not found"
  fi
else
  echo "SKIP: flutter (client/flutter) - flutter binary not installed"
fi
