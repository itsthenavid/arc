package authapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"arc/cmd/identity"
	"arc/cmd/internal/auth/session"

	paseto "aidanwoods.dev/go-paseto"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAuthAPI_LoginFailure_NoEnumeration(t *testing.T) {
	pool := mustOpenAuthTestPool(t)
	defer pool.Close()

	cfg := testAuthConfig()
	h := mustNewAuthHandler(t, pool, cfg)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux := http.NewServeMux()
		h.Register(mux)
		mux.ServeHTTP(w, r)
	}))
	defer ts.Close()

	client := ts.Client()
	idStore, err := identity.NewPostgresStore(pool)
	if err != nil {
		t.Fatalf("identity.NewPostgresStore: %v", err)
	}

	username := "auth_login_" + strings.ToLower(mustNewULIDLike(t))
	password := "Very-Strong-Password-1!"
	now := time.Now().UTC()

	createRes, err := idStore.CreateUser(context.Background(), identity.CreateUserInput{
		Username: &username,
		Password: password,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() { cleanupAuthUser(context.Background(), t, pool, createRes.User.ID) })

	statusA, bodyA := doJSON(t, client, ts.URL+"/auth/login", loginRequest{
		Username: strPtr("not_exists_" + username),
		Password: password,
		Platform: "ios",
	}, nil)
	if statusA != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unknown user, got %d", statusA)
	}

	var errA errorResponse
	if err := json.Unmarshal(bodyA, &errA); err != nil {
		t.Fatalf("decode errA: %v", err)
	}

	statusB, bodyB := doJSON(t, client, ts.URL+"/auth/login", loginRequest{
		Username: &username,
		Password: "Wrong-Password-1!",
		Platform: "ios",
	}, nil)
	if statusB != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong password, got %d", statusB)
	}

	var errB errorResponse
	if err := json.Unmarshal(bodyB, &errB); err != nil {
		t.Fatalf("decode errB: %v", err)
	}

	if errA.Error.Code != "invalid_credentials" || errB.Error.Code != "invalid_credentials" {
		t.Fatalf("expected uniform invalid_credentials errors, got %q and %q", errA.Error.Code, errB.Error.Code)
	}

	statusOK, bodyOK := doJSON(t, client, ts.URL+"/auth/login", loginRequest{
		Username: &username,
		Password: password,
		Platform: "ios",
	}, nil)
	if statusOK != http.StatusOK {
		t.Fatalf("expected 200 for successful login, got %d body=%s", statusOK, string(bodyOK))
	}

	var okResp loginResponse
	if err := json.Unmarshal(bodyOK, &okResp); err != nil {
		t.Fatalf("decode loginResponse: %v", err)
	}
	if okResp.Session.AccessToken == "" || okResp.Session.RefreshToken == "" {
		t.Fatalf("expected non-empty access and refresh tokens")
	}
}

