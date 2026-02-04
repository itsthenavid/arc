#!/usr/bin/env bash
set -euo pipefail

URL="${1:?missing url}"
TRIES="${2:-20}"

for i in $(seq 1 "${TRIES}"); do
  if curl -fsS "$URL" >/dev/null 2>&1; then
    echo "wait-http: ok ($URL)"
    exit 0
  fi
  sleep 1
done

echo "wait-http: timeout waiting for $URL"
exit 1
