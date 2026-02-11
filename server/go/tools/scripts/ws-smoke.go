// Package main provides a CI-friendly WebSocket smoke test for Arc realtime.
//
// It validates (English):
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
	"sync"
	"time"

	v1 "arc/shared/contracts/realtime/v1"

	"github.com/coder/websocket"
)

const (
	defaultSubprotocol = "arc.realtime.v1"
	maxReadBytes       = 1 << 20 // 1MiB

	defaultPerStepTimeout = 7 * time.Second

	defaultInboxSize = 512
)

type smokeClient struct {
	name      string
	conn      *websocket.Conn
	sessionID string

	inbox chan v1.Envelope
	errCh chan error

	ctx    context.Context
	cancel context.CancelFunc

	closeOnce sync.Once
}

func main() {
	var (
		wsURL   = flag.String("url", "ws://127.0.0.1:8080/ws", "WebSocket URL")
		origin  = flag.String("origin", "http://localhost", "Origin header to send (browser-like WS handshake)")
		convID  = flag.String("conv", "dev-room-1", "Conversation ID to join")
		kind    = flag.String("kind", "direct", "Conversation kind (echoed by server)")
		text    = flag.String("text", "hello arc 👋", "Message text to send")
		timeout = flag.Duration("timeout", defaultPerStepTimeout, "Per-step timeout")
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

	a := mustConnect(root, "A", *wsURL, *origin, *timeout, *verbose)
	defer a.Close()

	b := mustConnect(root, "B", *wsURL, *origin, *timeout, *verbose)
	defer b.Close()

	if *verbose {
		fmt.Printf("connected: A=%s B=%s origin=%q\n", a.sessionID, b.sessionID, *origin)
	}

	mustJoin(root, a, *convID, *kind, *timeout, *verbose)
	mustJoin(root, b, *convID, *kind, *timeout, *verbose)

	clientMsgID := fmt.Sprintf("cmsg-%d", time.Now().UnixNano())
	serverMsgID, seq := mustSendAndAssertAck(root, a, *convID, clientMsgID, *text, *timeout, *verbose)

	mustAssertNew(root, b, *convID, clientMsgID, serverMsgID, seq, a.sessionID, *text, *timeout, *verbose)

	// Sender might also receive fanout depending on server semantics; drain if present.
	_ = drainOptionalNew(root, a, 750*time.Millisecond)

	mustHistoryFetchContains(root, b, *convID, nil, 50, clientMsgID, serverMsgID, seq, a.sessionID, *text, *timeout, *verbose)

	after := seq
	mustHistoryFetchEmpty(root, b, *convID, &after, 50, *timeout, *verbose)

	// Dedupe: same client_msg_id must not create a new sequence.
	_, seq2 := mustSendAndAssertAck(root, a, *convID, clientMsgID, *text, *timeout, *verbose)
	if seq2 != seq {
		fatalf("dedupe: seq mismatch: first=%d second=%d", seq, seq2)
	}

	// Ensure no new fanout happened due to duplicate.
	mustAssertNoType(root, b, v1.TypeMessageNew, 1200*time.Millisecond, *verbose)
	mustAssertNoType(root, a, v1.TypeMessageNew, 1200*time.Millisecond, *verbose)

	fmt.Printf("OK: A=%s B=%s conv_id=%s seq=%d server_msg_id=%s\n", a.sessionID, b.sessionID, *convID, seq, serverMsgID)
}

// Close closes the client and stops the read loop (idempotent).
func (c *smokeClient) Close() {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() {
		if c.cancel != nil {
			c.cancel()
		}
		if c.conn != nil {
			_ = c.conn.Close(websocket.StatusNormalClosure, "bye")
		}
	})
}

// ---- validation ----

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

// ---- connect + hello ----

func mustConnect(parent context.Context, name, wsURL, origin string, stepTimeout time.Duration, verbose bool) *smokeClient {
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

	readCtx, readCancel := context.WithCancel(context.Background())

	c := &smokeClient{
		name:   name,
		conn:   conn,
		inbox:  make(chan v1.Envelope, defaultInboxSize),
		errCh:  make(chan error, 1),
		ctx:    readCtx,
		cancel: readCancel,
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

	ack := c.mustReadUntilType(parent, v1.TypeHelloAck, stepTimeout, verbose)

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
		// Some implementations may return nil response on success; best-effort skip.
		return
	}
	got := resp.Header.Get("Sec-WebSocket-Protocol")
	if strings.TrimSpace(want) == "" {
		return
	}
	if strings.TrimSpace(got) != want {
		fatalf("subprotocol mismatch: got=%q want=%q", got, want)
	}
}

