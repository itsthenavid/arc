# Arc Flutter Client (PR-012)

This client implements the auth flow for Arc:
- invite entry
- signup
- login
- profile completion (client-side in v1)
- authenticated shell with refresh/logout/invite controls

The UI is responsive, light/dark aware, and optimized for web + desktop + mobile.
Typography does not use runtime font fetching, so the app runs without external font network calls.

The client now includes:
- structured runtime error logging (`flutter.framework_error`, `flutter.platform_error`, `http.error`)
- backend health probe (`/healthz`) with live status/latency in the footer
- stronger API error mapping for network/timeout/unauthorized/server modes
- automatic safe cookie-mode behavior: web cookie transport is only used on web targets

## Quick Start (Full Auth Enabled)

Run from repo root:

```bash
cd /Users/navid/Development/Multi-Platform/arc

# 1) Bring up Postgres/Redis infra
bash tools/scripts/infra-up.sh
source tools/.state/infra.env

# 2) Configure DB URL for this shell
export ARC_DATABASE_URL="postgres://arc:arc_dev_password@127.0.0.1:${POSTGRES_PORT}/arc?sslmode=disable"

# 3) Apply schema
bash tools/scripts/apply-schema.sh

# 4) Generate a valid PASETO v4 secret key
export ARC_PASETO_V4_SECRET_KEY_HEX="$(bash tools/scripts/gen-paseto-key.sh)"

# 5) Start backend (auth endpoints require DB + PASETO key)
ARC_LOG_FORMAT=auto bash tools/scripts/run-server.sh
```

In a second terminal:

```bash
cd /Users/navid/Development/Multi-Platform/arc

# Optional when pub network is blocked but deps already resolved:
# export ARC_CLIENT_SKIP_PUB_GET=true

# Auto-selects device, or set ARC_FLUTTER_DEVICE=chrome|macos|ios|android
bash tools/scripts/run-client.sh
```

## End-to-End Live Test (Invite Creation + Signup)

Use this when you want to verify the full realistic flow.

### Terminal A: backend

```bash
cd /Users/navid/Development/Multi-Platform/arc
bash tools/scripts/infra-up.sh
source tools/.state/infra.env
export ARC_DATABASE_URL="postgres://arc:arc_dev_password@127.0.0.1:${POSTGRES_PORT}/arc?sslmode=disable"
bash tools/scripts/apply-schema.sh
export ARC_PASETO_V4_SECRET_KEY_HEX="$(bash tools/scripts/gen-paseto-key.sh)"
ARC_LOG_FORMAT=auto ARC_AUTH_INVITE_ONLY=false bash tools/scripts/run-server.sh
```

### Terminal B: first client session (bootstrap account)

```bash
cd /Users/navid/Development/Multi-Platform/arc
ARC_AUTH_INVITE_ONLY=false bash tools/scripts/run-client.sh
```

1. Sign up a first account (invite not required in bootstrap mode).
2. Log in and open **Authenticated** panel.
3. In **Invite Generator**, create an invite token and copy it.

### Terminal C: second client session (invite-only validation)

```bash
cd /Users/navid/Development/Multi-Platform/arc
ARC_AUTH_INVITE_ONLY=true bash tools/scripts/run-client.sh
```

1. Paste invite token in Invite screen.
2. Complete signup with a second account.
3. Verify authenticated state and session details.

### Optional stricter pass

Restart backend with invite-only enforced:

```bash
ARC_LOG_FORMAT=auto ARC_AUTH_INVITE_ONLY=true bash tools/scripts/run-server.sh
```

## Why Your Previous Run Failed

- `device_args[@]: unbound variable` came from Bash 3.2 + `set -u` array expansion. This is fixed.
- If backend is in `memory` mode, auth HTTP routes return `db_unavailable`. For real auth flow, run backend in Postgres mode using steps above.
- If running on desktop/mobile with `ARC_AUTH_WEB_COOKIE_MODE=true`, client now automatically falls back to token transport (cookie mode is web-only).
- macOS sandbox now includes `com.apple.security.network.client`, so desktop builds can call backend APIs.

## Run Client Script

`tools/scripts/run-client.sh` supports:
- `ARC_FLUTTER_DEVICE` target device id/name.
- `ARC_CLIENT_SKIP_PUB_GET=true` to skip `flutter pub get` (useful if pub.dev network/auth is blocked but `.dart_tool/package_config.json` already exists).
- `ARC_CLIENT_FORCE_PUB_GET=true` to force `flutter pub get` even when `.dart_tool/package_config.json` already exists.
- The script now passes `flutter run --no-pub` whenever local package config is available, so cached dependencies are reused and pub auth/network failures do not block launch.

Examples:

```bash
ARC_FLUTTER_DEVICE=chrome bash tools/scripts/run-client.sh
```

```bash
ARC_CLIENT_SKIP_PUB_GET=true ARC_FLUTTER_DEVICE=macos bash tools/scripts/run-client.sh
```

## Manual Flutter Run

```bash
cd client/flutter
flutter pub get
flutter run \
  --dart-define=ARC_API_BASE_URL=http://127.0.0.1:8080 \
  --dart-define=ARC_AUTH_INVITE_ONLY=true \
  --dart-define=ARC_AUTH_WEB_COOKIE_MODE=false \
  --dart-define=ARC_AUTH_CSRF_COOKIE_NAME=arc_csrf_token \
  --dart-define=ARC_AUTH_CSRF_HEADER_NAME=X-CSRF-Token
```

## Runtime Config

Dart defines:
- `ARC_API_BASE_URL` default: `http://127.0.0.1:8080`
- `ARC_AUTH_INVITE_ONLY` default: `true`
- `ARC_AUTH_WEB_COOKIE_MODE` default: `true`
- `ARC_AUTH_CSRF_COOKIE_NAME` default: `arc_csrf_token`
- `ARC_AUTH_CSRF_HEADER_NAME` default: `X-CSRF-Token`

Notes:
- `ARC_AUTH_WEB_COOKIE_MODE` is only effective on web.
- On desktop/mobile, refresh token transport uses secure local storage.

## Validation

From repo root:

```bash
bash tools/scripts/test.sh
```

Or Flutter-only:

```bash
cd client/flutter
flutter analyze
flutter test
```
