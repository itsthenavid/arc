#!/usr/bin/env bash
set -euo pipefail

# Keep post-create deterministic: only installs repo-local prerequisites for dev container.
# No hidden global magic beyond the container.

echo "Devcontainer post-create: installing Go tools..."

go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.60.3

echo "Done."
