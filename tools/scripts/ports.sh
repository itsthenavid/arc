#!/usr/bin/env bash
set -euo pipefail

# Networking helpers for scripts.
# - is_tcp_port_listening host port
# - pick_free_port preferred_port

is_tcp_port_listening() {
  local host="$1"
  local port="$2"

  # macOS-friendly check: lsof returns 0 when a listener exists.
  if command -v lsof > /dev/null 2>&1; then
    lsof -nP -iTCP@"${host}":"${port}" -sTCP:LISTEN > /dev/null 2>&1
    return $?
  fi

  # Fallback: best-effort using nc if available.
  if command -v nc > /dev/null 2>&1; then
    nc -z "${host}" "${port}" > /dev/null 2>&1
    return $?
  fi

  echo "Error: neither lsof nor nc is available to check ports." >&2
  return 2
}

pick_free_port() {
  local preferred="${1:-0}"
  local host="127.0.0.1"

  # If preferred is free, use it.
  if [[ "$preferred" != "0" ]]; then
    if ! is_tcp_port_listening "$host" "$preferred"; then
      echo "$preferred"
      return 0
    fi
  fi

  # Otherwise, scan a reasonable ephemeral range.
  # We avoid 1024-49151 collisions with common dev services when possible.
  local port
  for port in $(seq 49152 65535); do
    if ! is_tcp_port_listening "$host" "$port"; then
      echo "$port"
      return 0
    fi
  done

  echo "Error: failed to find a free TCP port." >&2
  return 1
}
