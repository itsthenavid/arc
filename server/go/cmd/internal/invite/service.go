package invite

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"arc/cmd/security/token"

	"github.com/oklog/ulid/v2"
)

const defaultTokenBytes = 32

// Invite represents an invite row.
type Invite struct {
	ID         string
	CreatedBy  *string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	MaxUses    int
	UsedCount  int
	RevokedAt  *time.Time
	Note       *string
	ConsumedAt *time.Time
	ConsumedBy *string
}

// CreateInput describes invite creation.
type CreateInput struct {
	CreatedBy *string
	TTL       time.Duration
	MaxUses   int
	Note      *string
	Now       time.Time
}

// ConsumeInput describes invite consumption.
type ConsumeInput struct {
	Token      string
	ConsumedBy *string
	Now        time.Time
}

// Service manages invite creation, validation, and consumption.
type Service struct {
	store      Store
	tokenBytes int
}

// Option configures the Service.
type Option func(*Service) error

// WithTokenBytes sets the length of generated invite tokens in bytes.
func WithTokenBytes(n int) Option {
	return func(s *Service) error {
		if n <= 0 {
			return ErrInvalidInput
		}
		s.tokenBytes = n
		return nil
	}
}

// NewService constructs a Service with safe defaults.
func NewService(store Store, opts ...Option) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	s := &Service{store: store, tokenBytes: defaultTokenBytes}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// CreateInvite creates a new invite and returns the invite plus its plain token.
func (s *Service) CreateInvite(ctx context.Context, in CreateInput) (Invite, string, error) {
	if s == nil || s.store == nil {
		return Invite{}, "", ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return Invite{}, "", err
	}

	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ttl := in.TTL
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	maxUses := in.MaxUses
	if maxUses <= 0 {
		maxUses = 1
	}
	note := trimPtr(in.Note)
	if note != nil && len(*note) > 512 {
		return Invite{}, "", ErrInvalidInput
	}

	tokenPlain, err := newOpaqueToken(s.tokenBytes)
	if err != nil {
		return Invite{}, "", err
	}
	tokenHash := token.HashRefreshTokenHex(tokenPlain)

	inviteID, err := newULID(now)
	if err != nil {
		return Invite{}, "", err
	}

	expiresAt := now.Add(ttl)
	createdBy := trimPtr(in.CreatedBy)
	inv, err := s.store.Create(ctx, CreateRecord{
		ID:         inviteID,
		TokenHash:  tokenHash,
		CreatedBy:  createdBy,
		CreatedAt:  now,
		ExpiresAt:  expiresAt,
		MaxUses:    maxUses,
		UsedCount:  0,
		RevokedAt:  nil,
		Note:       note,
		ConsumedAt: nil,
		ConsumedBy: nil,
	})
	if err != nil {
		return Invite{}, "", err
	}
	return inv, tokenPlain, nil
}

// ValidateInvite checks whether a token is valid and active at the given time.
func (s *Service) ValidateInvite(ctx context.Context, tokenStr string, now time.Time) (bool, Invite, error) {
	if s == nil || s.store == nil {
		return false, Invite{}, ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return false, Invite{}, err
	}
	tokenStr = strings.TrimSpace(tokenStr)
	if tokenStr == "" {
		return false, Invite{}, ErrInvalidInput
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	tokenHash := token.HashRefreshTokenHex(tokenStr)
	inv, err := s.store.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, Invite{}, nil
		}
		return false, Invite{}, err
	}

	if inv.RevokedAt != nil {
		return false, inv, nil
	}
	if !inv.ExpiresAt.After(now) {
		return false, inv, nil
	}
	if inv.MaxUses > 0 && inv.UsedCount >= inv.MaxUses {
		return false, inv, nil
	}

	return true, inv, nil
}

// ConsumeInvite marks an invite as used.
func (s *Service) ConsumeInvite(ctx context.Context, in ConsumeInput) (Invite, error) {
	if s == nil || s.store == nil {
		return Invite{}, ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return Invite{}, err
	}
	tokenStr := strings.TrimSpace(in.Token)
	if tokenStr == "" {
		return Invite{}, ErrInvalidInput
	}
	consumedBy := trimPtr(in.ConsumedBy)
	if consumedBy == nil {
		return Invite{}, ErrInvalidInput
	}
	if in.Now.IsZero() {
		in.Now = time.Now().UTC()
	}

	tokenHash := token.HashRefreshTokenHex(tokenStr)
	return s.store.Consume(ctx, ConsumeRecord{
		TokenHash:  tokenHash,
		ConsumedBy: consumedBy,
		Now:        in.Now,
	})
}

func newOpaqueToken(nBytes int) (string, error) {
	if nBytes <= 0 {
		nBytes = defaultTokenBytes
	}
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func newULID(now time.Time) (string, error) {
	entropy := ulid.Monotonic(rand.Reader, 0)
	id, err := ulid.New(ulid.Timestamp(now), entropy)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func trimPtr(v *string) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil
	}
	return &s
}
