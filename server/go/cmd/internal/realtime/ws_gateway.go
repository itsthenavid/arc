package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	v1 "arc/shared/contracts/realtime/v1"

	"github.com/coder/websocket"
)

const (
	wsSubprotocolV1 = "arc.realtime.v1"

	wsDefaultSendQueueSize = 256
	wsMinSendQueueSize     = 32

	wsDefaultWriteTimeout = 5 * time.Second
	wsDefaultReadIdle     = 2 * time.Minute
	wsCloseGrace          = 1 * time.Second

	wsDefaultHistoryLimit = 50
	wsMaxHistoryLimit     = 200

	wsMaxPingFailures = 3

	// Security defaults:
	// - Origin is required by default.
	// - Only localhost is allowed by default (secure-by-default for dev).
	wsDefaultOriginRequired = true
	wsDefaultAllowedOrigins = "http://localhost,http://127.0.0.1"
)

// WSGateway is the WebSocket entrypoint for Arc realtime.
//
// It enforces origin policy, subprotocol selection, rate limits, heartbeats,
// and routes validated envelopes to the Hub and MessageStore.
type WSGateway struct {
	log   *slog.Logger
	hub   *Hub
	store MessageStore

	devInsecure    bool
	originRequired bool
	allowedOrigins []string

	// Derived for websocket.Accept origin checks.
	// Accept() authorizes same-host origins by default, but for cross-origin it requires OriginPatterns.
	originPatterns []string

	writeTimeout    time.Duration
	readIdleTimeout time.Duration
	sendQueueSize   int

	heartbeatEvery   time.Duration
	heartbeatTimeout time.Duration

	rateEvents int
	rateWindow time.Duration
}

// NewWSGateway constructs a gateway with secure defaults.
// When hub/store are nil, it falls back to in-memory implementations for dev.
func NewWSGateway(log *slog.Logger, hub *Hub, store MessageStore) *WSGateway {
	if log == nil {
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	if hub == nil {
		hub = NewHub(log)
	}
	if store == nil {
		store = NewInMemoryStore()
	}

	g := &WSGateway{log: log, hub: hub, store: store}

	// NOTE: InsecureSkipVerify is a dev-only knob (TLS verification). It is not an origin policy.
	g.devInsecure = envBoolWS("ARC_WS_DEV_INSECURE", false)

	g.originRequired = envBoolWS("ARC_WS_ORIGIN_REQUIRED", wsDefaultOriginRequired)
	g.allowedOrigins = envCSVWS("ARC_WS_ALLOWED_ORIGINS", wsDefaultAllowedOrigins)

	// IMPORTANT:
	// websocket.Accept enforces its own origin policy:
	// - same-host is ok
	// - cross-origin requires OriginPatterns (host patterns)
	// We derive these patterns from allowed origins so the two layers agree.
	g.originPatterns = deriveOriginPatternsFromAllowedOrigins(g.allowedOrigins)

	g.writeTimeout = envDurationWS("ARC_WS_WRITE_TIMEOUT", wsDefaultWriteTimeout)
	g.readIdleTimeout = envDurationWS("ARC_WS_READ_IDLE_TIMEOUT", wsDefaultReadIdle)

	g.sendQueueSize = envIntWS("ARC_WS_SEND_QUEUE", wsDefaultSendQueueSize)
	if g.sendQueueSize < wsMinSendQueueSize {
		g.sendQueueSize = wsMinSendQueueSize
	}

	g.heartbeatEvery = envDurationWS("ARC_WS_HEARTBEAT_INTERVAL", heartbeatInterval)
	g.heartbeatTimeout = envDurationWS("ARC_WS_HEARTBEAT_TIMEOUT", heartbeatTimeout)

	g.rateEvents = envIntWS("ARC_WS_RATE_EVENTS", rateLimitEvents)
	g.rateWindow = envDurationWS("ARC_WS_RATE_WINDOW", rateLimitWindow)

	return g
}

// ServeHTTP adapter so it can be mounted as http.Handler.
func (g *WSGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.HandleWS(w, r)
}

// HandleWS upgrades an HTTP request to a WebSocket session and runs the realtime loop.
func (g *WSGateway) HandleWS(w http.ResponseWriter, r *http.Request) {
	if err := g.enforceOrigin(r); err != nil {
		g.log.Info("ws.reject.origin", "err", err, "origin", r.Header.Get("Origin"), "remote", r.RemoteAddr)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{wsSubprotocolV1},

		// Fix for 403 "Origin ... is not authorized for Host ...":
		// Authorize allowed origin hosts (e.g. localhost) for cross-origin requests.
		OriginPatterns: g.originPatterns,

		// Dev-only escape hatch.
		InsecureSkipVerify: g.devInsecure,
	})
	if err != nil {
		g.log.Error("ws.accept.fail", "err", err)
		return
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "bye") }()

	if sp := conn.Subprotocol(); sp != wsSubprotocolV1 {
		g.log.Info("ws.reject.subprotocol", "got", sp, "want", wsSubprotocolV1)
		_ = conn.Close(websocket.StatusProtocolError, "subprotocol required")
		return
	}

	conn.SetReadLimit(maxFrameBytes)

	sessionID := NewRandomHex(10)
	client := NewClient(sessionID, g.sendQueueSize)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var (
		closeOnce sync.Once
		joined    *Conversation
	)

	// shutdown is idempotent. It does NOT close client.Send.
	// Broadcast safety: client.Send remains open and membership removal happens before client.Close.
	shutdown := func(code websocket.StatusCode, reason string) {
		closeOnce.Do(func() {
			if joined != nil {
				joined.Leave(sessionID)
				joined = nil
			}

			client.Close()
			_ = conn.Close(code, reason)
			cancel()
		})
	}

	rl := NewRateLimiter(g.rateEvents, g.rateWindow)

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)

		for {
			select {
			case <-ctx.Done():
				return
			case <-client.Done():
				return
			case env := <-client.Send:
				if err := writeEnvelope(ctx, conn, env, g.writeTimeout); err != nil {
					g.log.Info("ws.write.fail", "session_id", sessionID, "close_status", websocket.CloseStatus(err), "err", err)
					shutdown(websocket.StatusAbnormalClosure, "write failed")
					return
				}
			}
		}
	}()

	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)

		t := time.NewTicker(g.heartbeatEvery)
		defer t.Stop()

		failures := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-client.Done():
				return
			case <-t.C:
				hbCtx, hbCancel := context.WithTimeout(ctx, g.heartbeatTimeout)
				err := conn.Ping(hbCtx)
				hbCancel()

				if err != nil {
					failures++
					g.log.Info("ws.ping.fail", "session_id", sessionID, "failures", failures, "err", err)
					if failures >= wsMaxPingFailures {
						shutdown(websocket.StatusGoingAway, "heartbeat failed")
						return
					}
					continue
				}
				failures = 0
			}
		}
	}()

