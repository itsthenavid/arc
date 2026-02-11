## PR-008 — Auth Rate Limiting + Lockout + Audit (Defensive Controls)

### Objective
Add “big product” defensive controls: rate limiting, progressive lockout,
and moderate audit logging.

### Scope
**Go**
- Rate limiting:
  - per IP + per identifier (email/username) for login
  - per session for refresh
- Lockout:
  - progressive backoff (or temporary lock after N failures)
  - storage: in Postgres (preferred) or Redis later; choose per ADR
- Audit events:
  - login success/failure
  - session issued/rotated/revoked
  - invite used/revoked
  - suspicious refresh reuse detection

**DB**
- `auth_failures` (if persisted) OR `audit_log` (minimal):
  - id, type, user_id?, ip, ua, created_at, metadata jsonb

### Non-Goals
- No admin UI.
- No SIEM integrations.

### Testing / Gates
- Tests verifying lockout triggers and clears.
- Tests verifying rate limit behavior.
- Ensure logs do not leak secrets (tokens/passwords).