func TestAuthAPI_RefreshReuseDetected_RevokesAll(t *testing.T) {
	pool := mustOpenAuthTestPool(t)
	defer pool.Close()

	cfg := testAuthConfig()
	h := mustNewAuthHandler(t, pool, cfg)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux := http.NewServeMux()
		h.Register(mux)
		mux.ServeHTTP(w, r)
	}))
	defer ts.Close()

	client := ts.Client()
	idStore, err := identity.NewPostgresStore(pool)
	if err != nil {
		t.Fatalf("identity.NewPostgresStore: %v", err)
	}

	username := "auth_refresh_" + strings.ToLower(mustNewULIDLike(t))
	password := "Very-Strong-Password-2!"
	now := time.Now().UTC()

	createRes, err := idStore.CreateUser(context.Background(), identity.CreateUserInput{
		Username: &username,
		Password: password,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() { cleanupAuthUser(context.Background(), t, pool, createRes.User.ID) })

	statusLogin, bodyLogin := doJSON(t, client, ts.URL+"/auth/login", loginRequest{
		Username: &username,
		Password: password,
		Platform: "ios",
	}, nil)
	if statusLogin != http.StatusOK {
		t.Fatalf("login status=%d body=%s", statusLogin, string(bodyLogin))
	}

	var loginResp loginResponse
	if err := json.Unmarshal(bodyLogin, &loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}

	oldRefresh := loginResp.Session.RefreshToken
	statusRefresh, bodyRefresh := doJSON(t, client, ts.URL+"/auth/refresh", refreshRequest{
		RefreshToken: oldRefresh,
		Platform:     "ios",
	}, nil)
	if statusRefresh != http.StatusOK {
		t.Fatalf("first refresh status=%d body=%s", statusRefresh, string(bodyRefresh))
	}

	var rotated refreshResponse
	if err := json.Unmarshal(bodyRefresh, &rotated); err != nil {
		t.Fatalf("decode refresh response: %v", err)
	}
	newRefresh := rotated.Session.RefreshToken
	if newRefresh == "" || newRefresh == oldRefresh {
		t.Fatalf("expected rotated refresh token")
	}

	statusReuse, bodyReuse := doJSON(t, client, ts.URL+"/auth/refresh", refreshRequest{
		RefreshToken: oldRefresh,
		Platform:     "ios",
	}, nil)
	if statusReuse != http.StatusUnauthorized {
		t.Fatalf("expected 401 on refresh reuse, got %d body=%s", statusReuse, string(bodyReuse))
	}

	var errReuse errorResponse
	if err := json.Unmarshal(bodyReuse, &errReuse); err != nil {
		t.Fatalf("decode reuse error: %v", err)
	}
	if errReuse.Error.Code != "refresh_reuse_detected" {
		t.Fatalf("expected refresh_reuse_detected, got %q", errReuse.Error.Code)
	}

	statusRevoked, bodyRevoked := doJSON(t, client, ts.URL+"/auth/refresh", refreshRequest{
		RefreshToken: newRefresh,
		Platform:     "ios",
	}, nil)
	if statusRevoked != http.StatusUnauthorized {
		t.Fatalf("expected 401 after revoke-all, got %d body=%s", statusRevoked, string(bodyRevoked))
	}

	var errRevoked errorResponse
	if err := json.Unmarshal(bodyRevoked, &errRevoked); err != nil {
		t.Fatalf("decode revoked error: %v", err)
	}
	if errRevoked.Error.Code != "session_not_active" {
		t.Fatalf("expected session_not_active, got %q", errRevoked.Error.Code)
	}
}

func TestAuthAPI_LogoutAndLogoutAll(t *testing.T) {
	pool := mustOpenAuthTestPool(t)
	defer pool.Close()

	cfg := testAuthConfig()
	h := mustNewAuthHandler(t, pool, cfg)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux := http.NewServeMux()
		h.Register(mux)
		mux.ServeHTTP(w, r)
	}))
	defer ts.Close()

	client := ts.Client()
	idStore, err := identity.NewPostgresStore(pool)
	if err != nil {
		t.Fatalf("identity.NewPostgresStore: %v", err)
	}

	username := "auth_logout_" + strings.ToLower(mustNewULIDLike(t))
	password := "Very-Strong-Password-3!"
	now := time.Now().UTC()

	createRes, err := idStore.CreateUser(context.Background(), identity.CreateUserInput{
		Username: &username,
		Password: password,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() { cleanupAuthUser(context.Background(), t, pool, createRes.User.ID) })

	login1 := mustLoginForTest(t, client, ts.URL, username, password, "ios")
	login2 := mustLoginForTest(t, client, ts.URL, username, password, "android")

	statusLogout, bodyLogout := doJSON(t, client, ts.URL+"/auth/logout", struct{}{}, map[string]string{
		"Authorization": "Bearer " + login1.Session.AccessToken,
	})
	if statusLogout != http.StatusNoContent {
		t.Fatalf("logout status=%d body=%s", statusLogout, string(bodyLogout))
	}

	statusR1, bodyR1 := doJSON(t, client, ts.URL+"/auth/refresh", refreshRequest{
		RefreshToken: login1.Session.RefreshToken,
		Platform:     "ios",
	}, nil)
	if statusR1 != http.StatusUnauthorized {
		t.Fatalf("expected first session revoked, got %d body=%s", statusR1, string(bodyR1))
	}

	var errR1 errorResponse
	if err := json.Unmarshal(bodyR1, &errR1); err != nil {
		t.Fatalf("decode refresh err1: %v", err)
	}
	if errR1.Error.Code != "session_not_active" {
		t.Fatalf("expected session_not_active for revoked session, got %q", errR1.Error.Code)
	}

	statusR2, bodyR2 := doJSON(t, client, ts.URL+"/auth/refresh", refreshRequest{
		RefreshToken: login2.Session.RefreshToken,
		Platform:     "android",
	}, nil)
	if statusR2 != http.StatusOK {
		t.Fatalf("expected second session still active, got %d body=%s", statusR2, string(bodyR2))
	}

	var refreshed2 refreshResponse
	if err := json.Unmarshal(bodyR2, &refreshed2); err != nil {
		t.Fatalf("decode refresh2: %v", err)
	}

	statusLogoutAll, bodyLogoutAll := doJSON(t, client, ts.URL+"/auth/logout_all", struct{}{}, map[string]string{
		"Authorization": "Bearer " + refreshed2.Session.AccessToken,
	})
	if statusLogoutAll != http.StatusNoContent {
		t.Fatalf("logout_all status=%d body=%s", statusLogoutAll, string(bodyLogoutAll))
	}

	statusR3, bodyR3 := doJSON(t, client, ts.URL+"/auth/refresh", refreshRequest{
		RefreshToken: refreshed2.Session.RefreshToken,
		Platform:     "android",
	}, nil)
	if statusR3 != http.StatusUnauthorized {
		t.Fatalf("expected session_not_active after logout_all, got %d body=%s", statusR3, string(bodyR3))
	}
}

