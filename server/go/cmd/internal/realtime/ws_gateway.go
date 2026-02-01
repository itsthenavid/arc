package realtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	v1 "arc/shared/contracts/realtime/v1"

	"github.com/coder/websocket"
)

const (
	wsSubprotocolV1 = "arc.realtime.v1"

	// PR-001 dev defaults.
	defaultSendQueueSize       = 128
	defaultWriteTimeout        = 5 * time.Second
	defaultCloseTimeout        = 1 * time.Second
	maxConsecutivePingFailures = 3

	// Backpressure policy (PR-001): fail fast on saturated send queue.
)

// WSGateway terminates websocket connections and bridges them into Hub conversations.
// PR-001 scope: dev-only transport skeleton + basic rate limiting + message fanout.
type WSGateway struct {
	log *slog.Logger
	hub *Hub
}

func NewWSGateway(log *slog.Logger, hub *Hub) *WSGateway {
	return &WSGateway{log: log, hub: hub}
}

func (g *WSGateway) HandleWS(w http.ResponseWriter, r *http.Request) {
	// NOTE: PR-001 is dev-only. Proper origin checks + auth will be introduced in later PRs.
	// We still enforce subprotocol to ensure client speaks the expected contract version.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols:       []string{wsSubprotocolV1},
		InsecureSkipVerify: true, // PR-001 dev-only
	})
	if err != nil {
		g.log.Error("ws.accept.fail", "err", err)
		return
	}

	// Safety net. Primary shutdown happens via shutdown().
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "bye") }()

	// Protect memory: refuse oversized frames early.
	conn.SetReadLimit(maxFrameBytes)

	sessionID := newRandomHex(10)
	client := &Client{
		SessionID: sessionID,
		Send:      make(chan v1.Envelope, defaultSendQueueSize),
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var (
		joined  *Conversation
		closeMu sync.Once
	)

	shutdown := func(status websocket.StatusCode, reason string) {
		closeMu.Do(func() {
			// Best-effort bounded close handshake.
			// websocket.Conn.Close is not context-aware; we bound our own shutdown sequence.
			_ = conn.Close(status, reason)

			// Ensure goroutines stop.
			cancel()

			// Closing send queue lets writer exit quickly.
			close(client.Send)
		})
	}

	limiter := &RateLimiter{}

	// Writer: drains client.Send and writes to the websocket.
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)

		for {
			select {
			case <-ctx.Done():
				return

			case env, ok := <-client.Send:
				if !ok {
					return
				}

				if err := writeEnvelope(ctx, conn, env, defaultWriteTimeout); err != nil {
					g.log.Info(
						"ws.write.fail",
						"session_id", sessionID,
						"close_status", websocket.CloseStatus(err),
						"err", err,
					)
					shutdown(websocket.StatusAbnormalClosure, "write failed")
					return
				}
			}
		}
	}()

	// Heartbeat: periodic ping to detect dead connections.
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)

		t := time.NewTicker(heartbeatInterval)
		defer t.Stop()

		failures := 0
		for {
			select {
			case <-ctx.Done():
				return

			case <-t.C:
				hbCtx, hbCancel := context.WithTimeout(ctx, heartbeatTimeout)
				err := conn.Ping(hbCtx)
				hbCancel()

				if err != nil {
					failures++
					g.log.Info(
						"ws.ping.fail",
						"session_id", sessionID,
						"failures", failures,
						"close_status", websocket.CloseStatus(err),
						"err", err,
					)
					if failures >= maxConsecutivePingFailures {
						shutdown(websocket.StatusGoingAway, "heartbeat failed")
						return
					}
					continue
				}

				failures = 0
			}
		}
	}()

	// Read loop: reads websocket frames -> JSON -> v1.Envelope -> validation -> dispatch.
