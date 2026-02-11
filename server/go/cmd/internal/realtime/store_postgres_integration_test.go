package realtime

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Integration tests are enabled when ARC_DATABASE_URL is set.
// This keeps local "go test ./..." fast & deterministic without requiring Postgres.

func TestPostgresStore_Append_Dedupe_NoSeqWaste(t *testing.T) {
	t.Parallel()

	pool := mustOpenTestPool(t)
	defer pool.Close()

	schema := mustCreateTestSchema(t, pool)
	t.Cleanup(func() { mustDropSchema(t, pool, schema) })

	mustApplySchema(t, pool, schema)

	store := mustNewStore(t, pool, schema)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	convID := "it-dedupe-" + NewRandomHex(8)

	clientMsgID := "cmsg-" + NewRandomHex(8)
	now := time.Now().UTC()

	first, err := store.AppendMessage(ctx, AppendMessageInput{
		ConversationID: convID,
		ClientMsgID:    clientMsgID,
		SenderSession:  "session-a",
		Text:           "hello",
		Now:            now,
	})
	if err != nil {
		t.Fatalf("append first: %v", err)
	}
	if first.Duplicated {
		t.Fatalf("append first: expected Duplicated=false")
	}
	if first.Stored.Seq != 1 {
		t.Fatalf("append first: expected seq=1 got=%d", first.Stored.Seq)
	}
	if strings.TrimSpace(first.Stored.ServerMsgID) == "" {
		t.Fatalf("append first: expected non-empty server_msg_id")
	}

	second, err := store.AppendMessage(ctx, AppendMessageInput{
		ConversationID: convID,
		ClientMsgID:    clientMsgID, // duplicate on purpose
		SenderSession:  "session-a",
		Text:           "hello",
		Now:            now.Add(1 * time.Second),
	})
	if err != nil {
		t.Fatalf("append duplicate: %v", err)
	}
	if !second.Duplicated {
		t.Fatalf("append duplicate: expected Duplicated=true")
	}
	if second.Stored.Seq != first.Stored.Seq {
		t.Fatalf("append duplicate: seq mismatch: first=%d second=%d", first.Stored.Seq, second.Stored.Seq)
	}
	if second.Stored.ServerMsgID != first.Stored.ServerMsgID {
		t.Fatalf("append duplicate: server_msg_id mismatch")
	}

	cnt := mustCountMessages(t, pool, schema, convID)
	if cnt != 1 {
		t.Fatalf("expected 1 message row, got %d", cnt)
	}
}

