package realtime

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewRandomHex returns a cryptographically secure random hex string.
// nBytes controls entropy; resulting length is nBytes*2 chars.
func NewRandomHex(nBytes int) string {
	if nBytes <= 0 {
		nBytes = 8
	}
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		// This should basically never happen; if it does, crash loudly.
		panic(fmt.Errorf("crypto/rand failed: %w", err))
	}
	return hex.EncodeToString(b)
}
