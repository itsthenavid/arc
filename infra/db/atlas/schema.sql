-- Arc schema is fully schema-qualified (arc.*). Do not rely on search_path.

CREATE SCHEMA IF NOT EXISTS arc;

CREATE SCHEMA IF NOT EXISTS public;

COMMENT ON SCHEMA public IS 'standard public schema';

-- =========================
-- Helpers
-- =========================

-- Standard updated_at trigger.
CREATE OR REPLACE FUNCTION arc.set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- =========================
-- Realtime core (PR-001/002)
-- =========================

CREATE TABLE IF NOT EXISTS arc.conversations (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('direct', 'group')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_conversations_id_nonempty CHECK (char_length(id) > 0)
);

-- next_seq is the next allocatable sequence number (starts at 1).
CREATE TABLE IF NOT EXISTS arc.conversation_cursors (
    conversation_id TEXT PRIMARY KEY REFERENCES arc.conversations (id) ON DELETE CASCADE,
    next_seq BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_conversation_cursors_next_seq_positive CHECK (next_seq >= 1)
);

DROP TRIGGER IF EXISTS trg_conversation_cursors_updated_at ON arc.conversation_cursors;

CREATE TRIGGER trg_conversation_cursors_updated_at
BEFORE UPDATE ON arc.conversation_cursors
FOR EACH ROW
EXECUTE FUNCTION arc.set_updated_at();

-- =========================
-- Identity & Auth Foundation (PR-003)
-- ADR-0003 aligned + PR-005-ready
-- =========================


CREATE TABLE IF NOT EXISTS arc.users (
  id TEXT PRIMARY KEY,

  username TEXT NULL,
  username_norm TEXT NULL,

  email TEXT NULL,
  email_norm TEXT NULL,

  display_name TEXT NULL,
  bio TEXT NULL,

  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

  CONSTRAINT chk_users_id_ulid_len CHECK (char_length(id) = 26),

-- Keep raw and normalized columns consistent: either both NULL or both non-NULL.
CONSTRAINT chk_users_username_pair CHECK (
    (username IS NULL) = (username_norm IS NULL)
),
CONSTRAINT chk_users_email_pair CHECK (
    (email IS NULL) = (email_norm IS NULL)
),

-- Prevent empty strings (if present).
CONSTRAINT chk_users_username_nonempty CHECK (username IS NULL OR char_length(btrim(username)) > 0),
  CONSTRAINT chk_users_username_norm_nonempty CHECK (username_norm IS NULL OR char_length(btrim(username_norm)) > 0),
  CONSTRAINT chk_users_email_nonempty CHECK (email IS NULL OR char_length(btrim(email)) > 0),
  CONSTRAINT chk_users_email_norm_nonempty CHECK (email_norm IS NULL OR char_length(btrim(email_norm)) > 0),

  CONSTRAINT chk_users_username_len CHECK (username IS NULL OR (char_length(username) >= 3 AND char_length(username) <= 32)),
  CONSTRAINT chk_users_username_norm_len CHECK (username_norm IS NULL OR (char_length(username_norm) >= 3 AND char_length(username_norm) <= 32)),

  CONSTRAINT chk_users_email_len CHECK (email IS NULL OR (char_length(email) >= 3 AND char_length(email) <= 320)),
  CONSTRAINT chk_users_email_norm_len CHECK (email_norm IS NULL OR (char_length(email_norm) >= 3 AND char_length(email_norm) <= 320)),

  CONSTRAINT chk_users_display_name_len CHECK (display_name IS NULL OR char_length(display_name) <= 80),
  CONSTRAINT chk_users_bio_len CHECK (bio IS NULL OR char_length(bio) <= 512)
);

DROP TRIGGER IF EXISTS trg_users_updated_at ON arc.users;

CREATE TRIGGER trg_users_updated_at
BEFORE UPDATE ON arc.users
FOR EACH ROW
EXECUTE FUNCTION arc.set_updated_at();

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_username_norm ON arc.users (username_norm);

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_email_norm ON arc.users (email_norm);

CREATE INDEX IF NOT EXISTS idx_users_created_at ON arc.users (created_at DESC);

-- One credentials row per user.
CREATE TABLE IF NOT EXISTS arc.user_credentials (
    user_id TEXT PRIMARY KEY REFERENCES arc.users (id) ON DELETE CASCADE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_user_credentials_user_id_ulid_len CHECK (char_length(user_id) = 26),
    CONSTRAINT chk_user_credentials_hash_len CHECK (
        char_length(password_hash) >= 20
        AND char_length(password_hash) <= 1024
    )
);

