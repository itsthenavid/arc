package invite

import "errors"

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("invite not found")
	ErrNotActive    = errors.New("invite not active")
)
