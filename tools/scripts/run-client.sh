#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLIENT_DIR="${ROOT_DIR}/client/flutter"

ARC_API_BASE_URL="${ARC_API_BASE_URL:-http://127.0.0.1:8080}"
ARC_AUTH_INVITE_ONLY="${ARC_AUTH_INVITE_ONLY:-true}"
ARC_AUTH_WEB_COOKIE_MODE="${ARC_AUTH_WEB_COOKIE_MODE:-true}"
ARC_AUTH_CSRF_COOKIE_NAME="${ARC_AUTH_CSRF_COOKIE_NAME:-arc_csrf_token}"
ARC_AUTH_CSRF_HEADER_NAME="${ARC_AUTH_CSRF_HEADER_NAME:-X-CSRF-Token}"
ARC_FLUTTER_DEVICE="${ARC_FLUTTER_DEVICE:-${1:-}}"
ARC_CLIENT_SKIP_PUB_GET="${ARC_CLIENT_SKIP_PUB_GET:-false}"
ARC_CLIENT_FORCE_PUB_GET="${ARC_CLIENT_FORCE_PUB_GET:-false}"
RUN_NO_PUB="false"

if ! command -v flutter > /dev/null 2>&1; then
  echo "ERROR: flutter is not installed or not in PATH"
  exit 1
fi

if [[ ! -d "${CLIENT_DIR}" ]]; then
  echo "ERROR: client directory not found at ${CLIENT_DIR}"
  exit 1
fi

pushd "${CLIENT_DIR}" > /dev/null

if [[ "${ARC_CLIENT_SKIP_PUB_GET}" == "true" ]]; then
  if [[ ! -f .dart_tool/package_config.json ]]; then
    echo "ERROR: ARC_CLIENT_SKIP_PUB_GET=true but .dart_tool/package_config.json is missing."
    echo "       Run once without ARC_CLIENT_SKIP_PUB_GET or set ARC_CLIENT_FORCE_PUB_GET=true."
    exit 1
  fi
  RUN_NO_PUB="true"
elif [[ -f .dart_tool/package_config.json && "${ARC_CLIENT_FORCE_PUB_GET}" != "true" ]]; then
  echo "Using existing Flutter dependencies (.dart_tool/package_config.json)."
  echo "Set ARC_CLIENT_FORCE_PUB_GET=true to force dependency refresh."
  RUN_NO_PUB="true"
else
  echo "Preparing Flutter dependencies..."
  if ! flutter pub get > /dev/null; then
    if [[ -f .dart_tool/package_config.json ]]; then
      echo "WARN: flutter pub get failed; continuing with existing package_config.json"
      echo "      If build fails, fix pub access or run: ARC_CLIENT_SKIP_PUB_GET=true bash tools/scripts/run-client.sh"
      RUN_NO_PUB="true"
    else
      echo "ERROR: flutter pub get failed and no existing package_config.json found"
      echo "       Check ~/.pub-cache/log/pub_log.txt for details"
      exit 1
    fi
  else
    RUN_NO_PUB="true"
  fi
fi

echo "Arc Flutter Runtime"
echo "------------------------------"
echo "api_base_url: ${ARC_API_BASE_URL}"
echo "invite_only: ${ARC_AUTH_INVITE_ONLY}"
echo "web_cookie_mode: ${ARC_AUTH_WEB_COOKIE_MODE}"
echo "csrf_cookie: ${ARC_AUTH_CSRF_COOKIE_NAME}"
echo "csrf_header: ${ARC_AUTH_CSRF_HEADER_NAME}"
if [[ -n "${ARC_FLUTTER_DEVICE}" ]]; then
  echo "device: ${ARC_FLUTTER_DEVICE}"
else
  echo "device: auto"
fi
if [[ "${RUN_NO_PUB}" == "true" ]]; then
  echo "pub: --no-pub"
else
  echo "pub: auto"
fi
echo "------------------------------"

common_args=()
if [[ "${RUN_NO_PUB}" == "true" ]]; then
  common_args+=(--no-pub)
fi
common_args+=(
  --dart-define="ARC_API_BASE_URL=${ARC_API_BASE_URL}"
  --dart-define="ARC_AUTH_INVITE_ONLY=${ARC_AUTH_INVITE_ONLY}"
  --dart-define="ARC_AUTH_WEB_COOKIE_MODE=${ARC_AUTH_WEB_COOKIE_MODE}"
  --dart-define="ARC_AUTH_CSRF_COOKIE_NAME=${ARC_AUTH_CSRF_COOKIE_NAME}"
  --dart-define="ARC_AUTH_CSRF_HEADER_NAME=${ARC_AUTH_CSRF_HEADER_NAME}"
)

if [[ -n "${ARC_FLUTTER_DEVICE}" ]]; then
  exec flutter run \
    -d "${ARC_FLUTTER_DEVICE}" \
    "${common_args[@]}"
else
  exec flutter run \
    "${common_args[@]}"
fi
