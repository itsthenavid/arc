// Package main provides a CI-friendly websocket smoke test for PR-001.
// It connects TWO clients and asserts:
//   - hello -> hello.ack
//   - conversation.join echo
//   - message.send -> message.ack + message.new broadcast
//   - dedupe: second send with same client_msg_id yields ack but no second message.new
//
// Design notes:
//   - The server uses Ping() and expects the peer to process control frames.
//     Therefore, each smoke client runs a dedicated read loop to keep the connection healthy.
//   - This is a smoke test, not a fuzz test. Fail fast on unexpected conditions.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	v1 "arc/shared/contracts/realtime/v1"

	"github.com/coder/websocket"
)

type smokeClient struct {
	name      string
	conn      *websocket.Conn
	sessionID string

	// inbox carries validated envelopes. It is bounded to avoid test deadlocks.
	inbox chan v1.Envelope

	// errCh receives the first terminal read-loop error (best-effort).
	errCh chan error
}

func main() {
	var (
		wsURL   = flag.String("url", "ws://localhost:8080/ws", "WebSocket URL")
		convID  = flag.String("conv", "dev-room-1", "Conversation ID to join")
		kind    = flag.String("kind", "direct", "Conversation kind (echoed by server)")
		text    = flag.String("text", "hello arc 👋", "Message text to send")
		timeout = flag.Duration("timeout", 5*time.Second, "Per-step timeout")
		verbose = flag.Bool("v", false, "Verbose output")
	)
	flag.Parse()

	if err := validateWSURL(*wsURL); err != nil {
		fatalf("invalid -url: %v", err)
	}

	rootCtx := context.Background()

	a, err := connect(rootCtx, "A", *wsURL, *timeout)
	if err != nil {
		fatalf("connect A: %v", err)
	}
	defer closeWS(a.conn)

	b, err := connect(rootCtx, "B", *wsURL, *timeout)
	if err != nil {
		fatalf("connect B: %v", err)
	}
	defer closeWS(b.conn)

	if *verbose {
		fmt.Printf("connected: A=%s B=%s\n", a.sessionID, b.sessionID)
	}

	if err := join(rootCtx, a, *convID, *kind, *timeout); err != nil {
		fatalf("join A: %v", err)
	}
	if err := join(rootCtx, b, *convID, *kind, *timeout); err != nil {
		fatalf("join B: %v", err)
	}

	clientMsgID := fmt.Sprintf("cmsg-%d", time.Now().UnixNano())

	// First send from A.
	serverMsgID, seq, err := sendAndAssertAck(rootCtx, a, *convID, clientMsgID, *text, *timeout)
	if err != nil {
		fatalf("send/ack: %v", err)
	}

	// Broadcast: assert B receives message.new.
	if err := assertNew(rootCtx, b, *convID, clientMsgID, serverMsgID, seq, a.sessionID, *text, *timeout); err != nil {
		fatalf("broadcast assert (B): %v", err)
	}

	// Sender may also receive message.new; drain it if present to keep inbox clean.
	_ = drainOptionalNew(rootCtx, a, *convID, clientMsgID, serverMsgID, seq, a.sessionID, *text, 750*time.Millisecond)

	// Dedupe: send same client_msg_id again (A should receive ack, but no new broadcast).
	_, seq2, err := sendAndAssertAck(rootCtx, a, *convID, clientMsgID, *text, *timeout)
	if err != nil {
		fatalf("dedupe send/ack: %v", err)
	}
	if seq2 != seq {
		fatalf("dedupe: seq mismatch: first=%d second=%d", seq, seq2)
	}

	// Dedupe assert: no second message.new should arrive on B (or A).
	if err := assertNoType(rootCtx, b, v1.TypeMessageNew, 1200*time.Millisecond); err != nil {
		fatalf("dedupe: expected no second message.new on B: %v", err)
	}
	if err := assertNoType(rootCtx, a, v1.TypeMessageNew, 1200*time.Millisecond); err != nil {
		fatalf("dedupe: expected no second message.new on A: %v", err)
	}

	fmt.Printf("OK: A=%s B=%s conv_id=%s seq=%d server_msg_id=%s\n", a.sessionID, b.sessionID, *convID, seq, serverMsgID)
}

func validateWSURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("missing host")
	}
	if u.Path == "" {
		return errors.New("missing path")
	}
	return nil
}

func connect(parent context.Context, name, wsURL string, stepTimeout time.Duration) (*smokeClient, error) {
	ctx, cancel := context.WithTimeout(parent, stepTimeout)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		Subprotocols: []string{"arc.realtime.v1"},
	})
	if err != nil {
		return nil, err
	}

	// Keep memory bounded in case a server misbehaves.
	conn.SetReadLimit(1 << 20)

	c := &smokeClient{
		name:  name,
		conn:  conn,
		inbox: make(chan v1.Envelope, 128),
		errCh: make(chan error, 1),
	}
	c.startReadLoop()

	hello := v1.Envelope{
		V:       v1.Version,
		Type:    v1.TypeHello,
		ID:      fmt.Sprintf("%s-hello", name),
		TS:      time.Now().UTC(),
		Payload: mustJSON(v1.HelloPayload{}),
	}
	if err := writeWithTimeout(parent, conn, hello, stepTimeout); err != nil {
		return nil, fmt.Errorf("write hello: %w", err)
	}

	ack, err := c.readUntilType(parent, v1.TypeHelloAck, stepTimeout, nil)
	if err != nil {
		return nil, fmt.Errorf("read hello.ack: %w", err)
	}

	var p v1.HelloAckPayload
	if err := json.Unmarshal(ack.Payload, &p); err != nil {
		return nil, fmt.Errorf("unmarshal hello.ack payload: %w", err)
	}
	if strings.TrimSpace(p.SessionID) == "" {
		return nil, errors.New("hello.ack missing session_id")
	}

	c.sessionID = p.SessionID
	return c, nil
}

func (c *smokeClient) startReadLoop() {
	// Background read loop: keeps draining frames so control frames are processed.
	go func() {
		defer close(c.inbox)

		for {
			mt, data, err := c.conn.Read(context.Background())
			if err != nil {
				select {
				case c.errCh <- err:
				default:
				}
				return
			}

			if mt != websocket.MessageText && mt != websocket.MessageBinary {
				select {
				case c.errCh <- fmt.Errorf("unsupported message type: %v", mt):
				default:
				}
				return
			}

			var env v1.Envelope
			if err := json.Unmarshal(data, &env); err != nil {
				select {
				case c.errCh <- fmt.Errorf("bad json: %w", err):
				default:
				}
				return
			}
			if err := env.Validate(); err != nil {
				select {
				case c.errCh <- fmt.Errorf("bad envelope: %w", err):
				default:
				}
				return
			}

			// Deliver to inbox; if consumer is too slow, fail hard to avoid deadlock.
			select {
			case c.inbox <- env:
			default:
				select {
				case c.errCh <- errors.New("inbox overflow: consumer too slow"):
				default:
				}
				return
			}
		}
	}()
}

func join(parent context.Context, c *smokeClient, convID, kind string, stepTimeout time.Duration) error {
	env := v1.Envelope{
		V:    v1.Version,
		Type: v1.TypeConversationJoin,
		ID:   fmt.Sprintf("%s-join", c.name),
		TS:   time.Now().UTC(),
		Payload: mustJSON(v1.ConversationJoinPayload{
			ConversationID: convID,
			Kind:           kind,
		}),
	}
	if err := writeWithTimeout(parent, c.conn, env, stepTimeout); err != nil {
		return fmt.Errorf("write join: %w", err)
	}

	echo, err := c.readUntilType(parent, v1.TypeConversationJoin, stepTimeout, nil)
	if err != nil {
		return fmt.Errorf("read join echo: %w", err)
	}

	var p v1.ConversationJoinPayload
	if err := json.Unmarshal(echo.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal join echo payload: %w", err)
	}
	if p.ConversationID != convID {
		return fmt.Errorf("join echo conv_id mismatch: got=%q want=%q", p.ConversationID, convID)
	}
	if strings.TrimSpace(p.Kind) == "" {
		return errors.New("join echo missing kind")
	}
	return nil
}