readLoop:
	for {
		env, err := readEnvelope(ctx, conn)
		if err != nil {
			switch classifyWSReadErr(err) {
			case readErrClose:
				g.log.Info(
					"ws.read.close",
					"session_id", sessionID,
					"close_status", websocket.CloseStatus(err),
				)
				shutdown(websocket.StatusNormalClosure, "peer closed")
				break readLoop

			case readErrCtxDone:
				g.log.Info("ws.read.ctx_done", "session_id", sessionID, "err", err)
				shutdown(websocket.StatusNormalClosure, "context done")
				break readLoop

			case readErrConnClosed:
				g.log.Info("ws.read.conn_closed", "session_id", sessionID, "err", err)
				shutdown(websocket.StatusAbnormalClosure, "conn closed")
				break readLoop

			case readErrBadJSON:
				// Dev policy: tolerate malformed/partial frames.
				g.log.Info("ws.read.bad_json", "session_id", sessionID, "err", err)
				_ = g.sendError(ctx, client, "bad_json", "invalid JSON frame")
				continue readLoop

			default:
				g.log.Info("ws.read.fail", "session_id", sessionID, "err", err)
				shutdown(websocket.StatusAbnormalClosure, "read failed")
				break readLoop
			}
		}

		now := time.Now().UTC()
		if !limiter.Allow(now) {
			_ = g.sendError(ctx, client, "rate_limited", "too many events")
			shutdown(websocket.StatusPolicyViolation, "rate limited")
			break readLoop
		}

		if err := env.Validate(); err != nil {
			_ = g.sendError(ctx, client, "bad_envelope", err.Error())
			continue readLoop
		}

		switch env.Type {
		case v1.TypeHello:
			if err := g.onHello(ctx, client, env); err != nil {
				_ = g.sendError(ctx, client, "hello_failed", err.Error())
				shutdown(websocket.StatusPolicyViolation, "hello failed")
				break readLoop
			}

		case v1.TypeConversationJoin:
			conv, err := g.onJoin(ctx, client, env)
			if err != nil {
				_ = g.sendError(ctx, client, "join_failed", err.Error())
				continue readLoop
			}
			joined = conv

		case v1.TypeMessageSend:
			if joined == nil {
				_ = g.sendError(ctx, client, "not_joined", "join a conversation first")
				continue readLoop
			}
			if err := g.onMessageSend(ctx, client, joined, env); err != nil {
				_ = g.sendError(ctx, client, "send_failed", err.Error())
				continue readLoop
			}

		default:
			_ = g.sendError(ctx, client, "unsupported", fmt.Sprintf("type not allowed in PR-001: %s", env.Type))
			continue readLoop
		}

		if ctx.Err() != nil {
			break readLoop
		}
	}

	if joined != nil {
		joined.Leave(sessionID)
	}

	// Ensure shutdown (idempotent).
	shutdown(websocket.StatusNormalClosure, "bye")

	<-writerDone

	// Give the heartbeat loop a bounded grace period to exit.
	select {
	case <-heartbeatDone:
	case <-time.After(defaultCloseTimeout):
	}
}

// ---- Event handlers ----

func (g *WSGateway) onHello(ctx context.Context, client *Client, env v1.Envelope) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	var p v1.HelloPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	ackPayload, _ := json.Marshal(v1.HelloAckPayload{SessionID: client.SessionID})
	ack := newEnvelope(v1.TypeHelloAck, ackPayload)

	if ok := g.enqueue(ctx, client, ack); !ok {
		return errors.New("client backpressure: hello.ack not delivered")
	}

	g.log.Info("hello.ack", "session_id", client.SessionID)
	return nil
}

func (g *WSGateway) onJoin(ctx context.Context, client *Client, env v1.Envelope) (*Conversation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

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
	echo := newEnvelope(v1.TypeConversationJoin, echoPayload)

	if ok := g.enqueue(ctx, client, echo); !ok {
		return nil, errors.New("client backpressure: join echo not delivered")
	}

	g.log.Info("conversation.join", "session_id", client.SessionID, "conversation_id", conv.ID)
	return conv, nil
}

