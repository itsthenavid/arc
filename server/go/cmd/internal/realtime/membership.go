package realtime

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	conversationVisibilityPublic  = "public"
	conversationVisibilityPrivate = "private"
)

var (
	// ErrConversationNotFound is returned when a conversation id does not exist.
	ErrConversationNotFound = errors.New("realtime: conversation not found")
	// ErrMembershipRequired is returned when the user is not a member of the conversation.
	ErrMembershipRequired = errors.New("realtime: membership required")
	// ErrConversationNotPrivate is returned when AddMember is called for a non-private conversation.
	ErrConversationNotPrivate = errors.New("realtime: conversation is not private")
)

// ConversationInfo represents the ACL-relevant metadata of a conversation.
type ConversationInfo struct {
	ID         string
	Kind       string
	Visibility string
}

// MembershipStore defines the authorization boundary for conversation membership.
type MembershipStore interface {
	// GetConversation returns conversation metadata needed for access-control decisions.
	GetConversation(ctx context.Context, conversationID string) (ConversationInfo, error)
	// IsMember returns true if userID is an active member of conversationID.
	IsMember(ctx context.Context, userID, conversationID string) (bool, error)
	// EnsureMember returns nil only if userID is a member of conversationID.
	EnsureMember(ctx context.Context, userID, conversationID string) error
	// AddMember adds userID to a private conversation (idempotent).
	AddMember(ctx context.Context, userID, conversationID string) error
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

// GetConversation fetches ACL metadata for a conversation.
func (s *PostgresMembershipStore) GetConversation(ctx context.Context, conversationID string) (ConversationInfo, error) {
	if s == nil || s.pool == nil {
		return ConversationInfo{}, errors.New("realtime: nil membership store")
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return ConversationInfo{}, errors.New("realtime: missing conversation_id")
	}
	if err := ctx.Err(); err != nil {
		return ConversationInfo{}, err
	}

	conversations := pgIdent(s.schema, "conversations")

	var info ConversationInfo
	err := s.pool.QueryRow(ctx,
		`SELECT id, kind, visibility
		   FROM `+conversations+`
		  WHERE id = $1`,
		conversationID,
	).Scan(&info.ID, &info.Kind, &info.Visibility)
	if errors.Is(err, pgx.ErrNoRows) {
		return ConversationInfo{}, ErrConversationNotFound
	}
	if err != nil {
		return ConversationInfo{}, err
	}

	info.Kind = normalizeConversationKind(info.Kind)
	switch strings.ToLower(strings.TrimSpace(info.Visibility)) {
	case conversationVisibilityPublic:
		info.Visibility = conversationVisibilityPublic
	default:
		// Fail closed: any unknown/empty visibility is treated as private.
		info.Visibility = conversationVisibilityPrivate
	}
	return info, nil
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

// EnsureMember checks membership and returns ErrMembershipRequired when absent.
func (s *PostgresMembershipStore) EnsureMember(ctx context.Context, userID, conversationID string) error {
	if s == nil || s.pool == nil {
		return errors.New("realtime: nil membership store")
	}
	userID = strings.TrimSpace(userID)
	conversationID = strings.TrimSpace(conversationID)
	if userID == "" || conversationID == "" {
		return errors.New("realtime: missing user_id or conversation_id")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	conversations := pgIdent(s.schema, "conversations")
	members := pgIdent(s.schema, "conversation_members")

	var isMember bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (
		     SELECT 1 FROM `+members+` m
		     WHERE m.conversation_id = c.id
		       AND m.user_id = $2
		   )
		   FROM `+conversations+` c
		  WHERE c.id = $1`,
		conversationID, userID,
	).Scan(&isMember)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConversationNotFound
	}
	if err != nil {
		return err
	}
	if !isMember {
		return ErrMembershipRequired
	}
	return nil
}

// AddMember adds a user to a private conversation (idempotent).
func (s *PostgresMembershipStore) AddMember(ctx context.Context, userID, conversationID string) error {
	if s == nil || s.pool == nil {
		return errors.New("realtime: nil membership store")
	}
	userID = strings.TrimSpace(userID)
	conversationID = strings.TrimSpace(conversationID)
	if userID == "" || conversationID == "" {
		return errors.New("realtime: missing user_id or conversation_id")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	now := time.Now().UTC()
	conversations := pgIdent(s.schema, "conversations")
	members := pgIdent(s.schema, "conversation_members")

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.ReadCommitted,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var visibility string
	err = tx.QueryRow(ctx,
		`SELECT visibility
		   FROM `+conversations+`
		  WHERE id = $1
		  FOR SHARE`,
		conversationID,
	).Scan(&visibility)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConversationNotFound
	}
	if err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(visibility)) != conversationVisibilityPrivate {
		return ErrConversationNotPrivate
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO `+members+` (conversation_id, user_id, joined_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (conversation_id, user_id) DO NOTHING`,
		conversationID, userID, now,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

var _ MembershipStore = (*PostgresMembershipStore)(nil)
