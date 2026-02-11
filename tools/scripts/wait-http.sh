#!/usr/bin/env bash
set -euo pipefail

URL="${1:?missing url}"
TRIES="${2:-20}"

attempt=1
while [[ "${attempt}" -le "${TRIES}" ]]; do
  if curl -fsS "${URL}" > /dev/null 2>&1; then
    echo "wait-http: ok (${URL})"
    exit 0
  fi

  echo "wait-http: waiting (${attempt}/${TRIES}) ${URL}"
  attempt=$((attempt + 1))
  sleep 1
done

echo "wait-http: timeout waiting for ${URL}"
exit 1
