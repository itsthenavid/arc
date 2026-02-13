package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"arc/cmd/internal/auth/session"
	v1 "arc/shared/contracts/realtime/v1"

	paseto "aidanwoods.dev/go-paseto"
	"github.com/coder/websocket"
)

func TestWSGateway_RequireAuth_UnauthorizedRejected(t *testing.T) {
	t.Setenv("ARC_WS_DEV_INSECURE", "false")
	t.Setenv("ARC_WS_REQUIRE_AUTH", "true")
	t.Setenv("ARC_WS_REQUIRE_MEMBERSHIP", "false")
	t.Setenv("ARC_WS_ORIGIN_REQUIRED", "false")

	now := time.Now().UTC()
	row := session.Row{
		ID:        "sess-auth-1",
		UserID:    "user-auth-1",
		CreatedAt: now,
		ExpiresAt: now.Add(1 * time.Hour),
		Platform:  session.PlatformWeb,
	}

	authSvc, _ := newWSAuthService(t, row, 15*time.Minute)
	gw := newWSAuthGateway(t, authSvc)
	ts := startWSTestServer(t, gw)
	defer ts.Close()

	_, resp, err := dialWS(t, ts.URL, wsDialInput{})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatalf("expected unauthorized handshake failure")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("expected 401, got status=%d err=%v", status, err)
	}
}

func TestWSGateway_RequireAuth_InvalidTokenRejected(t *testing.T) {
	t.Setenv("ARC_WS_DEV_INSECURE", "false")
	t.Setenv("ARC_WS_REQUIRE_AUTH", "true")
	t.Setenv("ARC_WS_REQUIRE_MEMBERSHIP", "false")
	t.Setenv("ARC_WS_ORIGIN_REQUIRED", "false")

	now := time.Now().UTC()
	row := session.Row{
		ID:        "sess-auth-2",
		UserID:    "user-auth-2",
		CreatedAt: now,
		ExpiresAt: now.Add(1 * time.Hour),
		Platform:  session.PlatformWeb,
	}

	authSvc, _ := newWSAuthService(t, row, 15*time.Minute)
	gw := newWSAuthGateway(t, authSvc)
	ts := startWSTestServer(t, gw)
	defer ts.Close()

	_, resp, err := dialWS(t, ts.URL, wsDialInput{
		Origin: "http://localhost",
		Bearer: "not-a-valid-token",
	})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatalf("expected unauthorized handshake failure")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("expected 401, got status=%d err=%v", status, err)
	}
}

func TestWSGateway_RequireAuth_ExpiredTokenRejected(t *testing.T) {
	t.Setenv("ARC_WS_DEV_INSECURE", "false")
	t.Setenv("ARC_WS_REQUIRE_AUTH", "true")
	t.Setenv("ARC_WS_REQUIRE_MEMBERSHIP", "false")
	t.Setenv("ARC_WS_ORIGIN_REQUIRED", "false")

	now := time.Now().UTC()
	row := session.Row{
		ID:        "sess-auth-3",
		UserID:    "user-auth-3",
		CreatedAt: now.Add(-2 * time.Hour),
		ExpiresAt: now.Add(1 * time.Hour),
		Platform:  session.PlatformWeb,
	}

	authSvc, tokens := newWSAuthService(t, row, 1*time.Minute)
	expiredToken, _, err := tokens.Issue(row.UserID, row.ID, now.Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	gw := newWSAuthGateway(t, authSvc)
	ts := startWSTestServer(t, gw)
	defer ts.Close()

	_, resp, err := dialWS(t, ts.URL, wsDialInput{
		Origin: "http://localhost",
		Bearer: expiredToken,
	})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatalf("expected unauthorized handshake failure")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("expected 401, got status=%d err=%v", status, err)
	}
}

