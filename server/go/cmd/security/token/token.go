package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
)

const (
	// HMACEnvKey is the env var name for the token HMAC secret.
	// #nosec G101 -- not a credential; it's an environment variable name.
	HMACEnvKey = "ARC_TOKEN_HMAC_KEY"
)

// HashSHA256Hex returns a SHA-256 hex digest of s.
func HashSHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// HashHMACSHA256Hex returns an HMAC-SHA256 hex digest of s using key.
func HashHMACSHA256Hex(s string, key []byte) string {
	m := hmac.New(sha256.New, key)
	_, _ = m.Write([]byte(s))
	return hex.EncodeToString(m.Sum(nil))
}

// HMACKeyFromEnv returns the configured HMAC key bytes (trimmed), enforcing a minimum byte length.
// If the env var is missing/blank -> ErrHMACKeyMissing.
// If too short -> ErrHMACKeyTooShort.
func HMACKeyFromEnv(minBytes int) ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv(HMACEnvKey))
	if raw == "" {
		return nil, ErrHMACKeyMissing
	}
	b := []byte(raw)
	if minBytes > 0 && len(b) < minBytes {
		return nil, ErrHMACKeyTooShort
	}
	return b, nil
}

// HMACEnabled reports whether the env key is present (non-empty after trim).
// Note: This does not enforce minimum length. Use HMACKeyFromEnv for policy checks.
func HMACEnabled() bool {
	raw := strings.TrimSpace(os.Getenv(HMACEnvKey))
	return raw != ""
}

// HashRefreshTokenHex hashes refresh tokens for server-side storage.
// Behavior:
// - If ARC_TOKEN_HMAC_KEY is set (non-empty), uses HMAC-SHA256(token, key).
// - Otherwise falls back to SHA-256(token) for dev/back-compat.
func HashRefreshTokenHex(token string) string {
	key := strings.TrimSpace(os.Getenv(HMACEnvKey))
	if key == "" {
		return HashSHA256Hex(token)
	}
	return HashHMACSHA256Hex(token, []byte(key))
}

// HashRefreshTokenHexRequireHMAC hashes refresh tokens in enforced-HMAC mode.
// It fails if the key is missing or too short.
func HashRefreshTokenHexRequireHMAC(token string, minBytes int) (string, error) {
	key, err := HMACKeyFromEnv(minBytes)
	if err != nil {
		return "", err
	}
	return HashHMACSHA256Hex(token, key), nil
}
