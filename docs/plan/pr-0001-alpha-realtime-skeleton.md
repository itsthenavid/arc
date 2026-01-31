# PR-001 — Alpha Realtime Skeleton (Foundational Execution)

## Objective

This PR delivers the **first executable proof** of Arc’s architecture by implementing
the minimal but complete **realtime messaging loop** defined by:

- ADR-0001 (Monorepo)
- ADR-0002 (Realtime Transport)
- Arc Realtime Protocol v1
- Alpha Specification
- Architecture and Manifesto principles

This PR exists to **validate architectural correctness**, not to deliver product features.

---

## Architectural Intent

This PR validates the following non-negotiable assumptions:

- Realtime messaging is the architectural core of Arc
- Contracts are explicit, versioned, and enforced
- Ordering and correctness are server-authoritative
- Execution follows documentation, not intuition
- Scope is intentionally constrained to avoid premature complexity

If these assumptions fail, future work must stop and architecture must be revised.

---

## Scope Definition

### Included (Must Be Implemented)

#### Transport Layer
- WebSocket endpoint: `GET /ws`
- Single connection per client session
- Explicit lifecycle management (connect / disconnect)
- Heartbeat or ping mechanism to maintain liveness

#### Handshake & Session
- `hello`
- `hello.ack`
- Dev-only anonymous session identity
- One session bound to one connection

#### Conversation Model (Alpha)
- Conversation-as-container abstraction
- Direct (1-to-1) conversations only
- In-memory membership and state
- No persistence of any kind

#### Messaging Flow
- `conversation.join`
- `message.send`
- `message.ack`
- `message.new`
- `error`

#### Ordering & Correctness
- Client-generated `client_msg_id`
- Server-generated `server_msg_id`
- Server-assigned monotonic `seq` per conversation
- Strict idempotency guarantees
- Ordering defined exclusively by `seq`

#### Shared Contracts
- Protocol version: v1
- JSON payloads only
- Explicit schemas for every event
- Zero undocumented behavior

#### Client (Flutter – Dev Only)
- Minimal development UI
- WebSocket connect / disconnect
- Join direct conversation
- Send plain text message
- Render messages strictly ordered by `seq`

#### Quality Gates
- No panics in server runtime
- Structured logging for realtime flow
- All CI and local quality gates passing
- No commented-out or placeholder logic

---

## Explicitly Out of Scope (Forbidden)

The following are **not allowed** in this PR under any circumstance:

- Group conversations (Arcset)
- Persistence (PostgreSQL)
- Redis or fan-out mechanisms
- Real authentication or authorization
- Presence indicators
- Typing indicators
- Media messages
- Message editing or deletion
- UI polish or animation
- Performance optimization
- Load testing
- End-to-end encryption

If a change feels useful but is not listed in scope, it belongs in a future PR.

---

## Execution Entry Points

### Server

server/go/internal/realtime/

Responsibilities:
- WebSocket gateway
- Event routing
- Conversation state
- Ordering and idempotency

### Shared

shared/contracts/realtime/v1/

Responsibilities:
- Envelope definition
- Event schemas
- Version enforcement

### Client

client/flutter/lib/realtime/

Responsibilities:
- WebSocket client abstraction
- Minimal state handling
- Message rendering

No other directories should be modified unless strictly required.

---

## Execution Order (Mandatory)

1. Define and lock shared realtime contracts (v1)
2. Implement WebSocket server endpoint
3. Implement handshake lifecycle
4. Implement conversation join logic
5. Implement message send → ack → broadcast pipeline
6. Enforce ordering and idempotency
7. Implement minimal Flutter client
8. Validate end-to-end realtime flow
9. Verify CI and local quality gates

Steps must be followed in order.

---

## Contract Enforcement

This PR implements **only** the following protocol events:

- `hello`
- `hello.ack`
- `conversation.join`
- `message.send`
- `message.ack`
- `message.new`
- `error`

All payloads MUST conform to:

docs/spec/realtime-v1.md

Any deviation requires an explicit spec change **before** code changes.

---

## Definition of Done

This PR is complete **only if all conditions below are met**:

- Two clients can connect via WebSocket
- Handshake completes successfully
- Both clients join the same direct conversation
- One client sends a message
- Sender receives `message.ack`
- Both clients receive `message.new`
- Message order is identical across clients
- Duplicate `client_msg_id` does not create duplicates
- Server remains stable under normal use
- CI passes without warnings
- All behavior is documented

Partial completion is not acceptable.

---

## Explicit Non-Goals

This PR does not aim to:

- Be production-ready
- Support multi-device users
- Handle advanced failure recovery
- Scale horizontally
- Deliver user-facing completeness

Those concerns are intentionally deferred.

---

## Resulting State After Merge

After this PR:

- Arc has a verified realtime messaging core
- The most complex architectural risk is resolved
- Contracts are enforced in code
- The project is ready for persistence (PR-002)
- Future work can proceed with confidence

---

## Follow-Up PRs (Not Part of This PR)

- PR-002: Message persistence and history
- PR-003: Authentication and session model
- PR-004: Presence and typing
- PR-005: Group conversations (Arcset)

---

## Guiding Rule

If a change:
- Increases scope
- Reduces clarity
- Bypasses documentation
- Feels “nice to have”

It does **not** belong in PR-001.
