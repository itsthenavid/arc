## PR-003 — Identity Data Model + Schema (Authoritative User & Profile)

### Objective
Introduce the canonical **User identity model** and optional **Profile** fields,
with strict constraints to support invite-only signup now and public signup later.

### Scope
**DB (infra/db/atlas/schema.sql)**
- Create `users` table:
  - `id` (UUID or ULID; choose one and standardize)
  - `email` (nullable, normalized; **unique where not null**)
  - `username` (nullable; **unique where not null**)
  - `password_hash` (NOT NULL)
  - `created_at`, `updated_at`
  - Optional: `email_verified_at` (nullable; future email verification)
- Create `profiles` table or embed fields in `users` (choose per ADR):
  - `user_id` PK/FK
  - `display_name` (nullable)
  - `bio` (nullable; length limit)
  - `avatar_ref` (nullable)
- Add indexes:
  - unique index on lower(email) (partial)
  - unique index on lower(username) (partial)
  - created_at index for admin/debug queries (optional)

**Go (server/go)**
- `cmd/internal/app/db.go`: migrate schema application path for CI/local (already present)
- `cmd/internal/identity/` (new package):
  - Domain types: `UserID`, `User`, `Profile`
  - Normalization helpers: email/username canonicalization

**Shared (shared/contracts)**
- Add identity contract types (DTO only; no transport coupling):
  - `UserPublic` (id, username, display_name, avatar_ref)
  - `Me` (id, username, email?, profile fields)
  - Error envelope codes (if shared across HTTP/WS)

### Non-Goals
- No signup/login endpoints.
- No session tokens.
- No invite system yet.

### Testing / Gates
- Integration test: schema applies cleanly on fresh DB.
- Unit tests: normalization, validation rules.
- CI: schema apply + go test.
