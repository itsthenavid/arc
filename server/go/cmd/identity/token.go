package identity

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"strings"
)

// Token hashing hardening:
//
// English comment:
// - If ARC_TOKEN_HMAC_KEY is set, we use HMAC-SHA256(token, key) and store its hex output.
// - If it is NOT set, we fall back to plain SHA-256(token) for dev/back-compat.
// - Output is always a 64-char hex string.
//
// Recommendation (prod):
//
//	Set ARC_TOKEN_HMAC_KEY to a long random secret (>= 32 bytes).
const tokenHMACEnvKey = "ARC_TOKEN_HMAC_KEY" // #nosec G101 -- not a credential; it's an environment variable name.

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
// This is kept for explicitness and for cases where you intentionally want raw SHA-256.
func HashTokenSHA256Hex(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// HashTokenHMACSHA256Hex returns an HMAC-SHA256 hex digest of token using key.
func HashTokenHMACSHA256Hex(token string, key []byte) string {
	m := hmac.New(sha256.New, key)
	_, _ = m.Write([]byte(token))
	return hex.EncodeToString(m.Sum(nil))
}

// HashRefreshTokenHex returns the server-stored hash for refresh tokens.
// It uses HMAC-SHA256 if ARC_TOKEN_HMAC_KEY is set; otherwise falls back to SHA-256.
func HashRefreshTokenHex(token string) string {
	key := strings.TrimSpace(os.Getenv(tokenHMACEnvKey))
	if key == "" {
		return HashTokenSHA256Hex(token)
	}
	return HashTokenHMACSHA256Hex(token, []byte(key))
}
