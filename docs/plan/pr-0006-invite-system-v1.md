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
  - optional `created_by` (user id)
  - optional `note`
- Atomic consumption:
  - `used_count` increments safely
  - Expired/revoked/max-uses checks

**Go**
- Package: `cmd/internal/invite`
  - `CreateInvite(ctx, input) -> (invite, token)`
  - `ValidateInvite(ctx, token, now) -> (ok, invite)`
  - `ConsumeInvite(ctx, token, consumedBy, now) -> invite`
- Token design:
  - High entropy random token
  - Store only hash (HMAC-SHA256 when configured)
  - Database lookup by hash (no raw token storage)

**Config**
- `ARC_AUTH_INVITE_MAX_USES` (default 1)
- `ARC_AUTH_INVITE_MAX_USES_MAX` (upper bound for requested max uses; default 50)
- `note` length max: 512 chars (enforced in API + DB)

### Non-Goals
- No email sending or captcha integration (only stubs/interfaces).
- No public signup switch exposed.

### Testing / Gates
- Concurrency tests: multiple consumers can’t exceed max_uses.
- Expired/revoked behavior tests.
- Validation tests cover max_uses, revoke, and expiry.