func sendAndAssertAck(parent context.Context, c *smokeClient, convID, clientMsgID, text string, stepTimeout time.Duration) (serverMsgID string, seq int64, err error) {
	env := v1.Envelope{
		V:    v1.Version,
		Type: v1.TypeMessageSend,
		ID:   fmt.Sprintf("%s-send-%s", c.name, clientMsgID),
		TS:   time.Now().UTC(),
		Payload: mustJSON(v1.MessageSendPayload{
			ConversationID: convID,
			ClientMsgID:    clientMsgID,
			Text:           text,
		}),
	}
	if err := writeWithTimeout(parent, c.conn, env, stepTimeout); err != nil {
		return "", 0, fmt.Errorf("write message.send: %w", err)
	}

	// Some servers may deliver message.new to sender; skip it while waiting for ack.
	skip := map[string]struct{}{
		v1.TypeMessageNew: {},
	}
	ack, err := c.readUntilType(parent, v1.TypeMessageAck, stepTimeout, skip)
	if err != nil {
		return "", 0, fmt.Errorf("read message.ack: %w", err)
	}

	var p v1.MessageAckPayload
	if err := json.Unmarshal(ack.Payload, &p); err != nil {
		return "", 0, fmt.Errorf("unmarshal message.ack payload: %w", err)
	}
	if p.ConversationID != convID {
		return "", 0, fmt.Errorf("ack conv_id mismatch: got=%q want=%q", p.ConversationID, convID)
	}
	if p.ClientMsgID != clientMsgID {
		return "", 0, fmt.Errorf("ack client_msg_id mismatch: got=%q want=%q", p.ClientMsgID, clientMsgID)
	}
	if strings.TrimSpace(p.ServerMsgID) == "" {
		return "", 0, errors.New("ack missing server_msg_id")
	}
	if p.Seq <= 0 {
		return "", 0, fmt.Errorf("ack invalid seq: %d", p.Seq)
	}

	return p.ServerMsgID, p.Seq, nil
}

func assertNew(parent context.Context, c *smokeClient, convID, clientMsgID, serverMsgID string, seq int64, senderSessionID, text string, stepTimeout time.Duration) error {
	env, err := c.readUntilType(parent, v1.TypeMessageNew, stepTimeout, nil)
	if err != nil {
		return fmt.Errorf("read message.new: %w", err)
	}

	var p v1.MessageNewPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal message.new payload: %w", err)
	}

	if p.ConversationID != convID {
		return fmt.Errorf("new conv_id mismatch: got=%q want=%q", p.ConversationID, convID)
	}
	if p.ClientMsgID != clientMsgID {
		return fmt.Errorf("new client_msg_id mismatch: got=%q want=%q", p.ClientMsgID, clientMsgID)
	}
	if p.ServerMsgID != serverMsgID {
		return fmt.Errorf("new server_msg_id mismatch: got=%q want=%q", p.ServerMsgID, serverMsgID)
	}
	if p.Seq != seq {
		return fmt.Errorf("new seq mismatch: got=%d want=%d", p.Seq, seq)
	}
	if p.Sender != senderSessionID {
		return fmt.Errorf("new sender_session_id mismatch: got=%q want=%q", p.Sender, senderSessionID)
	}
	if p.Text != text {
		return fmt.Errorf("new text mismatch: got=%q want=%q", p.Text, text)
	}
	if p.ServerTS.IsZero() {
		return errors.New("new server_ts missing/zero")
	}

	return nil
}

