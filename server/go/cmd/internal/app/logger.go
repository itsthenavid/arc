package app

import (
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strconv"
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
		return newPrettyHandler(out, &slog.HandlerOptions{
			Level:     level,
			AddSource: level <= slog.LevelDebug,
		}, color)
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
			return slog.String("ts", t.UTC().Format(time.RFC3339))
		}
	case slog.LevelKey:
		return slog.String("lvl", strings.ToUpper(a.Value.String()))
	case slog.SourceKey:
		if src, ok := anyToSource(a.Value.Any()); ok {
			return slog.String("src", fmt.Sprintf("%s:%d", filepath.Base(src.File), src.Line))
		}
	case "duration_ms":
		if ms, ok := valueToInt64(a.Value); ok {
			return slog.String("duration", fmt.Sprintf("%dms", ms))
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

func valueToInt64(v slog.Value) (int64, bool) {
	switch v.Kind() {
	case slog.KindInt64:
		return v.Int64(), true
	case slog.KindUint64:
		u := v.Uint64()
		if u > uint64(math.MaxInt64) {
			return 0, false
		}
		return int64(u), true
	case slog.KindFloat64:
		return int64(v.Float64()), true
	case slog.KindString:
		n, err := strconv.ParseInt(strings.TrimSpace(v.String()), 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		switch x := v.Any().(type) {
		case int:
			return int64(x), true
		case int64:
			return x, true
		case int32:
			return int64(x), true
		case uint:
			if x > uint(math.MaxInt64) {
				return 0, false
			}
			return int64(x), true
		case uint64:
			if x > uint64(math.MaxInt64) {
				return 0, false
			}
			return int64(x), true
		default:
			return 0, false
		}
	}
}

func isLikelyTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func colorizeHTTPMethod(method string, color bool) string {
	if !color {
		return method
	}
	switch method {
	case "GET":
		return ansiBlue + method + ansiReset
	case "POST":
		return ansiGreen + method + ansiReset
	case "PUT", "PATCH":
		return ansiYellow + method + ansiReset
	case "DELETE":
		return ansiRed + method + ansiReset
	default:
		return ansiMagenta + method + ansiReset
	}
}

func colorizeStatusCode(code int, color bool) string {
	s := strconv.Itoa(code)
	if !color {
		return s
	}
	switch {
	case code >= 500:
		return ansiRed + s + ansiReset
	case code >= 400:
		return ansiYellow + s + ansiReset
	case code >= 300:
		return ansiMagenta + s + ansiReset
	default:
		return ansiGreen + s + ansiReset
	}
}

func colorizeStatusClass(class string, color bool) string {
	if !color {
		return class
	}
	switch class {
	case "5xx":
		return ansiRed + class + ansiReset
	case "4xx":
		return ansiYellow + class + ansiReset
	case "3xx":
		return ansiMagenta + class + ansiReset
	default:
		return ansiGreen + class + ansiReset
	}
}

func colorizeDurationMS(ms int64, color bool) string {
	s := fmt.Sprintf("%dms", ms)
	if !color {
		return s
	}
	switch {
	case ms >= 1500:
		return ansiRed + s + ansiReset
	case ms >= 400:
		return ansiYellow + s + ansiReset
	default:
		return ansiGreen + s + ansiReset
	}
}

func colorizeResult(result string, color bool) string {
	if !color {
		return result
	}
	switch result {
	case "success":
		return ansiGreen + result + ansiReset
	case "redirect":
		return ansiMagenta + result + ansiReset
	case "client_error":
		return ansiYellow + result + ansiReset
	case "server_error", "failed", "error":
		return ansiRed + result + ansiReset
	default:
		return ansiBlue + result + ansiReset
	}
}

const (
	ansiReset   = "\x1b[0m"
	ansiDim     = "\x1b[2m"
	ansiBright  = "\x1b[1m"
	ansiBlue    = "\x1b[34m"
	ansiGreen   = "\x1b[32m"
	ansiYellow  = "\x1b[33m"
	ansiMagenta = "\x1b[35m"
	ansiCyan    = "\x1b[36m"
	ansiRed     = "\x1b[31m"
)
