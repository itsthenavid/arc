## PR-011 — Email Verification & CAPTCHA Interfaces (Future-Ready Stubs)

### Objective
Introduce clean interfaces and DB fields so that email verification and captcha
can be added later without refactor, while remaining disabled now.

### Scope
- Interfaces:
  - `EmailSender` (noop implementation)
  - `CaptchaVerifier` (noop implementation)
- DB fields (if not added):
  - `email_verified_at`
  - verification tokens table (optional)
- Feature flags:
  - `ARC_AUTH_REQUIRE_EMAIL_VERIFIED` (default false)
  - `ARC_AUTH_ENABLE_CAPTCHA` (default false)

### Non-Goals
- Actually sending emails.
- Real captcha provider integration.

### Testing / Gates
- Unit tests verifying feature flags enforce behavior when enabled,
  using test doubles.
