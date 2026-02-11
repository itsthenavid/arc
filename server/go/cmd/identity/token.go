package identity

import (
	"crypto/rand"
	"encoding/base64"

	"arc/cmd/security/token"
)

// Token hashing hardening:
//
// English comment:
// - identity delegates refresh-token hashing to cmd/security/token as the single source of truth.
// - Output is always a 64-char hex string.
//
// Recommendation (prod):
// - Set ARC_TOKEN_HMAC_KEY to a long random secret (>= 32 bytes).

// NewOpaqueToken returns a cryptographically random token suitable for refresh tokens.
// It is URL-safe (base64url) and SHOULD be stored only on the client.
// The server stores only a hash (see HashRefreshTokenHex).
func NewOpaqueToken(nBytes int) (string, error) {
	if nBytes <= 0 {
		nBytes = 32
	}

	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	// URL-safe, no padding.
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashTokenSHA256Hex returns a SHA-256 hex hash of the token.
func HashTokenSHA256Hex(tokenStr string) string { return token.HashSHA256Hex(tokenStr) }

// HashTokenHMACSHA256Hex returns an HMAC-SHA256 hex digest of token using key.
func HashTokenHMACSHA256Hex(tokenStr string, key []byte) string {
	return token.HashHMACSHA256Hex(tokenStr, key)
}

// HashRefreshTokenHex returns the server-stored hash for refresh tokens.
// It uses HMAC-SHA256 if ARC_TOKEN_HMAC_KEY is set; otherwise falls back to SHA-256.
func HashRefreshTokenHex(tokenStr string) string { return token.HashRefreshTokenHex(tokenStr) }
