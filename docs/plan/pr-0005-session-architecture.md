## PR-005 — Session Architecture v1 (Multi-Device + Refresh Rotation)

### Objective
Implement Arc’s **multi-device session model** with refresh-token rotation,
reuse detection, per-device revocation, and configurable TTL policy
(web shorter; native longer; remember-me supported).

### Scope
**DB**
- `sessions` table:
  - `id` (UUID/ULID)
  - `user_id` FK
  - `refresh_token_hash` (NOT NULL, unique)
  - `created_at`, `last_used_at`, `expires_at`
  - `revoked_at` (nullable)
  - `replaced_by_session_id` (nullable; rotation chain / reuse detection)
  - `user_agent`, `ip` (nullable; audit-grade)
  - `platform` enum/text (`web`, `ios`, `android`, `desktop`, `unknown`)
- Indexes:
  - (user_id, revoked_at, expires_at)
  - expires_at cleanup scans

**Go**
- Package: `cmd/internal/auth/session`
  - `IssueSession(user, deviceCtx) -> {accessToken, refreshToken, sessionID}`
  - `RotateRefresh(refreshToken) -> new tokens`
  - `RevokeSession(sessionID)`
  - `RevokeAll(userID)`
  - `ValidateAccessToken(token) -> claims`
- Token formats (per ADR):
  - Access token: JWT (short TTL) OR PASETO (preferred if ADR chose it)
  - Refresh token: opaque random string, stored **hashed**
- Rotation rules:
  - Every refresh rotates and invalidates previous
  - Reuse detection:
    - If a revoked/replaced refresh is presented -> revoke entire chain (or all sessions) per ADR

**Env Config**
- Access TTL: short (minutes)
- Refresh TTL:
  - Web: 7 days (hard)
  - Native: longer (e.g., 30–90 days) + “remember me”
- Clock skew tolerance

### Non-Goals
- No HTTP endpoints yet.
- No WS binding yet.

### Testing / Gates
- Unit tests: rotation, reuse detection, TTL handling.
- Integration tests: session lifecycle with Postgres.
- CI: schema apply + go test.
