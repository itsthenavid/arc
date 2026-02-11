## PR-006 — Invite System v1 (Invite-Only Onboarding + Future Public Signup)

### Objective
Ship invite-only onboarding as the default, while keeping code structure ready
to enable public signup later without refactor.

### Scope
**DB**
- `invites` table:
  - `id`
  - `token_hash` (NOT NULL, unique)
  - `created_at`, `expires_at`
  - `max_uses`, `used_count`
  - `revoked_at`
  - optional `created_by_user_id`
  - optional `note`
- Atomic consumption:
  - `used_count` increments safely
  - Expired/revoked checks

**Go**
- Package: `cmd/internal/invite`
  - `CreateInvite(maxUses, ttl, createdBy?) -> inviteToken`
  - `ValidateInvite(token) -> ok`
  - `ConsumeInvite(token) -> ok`
- Token design:
  - High entropy random token
  - Store only hash
  - Constant-time compare

### Non-Goals
- No email sending or captcha integration (only stubs/interfaces).
- No public signup switch exposed.

### Testing / Gates
- Concurrency tests: multiple consumers can’t exceed max_uses.
- Expired/revoked behavior tests.
