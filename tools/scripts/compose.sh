#!/usr/bin/env bash
set -euo pipefail

# Resolve docker compose file path in a monorepo-friendly way.
# This avoids brittle assumptions about where the compose file lives.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

compose_file() {
  local candidates=(
    "${ROOT_DIR}/compose.yml"
    "${ROOT_DIR}/compose.yaml"
    "${ROOT_DIR}/docker-compose.yml"
    "${ROOT_DIR}/docker-compose.yaml"
    "${ROOT_DIR}/infra/compose.yml"
    "${ROOT_DIR}/infra/compose.yaml"
    "${ROOT_DIR}/infra/docker-compose.yml"
    "${ROOT_DIR}/infra/docker-compose.yaml"
    "${ROOT_DIR}/tools/infra/compose.yml"
    "${ROOT_DIR}/tools/infra/compose.yaml"
    "${ROOT_DIR}/tools/infra/docker-compose.yml"
    "${ROOT_DIR}/tools/infra/docker-compose.yaml"
  )

  local f
  for f in "${candidates[@]}"; do
    if [[ -f "$f" ]]; then
      echo "$f"
      return 0
    fi
  done

  echo "Error: docker compose file not found." >&2
  echo "Searched candidates:" >&2
  for f in "${candidates[@]}"; do
    echo "  - $f" >&2
  done
  echo "Fix: add a compose file in one of these locations or update tools/scripts/compose.sh." >&2
  return 1
}

compose_env_file() {
  local candidates=(
    "${ROOT_DIR}/infra/.env"
    "${ROOT_DIR}/.env"
    "${ROOT_DIR}/infra/.env.example"
  )

  local f
  for f in "${candidates[@]}"; do
    if [[ -f "${f}" ]]; then
      echo "${f}"
      return 0
    fi
  done

  return 1
}

compose_cmd() {
  local local_compose_file="${1:?missing compose file}"
  shift

  local env_file=""
  if env_file="$(compose_env_file)"; then
    docker compose --env-file "${env_file}" -f "${local_compose_file}" "$@"
    return $?
  fi

  docker compose -f "${local_compose_file}" "$@"
}
