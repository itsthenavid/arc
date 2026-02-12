package session

import (
	"context"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	paseto "aidanwoods.dev/go-paseto"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

// Integration tests are enabled when ARC_DATABASE_URL is set.
// In non-CI runs, unreachable Postgres skips these tests to keep local runs fast.

func TestPostgresSession_IssueAndRotateRefresh_Succeeds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbURL := os.Getenv("ARC_DATABASE_URL")
	if dbURL == "" {
		t.Skip("ARC_DATABASE_URL is not set; skipping Postgres integration test")
	}

	pool := mustPGXPool(ctx, t, dbURL)
	defer pool.Close()

	cfg, tokens := mustTestConfigAndTokens(t)
	store := NewPostgresStore(pool)
	svc := NewService(cfg, pool, store, tokens)

	userID := newULID(t)
	mustCreateUser(ctx, t, pool, userID)
	t.Cleanup(func() { cleanupUserData(ctx, t, pool, userID) })

	now := time.Now().UTC()
	dev := DeviceContext{
		Platform:   PlatformWeb,
		RememberMe: false,
		UserAgent:  "arc-test/1.0",
		IP:         nil,
	}

	issued1, err := svc.IssueSession(ctx, now, userID, dev)
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}
	if issued1.SessionID == "" || issued1.AccessToken == "" || issued1.RefreshToken == "" {
		t.Fatalf("IssueSession: expected non-empty tokens and sessionID")
	}

	claims, err := svc.ValidateAccessToken(ctx, issued1.AccessToken, now.Add(1*time.Second))
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if claims.UserID != userID {
		t.Fatalf("ValidateAccessToken: expected userID=%q, got %q", userID, claims.UserID)
	}
	if claims.SessionID != issued1.SessionID {
		t.Fatalf("ValidateAccessToken: expected sessionID=%q, got %q", issued1.SessionID, claims.SessionID)
	}

	issued2, err := svc.RotateRefresh(ctx, now.Add(2*time.Second), issued1.RefreshToken, dev)
	if err != nil {
		t.Fatalf("RotateRefresh: %v", err)
	}
	if issued2.SessionID == "" || issued2.SessionID == issued1.SessionID {
		t.Fatalf("RotateRefresh: expected a new sessionID")
	}
	if issued2.RefreshToken == "" || issued2.RefreshToken == issued1.RefreshToken {
		t.Fatalf("RotateRefresh: expected a new refresh token")
	}

	oldRow := mustGetSessionByID(ctx, t, pool, issued1.SessionID)
	if oldRow.RevokedAt == nil {
		t.Fatalf("expected old session revoked_at to be set")
	}
	if oldRow.ReplacedBySessionID == nil || *oldRow.ReplacedBySessionID != issued2.SessionID {
		t.Fatalf("expected old session replaced_by_session_id=%q, got %+v", issued2.SessionID, oldRow.ReplacedBySessionID)
	}

	newRow := mustGetSessionByID(ctx, t, pool, issued2.SessionID)
	if newRow.RevokedAt != nil {
		t.Fatalf("expected new session to be active, got revoked_at=%v", newRow.RevokedAt)
	}
}

