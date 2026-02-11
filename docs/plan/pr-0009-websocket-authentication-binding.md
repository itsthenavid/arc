## PR-009 — WebSocket Authentication Binding (Identity-Aware Realtime)

### Objective
Bind identity to realtime connections and enforce authenticated actions,
while preserving PR-001/PR-002 realtime correctness.

### Scope
**WS Handshake**
- Accept auth via:
  - Authorization: Bearer <access_token> (preferred)
  - (Optional web fallback) cookie-based access token if ADR chose
- Validate token during handshake; reject unauthorized with 401/403.
- Attach `UserID` to `Client`:
  - extend `Client` struct: `UserID *UserID` (nil for unauthenticated if allowed)
  - or require auth always (recommended)

**Protocol Semantics**
- If auth required:
  - reject any envelope before auth
  - or treat WS connect as authenticated state immediately (preferred)
- Enforce:
  - join requires auth
  - send requires auth
  - history fetch requires auth
- Extend ws-smoke to cover:
  - unauthorized connect fails
  - authorized connect passes and sends/receives

### Non-Goals
- Room ACL/membership rules (next PR).
- Presence, typing, receipts.

### Testing / Gates
- WS integration tests + ws-smoke enhancements.
- Negative tests for invalid/expired tokens.
