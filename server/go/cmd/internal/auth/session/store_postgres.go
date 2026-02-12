package session

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

// PostgresStore implements Store using PostgreSQL (arc.sessions).
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a Postgres-backed session store.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// Create inserts a new session row and returns its ULID.
func (s *PostgresStore) Create(ctx context.Context, now time.Time, userID string, dev DeviceContext, refreshHash string, expiresAt time.Time, revocationReason *string) (string, error) {
	id := ulid.Make().String()

	var ip net.IP
	if dev.IP != nil {
		ip = dev.IP
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO arc.sessions (
			id, user_id, refresh_token_hash,
			created_at, last_used_at, expires_at, revoked_at,
			replaced_by_session_id, user_agent, ip, platform, revocation_reason
		) VALUES (
			$1, $2, $3,
			$4, $4, $5, NULL,
			NULL, $6, $7, $8, $9
		)
	`, id, userID, refreshHash, now, expiresAt, nullIfEmpty(dev.UserAgent), ip, string(dev.Platform), revocationReason)
	if err != nil {
		return "", err
	}

	return id, nil
}

// GetByID loads a session row by ID.
func (s *PostgresStore) GetByID(ctx context.Context, sessionID string) (Row, error) {
	var row Row

	err := s.pool.QueryRow(ctx, `
		SELECT
			id, user_id, refresh_token_hash,
			created_at, last_used_at, expires_at, revoked_at,
			replaced_by_session_id, platform
		FROM arc.sessions
		WHERE id = $1
	`, sessionID).Scan(
		&row.ID,
		&row.UserID,
		&row.RefreshTokenHash,
		&row.CreatedAt,
		&row.LastUsedAt,
		&row.ExpiresAt,
		&row.RevokedAt,
		&row.ReplacedBySessionID,
		&row.Platform,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Row{}, ErrSessionNotFound
	}
	if err != nil {
		return Row{}, err
	}

	return row, nil
}

// GetByRefreshHashForUpdate loads a session by refresh token hash and locks it.
func (s *PostgresStore) GetByRefreshHashForUpdate(ctx context.Context, refreshHash string) (Row, error) {
	var row Row

	err := s.pool.QueryRow(ctx, `
		SELECT
			id, user_id, refresh_token_hash,
			created_at, last_used_at, expires_at, revoked_at,
			replaced_by_session_id, platform
		FROM arc.sessions
		WHERE refresh_token_hash = $1
		FOR UPDATE
	`, refreshHash).Scan(
		&row.ID,
		&row.UserID,
		&row.RefreshTokenHash,
		&row.CreatedAt,
		&row.LastUsedAt,
		&row.ExpiresAt,
		&row.RevokedAt,
		&row.ReplacedBySessionID,
		&row.Platform,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Row{}, ErrSessionNotFound
	}
	if err != nil {
		return Row{}, err
	}

	return row, nil
}

// MarkRotated revokes the old session and links it to the replacement session.
func (s *PostgresStore) MarkRotated(ctx context.Context, now time.Time, sessionID string, replacedBy string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE arc.sessions
		SET
			last_used_at = $2,
			revoked_at = $2,
			replaced_by_session_id = $3,
			revocation_reason = 'rotation'
		WHERE id = $1
	`, sessionID, now, replacedBy)
	return err
}

// Touch updates last_used_at for a session.
func (s *PostgresStore) Touch(ctx context.Context, now time.Time, sessionID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE arc.sessions
		SET last_used_at = $2
		WHERE id = $1
	`, sessionID, now)
	return err
}

// Revoke revokes a single session (idempotent).
func (s *PostgresStore) Revoke(ctx context.Context, now time.Time, sessionID string, reason string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE arc.sessions
		SET revoked_at = COALESCE(revoked_at, $2),
		    revocation_reason = COALESCE(revocation_reason, $3)
		WHERE id = $1
	`, sessionID, now, reason)
	return err
}

// RevokeAll revokes all sessions for a user (idempotent).
func (s *PostgresStore) RevokeAll(ctx context.Context, now time.Time, userID string, reason string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE arc.sessions
		SET revoked_at = COALESCE(revoked_at, $2),
		    revocation_reason = COALESCE(revocation_reason, $3)
		WHERE user_id = $1
	`, userID, now, reason)
	return err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
