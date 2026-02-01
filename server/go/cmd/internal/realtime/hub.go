package realtime

import (
	"log/slog"
	"sync"
	"time"

	v1 "arc/shared/contracts/realtime/v1"
)

type Client struct {
	SessionID string
	Send      chan v1.Envelope
}

type StoredMessage struct {
	ConversationID string
	ClientMsgID    string
	ServerMsgID    string
	Seq            int64
	SenderSession  string
	Text           string
	ServerTS       time.Time
}

type Conversation struct {
	ID   string
	Kind string // "direct"

	mu sync.Mutex

	seq int64

	// session_id -> client
	members map[string]*Client

	// idempotency: client_msg_id -> stored message
	dedupe map[string]StoredMessage
}

func NewConversation(id string) *Conversation {
	return &Conversation{
		ID:      id,
		Kind:    "direct",
		members: make(map[string]*Client),
		dedupe:  make(map[string]StoredMessage),
	}
}

type Hub struct {
	log *slog.Logger

	mu            sync.RWMutex
	conversations map[string]*Conversation
}

func NewHub(log *slog.Logger) *Hub {
	return &Hub{
		log:           log,
		conversations: make(map[string]*Conversation),
	}
}

func (h *Hub) GetOrCreateConversation(conversationID string) *Conversation {
	h.mu.Lock()
	defer h.mu.Unlock()

	if c, ok := h.conversations[conversationID]; ok {
		return c
	}
	c := NewConversation(conversationID)
	h.conversations[conversationID] = c
	return c
}

func (c *Conversation) Join(cl *Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.members[cl.SessionID] = cl
}

func (c *Conversation) Leave(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.members, sessionID)
}

func (c *Conversation) Broadcast(env v1.Envelope) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, m := range c.members {
		// Non-blocking send: if a client is slow, it will be disconnected upstream.
		select {
		case m.Send <- env:
		default:
		}
	}
}

func (c *Conversation) SendMessage(senderSessionID string, clientMsgID string, text string, serverMsgID string, now time.Time) (StoredMessage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if existing, ok := c.dedupe[clientMsgID]; ok {
		return existing, true
	}

	c.seq++
	msg := StoredMessage{
		ConversationID: c.ID,
		ClientMsgID:    clientMsgID,
		ServerMsgID:    serverMsgID,
		Seq:            c.seq,
		SenderSession:  senderSessionID,
		Text:           text,
		ServerTS:       now,
	}
	c.dedupe[clientMsgID] = msg
	return msg, false
}
