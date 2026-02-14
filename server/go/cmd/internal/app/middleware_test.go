package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestLogMeta(t *testing.T) {
	t.Parallel()

	cases := []struct {
		status     int
		wantLevel  slog.Level
		wantResult string
		wantClass  string
	}{
		{status: 200, wantLevel: slog.LevelInfo, wantResult: "success", wantClass: "2xx"},
		{status: 302, wantLevel: slog.LevelInfo, wantResult: "redirect", wantClass: "3xx"},
		{status: 404, wantLevel: slog.LevelWarn, wantResult: "client_error", wantClass: "4xx"},
		{status: 503, wantLevel: slog.LevelError, wantResult: "server_error", wantClass: "5xx"},
	}

	for _, tc := range cases {
		level, result := requestLogMeta(tc.status)
		if level != tc.wantLevel || result != tc.wantResult {
			t.Fatalf("status=%d level=%v result=%q; want level=%v result=%q", tc.status, level, result, tc.wantLevel, tc.wantResult)
		}
		if got := statusClass(tc.status); got != tc.wantClass {
			t.Fatalf("statusClass(%d)=%q want=%q", tc.status, got, tc.wantClass)
		}
	}
}

func TestWithCORS_PreflightAllowed(t *testing.T) {
	cfg := Config{
		CORSAllowedOrigins:   []string{"https://app.example.com"},
		CORSAllowCredentials: true,
		CORSMaxAgeSeconds:    600,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	h := WithCORS(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatalf("next handler should not be called for preflight")
	}), cfg, log)

	req := httptest.NewRequest(http.MethodOptions, "/auth/refresh", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, X-CSRF-Token")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("allow-origin mismatch: %q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("allow-credentials mismatch: %q", got)
	}
}

func TestWithCORS_DisallowedOrigin(t *testing.T) {
	cfg := Config{
		CORSAllowedOrigins: []string{"https://app.example.com"},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	called := false
	h := WithCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}), cfg, log)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Origin", "https://evil.example.com")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rr.Code)
	}
	if called {
		t.Fatalf("next handler must not be called for denied origin")
	}
}

func TestWithCORS_WildcardPortAllowed(t *testing.T) {
	cfg := Config{
		CORSAllowedOrigins: []string{"http://127.0.0.1:*"},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	h := WithCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), cfg, log)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "http://127.0.0.1:55123")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:55123" {
		t.Fatalf("allow-origin mismatch: %q", got)
	}
}

func TestWithSecurityHeaders(t *testing.T) {
	h := WithSecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", rr.Code)
	}
	if got := rr.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("missing nosniff: %q", got)
	}
	if got := rr.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("missing frame options: %q", got)
	}
	if got := rr.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("missing referrer policy: %q", got)
	}
}
