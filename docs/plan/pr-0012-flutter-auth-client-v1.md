## PR-012 — Flutter Auth Client v1 (Invite → Signup → Profile, Multi-Session)

### Objective
Ship a platform-consistent Flutter auth flow aligned with server semantics:
invite entry → signup → profile → authenticated realtime.

### Scope
- Auth repository:
  - store refresh/access
  - refresh orchestration
- Platform storage:
  - secure storage on mobile/desktop
  - web storage strategy per ADR (cookie-based refresh)
- Screens:
  - Invite entry
  - Signup (username + password; email optional)
  - Profile completion (display name, username optional, bio)
  - Login
- Networking:
  - interceptors to attach access token
  - auto-refresh on 401

### Non-Goals
- Full chat UI/UX.
- Settings and session management UI (can be minimal v1).

### Testing / Gates
- Unit tests for repository + token refresh.
- Widget tests for main auth states.
