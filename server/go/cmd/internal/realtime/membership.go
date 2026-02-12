package realtime

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MembershipStore defines the authorization boundary for conversation membership.
type MembershipStore interface {
	// IsMember returns true if userID is an active member of conversationID.
	IsMember(ctx context.Context, userID, conversationID string) (bool, error)
}

// PostgresMembershipStore checks membership via arc.conversation_members.
type PostgresMembershipStore struct {
	pool   *pgxpool.Pool
	schema string
}

// MembershipOption configures PostgresMembershipStore behavior.
type MembershipOption func(*PostgresMembershipStore) error

// WithMembershipSchema sets the DB schema used by the membership store (default: "arc").
func WithMembershipSchema(schema string) MembershipOption {
	return func(s *PostgresMembershipStore) error {
		schema = strings.TrimSpace(schema)
		if schema == "" {
			return errors.New("realtime: empty schema")
		}
		if !isValidPGIdent(schema) {
			return errors.New("realtime: invalid schema identifier")
		}
		s.schema = schema
		return nil
	}
}

// NewPostgresMembershipStore constructs a membership store backed by PostgreSQL.
func NewPostgresMembershipStore(pool *pgxpool.Pool, opts ...MembershipOption) (*PostgresMembershipStore, error) {
	st := &PostgresMembershipStore{
		pool:   pool,
		schema: "arc",
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(st); err != nil {
			return nil, err
		}
	}
	if st.pool == nil {
		return nil, errors.New("realtime: nil pool")
	}
	return st, nil
}

// IsMember checks if userID is a member of conversationID.
func (s *PostgresMembershipStore) IsMember(ctx context.Context, userID, conversationID string) (bool, error) {
	if s == nil || s.pool == nil {
		return false, errors.New("realtime: nil membership store")
	}
	userID = strings.TrimSpace(userID)
	conversationID = strings.TrimSpace(conversationID)
	if userID == "" || conversationID == "" {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	members := pgIdent(s.schema, "conversation_members")

	var one int
	err := s.pool.QueryRow(ctx,
		`SELECT 1 FROM `+members+` WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID,
	).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
