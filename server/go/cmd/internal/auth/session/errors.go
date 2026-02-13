package session

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrInvalidToken is returned when an access token fails verification or validation.
	ErrInvalidToken = errors.New("invalid token")

	// ErrSessionNotFound is returned when a refresh token does not match any session.
	ErrSessionNotFound = errors.New("session not found")

	// ErrSessionExpired is returned when the session is expired.
	ErrSessionExpired = errors.New("session expired")

	// ErrSessionRevoked is returned when the session has been revoked.
	ErrSessionRevoked = errors.New("session revoked")

	// ErrRefreshReuseDetected is returned when a rotated (replaced) refresh token is presented again.
	// Caller should revoke all sessions for the user.
	ErrRefreshReuseDetected = errors.New("refresh token reuse detected")

	// ErrRefreshRateLimited is returned when refresh is attempted too frequently for a session.
	ErrRefreshRateLimited = errors.New("refresh rate limited")

	// ErrConfig is returned for invalid configuration.
	ErrConfig = errors.New("invalid config")
)

// RefreshRateLimitError carries retry metadata for refresh throttling.
type RefreshRateLimitError struct {
	SessionID  string
	RetryAfter time.Duration
}

func (e RefreshRateLimitError) Error() string {
	if e.RetryAfter <= 0 {
		return ErrRefreshRateLimited.Error()
	}
	return fmt.Sprintf("%s: retry after %s", ErrRefreshRateLimited.Error(), e.RetryAfter)
}

func (e RefreshRateLimitError) Unwrap() error { return ErrRefreshRateLimited }
