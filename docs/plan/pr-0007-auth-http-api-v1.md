## PR-007 — Auth HTTP API v1 (Invite Signup + Login + Refresh + Logout)

### Objective
Expose a stable **HTTP auth surface** for Flutter (and future web),
with correct cookie/CSRF behavior for web and token headers for native.

### Scope
**HTTP Routes (server/go/cmd/internal/app/http.go wiring)**
- `POST /auth/invites/create` (auth required; returns invite token)
- `POST /auth/invites/consume` (invite + username/password; email optional)
- `POST /auth/login` (username OR email + password)
- `POST /auth/refresh`
- `POST /auth/logout` (current session)
- `POST /auth/logout_all`
- `GET  /me`

**Request/Response Contracts (shared/contracts/auth/v1 OR server-local if ADR chose)**
- Invite create:
  - input: `expires_in_seconds?`, `max_uses?`, `note?` (note max 512 chars)
  - output: invite_id + invite_token + expires_at
- Invite consume (signup):
  - input: invite_token (required if invite-only), username?, email?, password
  - output: me + tokens (or cookie set)
- Login:
  - input: identifier (email/username), password, remember_me?, device info
- Refresh:
  - input: refresh token (cookie or body, per platform)
- Logout:
  - input: none; uses auth context

**Platform Semantics**
- Web:
  - refresh token in **HttpOnly Secure SameSite** cookie
  - CSRF protection for state-changing endpoints:
    - double-submit cookie or CSRF token header
- Native/Desktop:
  - refresh token returned to client and stored in secure storage
  - access token via Authorization header

**Hardening**
- Strict CORS allowlist (env-configured)
- Standard security headers baseline
- Uniform error responses (no user enumeration)
  - login failure: same response for wrong user vs wrong password

### Non-Goals
- No UI/Flutter changes (server-only).
- No WS auth yet.

### Testing / Gates
- Integration tests:
  - signup with invite
  - login success/failure (no enumeration)
  - refresh rotation correctness
  - logout single session + logout all
  - cookie + CSRF flow (web mode)
- CI: go test + postgres integration tests.
