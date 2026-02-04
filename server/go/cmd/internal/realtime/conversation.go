package realtime

import (
	"log/slog"
	"sync"

	v1 "arc/shared/contracts/realtime/v1"
)

// Conversation is an in-memory membership + broadcast fanout primitive.
//
// Concurrency guarantees:
// - Join/Leave are safe under concurrent Broadcast.
// - Broadcast never blocks (drops under backpressure).
// - Broadcast is panic-safe because Client.Send is never closed by the server.
type Conversation struct {
	log  *slog.Logger
	ID   string
	Kind string

	mu      sync.RWMutex
	members map[string]*Client
}

// NewConversation constructs a conversation.
func NewConversation(log *slog.Logger, id, kind string) *Conversation {
	return &Conversation{
		log:     log,
		ID:      id,
		Kind:    kind,
		members: make(map[string]*Client),
	}
}

// Join adds a client to membership.
func (c *Conversation) Join(client *Client) {
	if c == nil || client == nil || client.SessionID == "" {
		return
	}

	c.mu.Lock()
	c.members[client.SessionID] = client
	c.mu.Unlock()

	c.log.Info("conversation.member.join", "conversation_id", c.ID, "session_id", client.SessionID)
}

// Leave removes a client from membership and signals shutdown for that client.
func (c *Conversation) Leave(sessionID string) {
	if c == nil || sessionID == "" {
		return
	}

	var cl *Client

	c.mu.Lock()
	cl = c.members[sessionID]
	delete(c.members, sessionID)
	c.mu.Unlock()

	// Signal client shutdown after removing from membership.
	// This ordering avoids race windows where a broadcaster still holds a pointer
	// while the client goroutines are being torn down.
	if cl != nil {
		cl.Close()
	}

	c.log.Info("conversation.member.leave", "conversation_id", c.ID, "session_id", sessionID)
}

// Broadcast fanouts an envelope to all members.
// Non-blocking: if a member queue is full or the client is shutting down, it is dropped.
func (c *Conversation) Broadcast(env v1.Envelope) {
	if c == nil {
		return
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, m := range c.members {
		if m == nil {
			continue
		}

		select {
		case <-m.Done():
			// Skip clients that are shutting down.
			continue
		default:
		}

		select {
		case m.Send <- env:
		default:
			// Drop rather than block the whole conversation.
		}
	}
}
