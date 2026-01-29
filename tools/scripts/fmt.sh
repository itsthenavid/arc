#!/usr/bin/env bash
set -euo pipefail

( cd server/go && gofmt -w . )
echo "OK: gofmt"
