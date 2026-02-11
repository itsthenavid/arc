package identity

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore implements identity persistence over PostgreSQL.
//
// English design notes:
// - The pgx pool is owned by the caller; this store must NOT close it.
// - Schema/table identifiers are safely quoted to avoid SQL injection via identifiers.
// - RotateRefreshToken is fully atomic and serialized via SELECT ... FOR UPDATE on the session row.
// - Errors are mapped to identity sentinel kinds where appropriate.
type PostgresStore struct {
	pool   *pgxpool.Pool
	schema string
}

// PostgresOption configures the store.
type PostgresOption func(*PostgresStore) error

var pgIdentRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// WithSchema sets the Postgres schema used by the identity store (default "arc").
// The schema name is validated to be a legal PostgreSQL identifier.
func WithSchema(schema string) PostgresOption {
	return func(s *PostgresStore) error {
		schema = strings.TrimSpace(schema)
		if schema == "" {
			return fmt.Errorf("identity: empty schema")
		}
		if !pgIdentIsValid(schema) {
			return fmt.Errorf("identity: invalid schema identifier")
		}
		s.schema = schema
		return nil
	}
}

// NewPostgresStore constructs a PostgresStore with secure defaults.
func NewPostgresStore(pool *pgxpool.Pool, opts ...PostgresOption) (*PostgresStore, error) {
	st := &PostgresStore{
		pool:   pool,
		schema: "arc",
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(st); err != nil {
			return nil, err
		}
	}
	if st.pool == nil {
		return nil, fmt.Errorf("identity: nil pool")
	}
	return st, nil
}

const (
	defaultSessionTTL = 30 * 24 * time.Hour
	maxSessionTTL     = 180 * 24 * time.Hour
)

