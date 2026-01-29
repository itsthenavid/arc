#!/usr/bin/env bash
set -euo pipefail

echo "Arc doctor:"
echo "- git: $(git --version || true)"
echo "- docker: $(docker --version || true)"
echo "- docker compose: $(docker compose version || true)"
echo "- go: $(go version || true)"
echo "- golangci-lint: $(golangci-lint --version 2>/dev/null | head -n 1 || true)"
echo "- flutter: $(flutter --version 2>/dev/null | head -n 1 || true)"
echo "- mise: $(mise --version 2>/dev/null || true)"
echo "- lefthook: $(lefthook version 2>/dev/null || true)"
