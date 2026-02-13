package realtime

import (
	"log/slog"
	"strings"
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
	return h.GetOrCreateConversationWithKind(conversationID, "direct")
}

// GetOrCreateConversationWithKind returns a stable conversation handle and records a normalized kind.
func (h *Hub) GetOrCreateConversationWithKind(conversationID, kind string) *Conversation {
	kind = normalizeConversationKind(kind)

	h.mu.Lock()
	defer h.mu.Unlock()

	if c, ok := h.conversations[conversationID]; ok {
		return c
	}

	c := NewConversation(h.log, conversationID, kind)
	h.conversations[conversationID] = c
	return c
}

func normalizeConversationKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "direct", "group", "room":
		return strings.ToLower(strings.TrimSpace(kind))
	default:
		return "direct"
	}
}
