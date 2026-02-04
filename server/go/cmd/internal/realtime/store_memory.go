package realtime

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

const (
	memMaxMessagesPerConversation = 10_000
)

// InMemoryStore is a dev-only fallback when DB is not configured.
// It supports:
//   - AppendMessage: idempotent + seq allocation
//   - FetchHistory: paging by after_seq (for CI/smoke determinism)
type InMemoryStore struct {
	mu    sync.Mutex
	convs map[string]*memConv
}

type memConv struct {
	seq    int64
	dedupe map[string]StoredMessage // client_msg_id -> stored message
	msgs   []StoredMessage          // ordered by seq
}

// NewInMemoryStore constructs an in-memory MessageStore implementation.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		convs: make(map[string]*memConv),
	}
}

// Close closes the store (noop for in-memory).
func (s *InMemoryStore) Close() error { return nil }

// AppendMessage persists a message with idempotency and monotonic sequence allocation.
func (s *InMemoryStore) AppendMessage(ctx context.Context, in AppendMessageInput) (AppendMessageResult, error) {
	if in.ConversationID == "" || in.ClientMsgID == "" || in.SenderSession == "" {
		return AppendMessageResult{}, errors.New("invalid input")
	}
	if err := ctx.Err(); err != nil {
		return AppendMessageResult{}, err
	}

	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	c := s.convs[in.ConversationID]
	if c == nil {
		c = &memConv{
			dedupe: make(map[string]StoredMessage),
			msgs:   make([]StoredMessage, 0, 256),
		}
		s.convs[in.ConversationID] = c
	}

	if existing, ok := c.dedupe[in.ClientMsgID]; ok {
		return AppendMessageResult{Stored: existing, Duplicated: true}, nil
	}

	c.seq++
	msg := StoredMessage{
		ConversationID: in.ConversationID,
		ClientMsgID:    in.ClientMsgID,
		ServerMsgID:    NewRandomHex(16),
		Seq:            c.seq,
		SenderSession:  in.SenderSession,
		Text:           in.Text,
		ServerTS:       now,
	}
	c.dedupe[in.ClientMsgID] = msg
	c.msgs = append(c.msgs, msg)

	// Bound memory to avoid unbounded growth in dev.
	if len(c.msgs) > memMaxMessagesPerConversation {
		c.msgs = c.msgs[len(c.msgs)-memMaxMessagesPerConversation:]
	}

	return AppendMessageResult{Stored: msg, Duplicated: false}, nil
}

// FetchHistory returns messages ordered by seq ASC with paging via after_seq.
func (s *InMemoryStore) FetchHistory(ctx context.Context, in FetchHistoryInput) (FetchHistoryResult, error) {
	if in.ConversationID == "" {
		return FetchHistoryResult{}, errors.New("missing conversation_id")
	}
	if err := ctx.Err(); err != nil {
		return FetchHistoryResult{}, err
	}

	limit := in.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	fetch := limit + 1

	s.mu.Lock()
	c := s.convs[in.ConversationID]
	var snap []StoredMessage
	if c != nil {
		snap = append([]StoredMessage(nil), c.msgs...)
	}
	s.mu.Unlock()

	if len(snap) == 0 {
		return FetchHistoryResult{Messages: nil, HasMore: false}, nil
	}

	// Ensure ordering defensively.
	sort.Slice(snap, func(i, j int) bool { return snap[i].Seq < snap[j].Seq })

	start := 0
	if in.AfterSeq != nil {
		after := *in.AfterSeq
		start = sort.Search(len(snap), func(i int) bool { return snap[i].Seq > after })
		if start >= len(snap) {
			return FetchHistoryResult{Messages: nil, HasMore: false}, nil
		}
	}

	end := start + fetch
	if end > len(snap) {
		end = len(snap)
	}
	out := snap[start:end]

	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}

	return FetchHistoryResult{Messages: out, HasMore: hasMore}, nil
}
