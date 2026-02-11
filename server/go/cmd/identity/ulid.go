package identity

import (
	"time"

	"arc/cmd/identity/ids"
)

// NewULID returns a new ULID (26-char string).
func NewULID(now time.Time) (string, error) {
	return ids.NewULID(now)
}
