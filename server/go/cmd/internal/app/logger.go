package app

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Logger is the app-wide logger type (slog).
type Logger = *slog.Logger

// NewLogger creates an app logger with configurable level + format.
//
// ARC_LOG_FORMAT options:
// - "auto"   : pretty colored text on TTY, JSON otherwise (default)
// - "pretty" : human-friendly colored text
// - "text"   : slog text
// - "json"   : structured JSON
func NewLogger(level string, format string) *slog.Logger {
	lvl := parseLogLevel(level)
	h := newHandler(lvl, format)

	log := slog.New(h)
	slog.SetDefault(log)
	return log
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func newHandler(level slog.Level, format string) slog.Handler {
	out := os.Stdout
	format = strings.ToLower(strings.TrimSpace(format))
	color := isLikelyTerminal(out)

	if format == "" || format == "auto" {
		if color {
			format = "pretty"
		} else {
			format = "json"
		}
	}

	switch format {
	case "pretty":
		return slog.NewTextHandler(out, &slog.HandlerOptions{
			Level:     level,
			AddSource: level <= slog.LevelDebug,
			ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
				return replacePrettyAttr(a, color)
			},
		})
	case "text":
		return slog.NewTextHandler(out, &slog.HandlerOptions{
			Level:     level,
			AddSource: level <= slog.LevelDebug,
			ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
				return replaceTextAttr(a)
			},
		})
	default: // json
		return slog.NewJSONHandler(out, &slog.HandlerOptions{
			Level:     level,
			AddSource: true,
		})
	}
}

func replaceTextAttr(a slog.Attr) slog.Attr {
	switch a.Key {
	case slog.TimeKey:
		if t, ok := anyToTime(a.Value.Any()); ok {
			return slog.String(slog.TimeKey, t.UTC().Format(time.RFC3339))
		}
	case slog.SourceKey:
		if src, ok := anyToSource(a.Value.Any()); ok {
			return slog.String(slog.SourceKey, fmt.Sprintf("%s:%d", filepath.Base(src.File), src.Line))
		}
	}
	return a
}

func replacePrettyAttr(a slog.Attr, color bool) slog.Attr {
	switch a.Key {
	case slog.TimeKey:
		if t, ok := anyToTime(a.Value.Any()); ok {
			ts := t.Format("15:04:05.000")
			if color {
				ts = ansiDim + ts + ansiReset
			}
			return slog.String(slog.TimeKey, ts)
		}
	case slog.LevelKey:
		lvl := strings.ToUpper(a.Value.String())
		if color {
			return slog.String(slog.LevelKey, colorizeLevel(lvl))
		}
		return slog.String(slog.LevelKey, lvl)
	case slog.SourceKey:
		if src, ok := anyToSource(a.Value.Any()); ok {
			short := fmt.Sprintf("%s:%d", filepath.Base(src.File), src.Line)
			if color {
				short = ansiDim + short + ansiReset
			}
			return slog.String(slog.SourceKey, short)
		}
	}
	return a
}

func anyToTime(v any) (time.Time, bool) {
	t, ok := v.(time.Time)
	return t, ok
}

func anyToSource(v any) (slog.Source, bool) {
	switch x := v.(type) {
	case *slog.Source:
		if x == nil {
			return slog.Source{}, false
		}
		return *x, true
	case slog.Source:
		return x, true
	default:
		return slog.Source{}, false
	}
}

func isLikelyTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func colorizeLevel(level string) string {
	switch level {
	case "DEBUG":
		return ansiBlue + level + ansiReset
	case "WARN":
		return ansiYellow + level + ansiReset
	case "ERROR":
		return ansiRed + level + ansiReset
	default:
		return ansiGreen + level + ansiReset
	}
}

const (
	ansiReset  = "\x1b[0m"
	ansiDim    = "\x1b[2m"
	ansiBlue   = "\x1b[34m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
)
