## PR-004 — Credential Security Primitives (Argon2id + Password Policy)

### Objective
Provide the strongest practical password hashing and policy for a messaging app,
with **configurable Argon2id** and stable interfaces.

### Scope
**Go**
- Package: `cmd/security/password`
  - `Hash(password) -> hash`
  - `Verify(hash, password) -> bool`
  - Parameter config via env:
    - memory (KiB), iterations, parallelism, salt length, key length
- Password policy:
  - Minimum length (e.g., 10–12)
  - Max length (prevent DoS; e.g., 256)
  - Reject extremely weak patterns? (optional; prefer minimal policy)
- Constant-time comparisons
- Clear error taxonomy:
  - `ErrPasswordTooShort`, `ErrPasswordTooLong`, `ErrWeakPassword` (if used)

### Non-Goals
- No rate limiting yet.
- No login endpoints.

### Testing / Gates
- Unit tests for Hash/Verify correctness and parameter parsing.
- Micro-benchmark (non-blocking) to ensure parameters aren’t pathological.
