package app

import (
	"log/slog"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want slog.Level
	}{
		{in: "debug", want: slog.LevelDebug},
		{in: "INFO", want: slog.LevelInfo},
		{in: "warn", want: slog.LevelWarn},
		{in: "warning", want: slog.LevelWarn},
		{in: "error", want: slog.LevelError},
		{in: "unknown", want: slog.LevelInfo},
		{in: "", want: slog.LevelInfo},
	}

	for _, tc := range cases {
		got := parseLogLevel(tc.in)
		if got != tc.want {
			t.Fatalf("parseLogLevel(%q)=%v want=%v", tc.in, got, tc.want)
		}
	}
}

func TestLevelTag_NoColor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		level slog.Level
		want  string
	}{
		{level: slog.LevelDebug, want: "[DEBUG]"},
		{level: slog.LevelInfo, want: "[INFO]"},
		{level: slog.LevelWarn, want: "[WARN]"},
		{level: slog.LevelError, want: "[ERROR]"},
	}

	for _, tc := range cases {
		got := levelTag(tc.level, false)
		if got != tc.want {
			t.Fatalf("levelTag(%v,false)=%q want=%q", tc.level, got, tc.want)
		}
	}
}
