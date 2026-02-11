package token

import "errors"

// Public, stable errors for callers.
var (
	ErrHMACKeyMissing  = errors.New("token HMAC key missing")
	ErrHMACKeyTooShort = errors.New("token HMAC key too short")
)
