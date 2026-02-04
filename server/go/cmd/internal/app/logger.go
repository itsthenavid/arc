package app

import (
	"log/slog"
	"os"
	"strings"
)

// Logger is the app-wide logger type (slog).
type Logger = *slog.Logger

// NewLogger creates a JSON structured logger with an explicit log level.
func NewLogger(level string) *slog.Logger {
	lvl := slog.LevelInfo

	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}

	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     lvl,
		AddSource: true,
	})

	log := slog.New(h)
	slog.SetDefault(log)
	return log
}
