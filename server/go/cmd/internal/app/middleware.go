package app

import (
	"bufio"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// WithRequestLogging wraps an http.Handler and logs requests.
// IMPORTANT: ResponseWriter must preserve optional interfaces (Hijacker, Flusher, Pusher, ReaderFrom),
// otherwise WebSocket upgrades can fail.
func WithRequestLogging(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		lrw := &loggingResponseWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		next.ServeHTTP(lrw, r)

		level, result := requestLogMeta(lrw.status)
		log.LogAttrs(r.Context(), level, "http.request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", lrw.status),
			slog.String("status_class", statusClass(lrw.status)),
			slog.Int64("bytes", lrw.bytes),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.String("remote", r.RemoteAddr),
			slog.String("user_agent", r.UserAgent()),
			slog.String("result", result),
		)
	})
}

// WithSecurityHeaders applies a conservative baseline of security headers.
func WithSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")

		// HSTS is meaningful only over HTTPS connections.
		if r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}

// WithCORS enforces an explicit allowlist and handles CORS preflight.
func WithCORS(next http.Handler, cfg Config, log *slog.Logger) http.Handler {
	if log == nil {
		log = slog.Default()
	}

	allowedOrigins := make([]string, 0, len(cfg.CORSAllowedOrigins))
	for _, origin := range cfg.CORSAllowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		allowedOrigins = append(allowedOrigins, origin)
	}

	allowedMethods := []string{http.MethodGet, http.MethodPost, http.MethodOptions}
	allowedMethodsHeader := strings.Join(allowedMethods, ", ")

	allowedHeaders := []string{"Authorization", "Content-Type", "X-CSRF-Token"}
	allowedHeadersHeader := strings.Join(allowedHeaders, ", ")
	allowedHeadersSet := make(map[string]struct{}, len(allowedHeaders))
	for _, h := range allowedHeaders {
		allowedHeadersSet[strings.ToLower(strings.TrimSpace(h))] = struct{}{}
	}

	maxAge := cfg.CORSMaxAgeSeconds
	if maxAge <= 0 {
		maxAge = 600
	}
	maxAgeHeader := strconv.Itoa(maxAge)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// WebSocket origin enforcement is handled by the WS gateway policies.
		if r.URL.Path == "/ws" {
			next.ServeHTTP(w, r)
			return
		}

		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		if !corsOriginAllowed(origin, allowedOrigins) {
			log.Warn("http.cors.origin_denied", "origin", origin, "path", r.URL.Path, "result", "client_error")
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}

		h := w.Header()
		h.Add("Vary", "Origin")
		h.Add("Vary", "Access-Control-Request-Method")
		h.Add("Vary", "Access-Control-Request-Headers")
		h.Set("Access-Control-Allow-Origin", origin)
		if cfg.CORSAllowCredentials {
			h.Set("Access-Control-Allow-Credentials", "true")
		}

		if r.Method == http.MethodOptions && strings.TrimSpace(r.Header.Get("Access-Control-Request-Method")) != "" {
			reqMethod := strings.TrimSpace(r.Header.Get("Access-Control-Request-Method"))
			if !containsString(allowedMethods, reqMethod) {
				http.Error(w, "method not allowed", http.StatusForbidden)
				return
			}
			if !corsRequestHeadersAllowed(r.Header.Get("Access-Control-Request-Headers"), allowedHeadersSet) {
				http.Error(w, "header not allowed", http.StatusForbidden)
				return
			}
			h.Set("Access-Control-Allow-Methods", allowedMethodsHeader)
			h.Set("Access-Control-Allow-Headers", allowedHeadersHeader)
			h.Set("Access-Control-Max-Age", maxAgeHeader)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func corsRequestHeadersAllowed(raw string, allowed map[string]struct{}) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}

	parts := strings.Split(raw, ",")
	for _, p := range parts {
		name := strings.ToLower(strings.TrimSpace(p))
		if name == "" {
			continue
		}
		if _, ok := allowed[name]; !ok {
			return false
		}
	}
	return true
}

func corsOriginAllowed(origin string, allowed []string) bool {
	if len(allowed) == 0 {
		return false
	}

	originURL, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || originURL.Scheme == "" || originURL.Host == "" {
		return false
	}
	originScheme := strings.ToLower(originURL.Scheme)
	originHost := strings.ToLower(originURL.Hostname())
	originPort := originURL.Port()

	for _, rule := range allowed {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		if rule == "*" {
			return true
		}
		if strings.HasSuffix(rule, ":*") {
			base := strings.TrimSuffix(rule, ":*")
			ru, err := url.Parse(base)
			if err != nil || ru.Scheme == "" || ru.Host == "" {
				continue
			}
			if strings.EqualFold(ru.Scheme, originScheme) &&
				strings.EqualFold(ru.Hostname(), originHost) {
				return true
			}
			continue
		}

		ru, err := url.Parse(rule)
		if err != nil || ru.Scheme == "" || ru.Host == "" {
			continue
		}
		if !strings.EqualFold(ru.Scheme, originScheme) {
			continue
		}
		if !strings.EqualFold(ru.Hostname(), originHost) {
			continue
		}
		if strings.TrimSpace(ru.Port()) != strings.TrimSpace(originPort) {
			continue
		}
		return true
	}

	return false
}

func containsString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func requestLogMeta(status int) (level slog.Level, result string) {
	switch {
	case status >= 500:
		return slog.LevelError, "server_error"
	case status >= 400:
		return slog.LevelWarn, "client_error"
	case status >= 300:
		return slog.LevelInfo, "redirect"
	default:
		return slog.LevelInfo, "success"
	}
}

func statusClass(status int) string {
	switch {
	case status >= 500:
		return "5xx"
	case status >= 400:
		return "4xx"
	case status >= 300:
		return "3xx"
	case status >= 200:
		return "2xx"
	default:
		return "1xx"
	}
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *loggingResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *loggingResponseWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	return n, err
}

func (w *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("underlying ResponseWriter does not support hijacking")
	}
	return hj.Hijack()
}

func (w *loggingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *loggingResponseWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (w *loggingResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, err := rf.ReadFrom(r)
		w.bytes += n
		return n, err
	}
	n, err := io.Copy(w.ResponseWriter, r)
	w.bytes += n
	return n, err
}

func (w *loggingResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
