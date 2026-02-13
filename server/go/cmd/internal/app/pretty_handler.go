package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type prettyHandler struct {
	w      io.Writer
	opts   slog.HandlerOptions
	attrs  []slog.Attr
	groups []string
	color  bool
	mu     *sync.Mutex
}

func newPrettyHandler(w io.Writer, opts *slog.HandlerOptions, color bool) slog.Handler {
	h := &prettyHandler{
		w:     w,
		color: color,
		mu:    &sync.Mutex{},
	}
	if opts != nil {
		h.opts = *opts
	}
	return h
}

func (h *prettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

func (h *prettyHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder

	ts := r.Time
	if ts.IsZero() {
		ts = time.Now()
	}
	b.WriteString("ts=")
	b.WriteString(applyDim(ts.Format("15:04:05.000"), h.color))
	b.WriteByte(' ')
	b.WriteString("lvl=")
	b.WriteString(levelTag(r.Level, h.color))
	b.WriteByte(' ')
	b.WriteString("msg=")
	b.WriteString(applyBold(r.Message, h.color))

	if h.opts.AddSource && r.PC != 0 {
		frames := runtime.CallersFrames([]uintptr{r.PC})
		frame, _ := frames.Next()
		if frame.File != "" {
			b.WriteByte(' ')
			b.WriteString("src=")
			b.WriteString(applyDim(fmt.Sprintf("%s:%d", filepath.Base(frame.File), frame.Line), h.color))
		}
	}

	for _, a := range h.attrs {
		h.appendAttr(&b, a, "")
	}
	r.Attrs(func(a slog.Attr) bool {
		h.appendAttr(&b, a, "")
		return true
	})

	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cp := *h
	cp.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &cp
}

func (h *prettyHandler) WithGroup(name string) slog.Handler {
	if strings.TrimSpace(name) == "" {
		return h
	}
	cp := *h
	cp.groups = append(append([]string{}, h.groups...), name)
	return &cp
}

func (h *prettyHandler) appendAttr(b *strings.Builder, a slog.Attr, parent string) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}

	key := strings.TrimSpace(a.Key)
	if key == "" {
		return
	}

	fullKey := key
	if parent != "" {
		fullKey = parent + "." + key
	}
	if len(h.groups) > 0 {
		fullKey = strings.Join(h.groups, ".") + "." + fullKey
	}

	if a.Value.Kind() == slog.KindGroup {
		for _, ga := range a.Value.Group() {
			h.appendAttr(b, ga, fullKey)
		}
		return
	}

	b.WriteByte(' ')
	b.WriteString(remapPrettyKey(fullKey))
	b.WriteByte('=')
	b.WriteString(h.prettyValue(fullKey, a.Value))
}

func (h *prettyHandler) prettyValue(key string, v slog.Value) string {
	trimmedKey := strings.TrimSpace(key)

	switch trimmedKey {
	case "method":
		return colorizeHTTPMethod(strings.ToUpper(strings.TrimSpace(v.String())), h.color)
	case "path":
		path := strings.TrimSpace(v.String())
		if h.color {
			return ansiCyan + path + ansiReset
		}
		return path
	case "status":
		if n, ok := valueToInt64(v); ok {
			return colorizeStatusCode(int(n), h.color)
		}
	case "status_class", "class":
		return colorizeStatusClass(strings.TrimSpace(v.String()), h.color)
	case "duration_ms":
		if n, ok := valueToInt64(v); ok {
			return colorizeDurationMS(n, h.color)
		}
	case "result":
		return colorizeResult(strings.ToLower(strings.TrimSpace(v.String())), h.color)
	}

	plain := valueToString(v)
	return quoteIfNeeded(plain)
}

func remapPrettyKey(k string) string {
	switch k {
	case "status_class":
		return "class"
	case "duration_ms":
		return "duration"
	default:
		return k
	}
}

func valueToString(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindInt64:
		return strconv.FormatInt(v.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(v.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(v.Float64(), 'f', -1, 64)
	case slog.KindBool:
		if v.Bool() {
			return "true"
		}
		return "false"
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().Format(time.RFC3339)
	default:
		return fmt.Sprint(v.Any())
	}
}

func quoteIfNeeded(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, " \t\r\n\"=") {
		return strconv.Quote(s)
	}
	return s
}

func levelTag(level slog.Level, color bool) string {
	switch {
	case level >= slog.LevelError:
		if color {
			return ansiRed + "[ERROR]" + ansiReset
		}
		return "[ERROR]"
	case level >= slog.LevelWarn:
		if color {
			return ansiYellow + "[WARN]" + ansiReset
		}
		return "[WARN]"
	case level < slog.LevelInfo:
		if color {
			return ansiMagenta + "[DEBUG]" + ansiReset
		}
		return "[DEBUG]"
	default:
		// INFO is always blue by design.
		if color {
			return ansiBlue + "[INFO]" + ansiReset
		}
		return "[INFO]"
	}
}

func applyDim(s string, color bool) string {
	if !color {
		return s
	}
	return ansiDim + s + ansiReset
}

func applyBold(s string, color bool) string {
	if !color {
		return s
	}
	return ansiBright + s + ansiReset
}
