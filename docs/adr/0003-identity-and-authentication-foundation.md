# ADR-0003 — Identity & Authentication Foundation

## Status
Accepted

## Date
2026-02-04

## Context

Arc is a realtime-first messaging system designed with a **server-authoritative core**, a strict protocol boundary, and a long-lived architecture intended to scale in correctness before scale in users.

Following ADR-0001 (Monorepo Foundation) and the completion of PR-001 (Realtime Skeleton) and PR-002 (Message Persistence & History), the next foundational concern is **Identity & Authentication**.

Identity in Arc is not an auxiliary feature. It is a **security boundary** that directly affects:

- Realtime WebSocket authorization
- Message ownership and ordering
- Conversation membership
- Abuse prevention, rate limiting, and auditability
- Multi-device and multi-session correctness

This ADR defines the **foundational architecture** for identity, authentication, session management, and authorization in Arc. It intentionally avoids product-specific features (roles, admin panels, monetization, SSO) and focuses only on what must be correct, secure, and extensible at the foundation level.

---

## Decision

### 1. Identity Model

- The canonical security principal in Arc is **User**.
- Each user represents a complete and independent identity.
- All conversations (1-to-1 or group) are treated uniformly as **rooms**; identity semantics do not differ by room type.

#### Identifiers
- User identifiers use **ULID**.
  - Globally unique
  - Lexicographically sortable
  - Suitable for distributed systems and timeline-based queries
- Usernames are **optional**.
- Emails are **optional**, but the data model and security hooks must fully support email-based identity.

#### Login Identifiers
- If a user has a username:
  - Login is allowed via `username + password` **or** `email + password`.
- If a user has no username:
  - Login is allowed via `email + password` only.
- Username and email comparisons are **case-insensitive**.

---

### 2. Signup & Invite Policy

- Arc operates in **invite-only mode by default**.
- Open signup is not enabled but **must be fully supported by infrastructure** and activatable via configuration.
- Invite links:
  - Are **single-use**
  - Have a configurable expiry (default: 7 days)
  - Create a constrained signup context

#### Signup Flow (Invite-Based)
1. User consumes an invite link.
2. Signup form requests:
   - Username (optional)
   - Password (required)
3. Profile completion step:
   - Display name (optional)
   - Username confirmation or edit
   - Bio (optional)
4. A user identity and initial session are created atomically.

Email verification, captcha, and abuse-prevention mechanisms are **explicitly supported at the architectural level** but are disabled in this phase.

---

### 3. Authentication & Session Strategy

Arc uses a **hybrid token model** combining the strengths of JWTs and opaque sessions.

#### Tokens
- **Access Token**
  - Format: JWT
  - Lifetime: 15 minutes
  - Purpose: authorization and identity propagation (HTTP + WebSocket)
- **Refresh Token**
  - Format: Opaque, cryptographically random
  - Stored **hashed** in the database
  - Rotated on every refresh
  - Supports reuse detection

#### Sessions
- Each device/client maintains an independent session.
- Sessions are revocable individually or globally per user.

#### Session Lifetime
- Web:
  - Refresh token max lifetime: 7 days
- Native/Desktop:
  - With “Remember Me”: 60 days
  - Without: 14 days

#### Logout
- Logout of current session
- Logout of all sessions
- On refresh token reuse detection:
  - **All sessions for the user are revoked**

---

### 4. WebSocket Authentication

- Authentication is performed **during the WebSocket handshake**.
- Access token is provided via:
  - `Authorization: Bearer <access_token>`
- If authentication fails:
  - The connection is rejected **before protocol upgrade** with 401/403.
- Authenticated identity is attached to the WebSocket session context.
- No unauthenticated WebSocket connection may join a conversation.

---

### 5. Authorization Model

- All access is **membership-based**.
- Conversation types:
  - Public rooms
  - Private rooms
- Joining any room requires:
  - A valid authenticated user
  - Membership authorization
- Membership and access rules are **authoritative in the database**.
- No implicit trust is placed in client-side claims.

---

### 6. Security Requirements

Arc’s identity system must meet a **high-security standard appropriate for a production-grade messaging system**, without premature overengineering.

#### Passwords
- Hashed using **Argon2id**
- Enforced minimum length and entropy consistent with modern messaging platforms

#### Protections
- Login rate limiting
- Progressive lockout on repeated failures
- Moderate audit logging for security-relevant events
- Strict CORS and Origin validation
- CSRF protection:
  - Required for cookie-based auth (optional mode)
  - Header-based token auth remains CSRF-safe by design

Tokens are **never logged**. User identifiers may appear in logs with care.

---

### 7. API Surface

The authentication system is shared across HTTP and WebSocket layers.

Core endpoints:
- `POST /auth/login`
- `POST /auth/logout`
- `POST /auth/logout_all`
- `POST /auth/refresh`
- `GET /me`
- `POST /auth/invites/create`
- `POST /auth/invites/consume`

Public registration endpoints exist in code but are disabled by configuration.

---

### 8. Data Model

Foundational tables include:
- users
- sessions
- invites
- conversation_members
- audit_log

Soft deletes are **not used**. Security and correctness take precedence.

---

## Consequences

### Positive
- Strong, explicit security boundary for realtime messaging
- Proper multi-device and multi-session support
- Immediate compatibility with WebSocket authorization
- Clear upgrade path to email verification, captcha, admin tools, and SSO
- Architecture suitable for both portfolio-grade review and real-world use

### Trade-offs
- Increased implementation complexity compared to JWT-only systems
- Requires careful testing around session rotation and revocation
- Slightly higher database dependency for session validation

These trade-offs are intentional and aligned with Arc’s correctness-first philosophy.

---

## Non-Goals

- Role-based access control (RBAC)
- Admin panels or moderation tools
- OAuth / SSO providers
- Monetization or billing identity
- Federation or cross-instance identity

---

## References

- ADR-0001 — Monorepo Foundation
- PR-001 — Alpha Realtime Skeleton
- PR-002 — Message Persistence & History
- Arc Realtime Protocol v1
- OWASP Authentication Best Practices
- Telegram & Signal session lifecycle models