func TestAuthAPI_WebCookieCSRFRefreshFlow(t *testing.T) {
	pool := mustOpenAuthTestPool(t)
	defer pool.Close()

	cfg := testAuthConfig()
	cfg.WebRefreshCookieEnabled = true
	cfg.CookieSecure = false // httptest uses http://
	cfg.RefreshCookieName = "arc_refresh_token"
	cfg.CSRFCookieName = "arc_csrf_token"
	cfg.CSRFHeaderName = "X-CSRF-Token"
	cfg.CookieSameSite = http.SameSiteLaxMode
	cfg.CookiePath = "/"

	h := mustNewAuthHandler(t, pool, cfg)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux := http.NewServeMux()
		h.Register(mux)
		mux.ServeHTTP(w, r)
	}))
	defer ts.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}

	idStore, err := identity.NewPostgresStore(pool)
	if err != nil {
		t.Fatalf("identity.NewPostgresStore: %v", err)
	}

	inviteRes, err := idStore.CreateInvite(context.Background(), identity.CreateInviteInput{
		TTL:     24 * time.Hour,
		MaxUses: 1,
		Now:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	t.Cleanup(func() { cleanupInvite(context.Background(), t, pool, inviteRes.Invite.ID) })

	username := "auth_web_" + strings.ToLower(mustNewULIDLike(t))
	password := "Very-Strong-Password-4!"

	statusConsume, bodyConsume := doJSON(t, client, ts.URL+"/auth/invites/consume", inviteConsumeRequest{
		InviteToken: inviteRes.Token,
		Username:    &username,
		Password:    password,
		Platform:    "web",
	}, nil)
	if statusConsume != http.StatusOK {
		t.Fatalf("invite consume status=%d body=%s", statusConsume, string(bodyConsume))
	}

	var consumeResp inviteConsumeResponse
	if err := json.Unmarshal(bodyConsume, &consumeResp); err != nil {
		t.Fatalf("decode consume response: %v", err)
	}
	if consumeResp.User.ID == "" {
		t.Fatalf("expected created user id")
	}
	t.Cleanup(func() { cleanupAuthUser(context.Background(), t, pool, consumeResp.User.ID) })

	if consumeResp.Session.AccessToken == "" {
		t.Fatalf("expected access token for web session")
	}
	if consumeResp.Session.RefreshToken != "" {
		t.Fatalf("expected refresh token omitted in web cookie mode")
	}

	parsedURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}

	refreshCookie := cookieValueByName(jar.Cookies(parsedURL), cfg.RefreshCookieName)
	csrfCookie := cookieValueByName(jar.Cookies(parsedURL), cfg.CSRFCookieName)
	if refreshCookie == "" || csrfCookie == "" {
		t.Fatalf("expected refresh and csrf cookies to be set")
	}

	statusNoCSRF, bodyNoCSRF := doJSON(t, client, ts.URL+"/auth/refresh", struct{}{}, nil)
	if statusNoCSRF != http.StatusForbidden {
		t.Fatalf("expected 403 without csrf header, got %d body=%s", statusNoCSRF, string(bodyNoCSRF))
	}

	var errNoCSRF errorResponse
	if err := json.Unmarshal(bodyNoCSRF, &errNoCSRF); err != nil {
		t.Fatalf("decode csrf error: %v", err)
	}
	if errNoCSRF.Error.Code != "csrf_invalid" {
		t.Fatalf("expected csrf_invalid, got %q", errNoCSRF.Error.Code)
	}

	statusWithCSRF, bodyWithCSRF := doJSON(t, client, ts.URL+"/auth/refresh", struct{}{}, map[string]string{
		cfg.CSRFHeaderName: csrfCookie,
	})
	if statusWithCSRF != http.StatusOK {
		t.Fatalf("expected successful refresh with csrf, got %d body=%s", statusWithCSRF, string(bodyWithCSRF))
	}

	var refreshResp refreshResponse
	if err := json.Unmarshal(bodyWithCSRF, &refreshResp); err != nil {
		t.Fatalf("decode refresh response: %v", err)
	}
	if refreshResp.Session.RefreshToken != "" {
		t.Fatalf("expected refresh token omitted in web mode")
	}
	if refreshResp.Session.AccessToken == "" {
		t.Fatalf("expected access token from refresh")
	}
}

