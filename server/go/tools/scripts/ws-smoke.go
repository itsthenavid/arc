// Package main provides a CI-friendly WebSocket smoke test for Arc realtime.
//
// It validates:
//   - handshake + subprotocol selection
//   - hello/ack session establishment
//   - join echo
//   - send -> ack
//   - fanout message_new to another client
//   - history fetch
//   - idempotent dedupe by client_msg_id
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	v1 "arc/shared/contracts/realtime/v1"

	"github.com/coder/websocket"
)

const (
	defaultSubprotocol = "arc.realtime.v1"
	maxReadBytes       = 1 << 20 // 1MiB
)

type smokeClient struct {
	name      string
	conn      *websocket.Conn
	sessionID string

	inbox chan v1.Envelope
	errCh chan error
}

func main() {
	var (
		wsURL   = flag.String("url", "ws://127.0.0.1:8080/ws", "WebSocket URL")
		origin  = flag.String("origin", "http://localhost", "Origin header to send (browser-like WS handshake)")
		convID  = flag.String("conv", "dev-room-1", "Conversation ID to join")
		kind    = flag.String("kind", "direct", "Conversation kind (echoed by server)")
		text    = flag.String("text", "hello arc 👋", "Message text to send")
		timeout = flag.Duration("timeout", 7*time.Second, "Per-step timeout")
		verbose = flag.Bool("v", false, "Verbose output")
	)
	flag.Parse()

	if err := validateWSURL(*wsURL); err != nil {
		fatalf("invalid -url: %v", err)
	}
	if err := validateOrigin(*origin); err != nil {
		fatalf("invalid -origin: %v", err)
	}

	root := context.Background()

	a := mustConnect(root, "A", *wsURL, *origin, *timeout)
	defer closeWS(a.conn)

	b := mustConnect(root, "B", *wsURL, *origin, *timeout)
	defer closeWS(b.conn)

	if *verbose {
		fmt.Printf("connected: A=%s B=%s origin=%q\n", a.sessionID, b.sessionID, *origin)
	}

	mustJoin(root, a, *convID, *kind, *timeout)
	mustJoin(root, b, *convID, *kind, *timeout)

	clientMsgID := fmt.Sprintf("cmsg-%d", time.Now().UnixNano())

	serverMsgID, seq := mustSendAndAssertAck(root, a, *convID, clientMsgID, *text, *timeout)

	mustAssertNew(root, b, *convID, clientMsgID, serverMsgID, seq, a.sessionID, *text, *timeout)

	_ = drainOptionalNew(root, a, 750*time.Millisecond)

	mustHistoryFetchContains(root, b, *convID, nil, 50, clientMsgID, serverMsgID, seq, a.sessionID, *text, *timeout)

	after := seq
	mustHistoryFetchEmpty(root, b, *convID, &after, 50, *timeout)

	_, seq2 := mustSendAndAssertAck(root, a, *convID, clientMsgID, *text, *timeout)
	if seq2 != seq {
		fatalf("dedupe: seq mismatch: first=%d second=%d", seq, seq2)
	}

	mustAssertNoType(root, b, v1.TypeMessageNew, 1200*time.Millisecond)
	mustAssertNoType(root, a, v1.TypeMessageNew, 1200*time.Millisecond)

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
	if strings.TrimSpace(u.Host) == "" {
		return errors.New("missing host")
	}
	if strings.TrimSpace(u.Path) == "" {
		return errors.New("missing path")
	}
	return nil
}

func validateOrigin(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("origin must be http/https, got: %s", u.Scheme)
	}
	if strings.TrimSpace(u.Host) == "" {
		return errors.New("origin missing host")
	}
	return nil
}

func mustConnect(parent context.Context, name, wsURL, origin string, stepTimeout time.Duration) *smokeClient {
	ctx, cancel := context.WithTimeout(parent, stepTimeout)
	defer cancel()

	h := http.Header{}
	if strings.TrimSpace(origin) != "" {
		h.Set("Origin", origin)
	}

	conn, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		Subprotocols: []string{defaultSubprotocol},
		HTTPHeader:   h,
	})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	if err != nil {
		fatalf("connect %s: %v", name, err)
	}

	assertSubprotocol(resp, defaultSubprotocol)

	conn.SetReadLimit(maxReadBytes)

	c := &smokeClient{
		name:  name,
		conn:  conn,
		inbox: make(chan v1.Envelope, 512),
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
	mustWriteWithTimeout(parent, conn, hello, stepTimeout)

	ack := c.mustReadUntilType(parent, v1.TypeHelloAck, stepTimeout, nil)

	var p v1.HelloAckPayload
	if err := json.Unmarshal(ack.Payload, &p); err != nil {
		fatalf("unmarshal hello.ack payload (%s): %v", name, err)
	}
	if strings.TrimSpace(p.SessionID) == "" {
		fatalf("hello.ack missing session_id (%s)", name)
	}
	c.sessionID = p.SessionID

	return c
}

