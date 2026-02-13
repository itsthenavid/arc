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

type prettyField struct {
	key string
	val slog.Value
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
	ts := r.Time
	if ts.IsZero() {
		ts = time.Now()
	}

	fields := make([]prettyField, 0, 12)
	for _, a := range h.attrs {
		h.collectAttr(&fields, a, "")
	}
	r.Attrs(func(a slog.Attr) bool {
		h.collectAttr(&fields, a, "")
		return true
	})

	if h.opts.AddSource && r.PC != 0 {
		frames := runtime.CallersFrames([]uintptr{r.PC})
		frame, _ := frames.Next()
		if frame.File != "" {
			fields = append(fields, prettyField{
				key: "src",
				val: slog.StringValue(fmt.Sprintf("%s:%d", filepath.Base(frame.File), frame.Line)),
			})
		}
	}

	mainLine, detailLine := h.renderRecord(r, ts, fields)

	var b strings.Builder
	b.WriteString(mainLine)
	if detailLine != "" {
		b.WriteByte('\n')
		b.WriteString(detailLine)
	}
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

func (h *prettyHandler) collectAttr(dst *[]prettyField, a slog.Attr, parent string) {
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
			h.collectAttr(dst, ga, fullKey)
		}
		return
	}

	*dst = append(*dst, prettyField{
		key: fullKey,
		val: a.Value,
	})
}

func (h *prettyHandler) renderRecord(r slog.Record, ts time.Time, fields []prettyField) (string, string) {
	var main strings.Builder
	main.WriteString(applyDim(ts.Format("15:04:05.000"), h.color))
	main.WriteByte(' ')
	main.WriteString(levelTag(r.Level, h.color))
	main.WriteByte(' ')
	main.WriteString(applyBold(r.Message, h.color))

	if r.Message == "http.request" {
		h.renderHTTPRequestLine(&main, &fields)
	} else {
		h.renderGenericLine(&main, &fields)
	}

	return main.String(), h.renderDetailLine(fields)
}

func (h *prettyHandler) renderHTTPRequestLine(main *strings.Builder, fields *[]prettyField) {
	if f, ok := popField(fields, "method"); ok {
		method := strings.ToUpper(strings.TrimSpace(valueToString(f.val)))
		if method != "" {
			main.WriteByte(' ')
			main.WriteString(colorizeHTTPMethod(method, h.color))
		}
	}
	if f, ok := popField(fields, "path"); ok {
		path := strings.TrimSpace(valueToString(f.val))
		if path != "" {
			main.WriteByte(' ')
			if h.color {
				main.WriteString(ansiCyan + path + ansiReset)
			} else {
				main.WriteString(path)
			}
		}
	}
	if f, ok := popField(fields, "status"); ok {
		if n, okN := valueToInt64(f.val); okN {
			main.WriteByte(' ')
			main.WriteString(colorizeStatusCode(int(n), h.color))
		}
	}
	_, _ = popField(fields, "status_class")
	if f, ok := popField(fields, "duration_ms"); ok {
		if n, okN := valueToInt64(f.val); okN {
			main.WriteByte(' ')
			main.WriteString(colorizeDurationMS(n, h.color))
		}
	}
	if f, ok := popField(fields, "result"); ok {
		result := strings.ToLower(strings.TrimSpace(valueToString(f.val)))
		if result != "" {
			main.WriteByte(' ')
			main.WriteString(colorizeResult(result, h.color))
		}
	}
	if f, ok := popField(fields, "bytes"); ok {
		main.WriteByte(' ')
		main.WriteString("bytes=")
		main.WriteString(valueToString(f.val))
	}
}

func (h *prettyHandler) renderGenericLine(main *strings.Builder, fields *[]prettyField) {
	inline := takeByKeys(fields,
		"mode",
		"addr",
		"db_enabled",
		"log_format",
		"reason",
		"result",
		"err",
	)
	for _, f := range inline {
		main.WriteByte(' ')
		main.WriteString(h.styleKV(f))
	}
}

func (h *prettyHandler) renderDetailLine(fields []prettyField) string {
	if len(fields) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(applyDim("  ↳ ", h.color))

	const maxItems = 5
	items := 0
	for i, f := range fields {
		if items >= maxItems {
			break
		}
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(h.styleKV(f))
		items++
	}
	if len(fields) > maxItems {
		b.WriteString(applyDim(" …", h.color))
	}
	return b.String()
}

func takeByKeys(fields *[]prettyField, keys ...string) []prettyField {
	out := make([]prettyField, 0, len(keys))
	for _, k := range keys {
		if f, ok := popField(fields, k); ok {
			out = append(out, f)
		}
	}
	return out
}

func popField(fields *[]prettyField, key string) (prettyField, bool) {
	for i, f := range *fields {
		if f.key == key {
			*fields = append((*fields)[:i], (*fields)[i+1:]...)
			return f, true
		}
	}
	return prettyField{}, false
}

func (h *prettyHandler) styleKV(f prettyField) string {
	key := remapPrettyKey(f.key)
	val := h.prettyValue(key, f.val)
	return key + "=" + val
}

func (h *prettyHandler) prettyValue(key string, v slog.Value) string {
	switch key {
	case "method":
		return colorizeHTTPMethod(strings.ToUpper(strings.TrimSpace(valueToString(v))), h.color)
	case "path":
		path := strings.TrimSpace(valueToString(v))
		if h.color {
			return ansiCyan + path + ansiReset
		}
		return path
	case "status":
		if n, ok := valueToInt64(v); ok {
			return colorizeStatusCode(int(n), h.color)
		}
	case "class":
		return colorizeStatusClass(strings.TrimSpace(valueToString(v)), h.color)
	case "duration":
		if n, ok := valueToInt64(v); ok {
			return colorizeDurationMS(n, h.color)
		}
	case "result":
		return colorizeResult(strings.ToLower(strings.TrimSpace(valueToString(v))), h.color)
	case "user_agent":
		return quoteIfNeeded(truncateString(valueToString(v), 56))
	case "err":
		s := quoteIfNeeded(truncateString(valueToString(v), 96))
		if h.color {
			return ansiRed + s + ansiReset
		}
		return s
	case "src":
		return applyDim(quoteIfNeeded(valueToString(v)), h.color)
	}

	return quoteIfNeeded(valueToString(v))
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

func truncateString(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen-1]) + "…"
}

func levelTag(level slog.Level, color bool) string {
	switch {
	case level >= slog.LevelError:
		if color {
			return ansiRed + "❌ ERROR" + ansiReset
		}
		return "[ERROR]"
	case level >= slog.LevelWarn:
		if color {
			return ansiYellow + "⚠ WARN" + ansiReset
		}
		return "[WARN]"
	case level < slog.LevelInfo:
		if color {
			return ansiMagenta + "🛠 DEBUG" + ansiReset
		}
		return "[DEBUG]"
	default:
		// INFO is always blue by design.
		if color {
			return ansiBlue + "ℹ INFO" + ansiReset
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
