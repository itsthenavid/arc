package realtime

import (
	"context"
	"time"
)

// StoredMessage is the canonical persisted message representation.
type StoredMessage struct {
	ConversationID string
	ClientMsgID    string
	ServerMsgID    string
	Seq            int64
	SenderSession  string
	Text           string
	ServerTS       time.Time
}

// MessageStore persists and queries messages.
//
// Requirements:
//   - Idempotency per (conversation_id, client_msg_id)
//   - Monotonic seq per conversation (no gaps for duplicates)
//   - History query ordered by seq ASC
type MessageStore interface {
	AppendMessage(ctx context.Context, in AppendMessageInput) (AppendMessageResult, error)
	FetchHistory(ctx context.Context, in FetchHistoryInput) (FetchHistoryResult, error)
	Close() error
}

// AppendMessageInput describes a message append request.
type AppendMessageInput struct {
	ConversationID string
	ClientMsgID    string
	SenderSession  string
	Text           string
	Now            time.Time
}

// AppendMessageResult is the append operation result.
type AppendMessageResult struct {
	Stored     StoredMessage
	Duplicated bool
}

// FetchHistoryInput describes a history query request.
type FetchHistoryInput struct {
	ConversationID string
	AfterSeq       *int64
	Limit          int
}

// FetchHistoryResult contains the retrieved history window.
type FetchHistoryResult struct {
	Messages []StoredMessage
	HasMore  bool
}
