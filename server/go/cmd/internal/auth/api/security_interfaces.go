package authapi

import (
	"context"
	"errors"
	"net"
	"strings"
)

var (
	// ErrCaptchaRequired indicates captcha is enabled but token is missing.
	ErrCaptchaRequired = errors.New("captcha token required")
	// ErrCaptchaInvalid indicates captcha verification failed.
	ErrCaptchaInvalid = errors.New("captcha invalid")
	// ErrEmailNotVerified indicates login was blocked by verification policy.
	ErrEmailNotVerified = errors.New("email not verified")
)

// EmailVerificationMessage is the canonical payload for email verification delivery.
type EmailVerificationMessage struct {
	UserID string
	Email  string
}

// EmailSender sends verification emails.
//
// NOTE:
// PR-011 ships with no-op defaults only. Real delivery providers are wired later.
type EmailSender interface {
	SendEmailVerification(ctx context.Context, msg EmailVerificationMessage) error
}

// NoopEmailSender is the default email sender used in this phase.
type NoopEmailSender struct{}

// SendEmailVerification is a no-op implementation for PR-011 readiness.
func (NoopEmailSender) SendEmailVerification(_ context.Context, _ EmailVerificationMessage) error {
	return nil
}

// CaptchaVerifier verifies user-provided captcha tokens.
//
// NOTE:
// PR-011 ships with no-op defaults only. Real provider integrations are added later.
type CaptchaVerifier interface {
	Verify(ctx context.Context, token string, ip net.IP) error
}

// NoopCaptchaVerifier is the default captcha verifier used in this phase.
type NoopCaptchaVerifier struct{}

// Verify is a no-op implementation for PR-011 readiness.
func (NoopCaptchaVerifier) Verify(_ context.Context, _ string, _ net.IP) error { return nil }

func normalizeCaptchaToken(raw string) string { return strings.TrimSpace(raw) }