func mustLoginForTest(t *testing.T, client *http.Client, baseURL, username, password, platform string) loginResponse {
	t.Helper()
	status, body := doJSON(t, client, baseURL+"/auth/login", loginRequest{
		Username: &username,
		Password: password,
		Platform: platform,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("login status=%d body=%s", status, string(body))
	}
	var resp loginResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return resp
}

func testAuthConfig() Config {
	return Config{
		InviteOnly:             true,
		InviteTTL:              7 * 24 * time.Hour,
		InviteMaxTTL:           30 * 24 * time.Hour,
		InviteMaxUses:          1,
		InviteMaxUsesMax:       50,
		TrustProxy:             false,
		MaxBodyBytes:           1 << 20,
		LoginIPMax:             20,
		LoginIPWindow:          5 * time.Minute,
		LoginUserMax:           5,
		LoginUserWindow:        15 * time.Minute,
		LockoutShortThreshold:  5,
		LockoutShortDuration:   5 * time.Minute,
		LockoutLongThreshold:   10,
		LockoutLongDuration:    30 * time.Minute,
		LockoutSevereThreshold: 20,
		LockoutSevereDuration:  2 * time.Hour,
		RefreshCookieName:      "arc_refresh_token",
		CSRFCookieName:         "arc_csrf_token",
		CSRFHeaderName:         "X-CSRF-Token",
		CookieSecure:           true,
		CookieSameSite:         http.SameSiteLaxMode,
		CookiePath:             "/",
	}
}

func mustNewAuthHandler(t *testing.T, pool *pgxpool.Pool, cfg Config) *Handler {
	t.Helper()
	secret := paseto.NewV4AsymmetricSecretKey()
	sessCfg := session.DefaultConfig()
	sessCfg.PasetoV4SecretKeyHex = secret.ExportHex()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h, err := NewHandler(log, pool, cfg, sessCfg, true)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

func doJSON(t *testing.T, client *http.Client, url string, payload any, headers map[string]string) (int, []byte) {
	t.Helper()

	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("io.ReadAll: %v", err)
	}
	return resp.StatusCode, body
}

func cookieValueByName(cookies []*http.Cookie, name string) string {
	for _, c := range cookies {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func mustOpenAuthTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	raw := strings.TrimSpace(os.Getenv("ARC_DATABASE_URL"))
	if raw == "" {
		t.Skip("integration test skipped: ARC_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(raw)
	if err != nil {
		t.Fatalf("parse ARC_DATABASE_URL: %v", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer pingCancel()

	c, err := pool.Acquire(pingCtx)
	if err != nil {
		pool.Close()
		if shouldSkipAuthIntegration(err) {
			t.Skipf("integration test skipped: Postgres unreachable (ARC_DATABASE_URL set): %v", err)
		}
		t.Fatalf("acquire: %v", err)
	}
	c.Release()

	return pool
}

func shouldSkipAuthIntegration(err error) bool {
	if err == nil {
		return false
	}
	if os.Getenv("CI") != "" {
		return false
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "dial tcp") ||
		strings.Contains(msg, "no such host") {
		return true
	}
	return false
}

func cleanupAuthUser(ctx context.Context, t *testing.T, pool *pgxpool.Pool, userID string) {
	t.Helper()
	if strings.TrimSpace(userID) == "" {
		return
	}
	_, _ = pool.Exec(ctx, `DELETE FROM arc.sessions WHERE user_id = $1`, userID)
	_, _ = pool.Exec(ctx, `DELETE FROM arc.user_credentials WHERE user_id = $1`, userID)
	_, _ = pool.Exec(ctx, `DELETE FROM arc.users WHERE id = $1`, userID)
}

func cleanupInvite(ctx context.Context, t *testing.T, pool *pgxpool.Pool, inviteID string) {
	t.Helper()
	if strings.TrimSpace(inviteID) == "" {
		return
	}
	_, _ = pool.Exec(ctx, `DELETE FROM arc.invites WHERE id = $1`, inviteID)
}

func mustNewULIDLike(t *testing.T) string {
	t.Helper()
	id, err := identity.NewULID(time.Now().UTC())
	if err != nil {
		t.Fatalf("identity.NewULID: %v", err)
	}
	return id
}

func strPtr(s string) *string { return &s }