func TestPostgresSession_RotateRefresh_ReuseDetected_RevokesAll(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbURL := os.Getenv("ARC_DATABASE_URL")
	if dbURL == "" {
		t.Skip("ARC_DATABASE_URL is not set; skipping Postgres integration test")
	}

	pool := mustPGXPool(ctx, t, dbURL)
	defer pool.Close()

	cfg, tokens := mustTestConfigAndTokens(t)
	store := NewPostgresStore(pool)
	svc := NewService(cfg, pool, store, tokens)

	userID := newULID(t)
	mustCreateUser(ctx, t, pool, userID)
	t.Cleanup(func() { cleanupUserData(ctx, t, pool, userID) })

	now := time.Now().UTC()
	dev := DeviceContext{Platform: PlatformWeb, RememberMe: false, UserAgent: "arc-test/1.0"}

	issued1, err := svc.IssueSession(ctx, now, userID, dev)
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}

	issued2, err := svc.RotateRefresh(ctx, now.Add(2*time.Second), issued1.RefreshToken, dev)
	if err != nil {
		t.Fatalf("RotateRefresh(1): %v", err)
	}

	_, err = svc.RotateRefresh(ctx, now.Add(4*time.Second), issued1.RefreshToken, dev)
	if err == nil {
		t.Fatalf("expected error on refresh reuse, got nil")
	}
	if err != ErrRefreshReuseDetected {
		t.Fatalf("expected ErrRefreshReuseDetected, got %v", err)
	}

	row1 := mustGetSessionByID(ctx, t, pool, issued1.SessionID)
	row2 := mustGetSessionByID(ctx, t, pool, issued2.SessionID)

	if row1.RevokedAt == nil {
		t.Fatalf("expected session1 revoked after reuse detection")
	}
	if row2.RevokedAt == nil {
		t.Fatalf("expected session2 revoked after reuse detection")
	}
}

func TestPostgresSession_RotateRefresh_OnRevokedSession_ReturnsRevoked(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbURL := os.Getenv("ARC_DATABASE_URL")
	if dbURL == "" {
		t.Skip("ARC_DATABASE_URL is not set; skipping Postgres integration test")
	}

	pool := mustPGXPool(ctx, t, dbURL)
	defer pool.Close()

	cfg, tokens := mustTestConfigAndTokens(t)
	store := NewPostgresStore(pool)
	svc := NewService(cfg, pool, store, tokens)

	userID := newULID(t)
	mustCreateUser(ctx, t, pool, userID)
	t.Cleanup(func() { cleanupUserData(ctx, t, pool, userID) })

	now := time.Now().UTC()
	dev := DeviceContext{Platform: PlatformWeb, RememberMe: false, UserAgent: "arc-test/1.0"}

	issued, err := svc.IssueSession(ctx, now, userID, dev)
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}

	if err := svc.RevokeSession(ctx, now.Add(1*time.Second), issued.SessionID); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}

	_, err = svc.RotateRefresh(ctx, now.Add(2*time.Second), issued.RefreshToken, dev)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err != ErrSessionRevoked {
		t.Fatalf("expected ErrSessionRevoked, got %v", err)
	}
}

func TestPostgresSession_ValidateAccessToken_Revoked(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbURL := os.Getenv("ARC_DATABASE_URL")
	if dbURL == "" {
		t.Skip("ARC_DATABASE_URL is not set; skipping Postgres integration test")
	}

	pool := mustPGXPool(ctx, t, dbURL)
	defer pool.Close()

	cfg, tokens := mustTestConfigAndTokens(t)
	store := NewPostgresStore(pool)
	svc := NewService(cfg, pool, store, tokens)

	userID := newULID(t)
	mustCreateUser(ctx, t, pool, userID)
	t.Cleanup(func() { cleanupUserData(ctx, t, pool, userID) })

	now := time.Now().UTC()
	dev := DeviceContext{Platform: PlatformWeb, RememberMe: false, UserAgent: "arc-test/1.0"}

	issued, err := svc.IssueSession(ctx, now, userID, dev)
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}

	if err := svc.RevokeSession(ctx, now.Add(1*time.Second), issued.SessionID); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}

	_, err = svc.ValidateAccessToken(ctx, issued.AccessToken, now.Add(2*time.Second))
	if err != ErrSessionRevoked {
		t.Fatalf("expected ErrSessionRevoked, got %v", err)
	}
}