func TestWSGateway_RequireAuth_AuthorizedConnectAndActions(t *testing.T) {
	t.Setenv("ARC_WS_DEV_INSECURE", "false")
	t.Setenv("ARC_WS_REQUIRE_AUTH", "true")
	t.Setenv("ARC_WS_REQUIRE_MEMBERSHIP", "false")
	t.Setenv("ARC_WS_ORIGIN_REQUIRED", "false")

	now := time.Now().UTC()
	row := session.Row{
		ID:        "sess-auth-4",
		UserID:    "user-auth-4",
		CreatedAt: now,
		ExpiresAt: now.Add(1 * time.Hour),
		Platform:  session.PlatformWeb,
	}

	authSvc, tokens := newWSAuthService(t, row, 15*time.Minute)
	accessToken, _, err := tokens.Issue(row.UserID, row.ID, now)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	gw := newWSAuthGateway(t, authSvc)
	ts := startWSTestServer(t, gw)
	defer ts.Close()

	conn, resp, err := dialWS(t, ts.URL, wsDialInput{Bearer: accessToken})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("authorized dial failed: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "bye") }()

	writeEnvelopeWS(t, conn, v1.Envelope{
		V:       v1.Version,
		Type:    v1.TypeHello,
		ID:      "hello-1",
		TS:      time.Now().UTC(),
		Payload: mustJSONRaw(t, v1.HelloPayload{}),
	})

	helloAck := readUntilType(t, conn, v1.TypeHelloAck, 4)
	var ackP v1.HelloAckPayload
	if err := json.Unmarshal(helloAck.Payload, &ackP); err != nil {
		t.Fatalf("decode hello ack: %v", err)
	}
	if ackP.SessionID != row.ID {
		t.Fatalf("expected hello ack session_id=%q, got %q", row.ID, ackP.SessionID)
	}

	convID := "conv-auth-1"
	writeEnvelopeWS(t, conn, v1.Envelope{
		V:    v1.Version,
		Type: v1.TypeConversationJoin,
		ID:   "join-1",
		TS:   time.Now().UTC(),
		Payload: mustJSONRaw(t, v1.ConversationJoinPayload{
			ConversationID: convID,
			Kind:           "direct",
		}),
	})

	_ = readUntilType(t, conn, v1.TypeConversationJoin, 4)

	writeEnvelopeWS(t, conn, v1.Envelope{
		V:    v1.Version,
		Type: v1.TypeMessageSend,
		ID:   "send-1",
		TS:   time.Now().UTC(),
		Payload: mustJSONRaw(t, v1.MessageSendPayload{
			ConversationID: convID,
			ClientMsgID:    "client-msg-1",
			Text:           "hello",
		}),
	})

	msgAck := readUntilType(t, conn, v1.TypeMessageAck, 6)
	var msgAckP v1.MessageAckPayload
	if err := json.Unmarshal(msgAck.Payload, &msgAckP); err != nil {
		t.Fatalf("decode message ack: %v", err)
	}
	if msgAckP.ConversationID != convID {
		t.Fatalf("expected conv_id=%q, got %q", convID, msgAckP.ConversationID)
	}
}

func TestWSGateway_RequireAuth_DevInsecureDoesNotBypassAuth(t *testing.T) {
	t.Setenv("ARC_WS_DEV_INSECURE", "true")
	t.Setenv("ARC_WS_REQUIRE_AUTH", "true")
	t.Setenv("ARC_WS_REQUIRE_MEMBERSHIP", "false")
	t.Setenv("ARC_WS_ORIGIN_REQUIRED", "false")

	now := time.Now().UTC()
	row := session.Row{
		ID:        "sess-auth-5",
		UserID:    "user-auth-5",
		CreatedAt: now,
		ExpiresAt: now.Add(1 * time.Hour),
		Platform:  session.PlatformWeb,
	}

	authSvc, _ := newWSAuthService(t, row, 15*time.Minute)
	gw := newWSAuthGateway(t, authSvc)
	ts := startWSTestServer(t, gw)
	defer ts.Close()

	_, resp, err := dialWS(t, ts.URL, wsDialInput{})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatalf("expected unauthorized handshake failure")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("expected 401, got status=%d err=%v", status, err)
	}
}

