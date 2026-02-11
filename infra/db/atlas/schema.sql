-- Arc schema is fully schema-qualified (arc.*). Do not rely on search_path.

CREATE SCHEMA IF NOT EXISTS arc;

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

-- Messages table (FK to sessions is added later, after sessions exist).
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
  CONSTRAINT uq_messages_conversation_client_msg UNIQUE (conversation_id, client_msg_id),
  CONSTRAINT uq_messages_server_msg_id UNIQUE (server_msg_id),
  CONSTRAINT chk_messages_seq_positive CHECK (seq >= 1),
  CONSTRAINT chk_messages_text_len CHECK (char_length(text) > 0 AND char_length(text) <= 4096),
  CONSTRAINT chk_messages_client_msg_id_nonempty CHECK (char_length(client_msg_id) > 0),
  CONSTRAINT chk_messages_server_msg_id_nonempty CHECK (char_length(server_msg_id) > 0),
  CONSTRAINT chk_messages_sender_session_nonempty CHECK (char_length(sender_session) > 0)
);

CREATE INDEX IF NOT EXISTS idx_messages_conversation_seq_asc ON arc.messages (conversation_id, seq ASC);
CREATE INDEX IF NOT EXISTS idx_messages_conversation_seq_desc ON arc.messages (conversation_id, seq DESC);
CREATE INDEX IF NOT EXISTS idx_messages_conversation_client_msg ON arc.messages (conversation_id, client_msg_id);
CREATE INDEX IF NOT EXISTS idx_messages_server_msg_id ON arc.messages (server_msg_id);

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
  CONSTRAINT chk_users_username_pair CHECK ((username IS NULL) = (username_norm IS NULL)),
  CONSTRAINT chk_users_email_pair CHECK ((email IS NULL) = (email_norm IS NULL)),

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
  CONSTRAINT chk_user_credentials_hash_len CHECK (char_length(password_hash) >= 20 AND char_length(password_hash) <= 1024)
);

CREATE INDEX IF NOT EXISTS idx_user_credentials_user_id ON arc.user_credentials (user_id);

DROP TRIGGER IF EXISTS trg_user_credentials_updated_at ON arc.user_credentials;

CREATE TRIGGER trg_user_credentials_updated_at
BEFORE UPDATE ON arc.user_credentials
FOR EACH ROW
EXECUTE FUNCTION arc.set_updated_at();

-- Sessions: refresh tokens are opaque and stored hashed (sha256 hex => 64 chars).
-- PR-005-ready: rotation chain + platform, with correct invariants.
CREATE TABLE IF NOT EXISTS arc.sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES arc.users (id) ON DELETE CASCADE,

  refresh_token_hash TEXT NOT NULL,

  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_used_at TIMESTAMPTZ NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ NULL,

  -- Rotation chain: when refresh is rotated, old session can point to replacement.
  replaced_by_session_id TEXT NULL REFERENCES arc.sessions (id) ON DELETE SET NULL,

  user_agent TEXT NULL,
  ip INET NULL,

  -- Device/platform context (PR-005).
  platform TEXT NOT NULL DEFAULT 'unknown',

  CONSTRAINT chk_sessions_id_ulid_len CHECK (char_length(id) = 26),
  CONSTRAINT chk_sessions_user_id_ulid_len CHECK (char_length(user_id) = 26),
  CONSTRAINT chk_sessions_refresh_hash_len CHECK (char_length(refresh_token_hash) = 64),

  -- IMPORTANT: this stays strict (do not remove).
  -- If you need an "expired session", create it with created_at in the past and
  -- expires_at after created_at (but still in the past).
  CONSTRAINT chk_sessions_expires_after_created CHECK (expires_at > created_at),
  CONSTRAINT chk_sessions_revoked_after_created CHECK (revoked_at IS NULL OR revoked_at >= created_at),
  CONSTRAINT chk_sessions_last_used_after_created CHECK (last_used_at IS NULL OR last_used_at >= created_at),

  CONSTRAINT chk_sessions_platform CHECK (platform IN ('web','ios','android','desktop','unknown')),

  -- Replacement cannot point to self.
  CONSTRAINT chk_sessions_replaced_not_self CHECK (replaced_by_session_id IS NULL OR replaced_by_session_id <> id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_sessions_refresh_token_hash ON arc.sessions (refresh_token_hash);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON arc.sessions (user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON arc.sessions (expires_at);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id_revoked_expires ON arc.sessions (user_id, revoked_at, expires_at);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id_platform ON arc.sessions (user_id, platform);
CREATE INDEX IF NOT EXISTS idx_sessions_replaced_by ON arc.sessions (replaced_by_session_id);

-- Partial index for "active sessions" reads.
CREATE INDEX IF NOT EXISTS idx_sessions_active_by_user ON arc.sessions (user_id, expires_at DESC)
WHERE revoked_at IS NULL;

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

-- Invite-only by default.
CREATE TABLE IF NOT EXISTS arc.invites (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL,
  created_by TEXT NULL REFERENCES arc.users (id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ NULL,
  consumed_by TEXT NULL REFERENCES arc.users (id) ON DELETE SET NULL,

  CONSTRAINT chk_invites_id_ulid_len CHECK (char_length(id) = 26),
  CONSTRAINT chk_invites_token_hash_len CHECK (char_length(token_hash) = 64),
  CONSTRAINT chk_invites_expires_after_created CHECK (expires_at > created_at),
  CONSTRAINT chk_invites_single_use CHECK (consumed_at IS NULL OR consumed_at >= created_at),
  CONSTRAINT chk_invites_consumed_by_pair CHECK ((consumed_at IS NULL) = (consumed_by IS NULL))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_invites_token_hash ON arc.invites (token_hash);
CREATE INDEX IF NOT EXISTS idx_invites_expires_at ON arc.invites (expires_at);
CREATE INDEX IF NOT EXISTS idx_invites_consumed_at ON arc.invites (consumed_at);

-- Membership is authoritative.
CREATE TABLE IF NOT EXISTS arc.conversation_members (
  conversation_id TEXT NOT NULL REFERENCES arc.conversations (id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES arc.users (id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'member',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (conversation_id, user_id),
  CONSTRAINT chk_conversation_members_role CHECK (role IN ('member','owner','admin')),
  CONSTRAINT chk_conversation_members_user_id_ulid_len CHECK (char_length(user_id) = 26)
);

CREATE INDEX IF NOT EXISTS idx_conversation_members_user_id ON arc.conversation_members (user_id);

-- Minimal security audit log.
CREATE TABLE IF NOT EXISTS arc.audit_log (
  id BIGSERIAL PRIMARY KEY,
  user_id TEXT NULL REFERENCES arc.users (id) ON DELETE SET NULL,
  session_id TEXT NULL REFERENCES arc.sessions (id) ON DELETE SET NULL,
  action TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ip INET NULL,
  user_agent TEXT NULL,
  meta JSONB NULL,
  CONSTRAINT chk_audit_action_len CHECK (char_length(action) >= 3 AND char_length(action) <= 120)
);

CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON arc.audit_log (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_user_id ON arc.audit_log (user_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_session_id ON arc.audit_log (session_id);
