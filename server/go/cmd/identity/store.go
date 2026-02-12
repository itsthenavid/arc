package identity

import (
	"context"
	"net"
	"time"
)

// User is Arc's canonical security principal.
type User struct {
	ID           string
	Username     *string
	UsernameNorm *string
	Email        *string
	EmailNorm    *string

	DisplayName *string
	Bio         *string

	CreatedAt time.Time
}

// Session represents a refresh-token based session.
// IMPORTANT: RefreshTokenHash is stored server-side; the plain refresh token is never stored.
type Session struct {
	ID               string
	UserID           string
	RefreshTokenHash string

	CreatedAt  time.Time
	LastUsedAt *time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time

	// Rotation chain (PR-005-grade):
	// When a refresh token is rotated, the old session is revoked and points to the new session id.
	ReplacedBySessionID *string

	// Client/device context.
	Platform  string // web/ios/android/desktop/unknown
	UserAgent *string
	IP        *net.IP
}

// UserAuth is a user with its password hash (for login verification).
type UserAuth struct {
	User         User
	PasswordHash string
}

// Invite represents an invite token row.
type Invite struct {
	ID         string
	CreatedBy  *string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	ConsumedBy *string
}

// CreateUserInput describes a user registration request.
// At least one of Username or Email must be provided.
type CreateUserInput struct {
	Username *string
	Email    *string
	Password string
	Now      time.Time
}

// CreateUserResult returns the created user.
type CreateUserResult struct {
	User User
}

// CreateSessionInput creates a session for an authenticated user.
// TTL must be positive; if not, the store will apply a safe default.
type CreateSessionInput struct {
	UserID    string
	TTL       time.Duration
	Platform  string
	UserAgent *string
	IP        *net.IP
	Now       time.Time
}

// CreateSessionResult returns the created session and the *plain* refresh token.
// The refresh token must be shown to the client exactly once and never logged.
type CreateSessionResult struct {
	Session      Session
	RefreshToken string
}

// CreateInviteInput describes invite creation.
type CreateInviteInput struct {
	CreatedBy *string
	TTL       time.Duration
	Now       time.Time
}

// CreateInviteResult returns the created invite and its plain token.
type CreateInviteResult struct {
	Invite Invite
	Token  string
}

// ConsumeInviteInput describes invite consumption and user creation.
type ConsumeInviteInput struct {
	Token      string
	Username   *string
	Email      *string
	Password   string
	Now        time.Time
	SessionTTL time.Duration
	Platform   string
	UserAgent  *string
	IP         *net.IP
}

// ConsumeInviteResult returns the created user, session, and the consumed invite.
type ConsumeInviteResult struct {
	User         User
	Session      Session
	RefreshToken string
	Invite       Invite
}

// Store is the identity/auth persistence boundary.
type Store interface {
	CreateUser(ctx context.Context, in CreateUserInput) (CreateUserResult, error)
	GetUserByID(ctx context.Context, userID string) (User, error)
	GetUserAuthByUsername(ctx context.Context, username string) (UserAuth, error)
	GetUserAuthByEmail(ctx context.Context, email string) (UserAuth, error)
	CreateSession(ctx context.Context, in CreateSessionInput) (CreateSessionResult, error)
	CreateInvite(ctx context.Context, in CreateInviteInput) (CreateInviteResult, error)
	ConsumeInviteAndCreateUser(ctx context.Context, in ConsumeInviteInput) (ConsumeInviteResult, error)

	// RotateRefreshToken rotates refresh token for an active session.
	//
	// Security contract:
	// - Requires the old plain refresh token to match server-stored hash.
	// - Must be atomic (no window where both tokens are valid).
	// - Rotation is "chain-based":
	//     - creates a new session row with new token hash
	//     - revokes the old session
	//     - links old -> new via replaced_by_session_id
	// - Returns ErrNotActive if session is revoked/expired/missing OR token mismatch.
	RotateRefreshToken(ctx context.Context, sessionID string, oldRefreshToken string, now time.Time) (newPlain string, newHash string, err error)

	RevokeSession(ctx context.Context, sessionID string, now time.Time) error
	RevokeAllSessions(ctx context.Context, userID string, now time.Time) error
}
