package authapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"arc/cmd/identity"
)

func TestEnforceCaptcha_DisabledBypassesVerification(t *testing.T) {
	h := &Handler{
		cfg:     Config{EnableCaptcha: false},
		captcha: &captchaVerifierStub{err: errors.New("should not be called")},
	}

	if err := h.enforceCaptcha(context.Background(), "", nil); err != nil {
		t.Fatalf("expected nil when captcha disabled, got %v", err)
	}
}

func TestEnforceCaptcha_EnabledMissingToken(t *testing.T) {
	h := &Handler{
		cfg:     Config{EnableCaptcha: true},
		captcha: NoopCaptchaVerifier{},
	}

	err := h.enforceCaptcha(context.Background(), "   ", nil)
	if !errors.Is(err, ErrCaptchaRequired) {
		t.Fatalf("expected ErrCaptchaRequired, got %v", err)
	}
}

func TestEnforceCaptcha_EnabledInvalidToken(t *testing.T) {
	stub := &captchaVerifierStub{err: errors.New("provider rejected")}
	h := &Handler{
		cfg:     Config{EnableCaptcha: true},
		captcha: stub,
	}

	err := h.enforceCaptcha(context.Background(), "token-1", net.ParseIP("127.0.0.1"))
	if !errors.Is(err, ErrCaptchaInvalid) {
		t.Fatalf("expected ErrCaptchaInvalid, got %v", err)
	}
	if stub.calls != 1 {
		t.Fatalf("expected verifier to be called once, got %d", stub.calls)
	}
}

func TestEnforceCaptcha_EnabledValidToken(t *testing.T) {
	stub := &captchaVerifierStub{}
	ip := net.ParseIP("127.0.0.1")
	h := &Handler{
		cfg:     Config{EnableCaptcha: true},
		captcha: stub,
	}

	if err := h.enforceCaptcha(context.Background(), " token-ok ", ip); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if stub.calls != 1 {
		t.Fatalf("expected verifier to be called once, got %d", stub.calls)
	}
	if stub.lastToken != "token-ok" {
		t.Fatalf("expected trimmed token, got %q", stub.lastToken)
	}
	if stub.lastIP == nil || !stub.lastIP.Equal(ip) {
		t.Fatalf("expected ip=%v got=%v", ip, stub.lastIP)
	}
}

func TestEnforceEmailVerified(t *testing.T) {
	now := time.Now().UTC()
	email := "user@example.com"

	tests := []struct {
		name string
		cfg  Config
		user identity.User
		want error
	}{
		{
			name: "flag disabled",
			cfg:  Config{RequireEmailVerified: false},
			user: identity.User{},
			want: nil,
		},
		{
			name: "missing email",
			cfg:  Config{RequireEmailVerified: true},
			user: identity.User{},
			want: ErrEmailNotVerified,
		},
		{
			name: "unverified email",
			cfg:  Config{RequireEmailVerified: true},
			user: identity.User{Email: &email},
			want: ErrEmailNotVerified,
		},
		{
			name: "verified email",
			cfg:  Config{RequireEmailVerified: true},
			user: identity.User{Email: &email, EmailVerifiedAt: &now},
			want: nil,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{cfg: tc.cfg}
			err := h.enforceEmailVerified(tc.user)
			if tc.want == nil && err != nil {
				t.Fatalf("expected nil, got %v", err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func TestMaybeSendVerificationEmail(t *testing.T) {
	email := "person@example.com"
	now := time.Now().UTC()

	tests := []struct {
		name      string
		user      identity.User
		wantCalls int
	}{
		{
			name:      "missing email",
			user:      identity.User{ID: "u1"},
			wantCalls: 0,
		},
		{
			name:      "already verified",
			user:      identity.User{ID: "u2", Email: &email, EmailVerifiedAt: &now},
			wantCalls: 0,
		},
		{
			name:      "pending verification",
			user:      identity.User{ID: "u3", Email: &email},
			wantCalls: 1,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			stub := &emailSenderStub{}
			h := &Handler{
				log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
				emailSender: stub,
			}
			h.maybeSendVerificationEmail(context.Background(), tc.user)
			if stub.calls != tc.wantCalls {
				t.Fatalf("expected calls=%d, got %d", tc.wantCalls, stub.calls)
			}
		})
	}
}

type captchaVerifierStub struct {
	calls     int
	lastToken string
	lastIP    net.IP
	err       error
}

func (s *captchaVerifierStub) Verify(_ context.Context, token string, ip net.IP) error {
	s.calls++
	s.lastToken = token
	s.lastIP = ip
	return s.err
}

type emailSenderStub struct {
	calls int
}

func (s *emailSenderStub) SendEmailVerification(_ context.Context, _ EmailVerificationMessage) error {
	s.calls++
	return nil
}
