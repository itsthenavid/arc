package identity

import (
	"errors"
	"fmt"
)

// OpError is a typed operation error with a stable Op + Kind contract for callers/tests.
// English comment:
// - Kind MUST be one of the sentinel kinds when applicable (ErrInvalidInput, ErrNotFound, ...).
// - Msg may include human-readable context; do not include secrets.
type OpError struct {
	Op   string
	Kind error
	Msg  string
}

func (e OpError) Error() string {
	if e.Msg == "" {
		return fmt.Sprintf("%s: %v", e.Op, e.Kind)
	}
	return fmt.Sprintf("%s: %v: %s", e.Op, e.Kind, e.Msg)
}

func (e OpError) Unwrap() error { return e.Kind }

// ConflictError reports a uniqueness/constraint conflict for a specific logical field.
// Field should be a stable logical name: "username", "email", "refresh_token", ...
type ConflictError struct {
	Op    string
	Field string
}

func (e ConflictError) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("%s: %v", e.Op, ErrConflict)
	}
	return fmt.Sprintf("%s: %v: %s", e.Op, ErrConflict, e.Field)
}

func (e ConflictError) Unwrap() error { return ErrConflict }

// NotFoundError reports a missing referenced resource (e.g., FK violation) or missing row.
type NotFoundError struct {
	Op       string
	Resource string
}

func (e NotFoundError) Error() string {
	if e.Resource == "" {
		return fmt.Sprintf("%s: %v", e.Op, ErrNotFound)
	}
	return fmt.Sprintf("%s: %v: %s", e.Op, ErrNotFound, e.Resource)
}

func (e NotFoundError) Unwrap() error { return ErrNotFound }

// notActiveRotate is intentionally tied to RotateRefreshToken.
// English comment:
// - Always return the same indistinguishable failure reason to avoid token/session probing.
// - Used for: missing, expired, revoked, replaced, or token mismatch.
func notActiveRotate() error {
	return OpError{
		Op:   "identity.RotateRefreshToken",
		Kind: ErrNotActive,
		Msg:  "session not active or token mismatch",
	}
}

// IsConflict reports whether err is a ConflictError.
func IsConflict(err error) bool {
	var ce ConflictError
	return errors.As(err, &ce)
}

// IsNotFound reports whether err represents ErrNotFound (including NotFoundError).
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// IsInvalidInput reports whether err represents ErrInvalidInput.
func IsInvalidInput(err error) bool { return errors.Is(err, ErrInvalidInput) }

// IsNotActive reports whether err represents ErrNotActive.
func IsNotActive(err error) bool { return errors.Is(err, ErrNotActive) }