// startReadLoop starts a background reader that pushes envelopes into inbox.
func (c *smokeClient) startReadLoop() {
	go func() {
		defer func() {
			// Signal end-of-loop as an error so waiters can fail fast.
			select {
			case c.errCh <- errors.New("read loop ended"):
			default:
			}
		}()

		for {
			select {
			case <-c.ctx.Done():
				return
			default:
			}

			mt, data, err := c.conn.Read(c.ctx)
			if err != nil {
				select {
				case c.errCh <- err:
				default:
				}
				return
			}
			if mt != websocket.MessageText && mt != websocket.MessageBinary {
				continue
			}

			var env v1.Envelope
			if err := json.Unmarshal(data, &env); err != nil {
				select {
				case c.errCh <- fmt.Errorf("bad json: %w", err):
				default:
				}
				return
			}

			select {
			case c.inbox <- env:
			case <-c.ctx.Done():
				return
			}
		}
	}()
}

// ---- protocol actions ----

func mustJoin(parent context.Context, c *smokeClient, convID, wantKind string, stepTimeout time.Duration, verbose bool) {
	env := v1.Envelope{
		V:    v1.Version,
		Type: v1.TypeConversationJoin,
		ID:   fmt.Sprintf("%s-join-%d", c.name, time.Now().UnixNano()),
		TS:   time.Now().UTC(),
		Payload: mustJSON(v1.ConversationJoinPayload{
			ConversationID: convID,
		}),
	}
	mustWriteWithTimeout(parent, c.conn, env, stepTimeout)

	echo := c.mustReadUntilType(parent, v1.TypeConversationJoin, stepTimeout, verbose)

	var p v1.ConversationJoinPayload
	if err := json.Unmarshal(echo.Payload, &p); err != nil {
		fatalf("unmarshal join echo payload (%s): %v", c.name, err)
	}
	if strings.TrimSpace(p.ConversationID) != convID {
		fatalf("join echo conversation_id mismatch (%s): got=%q want=%q", c.name, p.ConversationID, convID)
	}
	if strings.TrimSpace(wantKind) != "" && strings.TrimSpace(p.Kind) != "" && p.Kind != wantKind {
		fatalf("join echo kind mismatch (%s): got=%q want=%q", c.name, p.Kind, wantKind)
	}
}

