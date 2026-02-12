package identity

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Integration tests are opt-in and require ARC_DATABASE_URL.
// In non-CI runs, unreachable Postgres skips these tests to keep local runs fast.

func TestPostgresStore_CreateUser_ConflictUsername_CaseInsensitive(t *testing.T) {
	t.Parallel()

	pool := mustOpenTestPool(t)
	defer pool.Close()

	schema := mustCreateTestSchema(t, pool)
	t.Cleanup(func() { mustDropSchema(t, pool, schema) })
	mustApplyIdentitySchema(t, pool, schema)

	s := mustNewIdentityStore(t, pool, schema)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	u1 := "Navid"
	_, err := s.CreateUser(ctx, CreateUserInput{
		Username: &u1,
		Email:    nil,
		Password: "very-strong-password-1",
		Now:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create user 1: %v", err)
	}

	// Same username (case-insensitive) should conflict.
	u2 := "nAvId"
	_, err = s.CreateUser(ctx, CreateUserInput{
		Username: &u2,
		Email:    nil,
		Password: "very-strong-password-2",
		Now:      time.Now().UTC(),
	})
	if err == nil {
		t.Fatalf("expected conflict, got nil")
	}
	if !IsConflict(err) {
		t.Fatalf("expected conflict error, got: %v", err)
	}
}

func TestPostgresStore_CreateUser_ConflictEmail_CaseInsensitive(t *testing.T) {
	t.Parallel()

	pool := mustOpenTestPool(t)
	defer pool.Close()

	schema := mustCreateTestSchema(t, pool)
	t.Cleanup(func() { mustDropSchema(t, pool, schema) })
	mustApplyIdentitySchema(t, pool, schema)

	s := mustNewIdentityStore(t, pool, schema)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	e1 := "User@Example.com"
	_, err := s.CreateUser(ctx, CreateUserInput{
		Username: nil,
		Email:    &e1,
		Password: "very-strong-password-11",
		Now:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create user 1: %v", err)
	}

	// Same email (case-insensitive) should conflict.
	e2 := "user@example.COM"
	_, err = s.CreateUser(ctx, CreateUserInput{
		Username: nil,
		Email:    &e2,
		Password: "very-strong-password-12",
		Now:      time.Now().UTC(),
	})
	if err == nil {
		t.Fatalf("expected conflict, got nil")
	}
	if !IsConflict(err) {
		t.Fatalf("expected conflict error, got: %v", err)
	}
}

func TestPostgresStore_CreateSession_WithIPAndUA(t *testing.T) {
	t.Parallel()

	pool := mustOpenTestPool(t)
	defer pool.Close()

	schema := mustCreateTestSchema(t, pool)
	t.Cleanup(func() { mustDropSchema(t, pool, schema) })
	mustApplyIdentitySchema(t, pool, schema)

	s := mustNewIdentityStore(t, pool, schema)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	u := "session-user-" + strings.ToLower(mustNewULIDLike(t))
	res, err := s.CreateUser(ctx, CreateUserInput{
		Username: &u,
		Email:    nil,
		Password: "very-strong-password-3",
		Now:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	ua := "arc-test-agent/1.0"
	ip := net.ParseIP("127.0.0.1")
	if ip == nil {
		t.Fatalf("parse ip failed")
	}

	out, err := s.CreateSession(ctx, CreateSessionInput{
		UserID:    res.User.ID,
		TTL:       24 * time.Hour,
		Platform:  "web",
		UserAgent: &ua,
		IP:        &ip,
		Now:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if out.Session.ID == "" {
		t.Fatalf("expected session id")
	}
	if out.RefreshToken == "" {
		t.Fatalf("expected refresh token")
	}
	if len(out.Session.RefreshTokenHash) != 64 {
		t.Fatalf("expected refresh hash len=64 got=%d", len(out.Session.RefreshTokenHash))
	}
	if out.Session.UserAgent == nil || *out.Session.UserAgent != ua {
		t.Fatalf("expected user agent=%q got=%v", ua, out.Session.UserAgent)
	}
	if out.Session.IP == nil || out.Session.IP.String() != "127.0.0.1" {
		t.Fatalf("expected ip=127.0.0.1 got=%v", out.Session.IP)
	}
}

func TestPostgresStore_RotateRefreshToken_Success_ThenRevoke(t *testing.T) {
	t.Parallel()

	pool := mustOpenTestPool(t)
	defer pool.Close()

	schema := mustCreateTestSchema(t, pool)
	t.Cleanup(func() { mustDropSchema(t, pool, schema) })
	mustApplyIdentitySchema(t, pool, schema)

	s := mustNewIdentityStore(t, pool, schema)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	u := "rotate-user-" + strings.ToLower(mustNewULIDLike(t))
	res, err := s.CreateUser(ctx, CreateUserInput{
		Username: &u,
		Email:    nil,
		Password: "very-strong-password-4",
		Now:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	sess, err := s.CreateSession(ctx, CreateSessionInput{
		UserID:   res.User.ID,
		TTL:      24 * time.Hour,
		Platform: "web",
		Now:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	plain2, hash2, err := s.RotateRefreshToken(ctx, sess.Session.ID, sess.RefreshToken, time.Now().UTC())
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if plain2 == "" || hash2 == "" {
		t.Fatalf("rotate: empty outputs")
	}
	if hash2 == sess.Session.RefreshTokenHash {
		t.Fatalf("rotate: hash did not change")
	}

	// Old token must no longer work (old session is revoked + replaced).
	_, _, err = s.RotateRefreshToken(ctx, sess.Session.ID, sess.RefreshToken, time.Now().UTC())
	if err == nil {
		t.Fatalf("expected ErrNotActive on old token reuse")
	}
	if !errors.Is(err, ErrNotActive) {
		t.Fatalf("expected ErrNotActive, got: %v", err)
	}

	if err := s.RevokeSession(ctx, sess.Session.ID, time.Now().UTC()); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	_, _, err = s.RotateRefreshToken(ctx, sess.Session.ID, plain2, time.Now().UTC())
	if err == nil {
		t.Fatalf("expected ErrNotActive after revoke")
	}
	if !errors.Is(err, ErrNotActive) {
		t.Fatalf("expected ErrNotActive, got: %v", err)
	}
}

func TestPostgresStore_RotateRefreshToken_WrongToken_ReturnsNotActive(t *testing.T) {
	t.Parallel()

	pool := mustOpenTestPool(t)
	defer pool.Close()

	schema := mustCreateTestSchema(t, pool)
	t.Cleanup(func() { mustDropSchema(t, pool, schema) })
	mustApplyIdentitySchema(t, pool, schema)

	s := mustNewIdentityStore(t, pool, schema)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	u := "wrongtok-user-" + strings.ToLower(mustNewULIDLike(t))
	res, err := s.CreateUser(ctx, CreateUserInput{
		Username: &u,
		Email:    nil,
		Password: "very-strong-password-5",
		Now:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	sess, err := s.CreateSession(ctx, CreateSessionInput{
		UserID:   res.User.ID,
		TTL:      24 * time.Hour,
		Platform: "web",
		Now:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	_, _, err = s.RotateRefreshToken(ctx, sess.Session.ID, "definitely-not-the-real-token", time.Now().UTC())
	if err == nil {
		t.Fatalf("expected ErrNotActive on mismatch")
	}
	if !errors.Is(err, ErrNotActive) {
		t.Fatalf("expected ErrNotActive, got: %v", err)
	}
}

func TestPostgresStore_InviteConsume_Succeeds_ThenRejectsReuse(t *testing.T) {
	t.Parallel()

	pool := mustOpenTestPool(t)
	defer pool.Close()

	schema := mustCreateTestSchema(t, pool)
	t.Cleanup(func() { mustDropSchema(t, pool, schema) })
	mustApplyIdentitySchema(t, pool, schema)

	s := mustNewIdentityStore(t, pool, schema)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	inv, err := s.CreateInvite(ctx, CreateInviteInput{
		CreatedBy: nil,
		TTL:       24 * time.Hour,
		Now:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	if inv.Token == "" || inv.Invite.ID == "" {
		t.Fatalf("expected invite token and id")
	}

	u := "invite-user-" + strings.ToLower(mustNewULIDLike(t))
	out, err := s.ConsumeInviteAndCreateUser(ctx, ConsumeInviteInput{
		Token:      inv.Token,
		Username:   &u,
		Email:      nil,
		Password:   "very-strong-password-8",
		Now:        time.Now().UTC(),
		SessionTTL: 24 * time.Hour,
		Platform:   "web",
		UserAgent:  nil,
		IP:         nil,
	})
	if err != nil {
		t.Fatalf("consume invite: %v", err)
	}
	if out.User.ID == "" || out.Session.ID == "" || out.RefreshToken == "" {
		t.Fatalf("expected user, session, refresh token")
	}
	if out.Invite.ID != inv.Invite.ID {
		t.Fatalf("expected invite id %q, got %q", inv.Invite.ID, out.Invite.ID)
	}

	// Reuse should fail.
	_, err = s.ConsumeInviteAndCreateUser(ctx, ConsumeInviteInput{
		Token:      inv.Token,
		Username:   &u,
		Email:      nil,
		Password:   "very-strong-password-9",
		Now:        time.Now().UTC(),
		SessionTTL: 24 * time.Hour,
		Platform:   "web",
		UserAgent:  nil,
		IP:         nil,
	})
	if err == nil || !errors.Is(err, ErrNotActive) {
		t.Fatalf("expected ErrNotActive on invite reuse, got: %v", err)
	}
}

func TestPostgresStore_RotateRefreshToken_ExpiredSession_ReturnsNotActive(t *testing.T) {
	t.Parallel()

	pool := mustOpenTestPool(t)
	defer pool.Close()

	schema := mustCreateTestSchema(t, pool)
	t.Cleanup(func() { mustDropSchema(t, pool, schema) })
	mustApplyIdentitySchema(t, pool, schema)

	s := mustNewIdentityStore(t, pool, schema)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	u := "expired-user-" + strings.ToLower(mustNewULIDLike(t))
	res, err := s.CreateUser(ctx, CreateUserInput{
		Username: &u,
		Email:    nil,
		Password: "very-strong-password-6",
		Now:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	sess, err := s.CreateSession(ctx, CreateSessionInput{
		UserID:   res.User.ID,
		TTL:      1 * time.Hour,
		Platform: "web",
		Now:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Force expiry in DB for this session only.
	// Keep constraint: expires_at > created_at.
	sessions := pgIdent(schema, "sessions")
	mustExec(t, pool,
		`UPDATE `+sessions+`
		    SET created_at = now() - interval '2 hours',
		        expires_at = now() - interval '1 second'
		  WHERE id = $1`,
		sess.Session.ID,
	)

	_, _, err = s.RotateRefreshToken(ctx, sess.Session.ID, sess.RefreshToken, time.Now().UTC())
	if err == nil {
		t.Fatalf("expected ErrNotActive after expiry")
	}
	if !errors.Is(err, ErrNotActive) {
		t.Fatalf("expected ErrNotActive, got: %v", err)
	}
}

func TestPostgresStore_RevokeAllSessions_Idempotent(t *testing.T) {
	t.Parallel()

	pool := mustOpenTestPool(t)
	defer pool.Close()

	schema := mustCreateTestSchema(t, pool)
	t.Cleanup(func() { mustDropSchema(t, pool, schema) })
	mustApplyIdentitySchema(t, pool, schema)

	s := mustNewIdentityStore(t, pool, schema)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	u := "revokeall-user-" + strings.ToLower(mustNewULIDLike(t))
	res, err := s.CreateUser(ctx, CreateUserInput{
		Username: &u,
		Email:    nil,
		Password: "very-strong-password-7",
		Now:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Create a couple sessions.
	s1, err := s.CreateSession(ctx, CreateSessionInput{UserID: res.User.ID, TTL: 24 * time.Hour, Platform: "web", Now: time.Now().UTC()})
	if err != nil {
		t.Fatalf("create session 1: %v", err)
	}
	s2, err := s.CreateSession(ctx, CreateSessionInput{UserID: res.User.ID, TTL: 24 * time.Hour, Platform: "web", Now: time.Now().UTC()})
	if err != nil {
		t.Fatalf("create session 2: %v", err)
	}

	now := time.Now().UTC()
	if err := s.RevokeAllSessions(ctx, res.User.ID, now); err != nil {
		t.Fatalf("revoke all: %v", err)
	}

	// Rotations must fail for both.
	_, _, err = s.RotateRefreshToken(ctx, s1.Session.ID, s1.RefreshToken, time.Now().UTC())
	if err == nil || !errors.Is(err, ErrNotActive) {
		t.Fatalf("expected ErrNotActive for session 1, got: %v", err)
	}

	_, _, err = s.RotateRefreshToken(ctx, s2.Session.ID, s2.RefreshToken, time.Now().UTC())
	if err == nil || !errors.Is(err, ErrNotActive) {
		t.Fatalf("expected ErrNotActive for session 2, got: %v", err)
	}

	// Idempotent second call must not error.
	if err := s.RevokeAllSessions(ctx, res.User.ID, time.Now().UTC()); err != nil {
		t.Fatalf("revoke all (second call): %v", err)
	}
}

// ---- helpers ----

func mustNewIdentityStore(t *testing.T, pool *pgxpool.Pool, schema string) *PostgresStore {
	t.Helper()
	s, err := NewPostgresStore(pool, WithSchema(schema))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return s
}

func mustOpenTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	raw := strings.TrimSpace(os.Getenv("ARC_DATABASE_URL"))
	if raw == "" {
		t.Skip("integration test skipped: ARC_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(raw)
	if err != nil {
		t.Fatalf("parse ARC_DATABASE_URL: %v", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}

	// Validate acquire quickly (fast fail).
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer pingCancel()

	c, err := pool.Acquire(pingCtx)
	if err != nil {
		pool.Close()
		if shouldSkipIntegration(err) {
			t.Skipf("integration test skipped: Postgres unreachable (ARC_DATABASE_URL set): %v", err)
		}
		t.Fatalf("acquire: %v", err)
	}
	c.Release()

	return pool
}

func mustCreateTestSchema(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()

	schema := "arc_it_" + strings.ToLower(mustNewULIDLike(t))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, `CREATE SCHEMA `+pgxIdent1(schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return schema
}

func mustDropSchema(t *testing.T, pool *pgxpool.Pool, schema string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, _ = pool.Exec(ctx, `DROP SCHEMA IF EXISTS `+pgxIdent1(schema)+` CASCADE`)
}

func mustApplyIdentitySchema(t *testing.T, pool *pgxpool.Pool, schema string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	users := pgIdent(schema, "users")
	creds := pgIdent(schema, "user_credentials")
	sessions := pgIdent(schema, "sessions")
	invites := pgIdent(schema, "invites")

	schemaSQL := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
  id TEXT PRIMARY KEY,
  username TEXT NULL,
  username_norm TEXT NULL,
  email TEXT NULL,
  email_norm TEXT NULL,
  display_name TEXT NULL,
  bio TEXT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

  CONSTRAINT chk_users_id_ulid_len CHECK (char_length(id) = 26),
  CONSTRAINT uq_users_username_norm UNIQUE (username_norm),
  CONSTRAINT uq_users_email_norm UNIQUE (email_norm)
);

CREATE TABLE IF NOT EXISTS %s (
  user_id TEXT PRIMARY KEY REFERENCES %s(id) ON DELETE CASCADE,
  password_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS %s (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES %s(id) ON DELETE CASCADE,
  refresh_token_hash TEXT NOT NULL,

  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_used_at TIMESTAMPTZ NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ NULL,

  replaced_by_session_id TEXT NULL REFERENCES %s(id) ON DELETE SET NULL,

  user_agent TEXT NULL,
  ip INET NULL,
  platform TEXT NOT NULL DEFAULT 'unknown',

  CONSTRAINT chk_sessions_id_ulid_len CHECK (char_length(id) = 26),
  CONSTRAINT chk_sessions_user_id_ulid_len CHECK (char_length(user_id) = 26),
  CONSTRAINT chk_sessions_refresh_hash_len CHECK (char_length(refresh_token_hash) = 64),

  CONSTRAINT uq_sessions_refresh_token_hash UNIQUE (refresh_token_hash),
  CONSTRAINT chk_sessions_expires_after_created CHECK (expires_at > created_at),
  CONSTRAINT chk_sessions_platform CHECK (platform IN ('web', 'ios', 'android', 'desktop', 'unknown')),
  CONSTRAINT chk_sessions_replaced_not_self CHECK (replaced_by_session_id IS NULL OR replaced_by_session_id <> id)
);

CREATE TABLE IF NOT EXISTS %s (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL,
  created_by TEXT NULL REFERENCES %s(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ NULL,
  consumed_by TEXT NULL REFERENCES %s(id) ON DELETE SET NULL,
  CONSTRAINT chk_invites_id_ulid_len CHECK (char_length(id) = 26),
  CONSTRAINT chk_invites_token_hash_len CHECK (char_length(token_hash) = 64)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_invites_token_hash ON %s (token_hash);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id
  ON %s (user_id);

CREATE INDEX IF NOT EXISTS idx_sessions_expires_at
  ON %s (expires_at);

CREATE INDEX IF NOT EXISTS idx_sessions_replaced_by
  ON %s (replaced_by_session_id);
`, users, creds, users, sessions, users, sessions, invites, users, users, invites, sessions, sessions, sessions)

	if _, err := pool.Exec(ctx, schemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
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

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("exec failed: %v", err)
	}
}

func mustNewULIDLike(t *testing.T) string {
	t.Helper()

	id, err := NewULID(time.Now().UTC())
	if err != nil {
		t.Fatalf("ulid: %v", err)
	}
	return id
}

func pgxIdent1(ident string) string {
	// pgx.Identifier safely quotes identifiers, preventing SQL injection in dynamic DDL.
	return pgx.Identifier{ident}.Sanitize()
}