// drainOptionalNew drains one message.new if it arrives within wait; otherwise OK.
func drainOptionalNew(parent context.Context, c *smokeClient, convID, clientMsgID, serverMsgID string, seq int64, senderSessionID, text string, wait time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, wait)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return nil

		case err := <-c.errCh:
			if err != nil {
				return fmt.Errorf("connection error while draining: %w", err)
			}
			return errors.New("connection closed while draining")

		case env, ok := <-c.inbox:
			if !ok {
				return errors.New("connection closed while draining")
			}
			if env.Type == v1.TypeMessageNew {
				// Best-effort assert (optional).
				var p v1.MessageNewPayload
				if err := json.Unmarshal(env.Payload, &p); err != nil {
					return fmt.Errorf("unmarshal drained message.new: %w", err)
				}
				if p.ConversationID != convID || p.ClientMsgID != clientMsgID || p.ServerMsgID != serverMsgID || p.Seq != seq || p.Sender != senderSessionID || p.Text != text {
					return fmt.Errorf("drained message.new mismatch: %+v", p)
				}
				return nil
			}
			// Ignore other types while draining.
		}
	}
}

// assertNoType waits for duration and fails if it receives forbiddenType.
func assertNoType(parent context.Context, c *smokeClient, forbiddenType string, wait time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, wait)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return nil

		case err := <-c.errCh:
			if err == nil {
				return errors.New("connection closed unexpectedly")
			}
			// Connection closed is NOT OK here; it hides real protocol issues.
			return fmt.Errorf("connection closed unexpectedly: %w", err)

		case env, ok := <-c.inbox:
			if !ok {
				return errors.New("connection closed unexpectedly")
			}
			if env.Type == v1.TypeError {
				var ep v1.ErrorPayload
				_ = json.Unmarshal(env.Payload, &ep)
				return fmt.Errorf("server error: code=%q msg=%q", ep.Code, ep.Message)
			}
			if env.Type == forbiddenType {
				return fmt.Errorf("unexpected %s received", forbiddenType)
			}
			// Ignore other messages and keep waiting.
		}
	}
}

// readUntilType reads from inbox until wantType or timeout.
// skipTypes (optional) defines message types to ignore while waiting.
func (c *smokeClient) readUntilType(parent context.Context, wantType string, stepTimeout time.Duration, skipTypes map[string]struct{}) (v1.Envelope, error) {
	ctx, cancel := context.WithTimeout(parent, stepTimeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return v1.Envelope{}, ctx.Err()

		case err := <-c.errCh:
			if err == nil {
				return v1.Envelope{}, errors.New("connection closed")
			}
			return v1.Envelope{}, err

		case env, ok := <-c.inbox:
			if !ok {
				return v1.Envelope{}, errors.New("connection closed")
			}

			if env.Type == wantType {
				return env, nil
			}

			if env.Type == v1.TypeError {
				var ep v1.ErrorPayload
				_ = json.Unmarshal(env.Payload, &ep)
				return v1.Envelope{}, fmt.Errorf("server error: code=%q msg=%q", ep.Code, ep.Message)
			}

			if skipTypes != nil {
				if _, ok := skipTypes[env.Type]; ok {
					continue
				}
			}

			return v1.Envelope{}, fmt.Errorf("unexpected envelope type: got=%q want=%q", env.Type, wantType)
		}
	}
}

func writeWithTimeout(parent context.Context, conn *websocket.Conn, env v1.Envelope, stepTimeout time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, stepTimeout)
	defer cancel()

	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, b)
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func closeWS(conn *websocket.Conn) {
	// Best-effort close; smoke test is short-lived.
	_ = conn.Close(websocket.StatusNormalClosure, "bye")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FAIL: "+format+"\n", args...)
	os.Exit(1)
}