func mustSendAndAssertAck(parent context.Context, c *smokeClient, convID, clientMsgID, text string, stepTimeout time.Duration, verbose bool) (serverMsgID string, seq int64) {
	env := v1.Envelope{
		V:    v1.Version,
		Type: v1.TypeMessageSend,
		ID:   fmt.Sprintf("%s-send-%d", c.name, time.Now().UnixNano()),
		TS:   time.Now().UTC(),
		Payload: mustJSON(v1.MessageSendPayload{
			ConversationID: convID,
			ClientMsgID:    clientMsgID,
			Text:           text,
		}),
	}
	mustWriteWithTimeout(parent, c.conn, env, stepTimeout)

	ack := c.mustReadUntilType(parent, v1.TypeMessageAck, stepTimeout, verbose)

	var p v1.MessageAckPayload
	if err := json.Unmarshal(ack.Payload, &p); err != nil {
		fatalf("unmarshal message.ack payload (%s): %v", c.name, err)
	}
	if p.ConversationID != convID {
		fatalf("ack conversation mismatch (%s): got=%q want=%q", c.name, p.ConversationID, convID)
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

func mustAssertNew(parent context.Context, c *smokeClient, convID, clientMsgID, serverMsgID string, seq int64, senderSession string, text string, stepTimeout time.Duration, verbose bool) {
	env := c.mustReadUntilType(parent, v1.TypeMessageNew, stepTimeout, verbose)

	var p v1.MessageNewPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		fatalf("unmarshal message.new payload (%s): %v", c.name, err)
	}
	if p.ConversationID != convID {
		fatalf("new conversation mismatch (%s): got=%q want=%q", c.name, p.ConversationID, convID)
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
	if senderSession != "" && p.Sender != senderSession {
		fatalf("new sender mismatch (%s): got=%q want=%q", c.name, p.Sender, senderSession)
	}
	if p.Text != text {
		fatalf("new text mismatch (%s): got=%q want=%q", c.name, p.Text, text)
	}
}

func mustHistoryFetchContains(parent context.Context, c *smokeClient, convID string, afterSeq *int64, limit int, wantClientMsgID, wantServerMsgID string, wantSeq int64, wantSender, wantText string, stepTimeout time.Duration, verbose bool) {
	env := v1.Envelope{
		V:    v1.Version,
		Type: v1.TypeConversationHistoryFetch,
		ID:   fmt.Sprintf("%s-history-%d", c.name, time.Now().UnixNano()),
		TS:   time.Now().UTC(),
		Payload: mustJSON(v1.ConversationHistoryFetchPayload{
			ConversationID: convID,
			AfterSeq:       afterSeq,
			Limit:          limit,
		}),
	}
	mustWriteWithTimeout(parent, c.conn, env, stepTimeout)

	chunk := c.mustReadUntilType(parent, v1.TypeConversationHistoryChunk, stepTimeout, verbose)

	var p v1.ConversationHistoryChunkPayload
	if err := json.Unmarshal(chunk.Payload, &p); err != nil {
		fatalf("unmarshal history.chunk payload (%s): %v", c.name, err)
	}
	if p.ConversationID != convID {
		fatalf("history conv mismatch (%s): got=%q want=%q", c.name, p.ConversationID, convID)
	}
	found := false
	for _, m := range p.Messages {
		if m.ClientMsgID == wantClientMsgID && m.ServerMsgID == wantServerMsgID && m.Seq == wantSeq {
			if wantSender != "" && m.Sender != wantSender {
				fatalf("history sender mismatch (%s): got=%q want=%q", c.name, m.Sender, wantSender)
			}
			if wantText != "" && m.Text != wantText {
				fatalf("history text mismatch (%s): got=%q want=%q", c.name, m.Text, wantText)
			}
			found = true
			break
		}
	}
	if !found {
		fatalf("history did not contain wanted message (%s): client_msg_id=%q server_msg_id=%q seq=%d", c.name, wantClientMsgID, wantServerMsgID, wantSeq)
	}
}

func mustHistoryFetchEmpty(parent context.Context, c *smokeClient, convID string, afterSeq *int64, limit int, stepTimeout time.Duration, verbose bool) {
	env := v1.Envelope{
		V:    v1.Version,
		Type: v1.TypeConversationHistoryFetch,
		ID:   fmt.Sprintf("%s-history-empty-%d", c.name, time.Now().UnixNano()),
		TS:   time.Now().UTC(),
		Payload: mustJSON(v1.ConversationHistoryFetchPayload{
			ConversationID: convID,
			AfterSeq:       afterSeq,
			Limit:          limit,
		}),
	}
	mustWriteWithTimeout(parent, c.conn, env, stepTimeout)

	chunk := c.mustReadUntilType(parent, v1.TypeConversationHistoryChunk, stepTimeout, verbose)

	var p v1.ConversationHistoryChunkPayload
	if err := json.Unmarshal(chunk.Payload, &p); err != nil {
		fatalf("unmarshal history.chunk payload (%s): %v", c.name, err)
	}
	if p.ConversationID != convID {
		fatalf("history conv mismatch (%s): got=%q want=%q", c.name, p.ConversationID, convID)
	}
	if len(p.Messages) != 0 {
		fatalf("expected empty history (%s), got %d messages", c.name, len(p.Messages))
	}
	if p.HasMore {
		fatalf("expected HasMore=false for empty history (%s)", c.name)
	}
}

// ---- assertions / drains ----

func drainOptionalNew(parent context.Context, c *smokeClient, dur time.Duration) bool {
	deadline := time.NewTimer(dur)
	defer deadline.Stop()

	for {
		select {
		case <-deadline.C:
			return false
		case <-parent.Done():
			return false
		case err := <-c.errCh:
			// If conn closed while draining, treat as failure (smoke must keep connection alive).
			fatalf("read error while draining (%s): %v", c.name, err)
		case env := <-c.inbox:
			if env.Type == v1.TypeMessageNew {
				return true
			}
			// Otherwise ignore.
		}
	}
}

func mustAssertNoType(parent context.Context, c *smokeClient, typ string, dur time.Duration, verbose bool) {
	t := time.NewTimer(dur)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			return
		case <-parent.Done():
			fatalf("context done while asserting no type (%s): %v", c.name, parent.Err())
		case err := <-c.errCh:
			fatalf("read error while asserting no type (%s): %v", c.name, err)
		case env := <-c.inbox:
			if verbose {
				fmt.Fprintf(os.Stderr, "[%s] recv type=%s id=%s\n", c.name, env.Type, env.ID)
			}
			if env.Type == typ {
				fatalf("unexpected envelope type=%s (%s)", typ, c.name)
			}
		}
	}
}

// ---- IO helpers ----

func mustWriteWithTimeout(parent context.Context, conn *websocket.Conn, env v1.Envelope, stepTimeout time.Duration) {
	ctx, cancel := context.WithTimeout(parent, stepTimeout)
	defer cancel()

	b, err := json.Marshal(env)
	if err != nil {
		fatalf("marshal envelope: %v", err)
	}

	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		fatalf("write: %v", err)
	}
}

func (c *smokeClient) mustReadUntilType(parent context.Context, typ string, stepTimeout time.Duration, verbose bool) v1.Envelope {
	ctx, cancel := context.WithTimeout(parent, stepTimeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			fatalf("timeout waiting for type=%s (%s)", typ, c.name)
		case err := <-c.errCh:
			fatalf("read error (%s): %v", c.name, err)
		case env := <-c.inbox:
			if verbose {
				fmt.Fprintf(os.Stderr, "[%s] recv type=%s id=%s\n", c.name, env.Type, env.ID)
			}
			if env.Type == v1.TypeError {
				var p v1.ErrorPayload
				_ = json.Unmarshal(env.Payload, &p)
				fatalf("server error (%s): code=%q msg=%q", c.name, p.Code, p.Message)
			}
			if env.Type == typ {
				return env
			}
			// Ignore everything else.
		}
	}
}

// ---- misc helpers ----

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		fatalf("json marshal: %v", err)
	}
	return b
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "ws-smoke: "+format+"\n", args...)
	os.Exit(1)
}
