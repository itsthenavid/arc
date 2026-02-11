#!/usr/bin/env bash
set -Eeuo pipefail

# shfmt.sh - Format shell scripts deterministically.

if ! command -v shfmt > /dev/null 2>&1; then
  echo "shfmt: shfmt not installed. Install it to enforce shell formatting." >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# Format all .sh files under tools/scripts
find "${ROOT_DIR}/tools/scripts" -type f -name "*.sh" -print0 \
  | xargs -0 shfmt -w -i 2 -bn -ci -sr

echo "OK: shfmt"
