package session

import (
	"time"

	paseto "aidanwoods.dev/go-paseto"
)

// AccessClaims is the minimal identity envelope propagated across HTTP/WS.
type AccessClaims struct {
	UserID    string
	SessionID string
	ExpiresAt time.Time
	IssuedAt  time.Time
	Issuer    string
}

// AccessTokenManager issues and verifies short-lived access tokens.
type AccessTokenManager interface {
	Issue(userID, sessionID string, now time.Time) (token string, exp time.Time, err error)
	Verify(token string, now time.Time) (AccessClaims, error)
	PublicKeyHex() string
}

type pasetoV4PublicManager struct {
	issuer    string
	ttl       time.Duration
	clockSkew time.Duration

	secret paseto.V4AsymmetricSecretKey
	public paseto.V4AsymmetricPublicKey
}

// NewPasetoV4PublicManager builds an AccessTokenManager based on PASETO v4.public.
//
// It uses an Ed25519 asymmetric keypair and enforces issuer and expiration rules.
// Clock skew is applied during verification via ValidAt to tolerate minor clock differences.
func NewPasetoV4PublicManager(cfg Config) (AccessTokenManager, error) {
	secret, err := paseto.NewV4AsymmetricSecretKeyFromHex(cfg.PasetoV4SecretKeyHex)
	if err != nil {
		return nil, ErrConfig
	}

	public := secret.Public()

	return &pasetoV4PublicManager{
		issuer:    cfg.Issuer,
		ttl:       cfg.AccessTokenTTL,
		clockSkew: cfg.ClockSkew,
		secret:    secret,
		public:    public,
	}, nil
}

func (m *pasetoV4PublicManager) PublicKeyHex() string {
	return m.public.ExportHex()
}

func (m *pasetoV4PublicManager) Issue(userID, sessionID string, now time.Time) (string, time.Time, error) {
	exp := now.Add(m.ttl)

	tok := paseto.NewToken()
	tok.SetIssuer(m.issuer)
	tok.SetIssuedAt(now)
	tok.SetNotBefore(now) // Access tokens valid immediately.
	tok.SetExpiration(exp)

	// Minimal, explicit claims.
	_ = tok.Set("uid", userID)
	_ = tok.Set("sid", sessionID)

	signed := tok.V4Sign(m.secret, nil)
	return signed, exp, nil
}

func (m *pasetoV4PublicManager) Verify(token string, now time.Time) (AccessClaims, error) {
	// Clock-skew tolerance:
	// Validate slightly in the future to avoid failing "nbf" when clocks differ.
	// This also makes expiration checks slightly stricter, which is typically desirable.
	validNow := now.Add(m.clockSkew)

	// Build a fresh parser per call to avoid accumulating rules across verifies.
	p := paseto.NewParser()
	p.AddRule(paseto.IssuedBy(m.issuer))
	p.AddRule(paseto.NotExpired())
	p.AddRule(paseto.ValidAt(validNow))

	parsed, err := p.ParseV4Public(m.public, token, nil)
	if err != nil {
		return AccessClaims{}, ErrInvalidToken
	}

	iss, _ := parsed.GetIssuer()
	exp, _ := parsed.GetExpiration()
	iat, _ := parsed.GetIssuedAt()

	uid, err := parsed.GetString("uid")
	if err != nil || uid == "" {
		return AccessClaims{}, ErrInvalidToken
	}
	sid, err := parsed.GetString("sid")
	if err != nil || sid == "" {
		return AccessClaims{}, ErrInvalidToken
	}

	return AccessClaims{
		UserID:    uid,
		SessionID: sid,
		ExpiresAt: exp,
		IssuedAt:  iat,
		Issuer:    iss,
	}, nil
}