func assertSubprotocol(resp *http.Response, want string) {
	if resp == nil {
		return
	}
	got := strings.TrimSpace(resp.Header.Get("Sec-WebSocket-Protocol"))
	if got == "" {
		return
	}
	if got != want {
		fatalf("subprotocol mismatch: got=%q want=%q", got, want)
	}
}

func (c *smokeClient) startReadLoop() {
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

func mustJoin(parent context.Context, c *smokeClient, convID, kind string, stepTimeout time.Duration) {
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
	mustWriteWithTimeout(parent, c.conn, env, stepTimeout)

	echo := c.mustReadUntilType(parent, v1.TypeConversationJoin, stepTimeout, nil)

	var p v1.ConversationJoinPayload
	if err := json.Unmarshal(echo.Payload, &p); err != nil {
		fatalf("unmarshal join echo payload (%s): %v", c.name, err)
	}
	if p.ConversationID != convID {
		fatalf("join echo conv_id mismatch (%s): got=%q want=%q", c.name, p.ConversationID, convID)
	}
	if strings.TrimSpace(p.Kind) == "" {
		fatalf("join echo missing kind (%s)", c.name)
	}
}

func mustSendAndAssertAck(parent context.Context, c *smokeClient, convID, clientMsgID, text string, stepTimeout time.Duration) (serverMsgID string, seq int64) {
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
	mustWriteWithTimeout(parent, c.conn, env, stepTimeout)

	skip := map[string]struct{}{v1.TypeMessageNew: {}}
	ack := c.mustReadUntilType(parent, v1.TypeMessageAck, stepTimeout, skip)

	var p v1.MessageAckPayload
	if err := json.Unmarshal(ack.Payload, &p); err != nil {
		fatalf("unmarshal message.ack payload (%s): %v", c.name, err)
	}
	if p.ConversationID != convID {
		fatalf("ack conv_id mismatch (%s): got=%q want=%q", c.name, p.ConversationID, convID)
	}
	if p.ClientMsgID != clientMsgID {
		fatalf("ack client_msg_id mismatch (%s): got=%q want=%q", c.name, p.ClientMsgID, clientMsgID)
	}
	if strings.TrimSpace(p.ServerMsgID) == "" {
		fatalf("ack missing server_msg_id (%s)", c.name)
	}
	if p.Seq <= 0 {
		fatalf("ack invalid seq (%s): %d", c.name, p.Seq)
	}
	return p.ServerMsgID, p.Seq
}

func mustAssertNew(parent context.Context, c *smokeClient, convID, clientMsgID, serverMsgID string, seq int64, senderSessionID, text string, stepTimeout time.Duration) {
	env := c.mustReadUntilType(parent, v1.TypeMessageNew, stepTimeout, nil)

	var p v1.MessageNewPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		fatalf("unmarshal message.new payload (%s): %v", c.name, err)
	}

	if p.ConversationID != convID {
		fatalf("new conv_id mismatch (%s): got=%q want=%q", c.name, p.ConversationID, convID)
	}
	if p.ClientMsgID != clientMsgID {
		fatalf("new client_msg_id mismatch (%s): got=%q want=%q", c.name, p.ClientMsgID, clientMsgID)
	}
	if p.ServerMsgID != serverMsgID {
		fatalf("new server_msg_id mismatch (%s): got=%q want=%q", c.name, p.ServerMsgID, serverMsgID)
	}
	if p.Seq != seq {
		fatalf("new seq mismatch (%s): got=%d want=%d", c.name, p.Seq, seq)
	}
	if p.Sender != senderSessionID {
		fatalf("new sender mismatch (%s): got=%q want=%q", c.name, p.Sender, senderSessionID)
	}
	if p.Text != text {
		fatalf("new text mismatch (%s): got=%q want=%q", c.name, p.Text, text)
	}
	if p.ServerTS.IsZero() {
		fatalf("new server_ts missing/zero (%s)", c.name)
	}
}

func mustHistoryFetchContains(
	parent context.Context,
	c *smokeClient,
	convID string,
	afterSeq *int64,
	limit int,
	clientMsgID, serverMsgID string,
	seq int64,
	senderSessionID, text string,
	stepTimeout time.Duration,
) {
	req := v1.Envelope{
		V:    v1.Version,
		Type: v1.TypeConversationHistoryFetch,
		ID:   fmt.Sprintf("%s-history-fetch", c.name),
		TS:   time.Now().UTC(),
		Payload: mustJSON(v1.ConversationHistoryFetchPayload{
			ConversationID: convID,
			AfterSeq:       afterSeq,
			Limit:          limit,
		}),
	}
	mustWriteWithTimeout(parent, c.conn, req, stepTimeout)

	chunk := c.mustReadUntilType(parent, v1.TypeConversationHistoryChunk, stepTimeout, nil)

	var p v1.ConversationHistoryChunkPayload
	if err := json.Unmarshal(chunk.Payload, &p); err != nil {
		fatalf("unmarshal history.chunk payload (%s): %v", c.name, err)
	}
	if p.ConversationID != convID {
		fatalf("history.chunk conv_id mismatch (%s): got=%q want=%q", c.name, p.ConversationID, convID)
	}

	found := false
	for _, m := range p.Messages {
		if m.ConversationID == convID &&
			m.ClientMsgID == clientMsgID &&
			m.ServerMsgID == serverMsgID &&
			m.Seq == seq &&
			m.Sender == senderSessionID &&
			m.Text == text &&
			!m.ServerTS.IsZero() {
			found = true
			break
		}
	}
	if !found {
		fatalf("history.chunk missing expected message (%s)", c.name)
	}
}