readLoop:
	for {
		readCtx, readCancel := context.WithTimeout(ctx, g.readIdleTimeout)
		env, err := readEnvelope(readCtx, conn)
		readCancel()

		if err != nil {
			switch classifyReadErr(err) {
			case readErrClose:
				shutdown(websocket.StatusNormalClosure, "peer closed")
				break readLoop
			case readErrCtxDone:
				shutdown(websocket.StatusNormalClosure, "context done")
				break readLoop
			case readErrConnClosed:
				shutdown(websocket.StatusAbnormalClosure, "conn closed")
				break readLoop
			case readErrBadJSON:
				g.trySendError(ctx, client, "bad_json", "invalid JSON")
				continue readLoop
			default:
				g.log.Info("ws.read.fail", "session_id", sessionID, "err", err)
				shutdown(websocket.StatusAbnormalClosure, "read failed")
				break readLoop
			}
		}

		now := time.Now().UTC()
		if !rl.Allow(now) {
			g.trySendError(ctx, client, "rate_limited", "too many events")
			shutdown(websocket.StatusPolicyViolation, "rate limited")
			break readLoop
		}

		if err := env.Validate(); err != nil {
			g.trySendError(ctx, client, "bad_envelope", err.Error())
			continue readLoop
		}

		switch env.Type {
		case v1.TypeHello:
			if err := g.onHello(ctx, client, env); err != nil {
				g.trySendError(ctx, client, "hello_failed", err.Error())
				shutdown(websocket.StatusPolicyViolation, "hello failed")
				break readLoop
			}

		case v1.TypeConversationJoin:
			conv, err := g.onJoin(ctx, client, env)
			if err != nil {
				g.trySendError(ctx, client, "join_failed", err.Error())
				continue readLoop
			}

			// Ensure membership stability: leave old conversation before switching.
			if joined != nil && joined.ID != conv.ID {
				joined.Leave(sessionID)
			}
			joined = conv

		case v1.TypeMessageSend:
			if joined == nil {
				g.trySendError(ctx, client, "not_joined", "join first")
				continue readLoop
			}
			if err := g.onMessageSend(ctx, client, joined, env, now); err != nil {
				g.trySendError(ctx, client, "send_failed", err.Error())
				continue readLoop
			}

		case v1.TypeConversationHistoryFetch:
			if joined == nil {
				g.trySendError(ctx, client, "not_joined", "join first")
				continue readLoop
			}
			if err := g.onHistoryFetch(ctx, client, joined, env); err != nil {
				g.trySendError(ctx, client, "history_failed", err.Error())
				continue readLoop
			}

		default:
			g.trySendError(ctx, client, "unsupported", fmt.Sprintf("unsupported type: %s", env.Type))
		}
	}

	shutdown(websocket.StatusNormalClosure, "bye")
	<-writerDone

	select {
	case <-heartbeatDone:
	case <-time.After(wsCloseGrace):
	}
}

