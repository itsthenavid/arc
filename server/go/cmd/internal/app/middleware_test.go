package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
