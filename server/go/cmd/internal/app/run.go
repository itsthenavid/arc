package app

import (
	"context"
	"os/signal"
	"syscall"
)

// Run is the CLI entrypoint used by cmd/arc.
// It returns an error instead of calling os.Exit to keep defers effective and lint clean.
func Run() error {
	cfg := LoadConfig()
	log := NewLogger(cfg.LogLevel)

	a, err := New(cfg, log)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	return a.Run(ctx)
}
