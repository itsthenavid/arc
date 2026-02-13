#!/usr/bin/env bash
set -euo pipefail

# Keep post-create deterministic: only installs repo-local prerequisites for dev container.
# No hidden global magic beyond the container.

echo "Devcontainer post-create: installing pinned tooling..."

# Keep in sync with CI + local scripts.
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.8.0
go install mvdan.cc/sh/v3/cmd/shfmt@v3.12.0

if command -v apt-get > /dev/null 2>&1; then
  sudo apt-get update
  sudo apt-get install -y shellcheck postgresql-client
fi

echo "Devcontainer post-create: done."
