package invite

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore persists invites in PostgreSQL.
type PostgresStore struct {
	pool   *pgxpool.Pool
	schema string
}

// StoreOption configures PostgresStore.
type StoreOption func(*PostgresStore) error

// WithSchema sets the DB schema used by the store (default: "arc").
func WithSchema(schema string) StoreOption {
	return func(s *PostgresStore) error {
		schema = strings.TrimSpace(schema)
		if schema == "" {
			return ErrInvalidInput
		}
		s.schema = schema
		return nil
	}
}

// NewPostgresStore constructs a PostgresStore.
func NewPostgresStore(pool *pgxpool.Pool, opts ...StoreOption) (*PostgresStore, error) {
	st := &PostgresStore{pool: pool, schema: "arc"}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(st); err != nil {
			return nil, err
		}
	}
	if st.pool == nil {
		return nil, ErrInvalidInput
	}
	return st, nil
}

// Create inserts a new invite record.
func (s *PostgresStore) Create(ctx context.Context, in CreateRecord) (Invite, error) {
	if s == nil || s.pool == nil {
		return Invite{}, ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return Invite{}, err
	}
	if strings.TrimSpace(in.ID) == "" || strings.TrimSpace(in.TokenHash) == "" {
		return Invite{}, ErrInvalidInput
	}
	if in.MaxUses <= 0 {
		return Invite{}, ErrInvalidInput
	}
	if in.Note != nil && len(strings.TrimSpace(*in.Note)) > 512 {
		return Invite{}, ErrInvalidInput
	}
	invites := pgIdent(s.schema, "invites")

	_, err := s.pool.Exec(ctx,
		`INSERT INTO `+invites+` (
		     id, token_hash, created_by, created_at, expires_at, max_uses, used_count, revoked_at, note, consumed_at, consumed_by
		   ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		in.ID,
		in.TokenHash,
		in.CreatedBy,
		in.CreatedAt,
		in.ExpiresAt,
		in.MaxUses,
		in.UsedCount,
		in.RevokedAt,
		in.Note,
		in.ConsumedAt,
		in.ConsumedBy,
	)
	if err != nil {
		return Invite{}, err
	}

	return Invite{
		ID:         in.ID,
		CreatedBy:  in.CreatedBy,
		CreatedAt:  in.CreatedAt,
		ExpiresAt:  in.ExpiresAt,
		MaxUses:    in.MaxUses,
		UsedCount:  in.UsedCount,
		RevokedAt:  in.RevokedAt,
		Note:       in.Note,
		ConsumedAt: in.ConsumedAt,
		ConsumedBy: in.ConsumedBy,
	}, nil
}

// GetByTokenHash fetches an invite by token hash.
func (s *PostgresStore) GetByTokenHash(ctx context.Context, tokenHash string) (Invite, error) {
	if s == nil || s.pool == nil {
		return Invite{}, ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return Invite{}, err
	}
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return Invite{}, ErrInvalidInput
	}

	invites := pgIdent(s.schema, "invites")
	var out Invite
	err := s.pool.QueryRow(ctx,
		`SELECT id, created_by, created_at, expires_at, max_uses, used_count, revoked_at, note, consumed_at, consumed_by
		   FROM `+invites+`
		  WHERE token_hash = $1`,
		tokenHash,
	).Scan(
		&out.ID,
		&out.CreatedBy,
		&out.CreatedAt,
		&out.ExpiresAt,
		&out.MaxUses,
		&out.UsedCount,
		&out.RevokedAt,
		&out.Note,
		&out.ConsumedAt,
		&out.ConsumedBy,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Invite{}, ErrNotFound
		}
		return Invite{}, err
	}
	return out, nil
}

// Consume increments used_count and marks last consumption.
func (s *PostgresStore) Consume(ctx context.Context, in ConsumeRecord) (Invite, error) {
	if s == nil || s.pool == nil {
		return Invite{}, ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return Invite{}, err
	}
	if strings.TrimSpace(in.TokenHash) == "" || in.ConsumedBy == nil {
		return Invite{}, ErrInvalidInput
	}
	if in.Now.IsZero() {
		in.Now = time.Now().UTC()
	}

	invites := pgIdent(s.schema, "invites")
	var out Invite
	err := s.pool.QueryRow(ctx,
		`UPDATE `+invites+`
		    SET used_count = used_count + 1,
		        consumed_at = $1,
		        consumed_by = $2
		  WHERE token_hash = $3
		    AND revoked_at IS NULL
		    AND expires_at > $1
		    AND used_count < max_uses
		RETURNING id, created_by, created_at, expires_at, max_uses, used_count, revoked_at, note, consumed_at, consumed_by`,
		in.Now,
		in.ConsumedBy,
		in.TokenHash,
	).Scan(
		&out.ID,
		&out.CreatedBy,
		&out.CreatedAt,
		&out.ExpiresAt,
		&out.MaxUses,
		&out.UsedCount,
		&out.RevokedAt,
		&out.Note,
		&out.ConsumedAt,
		&out.ConsumedBy,
	)
	if err == nil {
		return out, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, err
	}

	// Distinguish not-found vs not-active.
	_, selErr := s.GetByTokenHash(ctx, in.TokenHash)
	if selErr != nil {
		if errors.Is(selErr, ErrNotFound) {
			return Invite{}, ErrNotFound
		}
		return Invite{}, selErr
	}
	return Invite{}, ErrNotActive
}

func pgIdent(schema, table string) string {
	return pgx.Identifier{schema, table}.Sanitize()
}