func TestWSGateway_RequireAuth_QueryParamFallback(t *testing.T) {
	t.Setenv("ARC_WS_DEV_INSECURE", "false")
	t.Setenv("ARC_WS_REQUIRE_AUTH", "true")
	t.Setenv("ARC_WS_REQUIRE_MEMBERSHIP", "false")
	t.Setenv("ARC_WS_ORIGIN_REQUIRED", "false")
	t.Setenv("ARC_WS_AUTH_QUERY_PARAM", "access_token")

	now := time.Now().UTC()
	row := session.Row{
		ID:        "sess-auth-6",
		UserID:    "user-auth-6",
		CreatedAt: now,
		ExpiresAt: now.Add(1 * time.Hour),
		Platform:  session.PlatformWeb,
	}

	authSvc, tokens := newWSAuthService(t, row, 15*time.Minute)
	accessToken, _, err := tokens.Issue(row.UserID, row.ID, now)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	gw := newWSAuthGateway(t, authSvc)
	ts := startWSTestServer(t, gw)
	defer ts.Close()

	conn, resp, err := dialWS(t, ts.URL, wsDialInput{
		QueryParam: "access_token",
		QueryValue: accessToken,
	})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("query-param authorized dial failed: %v", err)
	}
	_ = conn.Close(websocket.StatusNormalClosure, "bye")
}

func TestWSGateway_RequireAuth_CookieFallback(t *testing.T) {
	t.Setenv("ARC_WS_DEV_INSECURE", "false")
	t.Setenv("ARC_WS_REQUIRE_AUTH", "true")
	t.Setenv("ARC_WS_REQUIRE_MEMBERSHIP", "false")
	t.Setenv("ARC_WS_ORIGIN_REQUIRED", "false")
	t.Setenv("ARC_WS_AUTH_COOKIE_NAME", "arc_access_token")

	now := time.Now().UTC()
	row := session.Row{
		ID:        "sess-auth-7",
		UserID:    "user-auth-7",
		CreatedAt: now,
		ExpiresAt: now.Add(1 * time.Hour),
		Platform:  session.PlatformWeb,
	}

	authSvc, tokens := newWSAuthService(t, row, 15*time.Minute)
	accessToken, _, err := tokens.Issue(row.UserID, row.ID, now)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	gw := newWSAuthGateway(t, authSvc)
	ts := startWSTestServer(t, gw)
	defer ts.Close()

	conn, resp, err := dialWS(t, ts.URL, wsDialInput{
		CookieName:  "arc_access_token",
		CookieValue: accessToken,
	})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("cookie authorized dial failed: %v", err)
	}
	_ = conn.Close(websocket.StatusNormalClosure, "bye")
}

func TestWSGateway_RequireAuth_RejectsOversizedToken(t *testing.T) {
	t.Setenv("ARC_WS_DEV_INSECURE", "false")
	t.Setenv("ARC_WS_REQUIRE_AUTH", "true")
	t.Setenv("ARC_WS_REQUIRE_MEMBERSHIP", "false")
	t.Setenv("ARC_WS_ORIGIN_REQUIRED", "false")

	now := time.Now().UTC()
	row := session.Row{
		ID:        "sess-auth-8",
		UserID:    "user-auth-8",
		CreatedAt: now,
		ExpiresAt: now.Add(1 * time.Hour),
		Platform:  session.PlatformWeb,
	}

	authSvc, _ := newWSAuthService(t, row, 15*time.Minute)
	gw := newWSAuthGateway(t, authSvc)
	ts := startWSTestServer(t, gw)
	defer ts.Close()

	oversized := strings.Repeat("a", wsMaxAccessToken+1)
	_, resp, err := dialWS(t, ts.URL, wsDialInput{Bearer: oversized})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatalf("expected unauthorized handshake failure")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("expected 401, got status=%d err=%v", status, err)
	}
}

func newWSAuthGateway(t *testing.T, authSvc *session.Service) *WSGateway {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewWSGateway(log, NewHub(log), NewInMemoryStore(), authSvc, nil)
}

func startWSTestServer(t *testing.T, gw *WSGateway) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("/ws", gw)
	return httptest.NewServer(mux)
}