// CreateUser creates a new user and its credentials transactionally.
func (s *PostgresStore) CreateUser(ctx context.Context, in CreateUserInput) (CreateUserResult, error) {
	const op = "identity.CreateUser"

	if s == nil || s.pool == nil {
		return CreateUserResult{}, OpError{Op: op, Kind: ErrInvalidInput, Msg: "nil store"}
	}
	if err := ctx.Err(); err != nil {
		return CreateUserResult{}, err
	}

	username := pgTrimPtr(in.Username)
	email := pgTrimPtr(in.Email)

	if username == nil && email == nil {
		return CreateUserResult{}, pgInvalid(op, "username or email is required")
	}
	if strings.TrimSpace(in.Password) == "" {
		return CreateUserResult{}, pgInvalid(op, "password is required")
	}

	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	// Normalize identity fields.
	var usernameNorm *string
	if username != nil {
		n := NormalizeUsername(*username)
		usernameNorm = &n
	}
	var emailNorm *string
	if email != nil {
		n := NormalizeEmail(*email)
		emailNorm = &n
	}

	// Hash password.
	pwHash, err := HashPassword(in.Password, DefaultArgon2idParams())
	if err != nil {
		return CreateUserResult{}, pgInvalid(op, err.Error())
	}

	userID, err := NewULID(now)
	if err != nil {
		return CreateUserResult{}, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.ReadCommitted,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return CreateUserResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	users := pgIdent(s.schema, "users")
	creds := pgIdent(s.schema, "user_credentials")

	_, err = tx.Exec(ctx,
		`INSERT INTO `+users+` (
		     id, username, username_norm, email, email_norm, created_at
		   ) VALUES ($1, $2, $3, $4, $5, $6)`,
		userID,
		username,
		usernameNorm,
		email,
		emailNorm,
		now,
	)
	if err != nil {
		if field, ok := pgClassifyUniqueViolation(err); ok {
			return CreateUserResult{}, ConflictError{Op: op, Field: field}
		}
		return CreateUserResult{}, err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO `+creds+` (user_id, password_hash, created_at, updated_at)
		 VALUES ($1, $2, $3, $3)`,
		userID, pwHash, now,
	)
	if err != nil {
		// If FK fails here, it indicates programming/schema inconsistency.
		return CreateUserResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return CreateUserResult{}, err
	}

	out := User{
		ID:           userID,
		Username:     username,
		UsernameNorm: usernameNorm,
		Email:        email,
		EmailNorm:    emailNorm,
		CreatedAt:    now,
	}

	return CreateUserResult{User: out}, nil
}

// CreateSession creates a new refresh-token backed session for a user.
func (s *PostgresStore) CreateSession(ctx context.Context, in CreateSessionInput) (CreateSessionResult, error) {
	const op = "identity.CreateSession"

	if s == nil || s.pool == nil {
		return CreateSessionResult{}, OpError{Op: op, Kind: ErrInvalidInput, Msg: "nil store"}
	}
	if err := ctx.Err(); err != nil {
		return CreateSessionResult{}, err
	}
	if strings.TrimSpace(in.UserID) == "" {
		return CreateSessionResult{}, pgInvalid(op, "missing user_id")
	}

	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	ttl := in.TTL
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	if ttl > maxSessionTTL {
		ttl = maxSessionTTL
	}

	platform := strings.ToLower(strings.TrimSpace(in.Platform))
	if platform == "" {
		platform = "unknown"
	}
	switch platform {
	case "web", "ios", "android", "desktop", "unknown":
	default:
		platform = "unknown"
	}

	sessionID, err := NewULID(now)
	if err != nil {
		return CreateSessionResult{}, err
	}

	plain, err := NewOpaqueToken(32)
	if err != nil {
		return CreateSessionResult{}, err
	}
	hash := HashRefreshTokenHex(plain)

	expiresAt := now.Add(ttl)

	var ipVal any
	if in.IP != nil {
		ipVal = in.IP.String()
	}

	sessions := pgIdent(s.schema, "sessions")

	// English comment:
	// Set last_used_at at creation time to reflect immediate usage (login),
	// which simplifies auditing and analytics and matches rotation semantics.
	_, err = s.pool.Exec(ctx,
		`INSERT INTO `+sessions+` (
		     id, user_id, refresh_token_hash, created_at, last_used_at, expires_at, platform, user_agent, ip
		   ) VALUES ($1, $2, $3, $4, $4, $5, $6, $7, $8)`,
		sessionID,
		in.UserID,
		hash,
		now,
		expiresAt,
		platform,
		pgTrimPtr(in.UserAgent),
		ipVal,
	)
	if err != nil {
		if field, ok := pgClassifyUniqueViolation(err); ok {
			return CreateSessionResult{}, ConflictError{Op: op, Field: field}
		}
		if pgIsForeignKeyViolation(err) {
			return CreateSessionResult{}, NotFoundError{Op: op, Resource: "user"}
		}
		return CreateSessionResult{}, err
	}

	var ipOut *net.IP
	if in.IP != nil {
		parsed := net.ParseIP(in.IP.String())
		if parsed != nil {
			ipOut = &parsed
		}
	}

	lu := now

	out := Session{
		ID:               sessionID,
		UserID:           in.UserID,
		RefreshTokenHash: hash,
		CreatedAt:        now,
		LastUsedAt:       &lu,
		ExpiresAt:        expiresAt,
		Platform:         platform,
		UserAgent:        pgTrimPtr(in.UserAgent),
		IP:               ipOut,
	}

	return CreateSessionResult{Session: out, RefreshToken: plain}, nil
}

// RotateRefreshToken rotates the refresh token for an active session.
// It creates a replacement session row, and revokes the old one atomically.
//
// Returns ErrNotActive when:
// - session is missing, expired, revoked, already replaced, OR
// - old token does not match, OR
// - concurrent rotation already won.
func (s *PostgresStore) RotateRefreshToken(ctx context.Context, sessionID string, oldRefreshToken string, now time.Time) (string, string, error) {
	const op = "identity.RotateRefreshToken"

	if s == nil || s.pool == nil {
		return "", "", OpError{Op: op, Kind: ErrInvalidInput, Msg: "nil store"}
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(sessionID) == "" {
		return "", "", pgInvalid(op, "missing session_id")
	}

	oldRefreshToken = strings.TrimSpace(oldRefreshToken)
	if oldRefreshToken == "" {
		return "", "", pgInvalid(op, "missing old_refresh_token")
	}

	if now.IsZero() {
		now = time.Now().UTC()
	}

	oldHash := HashRefreshTokenHex(oldRefreshToken)

	newPlain, err := NewOpaqueToken(32)
	if err != nil {
		return "", "", err
	}
	newHash := HashRefreshTokenHex(newPlain)

	sessions := pgIdent(s.schema, "sessions")

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.ReadCommitted,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return "", "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the session row to serialize rotations (single-writer).
	var (
		userID     string
		dbHash     string
		revokedAt  *time.Time
		expiresAt  time.Time
		replacedBy *string
		platform   string
		userAgent  *string
		ipText     *string
	)

	err = tx.QueryRow(ctx,
		`SELECT user_id, refresh_token_hash, revoked_at, expires_at, replaced_by_session_id, platform, user_agent, ip::text
		   FROM `+sessions+`
		  WHERE id = $1
		  FOR UPDATE`,
		sessionID,
	).Scan(&userID, &dbHash, &revokedAt, &expiresAt, &replacedBy, &platform, &userAgent, &ipText)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", notActiveRotate()
		}
		return "", "", err
	}

	// Active checks.
	if revokedAt != nil {
		return "", "", notActiveRotate()
	}
	if !expiresAt.After(now) {
		return "", "", notActiveRotate()
	}
	if replacedBy != nil && strings.TrimSpace(*replacedBy) != "" {
		return "", "", notActiveRotate()
	}

	// Constant-time compare of stored hash vs computed hash.
	// English comment:
	// - Hashes are expected to be 64-char hex (SHA-256 / HMAC-SHA256).
	// - Enforce fixed-length comparison to avoid length-based side channels.
	if !ctEqHex64(dbHash, oldHash) {
		return "", "", notActiveRotate()
	}

	// Create replacement session row (rotation does not extend lifetime).
	newSessionID, err := NewULID(now)
	if err != nil {
		return "", "", err
	}

	var ipVal any
	if ipText != nil && strings.TrimSpace(*ipText) != "" {
		ipVal = *ipText
	}

	// Insert new session first, then revoke+link old one.
	_, err = tx.Exec(ctx,
		`INSERT INTO `+sessions+` (
		     id, user_id, refresh_token_hash, created_at, last_used_at, expires_at, revoked_at,
		     replaced_by_session_id, platform, user_agent, ip
		   ) VALUES ($1, $2, $3, $4, $4, $5, NULL, NULL, $6, $7, $8)`,
		newSessionID,
		userID,
		newHash,
		now,
		expiresAt,
		platform,
		userAgent,
		ipVal,
	)
	if err != nil {
		if field, ok := pgClassifyUniqueViolation(err); ok {
			return "", "", ConflictError{Op: op, Field: field}
		}
		return "", "", err
	}

	// Revoke old session and link to replacement (single-writer enforcement).
	ct, err := tx.Exec(ctx,
		`UPDATE `+sessions+`
		    SET revoked_at = $1,
		        last_used_at = $1,
		        replaced_by_session_id = $2
		  WHERE id = $3
		    AND revoked_at IS NULL
		    AND expires_at > $1
		    AND replaced_by_session_id IS NULL
		    AND refresh_token_hash = $4`,
		now, newSessionID, sessionID, oldHash,
	)
	if err != nil {
		return "", "", err
	}
	if ct.RowsAffected() != 1 {
		return "", "", notActiveRotate()
	}

	if err := tx.Commit(ctx); err != nil {
		return "", "", err
	}

	return newPlain, newHash, nil
}

// RevokeSession revokes a session by setting revoked_at (idempotent).
// Returns ErrNotFound if the session does not exist.
func (s *PostgresStore) RevokeSession(ctx context.Context, sessionID string, now time.Time) error {
	const op = "identity.RevokeSession"

	if s == nil || s.pool == nil {
		return OpError{Op: op, Kind: ErrInvalidInput, Msg: "nil store"}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(sessionID) == "" {
		return OpError{Op: op, Kind: ErrInvalidInput, Msg: "missing session_id"}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	sessions := pgIdent(s.schema, "sessions")

	ct, err := s.pool.Exec(ctx,
		`UPDATE `+sessions+`
		    SET revoked_at = COALESCE(revoked_at, $1)
		  WHERE id = $2`,
		now, sessionID,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RevokeAllSessions revokes all sessions for a user (idempotent).
func (s *PostgresStore) RevokeAllSessions(ctx context.Context, userID string, now time.Time) error {
	const op = "identity.RevokeAllSessions"

	if s == nil || s.pool == nil {
		return OpError{Op: op, Kind: ErrInvalidInput, Msg: "nil store"}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(userID) == "" {
		return OpError{Op: op, Kind: ErrInvalidInput, Msg: "missing user_id"}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	sessions := pgIdent(s.schema, "sessions")

	_, err := s.pool.Exec(ctx,
		`UPDATE `+sessions+`
		    SET revoked_at = COALESCE(revoked_at, $1),
		        last_used_at = COALESCE(last_used_at, $1)
		  WHERE user_id = $2
		    AND revoked_at IS NULL`,
		now, userID,
	)
	return err
}

// TouchSessionLastUsed updates last_used_at if session is active.
// If session is not active, returns ErrNotActive.
func (s *PostgresStore) TouchSessionLastUsed(ctx context.Context, sessionID string, now time.Time) error {
	const op = "identity.TouchSessionLastUsed"

	if s == nil || s.pool == nil {
		return OpError{Op: op, Kind: ErrInvalidInput, Msg: "nil store"}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(sessionID) == "" {
		return OpError{Op: op, Kind: ErrInvalidInput, Msg: "missing session_id"}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	sessions := pgIdent(s.schema, "sessions")

	ct, err := s.pool.Exec(ctx,
		`UPDATE `+sessions+`
		    SET last_used_at = $1
		  WHERE id = $2
		    AND revoked_at IS NULL
		    AND expires_at > $1
		    AND replaced_by_session_id IS NULL`,
		now, sessionID,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotActive
	}
	return nil
}

// GetSessionByRefreshToken finds an active session by refresh token.
// Returns ErrNotActive when token is unknown or session is not active.
func (s *PostgresStore) GetSessionByRefreshToken(ctx context.Context, refreshToken string, now time.Time) (Session, error) {
	const op = "identity.GetSessionByRefreshToken"

	if s == nil || s.pool == nil {
		return Session{}, OpError{Op: op, Kind: ErrInvalidInput, Msg: "nil store"}
	}
	if err := ctx.Err(); err != nil {
		return Session{}, err
	}
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return Session{}, OpError{Op: op, Kind: ErrInvalidInput, Msg: "missing refresh_token"}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	hash := HashRefreshTokenHex(refreshToken)
	sessions := pgIdent(s.schema, "sessions")

	var (
		out          Session
		userAgent    *string
		ipText       *string
		lastUsedAt   *time.Time
		revokedAt    *time.Time
		replacedByID *string
	)

	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, refresh_token_hash, created_at, last_used_at, expires_at, revoked_at,
		        replaced_by_session_id, platform, user_agent, ip::text
		   FROM `+sessions+`
		  WHERE refresh_token_hash = $1`,
		hash,
	).Scan(
		&out.ID,
		&out.UserID,
		&out.RefreshTokenHash,
		&out.CreatedAt,
		&lastUsedAt,
		&out.ExpiresAt,
		&revokedAt,
		&replacedByID,
		&out.Platform,
		&userAgent,
		&ipText,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, ErrNotActive
		}
		return Session{}, err
	}

	out.UserAgent = userAgent
	out.LastUsedAt = lastUsedAt
	out.RevokedAt = revokedAt
	out.ReplacedBySessionID = replacedByID

	if ipText != nil && strings.TrimSpace(*ipText) != "" {
		parsed := net.ParseIP(*ipText)
		if parsed != nil {
			outIP := parsed
			out.IP = &outIP
		}
	}

	// Active check.
	if out.RevokedAt != nil {
		return Session{}, ErrNotActive
	}
	if !out.ExpiresAt.After(now) {
		return Session{}, ErrNotActive
	}
	if out.ReplacedBySessionID != nil && strings.TrimSpace(*out.ReplacedBySessionID) != "" {
		return Session{}, ErrNotActive
	}

	return out, nil
}

