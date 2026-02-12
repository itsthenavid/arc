package realtime

import (
	"sync"

	v1 "arc/shared/contracts/realtime/v1"
)

// Client represents one connected websocket session.
//
// Design notes:
// - Send is intentionally NOT closed by the server to avoid panics from concurrent broadcasters.
// - done is used to signal goroutines to stop.
// - Close is idempotent.
type Client struct {
	SessionID string
	UserID    string
	Send      chan v1.Envelope

	done      chan struct{}
	closeOnce sync.Once
}

// NewClient constructs a Client with a bounded send queue.
func NewClient(userID, sessionID string, sendQueueSize int) *Client {
	if sendQueueSize <= 0 {
		sendQueueSize = 64
	}
	return &Client{
		SessionID: sessionID,
		UserID:    userID,
		Send:      make(chan v1.Envelope, sendQueueSize),
		done:      make(chan struct{}),
	}
}

// Done returns a channel that is closed when the client is shutting down.
func (c *Client) Done() <-chan struct{} {
	if c == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return c.done
}

// Close signals the client goroutines to stop (idempotent).
// It does NOT close Send to keep broadcast safe under concurrency.
func (c *Client) Close() {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() {
		close(c.done)
	})
}