func TestPostgresSession_ValidateAccessToken_Expired(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbURL := os.Getenv("ARC_DATABASE_URL")
	if dbURL == "" {
		t.Skip("ARC_DATABASE_URL is not set; skipping Postgres integration test")
	}

	pool := mustPGXPool(ctx, t, dbURL)
	defer pool.Close()

	cfg, tokens := mustTestConfigAndTokens(t)
	store := NewPostgresStore(pool)
	svc := NewService(cfg, pool, store, tokens)

	userID := newULID(t)
	mustCreateUser(ctx, t, pool, userID)
	t.Cleanup(func() { cleanupUserData(ctx, t, pool, userID) })

	now := time.Now().UTC()
	dev := DeviceContext{Platform: PlatformWeb, RememberMe: false, UserAgent: "arc-test/1.0"}

	issued, err := svc.IssueSession(ctx, now, userID, dev)
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}

	// Force expiry while respecting DB constraints.
	pastCreated := now.Add(-2 * time.Hour)
	pastLastUsed := now.Add(-90 * time.Minute)
	pastExpires := now.Add(-1 * time.Hour)
	_, err = pool.Exec(ctx, `
		UPDATE arc.sessions
		SET created_at = $1,
		    last_used_at = $2,
		    expires_at = $3
		WHERE id = $4
	`, pastCreated, pastLastUsed, pastExpires, issued.SessionID)
	if err != nil {
		t.Fatalf("expire session: %v", err)
	}

	_, err = svc.ValidateAccessToken(ctx, issued.AccessToken, now.Add(2*time.Second))
	if err != ErrSessionExpired {
		t.Fatalf("expected ErrSessionExpired, got %v", err)
	}
}

func TestPostgresSession_ValidateAccessToken_NotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbURL := os.Getenv("ARC_DATABASE_URL")
	if dbURL == "" {
		t.Skip("ARC_DATABASE_URL is not set; skipping Postgres integration test")
	}

	pool := mustPGXPool(ctx, t, dbURL)
	defer pool.Close()

	cfg, tokens := mustTestConfigAndTokens(t)
	store := NewPostgresStore(pool)
	svc := NewService(cfg, pool, store, tokens)

	userID := newULID(t)
	mustCreateUser(ctx, t, pool, userID)
	t.Cleanup(func() { cleanupUserData(ctx, t, pool, userID) })

	now := time.Now().UTC()
	dev := DeviceContext{Platform: PlatformWeb, RememberMe: false, UserAgent: "arc-test/1.0"}

	issued, err := svc.IssueSession(ctx, now, userID, dev)
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}

	_, err = pool.Exec(ctx, `DELETE FROM arc.sessions WHERE id = $1`, issued.SessionID)
	if err != nil {
		t.Fatalf("delete session: %v", err)
	}

	_, err = svc.ValidateAccessToken(ctx, issued.AccessToken, now.Add(2*time.Second))
	if err != ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestPostgresSession_ValidateAccessToken_UserMismatch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbURL := os.Getenv("ARC_DATABASE_URL")
	if dbURL == "" {
		t.Skip("ARC_DATABASE_URL is not set; skipping Postgres integration test")
	}

	pool := mustPGXPool(ctx, t, dbURL)
	defer pool.Close()

	cfg, tokens := mustTestConfigAndTokens(t)
	store := NewPostgresStore(pool)
	svc := NewService(cfg, pool, store, tokens)

	user1 := newULID(t)
	user2 := newULID(t)
	mustCreateUser(ctx, t, pool, user1)
	mustCreateUser(ctx, t, pool, user2)
	t.Cleanup(func() {
		cleanupUserData(ctx, t, pool, user1)
		cleanupUserData(ctx, t, pool, user2)
	})

	now := time.Now().UTC()
	dev := DeviceContext{Platform: PlatformWeb, RememberMe: false, UserAgent: "arc-test/1.0"}

	issued, err := svc.IssueSession(ctx, now, user1, dev)
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}

	_, err = pool.Exec(ctx, `UPDATE arc.sessions SET user_id = $1 WHERE id = $2`, user2, issued.SessionID)
	if err != nil {
		t.Fatalf("update session user_id: %v", err)
	}

	_, err = svc.ValidateAccessToken(ctx, issued.AccessToken, now.Add(2*time.Second))
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestPostgresSession_TouchSession_UpdatesLastUsed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbURL := os.Getenv("ARC_DATABASE_URL")
	if dbURL == "" {
		t.Skip("ARC_DATABASE_URL is not set; skipping Postgres integration test")
	}

	pool := mustPGXPool(ctx, t, dbURL)
	defer pool.Close()

	cfg, tokens := mustTestConfigAndTokens(t)
	store := NewPostgresStore(pool)
	svc := NewService(cfg, pool, store, tokens)

	userID := newULID(t)
	mustCreateUser(ctx, t, pool, userID)
	t.Cleanup(func() { cleanupUserData(ctx, t, pool, userID) })

	now := time.Now().UTC()
	dev := DeviceContext{Platform: PlatformWeb, RememberMe: false, UserAgent: "arc-test/1.0"}

	issued, err := svc.IssueSession(ctx, now, userID, dev)
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}

	next := now.Add(30 * time.Second)
	if err := svc.TouchSession(ctx, next, issued.SessionID); err != nil {
		t.Fatalf("TouchSession: %v", err)
	}

	row := mustGetSessionByID(ctx, t, pool, issued.SessionID)
	if row.LastUsedAt == nil {
		t.Fatalf("expected last_used_at set, got nil")
	}
	// Postgres timestamps are microsecond-precision; compare at that granularity.
	got := row.LastUsedAt.UTC().Truncate(time.Microsecond)
	want := next.UTC().Truncate(time.Microsecond)
	if !got.Equal(want) {
		t.Fatalf("expected last_used_at=%v, got %v", want, got)
	}
}