CREATE INDEX IF NOT EXISTS idx_user_credentials_user_id ON arc.user_credentials (user_id);

DROP TRIGGER IF EXISTS trg_user_credentials_updated_at ON arc.user_credentials;

CREATE TRIGGER trg_user_credentials_updated_at
BEFORE UPDATE ON arc.user_credentials
FOR EACH ROW
EXECUTE FUNCTION arc.set_updated_at();

-- =========================
-- Sessions (PR-005)
-- =========================

-- Sessions: refresh tokens are opaque and stored hashed (HMAC-SHA256 or SHA-256 hex => 64 chars).
-- PR-005: rotation chain + platform + DB-level invariants for correctness and safety.
CREATE TABLE IF NOT EXISTS arc.sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES arc.users (id) ON DELETE CASCADE,

-- HMAC-SHA256 or SHA-256 hex of opaque refresh token (64 chars).
refresh_token_hash TEXT NOT NULL,
created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
last_used_at TIMESTAMPTZ NULL,
expires_at TIMESTAMPTZ NOT NULL,
revoked_at TIMESTAMPTZ NULL,

-- Rotation chain: when refresh is rotated, old session points to its replacement.
replaced_by_session_id TEXT NULL REFERENCES arc.sessions (id) ON DELETE SET NULL,
user_agent TEXT NULL,
ip INET NULL,

-- Device/platform context.
platform TEXT NOT NULL DEFAULT 'unknown',

-- Optional: why a session was revoked (observability without changing semantics).
-- Keep nullable and conservative; do not depend on this for core logic.
revocation_reason TEXT NULL,
CONSTRAINT chk_sessions_id_ulid_len CHECK (char_length(id) = 26),
CONSTRAINT chk_sessions_user_id_ulid_len CHECK (char_length(user_id) = 26),
CONSTRAINT chk_sessions_refresh_hash_len CHECK (
    char_length(refresh_token_hash) = 64
),

-- IMPORTANT: keep strict. For an "expired session", set created_at in the past and
-- expires_at after created_at (but still in the past).
CONSTRAINT chk_sessions_expires_after_created CHECK (expires_at > created_at),
CONSTRAINT chk_sessions_revoked_after_created CHECK (
    revoked_at IS NULL
    OR revoked_at >= created_at
),
CONSTRAINT chk_sessions_last_used_after_created CHECK (
    last_used_at IS NULL
    OR last_used_at >= created_at
),

-- Sanity: last_used_at should not exceed expires_at.
CONSTRAINT chk_sessions_last_used_before_expires CHECK (
    last_used_at IS NULL
    OR last_used_at <= expires_at
),
CONSTRAINT chk_sessions_platform CHECK (
    platform IN (
        'web',
        'ios',
        'android',
        'desktop',
        'unknown'
    )
),

-- Replacement cannot point to self.
CONSTRAINT chk_sessions_replaced_not_self CHECK (
    replaced_by_session_id IS NULL
    OR replaced_by_session_id <> id
),

-- Rotation implies revocation: if replaced_by_session_id is set, revoked_at must be set.
CONSTRAINT chk_sessions_replaced_requires_revoked CHECK (
    replaced_by_session_id IS NULL
    OR revoked_at IS NOT NULL
),

-- Keep user_agent bounded to prevent pathological payload sizes.
CONSTRAINT chk_sessions_user_agent_len CHECK (
    user_agent IS NULL
    OR char_length(user_agent) <= 512
),

-- revocation_reason is optional but, when present, must be one of the known reasons.
CONSTRAINT chk_sessions_revocation_reason CHECK (
    revocation_reason IS NULL OR
    revocation_reason IN ('logout','rotation','reuse_detected','admin','security')
  )
);

-- Uniqueness on refresh token hash guarantees no two sessions share the same refresh token.
CREATE UNIQUE INDEX IF NOT EXISTS uq_sessions_refresh_token_hash ON arc.sessions (refresh_token_hash);

-- Common access patterns.
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON arc.sessions (user_id);

CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON arc.sessions (expires_at);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id_revoked_expires ON arc.sessions (
    user_id,
    revoked_at,
    expires_at
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id_platform ON arc.sessions (user_id, platform);

CREATE INDEX IF NOT EXISTS idx_sessions_replaced_by ON arc.sessions (replaced_by_session_id);

-- Partial index for "active sessions" reads.
CREATE INDEX IF NOT EXISTS idx_sessions_active_by_user ON arc.sessions (user_id, expires_at DESC)
WHERE
    revoked_at IS NULL;

-- Helpful index for rotated sessions (reuse detection and chain inspection).
CREATE INDEX IF NOT EXISTS idx_sessions_rotated ON arc.sessions (user_id, revoked_at DESC)
WHERE
    replaced_by_session_id IS NOT NULL;

-- Enforce replacement-chain invariants:
-- - replacement must exist
-- - replacement must belong to the same user
-- - replacement must not be created before the replaced session
CREATE OR REPLACE FUNCTION arc.sessions_validate_replacement_chain()
RETURNS TRIGGER AS $$
DECLARE
  v_user_id TEXT;
  v_created_at TIMESTAMPTZ;
BEGIN
  IF NEW.replaced_by_session_id IS NULL THEN
    RETURN NEW;
  END IF;

  -- Defensive: should be covered by chk_sessions_replaced_not_self.
  IF NEW.replaced_by_session_id = NEW.id THEN
    RAISE EXCEPTION 'sessions.replaced_by_session_id cannot reference self';
  END IF;

  SELECT s.user_id, s.created_at
    INTO v_user_id, v_created_at
  FROM arc.sessions s
  WHERE s.id = NEW.replaced_by_session_id;

  IF v_user_id IS NULL THEN
    RAISE EXCEPTION 'sessions.replaced_by_session_id references missing session: %', NEW.replaced_by_session_id;
  END IF;

  IF v_user_id <> NEW.user_id THEN
    RAISE EXCEPTION 'sessions.replaced_by_session_id must reference a session of the same user';
  END IF;

  -- Allow equal timestamps (same transaction time) but disallow replacement being earlier.
  IF v_created_at < NEW.created_at THEN
    RAISE EXCEPTION 'replacement session must not be created before the replaced session';
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_sessions_validate_replacement_chain ON arc.sessions;

CREATE TRIGGER trg_sessions_validate_replacement_chain
BEFORE INSERT OR UPDATE OF replaced_by_session_id, user_id, created_at
ON arc.sessions
FOR EACH ROW
EXECUTE FUNCTION arc.sessions_validate_replacement_chain();

-- =========================
-- Messages (PR-001/002, FK to sessions after sessions exist)
-- =========================

CREATE TABLE IF NOT EXISTS arc.messages (
    conversation_id TEXT NOT NULL REFERENCES arc.conversations (id) ON DELETE CASCADE,
    seq BIGINT NOT NULL,
    server_msg_id TEXT NOT NULL,
    client_msg_id TEXT NOT NULL,
    sender_session TEXT NOT NULL,
    text TEXT NOT NULL,
    server_ts TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id, seq),
    CONSTRAINT uq_messages_conversation_client_msg UNIQUE (
        conversation_id,
        client_msg_id
    ),
    CONSTRAINT uq_messages_server_msg_id UNIQUE (server_msg_id),
    CONSTRAINT chk_messages_seq_positive CHECK (seq >= 1),
    CONSTRAINT chk_messages_text_len CHECK (
        char_length(text) > 0
        AND char_length(text) <= 4096
    ),
    CONSTRAINT chk_messages_client_msg_id_nonempty CHECK (
        char_length(client_msg_id) > 0
    ),
    CONSTRAINT chk_messages_server_msg_id_nonempty CHECK (
        char_length(server_msg_id) > 0
    ),
    CONSTRAINT chk_messages_sender_session_nonempty CHECK (
        char_length(sender_session) > 0
    )
);

CREATE INDEX IF NOT EXISTS idx_messages_conversation_seq_asc ON arc.messages (conversation_id, seq ASC);

CREATE INDEX IF NOT EXISTS idx_messages_conversation_seq_desc ON arc.messages (conversation_id, seq DESC);

CREATE INDEX IF NOT EXISTS idx_messages_conversation_client_msg ON arc.messages (
    conversation_id,
    client_msg_id
);

CREATE INDEX IF NOT EXISTS idx_messages_server_msg_id ON arc.messages (server_msg_id);

