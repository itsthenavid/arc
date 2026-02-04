CREATE SCHEMA IF NOT EXISTS arc;

-- NOTE:
-- Arc server queries are schema-qualified (arc.*) and do not rely on search_path.

CREATE TABLE IF NOT EXISTS arc.conversations (
  id         TEXT PRIMARY KEY,
  kind       TEXT NOT NULL CHECK (kind IN ('direct', 'group')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- next_seq is the next allocatable sequence number (starts at 1).
CREATE TABLE IF NOT EXISTS arc.conversation_cursors (
  conversation_id TEXT PRIMARY KEY REFERENCES arc.conversations(id) ON DELETE CASCADE,
  next_seq        BIGINT NOT NULL DEFAULT 1,
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS arc.messages (
  conversation_id TEXT NOT NULL REFERENCES arc.conversations(id) ON DELETE CASCADE,
  seq             BIGINT NOT NULL,
  server_msg_id   TEXT NOT NULL,
  client_msg_id   TEXT NOT NULL,
  sender_session  TEXT NOT NULL,
  text            TEXT NOT NULL,
  server_ts       TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

  PRIMARY KEY (conversation_id, seq),
  CONSTRAINT uq_messages_conversation_client_msg UNIQUE (conversation_id, client_msg_id),
  CONSTRAINT uq_messages_server_msg_id UNIQUE (server_msg_id),
  CONSTRAINT chk_messages_text_len CHECK (char_length(text) > 0 AND char_length(text) <= 4096)
);

CREATE INDEX IF NOT EXISTS idx_messages_conversation_seq_asc
  ON arc.messages (conversation_id, seq ASC);

CREATE INDEX IF NOT EXISTS idx_messages_conversation_seq_desc
  ON arc.messages (conversation_id, seq DESC);

CREATE INDEX IF NOT EXISTS idx_messages_conversation_client_msg
  ON arc.messages (conversation_id, client_msg_id);