func TestPostgresStore_History_Order_AfterSeq_HasMore(t *testing.T) {
	t.Parallel()

	pool := mustOpenTestPool(t)
	defer pool.Close()

	schema := mustCreateTestSchema(t, pool)
	t.Cleanup(func() { mustDropSchema(t, pool, schema) })

	mustApplySchema(t, pool, schema)

	store := mustNewStore(t, pool, schema)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	convID := "it-history-" + NewRandomHex(8)

	// Insert 3 messages.
	for i := 0; i < 3; i++ {
		_, err := store.AppendMessage(ctx, AppendMessageInput{
			ConversationID: convID,
			ClientMsgID:    fmt.Sprintf("cmsg-%d-%s", i, NewRandomHex(4)),
			SenderSession:  "session-a",
			Text:           fmt.Sprintf("m%d", i),
			Now:            time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	// Fetch with limit=2 -> expect HasMore=true and seq 1..2.
	out1, err := store.FetchHistory(ctx, FetchHistoryInput{
		ConversationID: convID,
		AfterSeq:       nil,
		Limit:          2,
	})
	if err != nil {
		t.Fatalf("fetch history 1: %v", err)
	}
	if len(out1.Messages) != 2 {
		t.Fatalf("fetch history 1: expected 2 msgs got %d", len(out1.Messages))
	}
	if !out1.HasMore {
		t.Fatalf("fetch history 1: expected HasMore=true")
	}
	if out1.Messages[0].Seq != 1 || out1.Messages[1].Seq != 2 {
		t.Fatalf("fetch history 1: expected seq [1,2], got [%d,%d]", out1.Messages[0].Seq, out1.Messages[1].Seq)
	}

	after := out1.Messages[len(out1.Messages)-1].Seq
	out2, err := store.FetchHistory(ctx, FetchHistoryInput{
		ConversationID: convID,
		AfterSeq:       &after,
		Limit:          50,
	})
	if err != nil {
		t.Fatalf("fetch history 2: %v", err)
	}
	if len(out2.Messages) != 1 {
		t.Fatalf("fetch history 2: expected 1 msg got %d", len(out2.Messages))
	}
	if out2.HasMore {
		t.Fatalf("fetch history 2: expected HasMore=false")
	}
	if out2.Messages[0].Seq != 3 {
		t.Fatalf("fetch history 2: expected seq=3 got=%d", out2.Messages[0].Seq)
	}
}

func TestPostgresStore_ConcurrentAppend_StrictSeq_NoGaps(t *testing.T) {
	t.Parallel()

	pool := mustOpenTestPool(t)
	defer pool.Close()

	schema := mustCreateTestSchema(t, pool)
	t.Cleanup(func() { mustDropSchema(t, pool, schema) })

	mustApplySchema(t, pool, schema)

	store := mustNewStore(t, pool, schema)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	convID := "it-concurrency-" + NewRandomHex(8)

	const n = 32

	var wg sync.WaitGroup
	wg.Add(n)

	errCh := make(chan error, n)

	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()

			_, err := store.AppendMessage(ctx, AppendMessageInput{
				ConversationID: convID,
				ClientMsgID:    fmt.Sprintf("cmsg-%d-%s", i, NewRandomHex(5)),
				SenderSession:  "session-a",
				Text:           fmt.Sprintf("m%d", i),
				Now:            time.Now().UTC(),
			})
			if err != nil {
				errCh <- err
				return
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent append error: %v", err)
	}

	out, err := store.FetchHistory(ctx, FetchHistoryInput{
		ConversationID: convID,
		AfterSeq:       nil,
		Limit:          200,
	})
	if err != nil {
		t.Fatalf("fetch history: %v", err)
	}
	if len(out.Messages) != n {
		t.Fatalf("expected %d messages, got %d", n, len(out.Messages))
	}
	if out.HasMore {
		t.Fatalf("expected HasMore=false")
	}

	seqs := make([]int64, 0, len(out.Messages))
	seen := make(map[int64]struct{}, len(out.Messages))

	for _, m := range out.Messages {
		seqs = append(seqs, m.Seq)
		seen[m.Seq] = struct{}{}
	}

	sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })

	// Strict: seqs must be exactly 1..n.
	for want := int64(1); want <= n; want++ {
		if _, ok := seen[want]; !ok {
			t.Fatalf("missing seq=%d (gap)", want)
		}
	}
	if seqs[0] != 1 || seqs[len(seqs)-1] != n {
		t.Fatalf("seq range mismatch: min=%d max=%d want=[1,%d]", seqs[0], seqs[len(seqs)-1], n)
	}
}

// ---- test helpers ----

func mustNewStore(t *testing.T, pool *pgxpool.Pool, schema string) *PostgresStore {
	t.Helper()

	st, err := NewPostgresStore(pool, WithSchema(schema))
	if err != nil {
		t.Fatalf("new postgres store: %v", err)
	}
	return st
}

func mustOpenTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	raw := strings.TrimSpace(os.Getenv("ARC_DATABASE_URL"))
	if raw == "" {
		t.Skip("integration test skipped: ARC_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(raw)
	if err != nil {
		t.Fatalf("parse ARC_DATABASE_URL: %v", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}

	// Validate acquire quickly.
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer pingCancel()

	c, err := pool.Acquire(pingCtx)
	if err != nil {
		pool.Close()
		t.Fatalf("acquire: %v", err)
	}
	c.Release()

	return pool
}

func mustCreateTestSchema(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()

	schema := "arc_it_" + strings.ToLower(NewRandomHex(8))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return schema
}

func mustDropSchema(t *testing.T, pool *pgxpool.Pool, schema string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, _ = pool.Exec(ctx, `DROP SCHEMA IF EXISTS `+pgx.Identifier{schema}.Sanitize()+` CASCADE`)
}

func mustApplySchema(t *testing.T, pool *pgxpool.Pool, schema string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	conversations := pgIdent(schema, "conversations")
	cursors := pgIdent(schema, "conversation_cursors")
	messages := pgIdent(schema, "messages")

	// Minimal schema required by PostgresStore.
	// Must remain semantically aligned with infra/db/atlas/schema.sql.
	schemaSQL := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
  id         TEXT PRIMARY KEY,
  kind       TEXT NOT NULL CHECK (kind IN ('direct', 'group')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS %s (
  conversation_id TEXT PRIMARY KEY REFERENCES %s(id) ON DELETE CASCADE,
  next_seq        BIGINT NOT NULL DEFAULT 1,
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS %s (
  conversation_id TEXT NOT NULL REFERENCES %s(id) ON DELETE CASCADE,
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
  ON %s (conversation_id, seq ASC);

CREATE INDEX IF NOT EXISTS idx_messages_conversation_seq_desc
  ON %s (conversation_id, seq DESC);

CREATE INDEX IF NOT EXISTS idx_messages_conversation_client_msg
  ON %s (conversation_id, client_msg_id);
`, conversations, cursors, conversations, messages, conversations, messages, messages, messages)

	if _, err := pool.Exec(ctx, schemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
}

func mustCountMessages(t *testing.T, pool *pgxpool.Pool, schema string, conversationID string) int {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cnt int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM `+pgIdent(schema, "messages")+` WHERE conversation_id = $1`,
		conversationID,
	).Scan(&cnt); err != nil {
		t.Fatalf("count messages: %v", err)
	}

	return cnt
}