// ---- helpers ----

// ctEqHex64 compares two expected 64-char hex strings in constant time.
// English comment:
// - Rejects if either length != 64 to keep timing stable (and avoid oracle by length).
// - Uses ConstantTimeCompare on fixed-length byte slices.
func ctEqHex64(a, b string) bool {
	if len(a) != 64 || len(b) != 64 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// pgTrimPtr trims a string pointer, returning nil if result is empty.
func pgTrimPtr(p *string) *string {
	if p == nil {
		return nil
	}
	s := strings.TrimSpace(*p)
	if s == "" {
		return nil
	}
	return &s
}

// pgInvalid standardizes invalid input errors.
func pgInvalid(op, msg string) error {
	return OpError{Op: op, Kind: ErrInvalidInput, Msg: msg}
}

// pgIdentIsValid checks if a string is a safe Postgres identifier.
func pgIdentIsValid(s string) bool {
	return pgIdentRe.MatchString(s)
}

// pgIdent safely quotes a schema-qualified identifier: "schema"."name".
func pgIdent(schema, name string) string {
	return pgx.Identifier{schema, name}.Sanitize()
}

func pgIsForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23503" // foreign_key_violation
}

func pgClassifyUniqueViolation(err error) (field string, ok bool) {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return "", false
	}
	if pgErr.Code != "23505" { // unique_violation
		return "", false
	}

	// English comment:
	// Prefer stable schema constraint names. Fall back to heuristic substring matching.
	c := strings.ToLower(strings.TrimSpace(pgErr.ConstraintName))

	switch c {
	case "uq_users_username_norm":
		return "username", true
	case "uq_users_email_norm":
		return "email", true
	case "uq_sessions_refresh_token_hash":
		return "refresh_token", true
	default:
		switch {
		case strings.Contains(c, "username"):
			return "username", true
		case strings.Contains(c, "email"):
			return "email", true
		case strings.Contains(c, "refresh") && strings.Contains(c, "token"):
			return "refresh_token", true
		default:
			return "unique", true
		}
	}
}
