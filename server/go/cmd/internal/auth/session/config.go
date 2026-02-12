package session

import (
	"os"
	"strconv"
	"time"
)

// Config defines all runtime configuration for the session subsystem.
//
// It controls access-token TTL, refresh-token policies, clock skew tolerance,
// refresh entropy size, and PASETO v4 signing keys.
//
// This struct is intentionally explicit and environment-driven so that
// production deployments can tune security parameters without code changes.
type Config struct {
	// Issuer is the value set in the "iss" claim of access tokens.
	Issuer string

	// AccessTokenTTL defines the lifetime of PASETO access tokens.
	AccessTokenTTL time.Duration

	// Refresh token TTL policies per platform.
	RefreshTTLWeb         time.Duration
	RefreshTTLNative      time.Duration
	RefreshTTLNativeShort time.Duration

	// ClockSkew defines the allowed time skew during token validation.
	ClockSkew time.Duration

	// RefreshTokenBytes defines the number of random bytes used
	// to generate opaque refresh tokens.
	RefreshTokenBytes int

	// PasetoV4SecretKeyHex is the hex-encoded Ed25519 secret key
	// used to sign PASETO v4.public access tokens.
	PasetoV4SecretKeyHex string
}

// DefaultConfig returns a secure default configuration suitable for development.
//
// Production environments should override values via environment variables.
func DefaultConfig() Config {
	return Config{
		Issuer:                "arc",
		AccessTokenTTL:        15 * time.Minute,
		RefreshTTLWeb:         7 * 24 * time.Hour,
		RefreshTTLNative:      60 * 24 * time.Hour,
		RefreshTTLNativeShort: 14 * 24 * time.Hour,
		ClockSkew:             30 * time.Second,
		RefreshTokenBytes:     32,
	}
}

// LoadConfigFromEnv loads session configuration from environment variables.
//
// Required:
//   - ARC_PASETO_V4_SECRET_KEY_HEX
//
// Optional (durations must be valid Go duration strings):
//   - ARC_AUTH_ISSUER
//   - ARC_AUTH_ACCESS_TTL
//   - ARC_AUTH_REFRESH_TTL_WEB
//   - ARC_AUTH_REFRESH_TTL_NATIVE
//   - ARC_AUTH_REFRESH_TTL_NATIVE_SHORT
//   - ARC_AUTH_CLOCK_SKEW
//   - ARC_AUTH_REFRESH_TOKEN_BYTES
//
// Returns ErrConfig if configuration is invalid.
func LoadConfigFromEnv() (Config, error) {
	cfg := DefaultConfig()

	if v := os.Getenv("ARC_AUTH_ISSUER"); v != "" {
		cfg.Issuer = v
	}

	if v := os.Getenv("ARC_AUTH_ACCESS_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return Config{}, ErrConfig
		}
		cfg.AccessTokenTTL = d
	}

	if v := os.Getenv("ARC_AUTH_REFRESH_TTL_WEB"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return Config{}, ErrConfig
		}
		cfg.RefreshTTLWeb = d
	}

	if v := os.Getenv("ARC_AUTH_REFRESH_TTL_NATIVE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return Config{}, ErrConfig
		}
		cfg.RefreshTTLNative = d
	}

	if v := os.Getenv("ARC_AUTH_REFRESH_TTL_NATIVE_SHORT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return Config{}, ErrConfig
		}
		cfg.RefreshTTLNativeShort = d
	}

	if v := os.Getenv("ARC_AUTH_CLOCK_SKEW"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			return Config{}, ErrConfig
		}
		cfg.ClockSkew = d
	}

	if v := os.Getenv("ARC_AUTH_REFRESH_TOKEN_BYTES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 32 || n > 64 {
			return Config{}, ErrConfig
		}
		cfg.RefreshTokenBytes = n
	}

	cfg.PasetoV4SecretKeyHex = os.Getenv("ARC_PASETO_V4_SECRET_KEY_HEX")
	if cfg.PasetoV4SecretKeyHex == "" {
		return Config{}, ErrConfig
	}

	// Invariants: native "short" must not exceed native "long".
	if cfg.RefreshTTLNative < cfg.RefreshTTLNativeShort {
		return Config{}, ErrConfig
	}

	return cfg, nil
}
