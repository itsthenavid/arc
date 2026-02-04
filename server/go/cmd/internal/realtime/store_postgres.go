// Package realtime contains Arc's realtime WebSocket gateway and message persistence primitives.
package realtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is a MessageStore backed by PostgreSQL.
//
// Ownership model:
// - PostgresStore does NOT own the pgx pool. The caller must close the pool.
// - Close() is therefore a no-op.
//
// Concurrency model:
// - Uses per-conversation transactional advisory locks to guarantee:
//   - No sequence gaps caused by duplicates
//   - Strict monotonic ordering under concurrency
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore constructs a Postgres-backed MessageStore.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// Close is a no-op because the pool is owned by the caller.
func (s *PostgresStore) Close() error { return nil }

// AppendMessage appends a message with idempotency and monotonic sequence allocation.
func (s *PostgresStore) AppendMessage(ctx context.Context, in AppendMessageInput) (AppendMessageResult, error) {
	if in.ConversationID == "" || in.ClientMsgID == "" || in.SenderSession == "" {
		return AppendMessageResult{}, errors.New("invalid input")
	}

	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.ReadCommitted,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return AppendMessageResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialize all writes per conversation to guarantee:
	// - No seq waste for duplicates
	// - Strict monotonic ordering without races
	//
	// hashtextextended reduces collision risk vs hashtext (still a hash, but better).
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, in.ConversationID); err != nil {
		return AppendMessageResult{}, fmt.Errorf("advisory lock: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO arc.conversations (id, kind) VALUES ($1, 'direct')
		 ON CONFLICT (id) DO NOTHING`,
		in.ConversationID,
	); err != nil {
		return AppendMessageResult{}, err
	}

	existing, err := readMessageByClientMsgID(ctx, tx, in.ConversationID, in.ClientMsgID)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return AppendMessageResult{}, err
		}
		return AppendMessageResult{Stored: existing, Duplicated: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return AppendMessageResult{}, err
	}

	// Cursor row ensures monotonic seq allocation.
	if _, err := tx.Exec(ctx,
		`INSERT INTO arc.conversation_cursors (conversation_id, next_seq)
		 VALUES ($1, 1)
		 ON CONFLICT (conversation_id) DO NOTHING`,
		in.ConversationID,
	); err != nil {
		return AppendMessageResult{}, err
	}

	var seq int64
	if err := tx.QueryRow(ctx,
		`UPDATE arc.conversation_cursors
		    SET next_seq = next_seq + 1,
		        updated_at = now()
		  WHERE conversation_id = $1
		RETURNING (next_seq - 1)`,
		in.ConversationID,
	).Scan(&seq); err != nil {
		return AppendMessageResult{}, err
	}

	serverMsgID := NewRandomHex(16)

	if _, err := tx.Exec(ctx,
		`INSERT INTO arc.messages (
		     conversation_id, seq, server_msg_id, client_msg_id, sender_session, text, server_ts
		   ) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		in.ConversationID, seq, serverMsgID, in.ClientMsgID, in.SenderSession, in.Text, now,
	); err != nil {
		return AppendMessageResult{}, fmt.Errorf("insert message: %w", err)
	}

	out := StoredMessage{
		ConversationID: in.ConversationID,
		ClientMsgID:    in.ClientMsgID,
		ServerMsgID:    serverMsgID,
		Seq:            seq,
		SenderSession:  in.SenderSession,
		Text:           in.Text,
		ServerTS:       now,
	}

	if err := tx.Commit(ctx); err != nil {
		return AppendMessageResult{}, err
	}
	return AppendMessageResult{Stored: out, Duplicated: false}, nil
}

// FetchHistory returns messages ordered by seq ASC, with optional paging by AfterSeq.
func (s *PostgresStore) FetchHistory(ctx context.Context, in FetchHistoryInput) (FetchHistoryResult, error) {
	if in.ConversationID == "" {
		return FetchHistoryResult{}, errors.New("missing conversation_id")
	}

	limit := in.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	fetch := limit + 1

	var (
		rows pgx.Rows
		err  error
	)

	if in.AfterSeq == nil {
		rows, err = s.pool.Query(ctx,
			`SELECT conversation_id, client_msg_id, server_msg_id, seq, sender_session, text, server_ts
			   FROM arc.messages
			  WHERE conversation_id = $1
			  ORDER BY seq ASC
			  LIMIT $2`,
			in.ConversationID, fetch,
		)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT conversation_id, client_msg_id, server_msg_id, seq, sender_session, text, server_ts
			   FROM arc.messages
			  WHERE conversation_id = $1 AND seq > $2
			  ORDER BY seq ASC
			  LIMIT $3`,
			in.ConversationID, *in.AfterSeq, fetch,
		)
	}
	if err != nil {
		return FetchHistoryResult{}, err
	}
	defer rows.Close()

	msgs := make([]StoredMessage, 0, fetch)
	for rows.Next() {
		var m StoredMessage
		if err := rows.Scan(
			&m.ConversationID,
			&m.ClientMsgID,
			&m.ServerMsgID,
			&m.Seq,
			&m.SenderSession,
			&m.Text,
			&m.ServerTS,
		); err != nil {
			return FetchHistoryResult{}, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return FetchHistoryResult{}, err
	}

	hasMore := len(msgs) > limit
	if hasMore {
		msgs = msgs[:limit]
	}

	return FetchHistoryResult{Messages: msgs, HasMore: hasMore}, nil
}

func readMessageByClientMsgID(ctx context.Context, tx pgx.Tx, conversationID, clientMsgID string) (StoredMessage, error) {
	var m StoredMessage
	err := tx.QueryRow(ctx,
		`SELECT conversation_id, client_msg_id, server_msg_id, seq, sender_session, text, server_ts
		   FROM arc.messages
		  WHERE conversation_id = $1 AND client_msg_id = $2`,
		conversationID, clientMsgID,
	).Scan(&m.ConversationID, &m.ClientMsgID, &m.ServerMsgID, &m.Seq, &m.SenderSession, &m.Text, &m.ServerTS)
	return m, err
}
