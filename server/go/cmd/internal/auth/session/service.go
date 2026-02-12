package session

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Service implements the high-level session operations for Arc.
//
// It issues sessions (access + refresh), validates access tokens,
// supports per-session and per-user revocation, and performs refresh rotation
// with reuse detection under a strict transactional model.
type Service struct {
	cfg    Config
	tokens AccessTokenManager
	store  Store

	// pool is used to create explicit transactions for rotation safety.
	pool *pgxpool.Pool
}

// Issued is the result of issuing or rotating a session.
// It includes a short-lived access token and an opaque refresh token.
type Issued struct {
	SessionID    string
	AccessToken  string
	AccessExp    time.Time
	RefreshToken string
	RefreshExp   time.Time
}

// NewService constructs a Service with the provided configuration, store, and token manager.
//
// The pool is required for refresh rotation, which must run inside a single transaction.
func NewService(cfg Config, pool *pgxpool.Pool, store Store, tokens AccessTokenManager) *Service {
	return &Service{cfg: cfg, pool: pool, store: store, tokens: tokens}
}

func (s *Service) refreshTTL(dev DeviceContext) time.Duration {
	switch dev.Platform {
	case PlatformWeb:
		return s.cfg.RefreshTTLWeb
	case PlatformIOS, PlatformAndroid, PlatformDesktop:
		if dev.RememberMe {
			return s.cfg.RefreshTTLNative
		}
		return s.cfg.RefreshTTLNativeShort
	default:
		// Conservative default.
		return s.cfg.RefreshTTLWeb
	}
}

// IssueSession creates a new session row in the database and returns fresh tokens.
//
// Refresh tokens are opaque random strings and must never be persisted in plaintext.
// Only the SHA-256 hash (hex) is stored in the database.
func (s *Service) IssueSession(ctx context.Context, now time.Time, userID string, dev DeviceContext) (Issued, error) {
	refreshPlain, refreshHash, err := newOpaqueRefreshToken(s.cfg.RefreshTokenBytes)
	if err != nil {
		return Issued{}, err
	}

	refreshExp := now.Add(s.refreshTTL(dev))

	sessionID, err := s.store.Create(ctx, now, userID, dev, refreshHash, refreshExp, nil)
	if err != nil {
		return Issued{}, err
	}

	accessToken, accessExp, err := s.tokens.Issue(userID, sessionID, now)
	if err != nil {
		return Issued{}, err
	}

	return Issued{
		SessionID:    sessionID,
		AccessToken:  accessToken,
		AccessExp:    accessExp,
		RefreshToken: refreshPlain,
		RefreshExp:   refreshExp,
	}, nil
}

// IssueAccessToken issues a short-lived access token for an existing session.
func (s *Service) IssueAccessToken(userID, sessionID string, now time.Time) (token string, exp time.Time, err error) {
	return s.tokens.Issue(userID, sessionID, now)
}

// ValidateAccessToken verifies an access token and ensures the backing session is active.
func (s *Service) ValidateAccessToken(ctx context.Context, token string, now time.Time) (AccessClaims, error) {
	claims, err := s.tokens.Verify(token, now)
	if err != nil {
		return AccessClaims{}, err
	}

	// Server-authoritative session check to honor revocations.
	row, err := s.store.GetByID(ctx, claims.SessionID)
	if err != nil {
		return AccessClaims{}, err
	}

	if row.UserID != claims.UserID {
		return AccessClaims{}, ErrInvalidToken
	}
	if row.RevokedAt != nil || row.ReplacedBySessionID != nil {
		return AccessClaims{}, ErrSessionRevoked
	}
	if !row.ExpiresAt.After(now) {
		return AccessClaims{}, ErrSessionExpired
	}

	return claims, nil
}

// RevokeSession revokes a single session by ID (e.g., logout from a device).
func (s *Service) RevokeSession(ctx context.Context, now time.Time, sessionID string) error {
	return s.store.Revoke(ctx, now, sessionID, "logout")
}

// RevokeAll revokes all sessions for a user (e.g., logout everywhere).
func (s *Service) RevokeAll(ctx context.Context, now time.Time, userID string) error {
	return s.store.RevokeAll(ctx, now, userID, "logout")
}

// TouchSession updates last_used_at for a session (best-effort).
func (s *Service) TouchSession(ctx context.Context, now time.Time, sessionID string) error {
	return s.store.Touch(ctx, now, sessionID)
}

// RotateRefresh performs refresh rotation with reuse detection.
//
// Security model:
//   - Lock the session row by refresh hash (SELECT ... FOR UPDATE).
//   - If the token belongs to a rotated session (revoked + replaced_by), treat it as reuse:
//     revoke all sessions for the user and return ErrRefreshReuseDetected.
//   - If the token belongs to a revoked session without replacement, return ErrSessionRevoked.
//   - Otherwise, create a new session, revoke the old session, and link replaced_by_session_id.
//
// This method must be executed within a single database transaction to be safe.
func (s *Service) RotateRefresh(ctx context.Context, now time.Time, refreshTokenPlain string, dev DeviceContext) (Issued, error) {
	refreshTokenPlain = strings.TrimSpace(refreshTokenPlain)
	// Basic sanity bounds to avoid pathological inputs.
	if refreshTokenPlain == "" || len(refreshTokenPlain) > 4096 {
		return Issued{}, ErrSessionNotFound
	}

	// Hash refresh token in-memory (never persist the plain token).
	refreshHash := hashRefreshTokenHex(refreshTokenPlain)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Issued{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the session row by refresh hash to make rotation safe.
	row, err := getByRefreshHashForUpdateTx(ctx, tx, refreshHash)
	if err != nil {
		return Issued{}, err
	}

	// Expiry check.
	if !row.ExpiresAt.After(now) {
		return Issued{}, ErrSessionExpired
	}

	// Reuse detection: a rotated refresh token presented again.
	if row.RevokedAt != nil && row.ReplacedBySessionID != nil {
		// Revoke all sessions for the user. This is a security incident.
		if err := revokeAllTx(ctx, tx, now, row.UserID); err != nil {
			return Issued{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Issued{}, err
		}
		return Issued{}, ErrRefreshReuseDetected
	}

	// If revoked without replacement: treat as revoked (logout).
	if row.RevokedAt != nil {
		return Issued{}, ErrSessionRevoked
	}

	// Rotate: create new session + revoke old + point replaced_by.
	newRefreshPlain, newRefreshHash, err := newOpaqueRefreshToken(s.cfg.RefreshTokenBytes)
	if err != nil {
		return Issued{}, err
	}
	newRefreshExp := now.Add(s.refreshTTL(dev))

	newSessionID, err := createTx(ctx, tx, now, row.UserID, dev, newRefreshHash, newRefreshExp)
	if err != nil {
		return Issued{}, err
	}

	if err := markRotatedTx(ctx, tx, now, row.ID, newSessionID); err != nil {
		return Issued{}, err
	}

	accessToken, accessExp, err := s.tokens.Issue(row.UserID, newSessionID, now)
	if err != nil {
		return Issued{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Issued{}, err
	}

	return Issued{
		SessionID:    newSessionID,
		AccessToken:  accessToken,
		AccessExp:    accessExp,
		RefreshToken: newRefreshPlain,
		RefreshExp:   newRefreshExp,
	}, nil
}
