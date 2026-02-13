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
