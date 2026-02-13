package authapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"arc/cmd/internal/auth/session"
)

func TestShouldUseWebCookieTransport(t *testing.T) {
	h := &Handler{cfg: Config{WebRefreshCookieEnabled: true}}
	if !h.shouldUseWebCookieTransport(session.PlatformWeb) {
		t.Fatalf("expected web cookie transport enabled for web platform")
	}
	if h.shouldUseWebCookieTransport(session.PlatformIOS) {
		t.Fatalf("expected web cookie transport disabled for non-web platform")
	}
}

func TestSetWebSessionCookies(t *testing.T) {
	h := &Handler{cfg: Config{
		WebRefreshCookieEnabled: true,
		RefreshCookieName:       "arc_refresh_token",
		CSRFCookieName:          "arc_csrf_token",
		CookiePath:              "/",
		CookieSecure:            true,
		CookieSameSite:          http.SameSiteLaxMode,
	}}

	rr := httptest.NewRecorder()
	exp := time.Now().UTC().Add(30 * time.Minute)
	csrf, err := h.setWebSessionCookies(rr, "refresh-token-123", exp)
	if err != nil {
		t.Fatalf("setWebSessionCookies: %v", err)
	}
	if csrf == "" {
		t.Fatalf("expected csrf token")
	}

	res := rr.Result()
	if len(res.Cookies()) != 2 {
		t.Fatalf("expected 2 cookies, got %d", len(res.Cookies()))
	}
}

func TestCSRFDoublesubmitValidation(t *testing.T) {
	h := &Handler{cfg: Config{
		WebRefreshCookieEnabled: true,
		CSRFCookieName:          "arc_csrf_token",
		CSRFHeaderName:          "X-CSRF-Token",
	}}

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "arc_csrf_token", Value: "csrf-abc"})
	req.Header.Set("X-CSRF-Token", "csrf-abc")

	if !h.csrfDoubleSubmitValid(req) {
		t.Fatalf("expected csrf validation success")
	}

	req.Header.Set("X-CSRF-Token", "csrf-def")
	if h.csrfDoubleSubmitValid(req) {
		t.Fatalf("expected csrf validation failure on mismatch")
	}
}

func TestRefreshTokenFromCookie(t *testing.T) {
	h := &Handler{cfg: Config{
		WebRefreshCookieEnabled: true,
		RefreshCookieName:       "arc_refresh_token",
	}}

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "arc_refresh_token", Value: "tok-123"})

	token, ok := h.refreshTokenFromCookie(req)
	if !ok {
		t.Fatalf("expected cookie token to be found")
	}
	if token != "tok-123" {
		t.Fatalf("unexpected cookie token: %q", token)
	}
}
