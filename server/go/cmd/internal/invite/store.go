package invite

import (
	"context"
	"time"
)

// CreateRecord is a normalized invite insert payload.
type CreateRecord struct {
	ID         string
	TokenHash  string
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

// ConsumeRecord describes a token consumption.
type ConsumeRecord struct {
	TokenHash  string
	ConsumedBy *string
	Now        time.Time
}

// Store is the persistence boundary for invites.
type Store interface {
	Create(ctx context.Context, in CreateRecord) (Invite, error)
	GetByTokenHash(ctx context.Context, tokenHash string) (Invite, error)
	Consume(ctx context.Context, in ConsumeRecord) (Invite, error)
}
