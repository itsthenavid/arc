package realtime

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresMembershipStore_PublicConversation_RequiresExplicitMembership(t *testing.T) {
	t.Parallel()

	pool := mustOpenTestPool(t)
	defer pool.Close()

	schema := mustCreateTestSchema(t, pool)
	t.Cleanup(func() { mustDropSchema(t, pool, schema) })

	mustApplyMembershipSchemaRT(t, pool, schema)

	store, err := NewPostgresMembershipStore(pool, WithMembershipSchema(schema))
	if err != nil {
		t.Fatalf("new membership store: %v", err)
	}

	const (
		userID = "01HZZZZZZZZZZZZZZZZZZZZZZZ"
		convID = "conv-public-membership-1"
	)
	mustInsertMembershipUserRT(t, pool, schema, userID)
	mustInsertMembershipConversationRT(t, pool, schema, convID, "room", conversationVisibilityPublic)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	info, err := store.GetConversation(ctx, convID)
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	if info.Kind != "room" {
		t.Fatalf("expected kind=room, got %q", info.Kind)
	}
	if info.Visibility != conversationVisibilityPublic {
		t.Fatalf("expected visibility=public, got %q", info.Visibility)
	}

	if err := store.EnsureMember(ctx, userID, convID); !errors.Is(err, ErrMembershipRequired) {
		t.Fatalf("expected ErrMembershipRequired, got %v", err)
	}

	if err := store.AddMember(ctx, userID, convID); !errors.Is(err, ErrConversationNotPrivate) {
		t.Fatalf("expected ErrConversationNotPrivate, got %v", err)
	}
}

func TestPostgresMembershipStore_PrivateConversation_AddAndEnsureMember(t *testing.T) {
	t.Parallel()

	pool := mustOpenTestPool(t)
	defer pool.Close()

	schema := mustCreateTestSchema(t, pool)
	t.Cleanup(func() { mustDropSchema(t, pool, schema) })

	mustApplyMembershipSchemaRT(t, pool, schema)

	store, err := NewPostgresMembershipStore(pool, WithMembershipSchema(schema))
	if err != nil {
		t.Fatalf("new membership store: %v", err)
	}

	const (
		userID = "01HYYYYYYYYYYYYYYYYYYYYYYYY"
		convID = "conv-private-membership-1"
	)
	mustInsertMembershipUserRT(t, pool, schema, userID)
	mustInsertMembershipConversationRT(t, pool, schema, convID, "group", conversationVisibilityPrivate)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := store.EnsureMember(ctx, userID, convID); !errors.Is(err, ErrMembershipRequired) {
		t.Fatalf("expected ErrMembershipRequired before add, got %v", err)
	}

	if err := store.AddMember(ctx, userID, convID); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if err := store.AddMember(ctx, userID, convID); err != nil {
		t.Fatalf("add member idempotent: %v", err)
	}

	if err := store.EnsureMember(ctx, userID, convID); err != nil {
		t.Fatalf("ensure member after add: %v", err)
	}

	ok, err := store.IsMember(ctx, userID, convID)
	if err != nil {
		t.Fatalf("is member: %v", err)
	}
	if !ok {
		t.Fatalf("expected membership=true")
	}

	joinedAt := mustSelectJoinedAtRT(t, pool, schema, convID, userID)
	if joinedAt.IsZero() {
		t.Fatalf("expected joined_at to be set")
	}
}

func mustApplyMembershipSchemaRT(t *testing.T, pool *pgxpool.Pool, schema string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	users := pgIdent(schema, "users")
	conversations := pgIdent(schema, "conversations")
	members := pgIdent(schema, "conversation_members")

	schemaSQL := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
  id TEXT PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS %s (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('direct', 'group', 'room')),
  visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('public', 'private')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS %s (
  conversation_id TEXT NOT NULL REFERENCES %s(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES %s(id) ON DELETE CASCADE,
  joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  role TEXT NOT NULL DEFAULT 'member',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (conversation_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_conversation_members_user_id
  ON %s (user_id);
`, users, conversations, members, conversations, users, members)

	if _, err := pool.Exec(ctx, schemaSQL); err != nil {
		t.Fatalf("apply membership schema: %v", err)
	}
}

func mustInsertMembershipUserRT(t *testing.T, pool *pgxpool.Pool, schema, userID string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	users := pgIdent(schema, "users")
	if _, err := pool.Exec(ctx, `INSERT INTO `+users+` (id) VALUES ($1)`, userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

func mustInsertMembershipConversationRT(t *testing.T, pool *pgxpool.Pool, schema, conversationID, kind, visibility string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conversations := pgIdent(schema, "conversations")
	if _, err := pool.Exec(ctx,
		`INSERT INTO `+conversations+` (id, kind, visibility) VALUES ($1, $2, $3)`,
		conversationID, kind, visibility,
	); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
}

func mustSelectJoinedAtRT(t *testing.T, pool *pgxpool.Pool, schema, conversationID, userID string) time.Time {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	members := pgIdent(schema, "conversation_members")
	var joinedAt time.Time
	if err := pool.QueryRow(ctx,
		`SELECT joined_at FROM `+members+` WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID,
	).Scan(&joinedAt); err != nil {
		t.Fatalf("select joined_at: %v", err)
	}
	return joinedAt
}
