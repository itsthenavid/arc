// Package v1 defines the Arc Realtime Protocol v1 contract.
//
// This package is intentionally stable and dependency-light.
// It is shared between server and clients to keep the wire protocol authoritative.
package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Version is the protocol version identifier embedded into every envelope.
const Version = "v1"

// Type constants (wire-stable).
const (
	// TypeHello starts a session handshake (client -> server).
	TypeHello = "hello"
	// TypeHelloAck acknowledges the session handshake (server -> client).
	TypeHelloAck = "hello_ack"

	// TypeConversationJoin joins a conversation (client -> server) and is echoed back.
	TypeConversationJoin = "conversation_join"

	// TypeMessageSend requests sending a new message (client -> server).
	TypeMessageSend = "message_send"
	// TypeMessageAck acknowledges a send request (server -> client).
	TypeMessageAck = "message_ack"
	// TypeMessageNew broadcasts a newly accepted message (server -> conversation members).
	TypeMessageNew = "message_new"

	// TypeConversationHistoryFetch requests conversation history (client -> server).
	TypeConversationHistoryFetch = "conversation_history_fetch"
	// TypeConversationHistoryChunk returns a window of history (server -> client).
	TypeConversationHistoryChunk = "conversation_history_chunk"

	// TypeError is a generic error envelope (server -> client).
	TypeError = "error"
)

// Envelope is the canonical wire wrapper.
type Envelope struct {
	V       string          `json:"v"`
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	ConvID  string          `json:"conv_id,omitempty"`
	TS      time.Time       `json:"ts,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Validate performs strict structural validation for an Envelope.
func (e Envelope) Validate() error {
	if strings.TrimSpace(e.V) == "" {
		return errors.New("missing field: v")
	}
	if e.V != Version {
		return fmt.Errorf("unsupported protocol version: %q", e.V)
	}
	if strings.TrimSpace(e.Type) == "" {
		return errors.New("missing field: type")
	}

	switch e.Type {
	case TypeHello,
		TypeHelloAck,
		TypeConversationJoin,
		TypeMessageSend,
		TypeMessageAck,
		TypeMessageNew,
		TypeConversationHistoryFetch,
		TypeConversationHistoryChunk,
		TypeError:
		return nil
	default:
		return fmt.Errorf("unknown type: %q", e.Type)
	}
}

// ---- Payloads ----

// HelloPayload is sent by the client to initiate a session.
type HelloPayload struct{}

// HelloAckPayload must carry SessionID (used by ws-smoke + server logic).
type HelloAckPayload struct {
	SessionID string `json:"session_id"`
}

// ConversationJoinPayload requests membership in a conversation.
type ConversationJoinPayload struct {
	ConversationID string `json:"conversation_id"`
	Kind           string `json:"kind,omitempty"`
}

// MessageSendPayload requests sending a message into a conversation.
type MessageSendPayload struct {
	ConversationID string `json:"conversation_id"`
	ClientMsgID    string `json:"client_msg_id"`
	Text           string `json:"text"`
}

// MessageAckPayload acknowledges a send request and returns the canonical server ids.
type MessageAckPayload struct {
	ConversationID string `json:"conversation_id"`
	ClientMsgID    string `json:"client_msg_id"`
	ServerMsgID    string `json:"server_msg_id"`
	Seq            int64  `json:"seq"`
}

// MessageNewPayload is broadcast when a new message is accepted (non-duplicate).
type MessageNewPayload struct {
	ConversationID string    `json:"conversation_id"`
	ClientMsgID    string    `json:"client_msg_id"`
	ServerMsgID    string    `json:"server_msg_id"`
	Seq            int64     `json:"seq"`
	Sender         string    `json:"sender"`
	Text           string    `json:"text"`
	ServerTS       time.Time `json:"server_ts"`
}

// ConversationHistoryFetchPayload requests a history window for a conversation.
type ConversationHistoryFetchPayload struct {
	ConversationID string `json:"conversation_id"`
	AfterSeq       *int64 `json:"after_seq,omitempty"`
	Limit          int    `json:"limit,omitempty"`
}

// ConversationHistoryChunkPayload returns messages for a history fetch request.
type ConversationHistoryChunkPayload struct {
	ConversationID string              `json:"conversation_id"`
	Messages       []MessageNewPayload `json:"messages"`
	HasMore        bool                `json:"has_more"`
}

// ErrorPayload is a generic error response payload.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
