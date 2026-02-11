package identity

import "errors"

// Sentinel error kinds (stable for errors.Is and for mapping to API status codes).
var (
	ErrInvalidInput = errors.New("invalid_input")
	ErrNotFound     = errors.New("not_found")
	ErrConflict     = errors.New("conflict")
	ErrNotActive    = errors.New("not_active")
)
