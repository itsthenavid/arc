package invite

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

// Integration tests are enabled when ARC_DATABASE_URL is set.
// In non-CI runs, unreachable Postgres skips these tests to keep local runs fast.

func TestInviteService_CreateValidateConsume(t *testing.T) {
	t.Parallel()

	pool := mustOpenTestPool(t)
	defer pool.Close()

	schema := mustCreateTestSchema(t, pool)
	t.Cleanup(func() { mustDropSchema(t, pool, schema) })
	mustApplySchema(t, pool, schema)

	store, err := NewPostgresStore(pool, WithSchema(schema))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	service, err := NewService(store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	creator := newTestULID(t)
	mustInsertUser(t, pool, schema, creator)

	inv, token, err := service.CreateInvite(ctx, CreateInput{
		CreatedBy: &creator,
		TTL:       24 * time.Hour,
		MaxUses:   1,
		Now:       now,
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	if inv.ID == "" || token == "" {
		t.Fatalf("expected invite id and token")
	}

	ok, _, err := service.ValidateInvite(ctx, token, now)
	if err != nil {
		t.Fatalf("validate invite: %v", err)
	}
	if !ok {
		t.Fatalf("expected invite to be valid")
	}

	consumer := newTestULID(t)
	mustInsertUser(t, pool, schema, consumer)
	consumed, err := service.ConsumeInvite(ctx, ConsumeInput{Token: token, ConsumedBy: &consumer, Now: now.Add(1 * time.Second)})
	if err != nil {
		t.Fatalf("consume invite: %v", err)
	}
	if consumed.UsedCount != 1 {
		t.Fatalf("expected used_count=1, got %d", consumed.UsedCount)
	}

	ok, _, err = service.ValidateInvite(ctx, token, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("validate after consume: %v", err)
	}
	if ok {
		t.Fatalf("expected invite to be invalid after max uses")
	}
}

func TestInviteService_Validate_ExpiredRevoked(t *testing.T) {
	t.Parallel()

	pool := mustOpenTestPool(t)
	defer pool.Close()

	schema := mustCreateTestSchema(t, pool)
	t.Cleanup(func() { mustDropSchema(t, pool, schema) })
	mustApplySchema(t, pool, schema)

	store, err := NewPostgresStore(pool, WithSchema(schema))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	service, err := NewService(store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	ctx := context.Background()

	expired, token, err := service.CreateInvite(ctx, CreateInput{
		TTL:     1 * time.Hour,
		MaxUses: 1,
		Now:     time.Now().UTC().Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("create expired invite: %v", err)
	}
	ok, _, err := service.ValidateInvite(ctx, token, time.Now().UTC())
	if err != nil {
		t.Fatalf("validate expired invite: %v", err)
	}
	if ok {
		t.Fatalf("expected expired invite to be invalid")
	}

	invites := pgIdent(schema, "invites")
	if _, err := pool.Exec(ctx, `UPDATE `+invites+` SET revoked_at = $1 WHERE id = $2`, time.Now().UTC(), expired.ID); err != nil {
		t.Fatalf("revoke invite: %v", err)
	}
	ok, _, err = service.ValidateInvite(ctx, token, time.Now().UTC())
	if err != nil {
		t.Fatalf("validate revoked invite: %v", err)
	}
	if ok {
		t.Fatalf("expected revoked invite to be invalid")
	}
}

func TestInviteService_ConcurrentConsume_MaxUses(t *testing.T) {
	t.Parallel()

	pool := mustOpenTestPool(t)
	defer pool.Close()

	schema := mustCreateTestSchema(t, pool)
	t.Cleanup(func() { mustDropSchema(t, pool, schema) })
	mustApplySchema(t, pool, schema)

	store, err := NewPostgresStore(pool, WithSchema(schema))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	service, err := NewService(store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	ctx := context.Background()

	inv, token, err := service.CreateInvite(ctx, CreateInput{
		TTL:     24 * time.Hour,
		MaxUses: 2,
		Now:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	if inv.ID == "" || token == "" {
		t.Fatalf("expected invite id and token")
	}

	const attempts = 5
	var wg sync.WaitGroup
	wg.Add(attempts)
	errs := make(chan error, attempts)

	for i := 0; i < attempts; i++ {
		go func() {
			defer wg.Done()
			userID := newTestULID(t)
			mustInsertUser(t, pool, schema, userID)
			_, err := service.ConsumeInvite(ctx, ConsumeInput{Token: token, ConsumedBy: &userID, Now: time.Now().UTC()})
			if err != nil {
				errs <- err
				return
			}
			errs <- nil
		}()
	}

	wg.Wait()
	close(errs)

	success := 0
	for err := range errs {
		if err == nil {
			success++
			continue
		}
		if errors.Is(err, ErrNotActive) || errors.Is(err, ErrNotFound) {
			continue
		}
		t.Fatalf("unexpected error: %v", err)
	}
	if success != 2 {
		t.Fatalf("expected 2 successes, got %d", success)
	}
}

// ---- helpers ----

func mustOpenTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	raw := strings.TrimSpace(os.Getenv("ARC_DATABASE_URL"))
	if raw == "" {
		t.Skip("integration test skipped: ARC_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(raw)
	if err != nil {
		t.Fatalf("parse ARC_DATABASE_URL: %v", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}

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

func mustCreateTestSchema(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()

	schema := "arc_invite_it_" + strings.ToLower(newTestULID(t))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return schema
}

func mustDropSchema(t *testing.T, pool *pgxpool.Pool, schema string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, _ = pool.Exec(ctx, `DROP SCHEMA IF EXISTS `+pgx.Identifier{schema}.Sanitize()+` CASCADE`)
}

func mustApplySchema(t *testing.T, pool *pgxpool.Pool, schema string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	users := pgIdent(schema, "users")
	invites := pgIdent(schema, "invites")

	schemaSQL := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
  id TEXT PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS %s (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL,
  created_by TEXT NULL REFERENCES %s(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL,
  max_uses INT NOT NULL DEFAULT 1,
  used_count INT NOT NULL DEFAULT 0,
  revoked_at TIMESTAMPTZ NULL,
  note TEXT NULL,
  consumed_at TIMESTAMPTZ NULL,
  consumed_by TEXT NULL REFERENCES %s(id) ON DELETE SET NULL,
  CONSTRAINT chk_invites_id_ulid_len CHECK (char_length(id) = 26),
  CONSTRAINT chk_invites_token_hash_len CHECK (char_length(token_hash) = 64),
  CONSTRAINT chk_invites_max_uses CHECK (max_uses >= 1),
  CONSTRAINT chk_invites_used_count CHECK (used_count >= 0 AND used_count <= max_uses)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_invites_token_hash ON %s (token_hash);
`, users, invites, users, users, invites)

	if _, err := pool.Exec(ctx, schemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
}

func mustInsertUser(t *testing.T, pool *pgxpool.Pool, schema, userID string) {
	t.Helper()
	if strings.TrimSpace(userID) == "" {
		t.Fatalf("missing userID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	users := pgIdent(schema, "users")
	if _, err := pool.Exec(ctx, `INSERT INTO `+users+` (id, created_at) VALUES ($1, now())`, userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

func newTestULID(t *testing.T) string {
	t.Helper()
	id := ulid.MustNew(ulid.Timestamp(time.Now().UTC()), ulid.Monotonic(rand.Reader, 0)).String()
	if len(id) != 26 {
		t.Fatalf("expected ULID length 26, got %d", len(id))
	}
	return id
}
