package authapi

import (
	"net/http"
	"testing"
)

func TestLoadConfigFromEnv_CookieGuardrails(t *testing.T) {
	t.Setenv("ARC_AUTH_REFRESH_COOKIE_NAME", "arc_token")
	t.Setenv("ARC_AUTH_CSRF_COOKIE_NAME", "arc_token")
	t.Setenv("ARC_AUTH_COOKIE_SAMESITE", "none")
	t.Setenv("ARC_AUTH_COOKIE_SECURE", "false")

	cfg := LoadConfigFromEnv()

	if cfg.CSRFCookieName == cfg.RefreshCookieName {
		t.Fatalf("csrf cookie name must differ from refresh cookie name")
	}
	if cfg.CookieSameSite != http.SameSiteNoneMode {
		t.Fatalf("expected SameSite=None, got %v", cfg.CookieSameSite)
	}
	if !cfg.CookieSecure {
		t.Fatalf("SameSite=None requires Secure=true")
	}
}

func TestParseSameSite(t *testing.T) {
	tests := []struct {
		in   string
		want http.SameSite
	}{
		{in: "strict", want: http.SameSiteStrictMode},
		{in: "lax", want: http.SameSiteLaxMode},
		{in: "none", want: http.SameSiteNoneMode},
		{in: "default", want: http.SameSiteDefaultMode},
		{in: "unknown", want: http.SameSiteLaxMode},
	}

	for _, tc := range tests {
		got := parseSameSite(tc.in)
		if got != tc.want {
			t.Fatalf("parseSameSite(%q)=%v, want %v", tc.in, got, tc.want)
		}
	}
}