func (g *WSGateway) onMessageSend(ctx context.Context, client *Client, conv *Conversation, env v1.Envelope) error {
	if err := ctx.Err(); err != nil {
		return err
	}

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

	now := time.Now().UTC()
	serverMsgID := newRandomHex(16)

	stored, duplicated := conv.SendMessage(client.SessionID, p.ClientMsgID, text, serverMsgID, now)

	ackPayload, _ := json.Marshal(v1.MessageAckPayload{
		ConversationID: stored.ConversationID,
		ClientMsgID:    stored.ClientMsgID,
		ServerMsgID:    stored.ServerMsgID,
		Seq:            stored.Seq,
	})
	ack := newEnvelopeAt(v1.TypeMessageAck, ackPayload, now)

	// Always ACK, even for duplicates.
	if ok := g.enqueue(ctx, client, ack); !ok {
		return errors.New("client backpressure: ack not delivered")
	}

	if duplicated {
		g.log.Info(
			"message.dup",
			"conversation_id", conv.ID,
			"session_id", client.SessionID,
			"client_msg_id", p.ClientMsgID,
			"seq", stored.Seq,
		)
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
	newEnv := newEnvelopeAt(v1.TypeMessageNew, newPayload, now)
	conv.Broadcast(newEnv)

	g.log.Info("message.new", "conversation_id", conv.ID, "session_id", client.SessionID, "seq", stored.Seq)
	return nil
}

// ---- Sending helpers ----

func (g *WSGateway) sendError(ctx context.Context, client *Client, code string, msg string) error {
	p, _ := json.Marshal(v1.ErrorPayload{Code: code, Message: msg})
	env := newEnvelope(v1.TypeError, p)

	if ok := g.enqueue(ctx, client, env); !ok {
		return ctx.Err()
	}
	return nil
}

func (g *WSGateway) enqueue(ctx context.Context, client *Client, env v1.Envelope) bool {
	select {
	case client.Send <- env:
		return true
	case <-ctx.Done():
		return false
	default:
		return false
	}
}

// ---- Envelope helpers ----

func newEnvelope(typ string, payload json.RawMessage) v1.Envelope {
	return newEnvelopeAt(typ, payload, time.Now().UTC())
}

func newEnvelopeAt(typ string, payload json.RawMessage, ts time.Time) v1.Envelope {
	return v1.Envelope{
		V:       v1.Version,
		Type:    typ,
		ID:      newRandomHex(10),
		TS:      ts,
		Payload: payload,
	}
}

// ---- I/O helpers ----

func readEnvelope(parent context.Context, conn *websocket.Conn) (v1.Envelope, error) {
	// Read blocks until a complete message frame is received or ctx is done.
	mt, data, err := conn.Read(parent)
	if err != nil {
		return v1.Envelope{}, err
	}

	// Strictly accept JSON text/binary frames only.
	// (Some clients may send JSON as binary; both are OK.)
	if mt != websocket.MessageText && mt != websocket.MessageBinary {
		return v1.Envelope{}, fmt.Errorf("unsupported message type: %v", mt)
	}

	var env v1.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return v1.Envelope{}, err
	}
	return env, nil
}

func writeEnvelope(parent context.Context, conn *websocket.Conn, env v1.Envelope, d time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, d)
	defer cancel()

	b, err := json.Marshal(env)
	if err != nil {
		return err
	}

	// JSON is text. Using MessageText improves compatibility and debuggability.
	return conn.Write(ctx, websocket.MessageText, b)
}

// ---- Read error classification ----

type readErrKind uint8

const (
	readErrUnknown readErrKind = iota
	readErrClose
	readErrCtxDone
	readErrConnClosed
	readErrBadJSON
)

func classifyWSReadErr(err error) readErrKind {
	// CloseStatus returns -1 when the error is not a close frame / close error.
	if websocket.CloseStatus(err) != -1 {
		return readErrClose
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return readErrCtxDone
	}

	if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
		return readErrConnClosed
	}

	s := err.Error()
	if strings.Contains(s, "use of closed network connection") ||
		strings.Contains(s, "broken pipe") {
		return readErrConnClosed
	}

	// JSON parse failures (including truncated frames).
	if strings.Contains(s, "unexpected end of JSON input") ||
		strings.Contains(s, "invalid character") ||
		strings.Contains(s, "failed to unmarshal JSON") {
		return readErrBadJSON
	}

	return readErrUnknown
}

// ---- ID helpers (dev-only for PR-001) ----

func newRandomHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
