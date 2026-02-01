package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	Version = 1

	TypeHello            = "hello"
	TypeHelloAck         = "hello.ack"
	TypeConversationJoin = "conversation.join"
	TypeMessageSend      = "message.send"
	TypeMessageAck       = "message.ack"
	TypeMessageNew       = "message.new"
	TypeError            = "error"
)

var AllowedTypes = map[string]struct{}{
	TypeHello:            {},
	TypeHelloAck:         {},
	TypeConversationJoin: {},
	TypeMessageSend:      {},
	TypeMessageAck:       {},
	TypeMessageNew:       {},
	TypeError:            {},
}

type Envelope struct {
	V       int             `json:"v"`
	Type    string          `json:"type"`
	ID      string          `json:"id"`
	TS      time.Time       `json:"ts"`
	Payload json.RawMessage `json:"payload"`
}

func (e Envelope) Validate() error {
	if e.V != Version {
		return fmt.Errorf("invalid protocol version: got=%d want=%d", e.V, Version)
	}
	if e.Type == "" {
		return errors.New("missing type")
	}
	if _, ok := AllowedTypes[e.Type]; !ok {
		return fmt.Errorf("unsupported type: %s", e.Type)
	}
	if e.ID == "" {
		return errors.New("missing id")
	}
	if e.TS.IsZero() {
		return errors.New("missing ts")
	}
	if e.Payload == nil {
		return errors.New("missing payload")
	}
	return nil
}

type HelloPayload struct {
	Token string `json:"token,omitempty"`
}

type HelloAckPayload struct {
	SessionID string `json:"session_id"`
}

type ConversationJoinPayload struct {
	ConversationID string `json:"conversation_id"`
	Kind           string `json:"kind"`
}

type MessageSendPayload struct {
	ConversationID string `json:"conversation_id"`
	ClientMsgID    string `json:"client_msg_id"`
	Text           string `json:"text"`
}

type MessageAckPayload struct {
	ConversationID string `json:"conversation_id"`
	ClientMsgID    string `json:"client_msg_id"`
	ServerMsgID    string `json:"server_msg_id"`
	Seq            int64  `json:"seq"`
}

type MessageNewPayload struct {
	ConversationID string    `json:"conversation_id"`
	ClientMsgID    string    `json:"client_msg_id"`
	ServerMsgID    string    `json:"server_msg_id"`
	Seq            int64     `json:"seq"`
	Sender         string    `json:"sender_session_id"`
	Text           string    `json:"text"`
	ServerTS       time.Time `json:"server_ts"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
