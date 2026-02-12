package session

import (
	"context"
	"net"
	"time"
)

// Platform represents the client platform associated with a session.
type Platform string

const (
	// PlatformWeb is a browser-based session.
	PlatformWeb Platform = "web"
	// PlatformIOS is an iOS native session.
	PlatformIOS Platform = "ios"
	// PlatformAndroid is an Android native session.
	PlatformAndroid Platform = "android"
	// PlatformDesktop is a desktop (macOS/Windows/Linux) session.
	PlatformDesktop Platform = "desktop"
	// PlatformUnknown is used when the client platform is not known.
	PlatformUnknown Platform = "unknown"
)

// DeviceContext describes the client device that owns a session.
type DeviceContext struct {
	Platform   Platform
	RememberMe bool
	UserAgent  string
	IP         net.IP
}

// Row mirrors the arc.sessions row used by the session subsystem.
type Row struct {
	ID                  string
	UserID              string
	RefreshTokenHash    string
	CreatedAt           time.Time
	LastUsedAt          *time.Time
	ExpiresAt           time.Time
	RevokedAt           *time.Time
	ReplacedBySessionID *string
	Platform            Platform
}

// Store abstracts persistence for session state.
//
// Implementations must ensure refresh rotation safety, especially for
// GetByRefreshHashForUpdate semantics.
type Store interface {
	// Create creates a new session row.
	Create(
		ctx context.Context,
		now time.Time,
		userID string,
		dev DeviceContext,
		refreshHash string,
		expiresAt time.Time,
		revocationReason *string,
	) (sessionID string, err error)

	// GetByID loads a session row by ID.
	GetByID(ctx context.Context, sessionID string) (Row, error)

	// GetByRefreshHashForUpdate loads a session row by refresh hash and locks it for update (rotation safety).
	GetByRefreshHashForUpdate(ctx context.Context, refreshHash string) (Row, error)

	// MarkRotated updates the old session: revoked_at, replaced_by_session_id, last_used_at, revocation_reason.
	MarkRotated(ctx context.Context, now time.Time, sessionID string, replacedBy string) error

	// Touch updates last_used_at for a session.
	Touch(ctx context.Context, now time.Time, sessionID string) error

	// Revoke revokes a single session.
	Revoke(ctx context.Context, now time.Time, sessionID string, reason string) error

	// RevokeAll revokes all sessions for a user.
	RevokeAll(ctx context.Context, now time.Time, userID string, reason string) error
}