-- Now that sessions exist, enforce sender_session integrity for messages.
-- Keep column name as-is to avoid breaking Go code; enforce FK on the same column.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'fk_messages_sender_session'
      AND conrelid = 'arc.messages'::regclass
  ) THEN
    ALTER TABLE arc.messages
      ADD CONSTRAINT fk_messages_sender_session
      FOREIGN KEY (sender_session)
      REFERENCES arc.sessions (id)
      ON DELETE RESTRICT;

END IF;

END;

$$;

-- =========================
-- Invites (invite-only by default)
-- =========================

CREATE TABLE IF NOT EXISTS arc.invites (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL,
    created_by TEXT NULL REFERENCES arc.users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    max_uses INT NOT NULL DEFAULT 1,
    used_count INT NOT NULL DEFAULT 0,
    revoked_at TIMESTAMPTZ NULL,
    note TEXT NULL,
    consumed_at TIMESTAMPTZ NULL,
    consumed_by TEXT NULL REFERENCES arc.users (id) ON DELETE SET NULL,
    CONSTRAINT chk_invites_id_ulid_len CHECK (char_length(id) = 26),
    CONSTRAINT chk_invites_token_hash_len CHECK (char_length(token_hash) = 64),
    CONSTRAINT chk_invites_expires_after_created CHECK (expires_at > created_at),
    CONSTRAINT chk_invites_max_uses CHECK (max_uses >= 1),
    CONSTRAINT chk_invites_used_count CHECK (used_count >= 0 AND used_count <= max_uses),
    CONSTRAINT chk_invites_revoked_after_created CHECK (
        revoked_at IS NULL
        OR revoked_at >= created_at
    ),
    CONSTRAINT chk_invites_note_len CHECK (
        note IS NULL
        OR char_length(note) <= 512
    ),
    CONSTRAINT chk_invites_consumed_at_after_created CHECK (
        consumed_at IS NULL
        OR consumed_at >= created_at
    ),
    CONSTRAINT chk_invites_consumed_by_pair CHECK (
        (consumed_at IS NULL) = (consumed_by IS NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_invites_token_hash ON arc.invites (token_hash);

CREATE INDEX IF NOT EXISTS idx_invites_expires_at ON arc.invites (expires_at);

CREATE INDEX IF NOT EXISTS idx_invites_consumed_at ON arc.invites (consumed_at);

CREATE INDEX IF NOT EXISTS idx_invites_revoked_at ON arc.invites (revoked_at);

-- =========================
-- Membership (authoritative)
-- =========================

CREATE TABLE IF NOT EXISTS arc.conversation_members (
    conversation_id TEXT NOT NULL REFERENCES arc.conversations (id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES arc.users (id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id, user_id),
    CONSTRAINT chk_conversation_members_role CHECK (
        role IN ('member', 'owner', 'admin')
    ),
    CONSTRAINT chk_conversation_members_user_id_ulid_len CHECK (char_length(user_id) = 26)
);

CREATE INDEX IF NOT EXISTS idx_conversation_members_user_id ON arc.conversation_members (user_id);

-- =========================
-- Audit log (minimal security audit)
-- =========================

CREATE TABLE IF NOT EXISTS arc.audit_log (
    id BIGSERIAL PRIMARY KEY,
    user_id TEXT NULL REFERENCES arc.users (id) ON DELETE SET NULL,
    session_id TEXT NULL REFERENCES arc.sessions (id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ip INET NULL,
    user_agent TEXT NULL,
    meta JSONB NULL,
    CONSTRAINT chk_audit_action_len CHECK (
        char_length(action) >= 3
        AND char_length(action) <= 120
    ),
    CONSTRAINT chk_audit_user_agent_len CHECK (
        user_agent IS NULL
        OR char_length(user_agent) <= 512
    )
);

CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON arc.audit_log (created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_log_user_id ON arc.audit_log (user_id);

CREATE INDEX IF NOT EXISTS idx_audit_log_session_id ON arc.audit_log (session_id);

CREATE INDEX IF NOT EXISTS idx_audit_log_action_created_at ON arc.audit_log (action, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_log_login_failed_ip_created_at ON arc.audit_log (ip, created_at DESC) WHERE action = 'auth.login.failed'
AND ip IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_audit_log_login_failed_identifier_created_at ON arc.audit_log ((meta ->> 'identifier'), created_at DESC) WHERE action = 'auth.login.failed';