// ---- handlers ----

func (g *WSGateway) onHello(ctx context.Context, client *Client, env v1.Envelope) error {
	var p v1.HelloPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	ackPayload, _ := json.Marshal(v1.HelloAckPayload{SessionID: client.SessionID})
	ack := newEnvelope(v1.TypeHelloAck, ackPayload, time.Now().UTC())

	if !g.enqueue(ctx, client, ack) {
		return errors.New("backpressure: hello.ack")
	}
	return nil
}

func (g *WSGateway) onJoin(ctx context.Context, client *Client, env v1.Envelope) (*Conversation, error) {
	var p v1.ConversationJoinPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	convID := strings.TrimSpace(p.ConversationID)
	if convID == "" {
		return nil, errors.New("missing conversation_id")
	}

	conv := g.hub.GetOrCreateConversation(convID)
	conv.Join(client)

	echoPayload, _ := json.Marshal(v1.ConversationJoinPayload{
		ConversationID: conv.ID,
		Kind:           conv.Kind,
	})
	echo := newEnvelope(v1.TypeConversationJoin, echoPayload, time.Now().UTC())

	if !g.enqueue(ctx, client, echo) {
		conv.Leave(client.SessionID)
		return nil, errors.New("backpressure: join echo")
	}

	return conv, nil
}

func (g *WSGateway) onMessageSend(ctx context.Context, client *Client, conv *Conversation, env v1.Envelope, now time.Time) error {
	var p v1.MessageSendPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	if strings.TrimSpace(p.ConversationID) == "" || p.ConversationID != conv.ID {
		return errors.New("invalid conversation_id")
	}
	if strings.TrimSpace(p.ClientMsgID) == "" {
		return errors.New("missing client_msg_id")
	}

	text := strings.TrimSpace(p.Text)
	if text == "" {
		return errors.New("empty text")
	}
	if len([]rune(text)) > maxMessageChars {
		return fmt.Errorf("message too long: max=%d chars", maxMessageChars)
	}

	res, err := g.store.AppendMessage(ctx, AppendMessageInput{
		ConversationID: p.ConversationID,
		ClientMsgID:    p.ClientMsgID,
		SenderSession:  client.SessionID,
		Text:           text,
		Now:            now,
	})
	if err != nil {
		return fmt.Errorf("store append: %w", err)
	}

	stored := res.Stored

	ackPayload, _ := json.Marshal(v1.MessageAckPayload{
		ConversationID: stored.ConversationID,
		ClientMsgID:    stored.ClientMsgID,
		ServerMsgID:    stored.ServerMsgID,
		Seq:            stored.Seq,
	})
	ack := newEnvelope(v1.TypeMessageAck, ackPayload, now)

	if !g.enqueue(ctx, client, ack) {
		return errors.New("backpressure: ack")
	}

	if res.Duplicated {
		return nil
	}

	newPayload, _ := json.Marshal(v1.MessageNewPayload{
		ConversationID: stored.ConversationID,
		ClientMsgID:    stored.ClientMsgID,
		ServerMsgID:    stored.ServerMsgID,
		Seq:            stored.Seq,
		Sender:         stored.SenderSession,
		Text:           stored.Text,
		ServerTS:       stored.ServerTS,
	})
	newEnv := newEnvelope(v1.TypeMessageNew, newPayload, now)
	conv.Broadcast(newEnv)
	return nil
}

func (g *WSGateway) onHistoryFetch(ctx context.Context, client *Client, conv *Conversation, env v1.Envelope) error {
	var p v1.ConversationHistoryFetchPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	convID := strings.TrimSpace(p.ConversationID)
	if convID == "" {
		return errors.New("missing conversation_id")
	}
	if convID != conv.ID {
		return errors.New("not a member of conversation_id")
	}

	limit := p.Limit
	if limit <= 0 {
		limit = wsDefaultHistoryLimit
	}
	if limit > wsMaxHistoryLimit {
		limit = wsMaxHistoryLimit
	}

	out, err := g.store.FetchHistory(ctx, FetchHistoryInput{
		ConversationID: convID,
		AfterSeq:       p.AfterSeq,
		Limit:          limit,
	})
	if err != nil {
		return err
	}

	msgs := make([]v1.MessageNewPayload, 0, len(out.Messages))
	for _, m := range out.Messages {
		msgs = append(msgs, v1.MessageNewPayload{
			ConversationID: m.ConversationID,
			ClientMsgID:    m.ClientMsgID,
			ServerMsgID:    m.ServerMsgID,
			Seq:            m.Seq,
			Sender:         m.SenderSession,
			Text:           m.Text,
			ServerTS:       m.ServerTS,
		})
	}

	chunkPayload, _ := json.Marshal(v1.ConversationHistoryChunkPayload{
		ConversationID: convID,
		Messages:       msgs,
		HasMore:        out.HasMore,
	})
	chunk := newEnvelope(v1.TypeConversationHistoryChunk, chunkPayload, time.Now().UTC())

	if !g.enqueue(ctx, client, chunk) {
		return errors.New("backpressure: history chunk")
	}
	return nil
}

