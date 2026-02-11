#!/usr/bin/env bash
set -euo pipefail

# Centralized state paths for scripts (so scripts don't "guess" each other).
# Keep this directory gitignored.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STATE_DIR="${ROOT_DIR}/tools/.state"

infra_env_path() {
  echo "${STATE_DIR}/infra.env"
}

ensure_state_dir() {
  mkdir -p "${STATE_DIR}"
}
