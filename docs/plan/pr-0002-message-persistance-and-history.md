# PR-002 — Message Persistence & History (Authoritative State)

## Objective

This PR introduces **durable message persistence and history retrieval** to Arc,
building directly on the verified realtime core delivered in **PR-001**.

The goal is to transform Arc from a *purely ephemeral realtime loop* into a
**server-authoritative messaging system with durable state**, while preserving:

* Realtime correctness
* Ordering guarantees
* Explicit contracts
* Architectural clarity

This PR does **not** expand product scope.
It deepens correctness.

---

## Architectural Intent

PR-002 validates the following assumptions:

* Realtime delivery and persistence must be **atomically consistent**
* The server is the **single source of truth**
* Ordering (`seq`) is immutable once assigned
* History is a *projection of authoritative state*, not a reconstruction
* Persistence must not leak into the realtime transport layer

If persistence introduces ambiguity, hidden coupling, or race conditions,
architecture must be revised before continuing.

---

## Scope Definition

### Included (Must Be Implemented)

#### Persistence Layer

* PostgreSQL as the system of record
* Append-only message storage
* Durable conversation message history
* Transactional guarantees for message write + broadcast

#### Message Model (Alpha)

* Message types:
  * `text`
  * `system`
* Immutable messages (no edit, no delete)
* Explicit schema matching realtime payloads

#### History Retrieval

* Fetch message history by conversation
* Cursor-based pagination (by `seq`)
* Deterministic ordering
* Explicit limits

#### Realtime ↔ Persistence Integration

* `message.send` results in:
  1. Validation
  2. Persistent write
  3. `seq` assignment
  4. Realtime broadcast (`message.new`)
* No broadcast without successful persistence

#### Protocol Extensions (v1)

* `conversation.history.fetch`
* `conversation.history.chunk`

#### Quality Gates

* Zero data races
* No partial writes
* No message duplication
* CI and local gates fully passing
* Full documentation of schema and flows

---

## Explicitly Out of Scope (Forbidden)

The following are **not allowed** in this PR:

* Message editing or deletion
* Soft deletes
* Reactions
* Attachments or media
* Search
* Read receipts beyond existing protocol
* Presence or typing indicators
* Redis fan-out
* Caching layers
* Authentication hardening
* Performance optimizations

If it feels useful but not listed → future PR.

---

## Data Model (Authoritative)

### conversations

* `id`
* `kind` (`direct`)
* `created_at`

### conversation_members

* `conversation_id`
* `member_id`
* `joined_at`

### messages

* `id` (server_msg_id)
* `conversation_id`
* `sender_id`
* `client_msg_id`
* `seq`
* `type`
* `content`
* `created_at`

#### Invariants

* `(conversation_id, seq)` is unique
* `(conversation_id, client_msg_id)` is idempotent
* Messages are append-only
* `seq` is strictly monotonic per conversation

---

## Protocol Additions (v1)

### `conversation.history.fetch`

Client → Server

Payload:

* `conversation_id`
* `after_seq` (optional)
* `limit`

### `conversation.history.chunk`

Server → Client

Payload:

* `conversation_id`
* `messages[]`
* `has_more`

All messages **must** conform to existing `message.new` schema.

---

## Server Responsibilities

**server/go/internal/**

* Transactional message write
* Server-side `seq` allocation
* Idempotency enforcement
* History query execution
* Mapping DB rows → protocol payloads

Realtime code **must not** know how persistence works.

---

## Client Responsibilities (Flutter – Dev Level)

* Request history on conversation join
* Render history strictly ordered by `seq`
* Merge realtime messages without reordering
* Handle pagination deterministically

No UI polish. No animation work.

---

## Execution Order (Mandatory)

1. Define DB schema and migrations
2. Implement persistence abstractions
3. Enforce idempotent message writes
4. Integrate persistence into realtime pipeline
5. Implement history fetch protocol events
6. Implement server-side pagination
7. Implement Flutter history loading
8. Validate reconnect + history correctness
9. Verify CI and documentation

Steps may **not** be reordered.

---

## Definition of Done

This PR is complete **only if**:

* Messages survive server restart
* History fetch returns correct ordered messages
* Realtime messages are persisted before broadcast
* Duplicate `client_msg_id` never creates duplicates
* No message is lost or reordered
* Two clients see identical history
* CI passes without warnings
* Schema and flows are documented

Partial completion is not acceptable.

---

## Resulting State After Merge

After PR-002:

* Arc has durable message history
* Server state is authoritative
* Realtime and persistence are coherently integrated
* The system is ready for real authentication (PR-003)

---

## Follow-Up PRs (Not Part of This PR)

* PR-003: Authentication & session model
* PR-004: Presence and typing indicators
* PR-005: Group conversations (Arcset)

---

## Guiding Rule

If a change:

* Adds convenience over correctness
* Blurs realtime vs persistence boundaries
* Introduces hidden coupling
* Feels “clever”

It does **not** belong in PR-002.
