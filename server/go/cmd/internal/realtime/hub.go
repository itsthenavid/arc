package realtime

import (
	"log/slog"
	"sync"
)

// Hub owns in-memory conversations and provides stable conversation handles.
// It is intentionally minimal: persistence lives behind MessageStore.
type Hub struct {
	log *slog.Logger

	mu            sync.RWMutex
	conversations map[string]*Conversation
}

// NewHub constructs a Hub instance.
func NewHub(log *slog.Logger) *Hub {
	return &Hub{
		log:           log,
		conversations: make(map[string]*Conversation),
	}
}

// GetOrCreateConversation returns a stable in-memory conversation handle.
// Kind is currently "direct" in PR-001/PR-002.
func (h *Hub) GetOrCreateConversation(conversationID string) *Conversation {
	h.mu.Lock()
	defer h.mu.Unlock()

	if c, ok := h.conversations[conversationID]; ok {
		return c
	}

	c := NewConversation(h.log, conversationID, "direct")
	h.conversations[conversationID] = c
	return c
}
