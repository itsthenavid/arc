package invite

import "errors"

var (
	// ErrInvalidInput indicates invalid invite input or configuration.
	ErrInvalidInput = errors.New("invalid input")
	// ErrNotFound indicates the invite token hash was not found.
	ErrNotFound     = errors.New("invite not found")
	// ErrNotActive indicates the invite is expired, revoked, or out of uses.
	ErrNotActive    = errors.New("invite not active")
)