func mustPGXPool(ctx context.Context, t *testing.T, dbURL string) *pgxpool.Pool {
	t.Helper()

	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("pgxpool.ParseConfig: %v", err)
	}

	cfg.MaxConns = 4
	cfg.MinConns = 0
	cfg.MaxConnLifetime = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pgxpool.NewWithConfig: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		if shouldSkipIntegration(err) {
			t.Skipf("integration test skipped: Postgres unreachable (ARC_DATABASE_URL set): %v", err)
		}
		t.Fatalf("pool.Ping: %v", err)
	}

	return pool
}

func mustTestConfigAndTokens(t *testing.T) (Config, AccessTokenManager) {
	t.Helper()

	secret := paseto.NewV4AsymmetricSecretKey()

	cfg := DefaultConfig()
	cfg.PasetoV4SecretKeyHex = secret.ExportHex()

	tokens, err := NewPasetoV4PublicManager(cfg)
	if err != nil {
		t.Fatalf("NewPasetoV4PublicManager: %v", err)
	}

	return cfg, tokens
}

func shouldSkipIntegration(err error) bool {
	if err == nil {
		return false
	}
	if os.Getenv("CI") != "" {
		return false
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "dial tcp") ||
		strings.Contains(msg, "no such host") {
		return true
	}
	return false
}

func newULID(t *testing.T) string {
	t.Helper()

	entropy := ulid.Monotonic(rand.Reader, 0)

	id := ulid.MustNew(ulid.Timestamp(time.Now().UTC()), entropy).String()
	if len(id) != 26 {
		t.Fatalf("expected ULID length 26, got %d", len(id))
	}
	return id
}

func mustCreateUser(ctx context.Context, t *testing.T, pool *pgxpool.Pool, userID string) {
	t.Helper()

	_, err := pool.Exec(ctx, `
		INSERT INTO arc.users (id, created_at, updated_at)
		VALUES ($1, now(), now())
	`, userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

func cleanupUserData(ctx context.Context, t *testing.T, pool *pgxpool.Pool, userID string) {
	t.Helper()

	_, _ = pool.Exec(ctx, `DELETE FROM arc.sessions WHERE user_id = $1`, userID)
	_, _ = pool.Exec(ctx, `DELETE FROM arc.user_credentials WHERE user_id = $1`, userID)
	_, _ = pool.Exec(ctx, `DELETE FROM arc.users WHERE id = $1`, userID)
}

func mustGetSessionByID(ctx context.Context, t *testing.T, pool *pgxpool.Pool, sessionID string) Row {
	t.Helper()

	var row Row
	err := pool.QueryRow(ctx, `
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
	if err != nil {
		t.Fatalf("select session by id=%q: %v", sessionID, err)
	}
	return row
}
