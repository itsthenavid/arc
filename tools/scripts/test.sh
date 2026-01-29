#!/usr/bin/env bash
set -euo pipefail

( cd server/go && go test ./... )
echo "OK: go test"

( cd client/flutter && flutter test )
echo "OK: flutter test"
