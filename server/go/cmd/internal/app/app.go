// Package app wires the Arc server runtime: config, logging, HTTP routes, and realtime gateways.
//
// It is intentionally small and deterministic to keep CI gates strict and behavior predictable.
package app

import (
	"context"
	"errors"
	"net/http"
	"time"

	"arc/cmd/internal/auth/api"
	"arc/cmd/internal/auth/session"
	"arc/cmd/internal/realtime"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is a small app-level lifecycle abstraction.
// It exists to allow DB-backed resources to be closed gracefully.
type Store interface {
	Close(ctx context.Context) error
}

// nopStore is used for in-memory store mode.
type nopStore struct{}

func (nopStore) Close(_ context.Context) error { return nil }

// App is the Arc server runtime: it owns HTTP server wiring and realtime gateway dependencies.
type App struct {
	cfg Config
	log Logger

	store Store

	dbPool    *pgxpool.Pool
	dbEnabled bool

	ws *realtime.WSGateway

	auth *api.Handler
}

// New constructs a fully wired App instance from config and logger.
func New(cfg Config, log Logger) (*App, error) {
	if log == nil {
		log = NewLogger(cfg.LogLevel)
	}

	st, dbPool, dbEnabled, msgStore, err := newStore(context.Background(), cfg, log)
	if err != nil {
		return nil, err
	}

	var authHandler *api.Handler
	var sessionSvc *session.Service
	var memberStore realtime.MembershipStore

	if dbEnabled {
		sessCfg, err := session.LoadConfigFromEnv()
		if err != nil {
			return nil, err
		}
		authCfg := api.LoadConfigFromEnv()
		authHandler, err = api.NewHandler(log, dbPool, authCfg, sessCfg, dbEnabled)
		if err != nil {
			return nil, err
		}
		sessionSvc = authHandler.SessionService()

		members, err := realtime.NewPostgresMembershipStore(dbPool)
		if err != nil {
			return nil, err
		}
		memberStore = members
	}

	ws := realtime.NewWSGateway(log, realtime.NewHub(log), msgStore, sessionSvc, memberStore)

	return &App{
		cfg:       cfg,
		log:       log,
		store:     st,
		dbPool:    dbPool,
		dbEnabled: dbEnabled,
		ws:        ws,
		auth:      authHandler,
	}, nil
}

// Run starts the HTTP server and blocks until context cancellation or fatal server error.
func (a *App) Run(ctx context.Context) error {
	mux := http.NewServeMux()

	// Use the canonical HTTP registration from http.go (so it is not "unused").
	registerHTTP(mux, a.log, a.cfg, a.dbPool, a.dbEnabled, a.ws, a.auth)

	srv := &http.Server{
		Addr:              a.cfg.HTTPAddr,
		Handler:           WithRequestLogging(mux, a.log),
		ReadHeaderTimeout: nonZeroDuration(a.cfg.ReadHeaderTimeout, 5*time.Second),
		ReadTimeout:       nonZeroDuration(a.cfg.ReadTimeout, 15*time.Second),
		WriteTimeout:      nonZeroDuration(a.cfg.WriteTimeout, 15*time.Second),
		IdleTimeout:       nonZeroDuration(a.cfg.IdleTimeout, 60*time.Second),
		MaxHeaderBytes:    nonZeroInt(a.cfg.MaxHeaderBytes, 1<<20),
	}

	a.log.Info("server.start", "addr", a.cfg.HTTPAddr, "db_enabled", a.dbEnabled)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		a.log.Info("server.stop", "reason", "context_done")
	case err := <-errCh:
		a.log.Error("server.fail", "err", err)
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		a.log.Error("server.shutdown.fail", "err", err)
		return err
	}

	// Close store resources (pool etc).
	if err := a.store.Close(shutdownCtx); err != nil {
		a.log.Error("store.close.fail", "err", err)
	}

	a.log.Info("server.stopped")
	return nil
}

func nonZeroDuration(v, def time.Duration) time.Duration {
	if v <= 0 {
		return def
	}
	return v
}

func nonZeroInt(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

// newStore decides between Postgres-backed persistence and in-memory dev store.
func newStore(ctx context.Context, cfg Config, log Logger) (Store, *pgxpool.Pool, bool, realtime.MessageStore, error) {
	if cfg.DatabaseURL == "" {
		log.Info("db.disabled.inmemory_store")
		return nopStore{}, nil, false, realtime.NewInMemoryStore(), nil
	}

	pool, err := NewDBPool(ctx, cfg)
	if err != nil {
		return nil, nil, false, nil, err
	}

	log.Info("db.enabled.postgres_store")

	// Ownership model:
	// - app owns pool lifecycle
	// - PostgresStore.Close() is a no-op
	msgStore, err := realtime.NewPostgresStore(pool) // default schema "arc"
	if err != nil {
		pool.Close()
		return nil, nil, false, nil, err
	}

	return dbStore{pool: pool, msgStore: msgStore}, pool, true, msgStore, nil
}

type dbStore struct {
	pool     *pgxpool.Pool
	msgStore realtime.MessageStore
}

func (s dbStore) Close(_ context.Context) error {
	// MessageStore may have its own resources in the future.
	// Current PostgresStore.Close() is a no-op by design (pool is owned here).
	if s.msgStore != nil {
		_ = s.msgStore.Close()
	}
	if s.pool != nil {
		s.pool.Close()
	}
	return nil
}