// ---- send helpers ----

func (g *WSGateway) trySendError(ctx context.Context, client *Client, code, msg string) {
	p, _ := json.Marshal(v1.ErrorPayload{Code: code, Message: msg})
	env := newEnvelope(v1.TypeError, p, time.Now().UTC())
	_ = g.enqueue(ctx, client, env)
}

func (g *WSGateway) enqueue(ctx context.Context, client *Client, env v1.Envelope) bool {
	select {
	case <-ctx.Done():
		return false
	case <-client.Done():
		return false
	case client.Send <- env:
		return true
	default:
		return false
	}
}

// ---- envelope IO ----

func newEnvelope(typ string, payload json.RawMessage, ts time.Time) v1.Envelope {
	return v1.Envelope{
		V:       v1.Version,
		Type:    typ,
		ID:      NewRandomHex(10),
		TS:      ts,
		Payload: payload,
	}
}

func readEnvelope(ctx context.Context, conn *websocket.Conn) (v1.Envelope, error) {
	mt, data, err := conn.Read(ctx)
	if err != nil {
		return v1.Envelope{}, err
	}
	if mt != websocket.MessageText && mt != websocket.MessageBinary {
		return v1.Envelope{}, fmt.Errorf("unsupported message type: %v", mt)
	}
	var env v1.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return v1.Envelope{}, err
	}
	return env, nil
}

func writeEnvelope(parent context.Context, conn *websocket.Conn, env v1.Envelope, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, b)
}

// ---- read error classification ----

type readErrKind uint8

const (
	readErrUnknown readErrKind = iota
	readErrClose
	readErrCtxDone
	readErrConnClosed
	readErrBadJSON
)

func classifyReadErr(err error) readErrKind {
	if websocket.CloseStatus(err) != -1 {
		return readErrClose
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return readErrCtxDone
	}
	if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
		return readErrConnClosed
	}

	// JSON decode errors are typically returned by json.Unmarshal, not conn.Read.
	// This fallback exists for robustness when error strings are propagated.
	s := err.Error()
	if strings.Contains(s, "unexpected end of JSON input") || strings.Contains(s, "invalid character") {
		return readErrBadJSON
	}
	return readErrUnknown
}

// ---- origin policy ----

func (g *WSGateway) enforceOrigin(r *http.Request) error {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		if g.originRequired {
			return errors.New("missing origin")
		}
		return nil
	}

	if len(g.allowedOrigins) == 0 {
		return errors.New("origin not allowed (no allowlist)")
	}

	originHost := originHostOnly(origin)

	for _, a := range g.allowedOrigins {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if a == "*" {
			// Strongly discouraged, but honored if explicitly configured.
			return nil
		}

		// Full origin match (scheme + host + optional port).
		if origin == a {
			return nil
		}

		// Host match fallback (ignores port/scheme).
		if originHost != "" && originHost == originHostOnly(a) {
			return nil
		}
	}

	return fmt.Errorf("origin not allowed: %s", origin)
}

func originHostOnly(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	// URL form.
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return ""
		}
		h := strings.TrimSpace(u.Host)
		if h == "" {
			return ""
		}
		if host, _, err := net.SplitHostPort(h); err == nil {
			return strings.ToLower(host)
		}
		return strings.ToLower(h)
	}

	// host[:port] form.
	if host, _, err := net.SplitHostPort(s); err == nil {
		return strings.ToLower(host)
	}
	return strings.ToLower(s)
}

func deriveOriginPatternsFromAllowedOrigins(allowed []string) []string {
	// websocket.Accept matches OriginPatterns against the origin host using filepath.Match patterns.
	// We keep this strict: only hosts extracted from allowlist are accepted.
	seen := make(map[string]struct{}, len(allowed))

	for _, a := range allowed {
		h := originHostOnly(a)
		if h == "" || h == "*" {
			continue
		}
		seen[h] = struct{}{}
	}

	out := make([]string, 0, len(seen))
	for h := range seen {
		out = append(out, h)
	}

	// Stable in-file sort (avoid importing sort just for this).
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}

	return out
}

// ---- env helpers ----

func envBoolWS(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envIntWS(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func envDurationWS(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

func envCSVWS(key string, def string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		raw = def
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
