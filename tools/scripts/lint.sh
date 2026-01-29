#!/usr/bin/env bash
set -euo pipefail

( cd server/go && golangci-lint run --config .golangci.yml )
echo "OK: golangci-lint"

( cd client/flutter && flutter analyze )
echo "OK: flutter analyze"
