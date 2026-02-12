package session

import (
	"crypto/rand"
	"encoding/base64"

	"arc/cmd/security/token"
)

func newOpaqueRefreshToken(nBytes int) (plain string, hashHex string, err error) {
	b := make([]byte, nBytes)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}

	// URL-safe, no padding.
	plain = base64.RawURLEncoding.EncodeToString(b)

	hashHex = token.HashRefreshTokenHex(plain) // 64 hex chars

	return plain, hashHex, nil
}
