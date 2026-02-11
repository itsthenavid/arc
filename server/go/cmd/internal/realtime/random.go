package realtime

import (
	"crypto/rand"
	"encoding/hex"
)

// NewRandomHex returns a cryptographically secure random hex string of length 2*nBytes.
// If nBytes <= 0, it defaults to 16 bytes (32 hex chars).
func NewRandomHex(nBytes int) string {
	if nBytes <= 0 {
		nBytes = 16
	}

	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		// In the extremely rare case rand fails, return an empty string.
		// Callers should treat empty as an error-like condition in logs/tests.
		return ""
	}

	return hex.EncodeToString(b)
}
