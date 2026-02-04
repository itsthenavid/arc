package app

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewDBPool builds a pgxpool with sane defaults and validates connectivity.
// Note: it does NOT run migrations; schema management is handled by Atlas.
func NewDBPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	pcfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	if cfg.DBMaxConns > 0 {
		pcfg.MaxConns = cfg.DBMaxConns
	}
	if cfg.DBMinConns >= 0 {
		pcfg.MinConns = cfg.DBMinConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		return nil, err
	}

	if err := PingDB(ctx, pool, 3*time.Second); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}

// PingDB checks if we can acquire a connection within timeout.
func PingDB(parent context.Context, pool *pgxpool.Pool, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	conn.Release()
	return nil
}
