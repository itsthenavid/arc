## PR-010 — Conversation Access Control v1 (Public/Private Rooms + Membership)

### Objective
Implement room privacy and membership rules consistent with ADR:
- Public rooms exist
- Private rooms require membership

### Scope
**DB**
- `conversations` table:
  - id
  - kind (`direct`, `group`, `room`)
  - visibility (`public`, `private`)
  - created_at
- `conversation_members` table:
  - conversation_id
  - user_id
  - joined_at
  - unique (conversation_id, user_id)

**Go**
- Membership service:
  - `EnsureMember(user, conversationID)`
  - `AddMember` (for private rooms)
- Realtime enforcement:
  - `Join` checks:
    - if public: allow
    - if private: allow only if member
  - `MessageSend/HistoryFetch` require membership

### Non-Goals
- Roles inside rooms.
- Moderation actions.

### Testing / Gates
- Integration tests for:
  - public join allowed
  - private join denied unless member
  - send denied for non-members
- Performance checks: indexes validated.