type wsDialInput struct {
	Origin      string
	Bearer      string
	QueryParam  string
	QueryValue  string
	CookieName  string
	CookieValue string
}

func dialWS(t *testing.T, baseHTTPURL string, in wsDialInput) (*websocket.Conn, *http.Response, error) {
	t.Helper()

	u, err := url.Parse(baseHTTPURL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	u.Scheme = "ws"
	u.Path = "/ws"
	if strings.TrimSpace(in.QueryParam) != "" {
		q := u.Query()
		q.Set(strings.TrimSpace(in.QueryParam), in.QueryValue)
		u.RawQuery = q.Encode()
	}

	h := http.Header{}
	if strings.TrimSpace(in.Origin) != "" {
		h.Set("Origin", in.Origin)
	}
	if strings.TrimSpace(in.Bearer) != "" {
		h.Set("Authorization", "Bearer "+in.Bearer)
	}
	if strings.TrimSpace(in.CookieName) != "" {
		h.Set("Cookie", strings.TrimSpace(in.CookieName)+"="+in.CookieValue)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return websocket.Dial(ctx, u.String(), &websocket.DialOptions{
		Subprotocols: []string{wsSubprotocolV1},
		HTTPHeader:   h,
	})
}

func writeEnvelopeWS(t *testing.T, conn *websocket.Conn, env v1.Envelope) {
	t.Helper()
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("conn.Write: %v", err)
	}
}

func readUntilType(t *testing.T, conn *websocket.Conn, typ string, maxReads int) v1.Envelope {
	t.Helper()
	if maxReads <= 0 {
		maxReads = 1
	}
	for i := 0; i < maxReads; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, b, err := conn.Read(ctx)
		cancel()
		if err != nil {
			t.Fatalf("conn.Read: %v", err)
		}
		var env v1.Envelope
		if err := json.Unmarshal(b, &env); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		if env.Type == typ {
			return env
		}
	}
	t.Fatalf("did not receive envelope type %q", typ)
	return v1.Envelope{}
}

func mustJSONRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return b
}

type wsAuthStore struct {
	rows map[string]session.Row
}

func newWSAuthService(t *testing.T, row session.Row, accessTTL time.Duration) (*session.Service, session.AccessTokenManager) {
	t.Helper()
	secret := paseto.NewV4AsymmetricSecretKey()

	cfg := session.DefaultConfig()
	cfg.AccessTokenTTL = accessTTL
	cfg.PasetoV4SecretKeyHex = secret.ExportHex()

	tokens, err := session.NewPasetoV4PublicManager(cfg)
	if err != nil {
		t.Fatalf("NewPasetoV4PublicManager: %v", err)
	}

	store := &wsAuthStore{rows: map[string]session.Row{row.ID: row}}
	svc := session.NewService(cfg, nil, store, tokens)
	return svc, tokens
}

func (s *wsAuthStore) Create(context.Context, time.Time, string, session.DeviceContext, string, time.Time, *string) (string, error) {
	return "", errors.New("not implemented")
}

func (s *wsAuthStore) GetByID(_ context.Context, sessionID string) (session.Row, error) {
	if s == nil || s.rows == nil {
		return session.Row{}, session.ErrSessionNotFound
	}
	row, ok := s.rows[sessionID]
	if !ok {
		return session.Row{}, session.ErrSessionNotFound
	}
	return row, nil
}

func (s *wsAuthStore) GetByRefreshHashForUpdate(context.Context, string) (session.Row, error) {
	return session.Row{}, errors.New("not implemented")
}

func (s *wsAuthStore) MarkRotated(context.Context, time.Time, string, string) error {
	return errors.New("not implemented")
}

func (s *wsAuthStore) Touch(context.Context, time.Time, string) error { return nil }

func (s *wsAuthStore) Revoke(context.Context, time.Time, string, string) error {
	return errors.New("not implemented")
}

func (s *wsAuthStore) RevokeAll(context.Context, time.Time, string, string) error {
	return errors.New("not implemented")
}

var _ session.Store = (*wsAuthStore)(nil)