func mustHistoryFetchEmpty(parent context.Context, c *smokeClient, convID string, afterSeq *int64, limit int, stepTimeout time.Duration) {
	req := v1.Envelope{
		V:    v1.Version,
		Type: v1.TypeConversationHistoryFetch,
		ID:   fmt.Sprintf("%s-history-fetch-empty", c.name),
		TS:   time.Now().UTC(),
		Payload: mustJSON(v1.ConversationHistoryFetchPayload{
			ConversationID: convID,
			AfterSeq:       afterSeq,
			Limit:          limit,
		}),
	}
	mustWriteWithTimeout(parent, c.conn, req, stepTimeout)

	chunk := c.mustReadUntilType(parent, v1.TypeConversationHistoryChunk, stepTimeout, nil)

	var p v1.ConversationHistoryChunkPayload
	if err := json.Unmarshal(chunk.Payload, &p); err != nil {
		fatalf("unmarshal history.chunk payload (%s): %v", c.name, err)
	}
	if p.ConversationID != convID {
		fatalf("history.chunk conv_id mismatch (%s): got=%q want=%q", c.name, p.ConversationID, convID)
	}
	if len(p.Messages) != 0 {
		fatalf("expected empty history chunk (%s), got=%d", c.name, len(p.Messages))
	}
}

func drainOptionalNew(parent context.Context, c *smokeClient, wait time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, wait)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-c.errCh:
			if err != nil {
				return err
			}
			return errors.New("connection closed while draining")
		case env, ok := <-c.inbox:
			if !ok {
				return errors.New("connection closed while draining")
			}
			if env.Type == v1.TypeMessageNew {
				return nil
			}
		}
	}
}

func mustAssertNoType(parent context.Context, c *smokeClient, forbiddenType string, wait time.Duration) {
	ctx, cancel := context.WithTimeout(parent, wait)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-c.errCh:
			if err == nil {
				fatalf("connection closed unexpectedly (%s)", c.name)
			}
			fatalf("connection closed unexpectedly (%s): %v", c.name, err)
		case env, ok := <-c.inbox:
			if !ok {
				fatalf("connection closed unexpectedly (%s)", c.name)
			}
			if env.Type == v1.TypeError {
				var ep v1.ErrorPayload
				_ = json.Unmarshal(env.Payload, &ep)
				fatalf("server error (%s): code=%q msg=%q", c.name, ep.Code, ep.Message)
			}
			if env.Type == forbiddenType {
				fatalf("unexpected %s received (%s)", forbiddenType, c.name)
			}
		}
	}
}

func (c *smokeClient) mustReadUntilType(parent context.Context, wantType string, stepTimeout time.Duration, skipTypes map[string]struct{}) v1.Envelope {
	ctx, cancel := context.WithTimeout(parent, stepTimeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			fatalf("timeout waiting for %q (%s): %v", wantType, c.name, ctx.Err())
		case err := <-c.errCh:
			if err == nil {
				fatalf("connection closed while waiting for %q (%s)", wantType, c.name)
			}
			fatalf("connection error while waiting for %q (%s): %v", wantType, c.name, err)
		case env, ok := <-c.inbox:
			if !ok {
				fatalf("connection closed while waiting for %q (%s)", wantType, c.name)
			}
			if env.Type == wantType {
				return env
			}
			if env.Type == v1.TypeError {
				var ep v1.ErrorPayload
				_ = json.Unmarshal(env.Payload, &ep)
				fatalf("server error (%s): code=%q msg=%q", c.name, ep.Code, ep.Message)
			}
			if skipTypes != nil {
				if _, ok := skipTypes[env.Type]; ok {
					continue
				}
			}
			fatalf("unexpected envelope type (%s): got=%q want=%q", c.name, env.Type, wantType)
		}
	}
}

func mustWriteWithTimeout(parent context.Context, conn *websocket.Conn, env v1.Envelope, stepTimeout time.Duration) {
	ctx, cancel := context.WithTimeout(parent, stepTimeout)
	defer cancel()

	b, err := json.Marshal(env)
	if err != nil {
		fatalf("marshal envelope: %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		fatalf("write failed: %v", err)
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func closeWS(conn *websocket.Conn) {
	_ = conn.Close(websocket.StatusNormalClosure, "bye")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FAIL: "+format+"\n", args...)
	os.Exit(1)
}
