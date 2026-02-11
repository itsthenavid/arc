package password

import "errors"

// Public, stable errors for callers.
var (
	ErrPasswordTooShort = errors.New("password too short")
	ErrPasswordTooLong  = errors.New("password too long")
	ErrWeakPassword     = errors.New("weak password")
	ErrInvalidHash      = errors.New("invalid password hash")
)
