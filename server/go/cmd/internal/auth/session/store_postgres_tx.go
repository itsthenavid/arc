package session

import (
	"context"
	"errors"
	"net"
	"time"

	"arc/cmd/security/token"

	"github.com/jackc/pgx/v5"
	"github.com/oklog/ulid/v2"
)

func hashRefreshTokenHex(s string) string {
	return token.HashRefreshTokenHex(s)
}

func getByRefreshHashForUpdateTx(ctx context.Context, tx pgx.Tx, refreshHash string) (Row, error) {
	var row Row

	err := tx.QueryRow(ctx, `
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

func createTx(
	ctx context.Context,
	tx pgx.Tx,
	now time.Time,
	userID string,
	dev DeviceContext,
	refreshHash string,
	expiresAt time.Time,
) (string, error) {
	id := ulid.Make().String()

	var ip net.IP
	if dev.IP != nil {
		ip = dev.IP
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO arc.sessions (
			id, user_id, refresh_token_hash,
			created_at, last_used_at, expires_at, revoked_at,
			replaced_by_session_id, user_agent, ip, platform, revocation_reason
		) VALUES (
			$1, $2, $3,
			$4, $4, $5, NULL,
			NULL, $6, $7, $8, NULL
		)
	`, id, userID, refreshHash, now, expiresAt, nullIfEmpty(dev.UserAgent), ip, string(dev.Platform))
	if err != nil {
		return "", err
	}

	return id, nil
}

func markRotatedTx(ctx context.Context, tx pgx.Tx, now time.Time, oldID string, newID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE arc.sessions
		SET
			last_used_at = $2,
			revoked_at = $2,
			replaced_by_session_id = $3,
			revocation_reason = 'rotation'
		WHERE id = $1
	`, oldID, now, newID)
	return err
}

func revokeAllTx(ctx context.Context, tx pgx.Tx, now time.Time, userID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE arc.sessions
		SET revoked_at = COALESCE(revoked_at, $2),
		    revocation_reason = COALESCE(revocation_reason, 'reuse_detected')
		WHERE user_id = $1
	`, userID, now)
	return err
}
