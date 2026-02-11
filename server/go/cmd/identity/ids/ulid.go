// Package ids provides identity ID primitives (e.g., ULID) used by the identity service.
package ids

import (
	"crypto/rand"
	"time"

	"github.com/oklog/ulid/v2"
)

// NewULID returns a new ULID string (26 chars).
// ULIDs are lexicographically sortable and work well in distributed systems.
func NewULID(now time.Time) (string, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}

	id, err := ulid.New(ulid.Timestamp(now), rand.Reader)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
