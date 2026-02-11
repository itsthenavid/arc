package realtime

import (
	"time"

	"arc/cmd/identity/ids"
)

// NewSessionID returns a ULID used as websocket session id.
// Must be 26 chars to satisfy DB constraints / FKs.
func NewSessionID(now time.Time) (string, error) {
	return ids.NewULID(now)
}

// NewEnvelopeID returns a ULID used as envelope id.
// ULID is preferable to random hex for tracing and ordering in logs.
func NewEnvelopeID(now time.Time) (string, error) {
	return ids.NewULID(now)
}

// NewServerMsgID returns a ULID used as server_msg_id.
// This keeps IDs uniform across the system.
func NewServerMsgID(now time.Time) (string, error) {
	return ids.NewULID(now)
}
