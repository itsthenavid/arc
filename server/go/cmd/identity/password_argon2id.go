// Package identity password hashing (Argon2id).
//
// This file preserves identity's public API:
//
//   - Argon2idParams
//   - DefaultArgon2idParams
//   - HashPassword
//   - VerifyPassword
//
// while using cmd/security/password as the single source of truth for:
//   - Argon2id parameters (defaults + env overrides)
//   - password policy (defaults + env overrides)
//   - strict PHC decoding + anti-DoS bounds during Verify
//
// English notes:
// - identity MUST NOT silently drift from security/password configuration.
// - identity keeps a historical baseline of min length 8, but will honor stricter env policy.
package identity

import (
	"errors"

	"arc/cmd/security/password"
)

// Argon2idParams defines Argon2id hashing parameters for password hashing.
// These values must be chosen carefully to balance security and performance.
//
// IMPORTANT:
// identity keeps this type for API compatibility. Internally we merge it with
// the security/password config (env + defaults) to avoid split-brain settings.
type Argon2idParams struct {
	MemoryKiB uint32
	Time      uint32
	Threads   uint8
	SaltLen   uint32
	KeyLen    uint32
}

// DefaultArgon2idParams returns the effective defaults based on security/password.
// This is the canonical "default" surface for identity callers.
func DefaultArgon2idParams() Argon2idParams {
	cfg, err := password.FromEnv()
	if err != nil {
		// English comment:
		// FromEnv should never fail under normal circumstances. If it does, fall back to DefaultConfig.
		cfg = password.DefaultConfig()
	}

	return Argon2idParams{
		MemoryKiB: cfg.Params.MemoryKiB,
		Time:      cfg.Params.Iterations,
		Threads:   cfg.Params.Parallelism,
		SaltLen:   cfg.Params.SaltLength,
		KeyLen:    cfg.Params.KeyLength,
	}
}

// HashPassword returns a PHC-style Argon2id hash string.
//
// Security contract:
// - Enforces a baseline min length of 8 for backwards compatibility.
// - Will honor stricter password policy from env (via security/password).
func HashPassword(passwordPlain string, p Argon2idParams) (string, error) {
	if len(passwordPlain) < 8 {
		return "", errors.New("password too short")
	}

	cfg, err := password.FromEnv()
	if err != nil {
		// Treat invalid env as an operational error, not a weak fallback.
		return "", err
	}

	// English comment:
	// identity baseline is min 8 chars, but env may be stricter. We always take the stricter one.
	if cfg.Policy.MinLength < 8 {
		cfg.Policy.MinLength = 8
	}
	// Keep identity historical cap if env is smaller (but allow env to tighten it).
	if cfg.Policy.MaxLength <= 0 {
		cfg.Policy.MaxLength = 256
	}

	// Merge caller-provided params (non-zero fields override env/defaults).
	cfg.Params = mergeIdentityParams(cfg.Params, p)

	enc, err := cfg.Hash(passwordPlain)
	if err != nil {
		// English comment:
		// Use errors.Is (not equality) to remain correct if security/password wraps errors.
		switch {
		case errors.Is(err, password.ErrPasswordTooShort):
			return "", errors.New("password too short")
		case errors.Is(err, password.ErrPasswordTooLong):
			return "", errors.New("password too long")
		case errors.Is(err, password.ErrWeakPassword):
			return "", errors.New("weak password")
		default:
			return "", err
		}
	}

	return enc, nil
}

// VerifyPassword checks a password against a PHC Argon2id hash.
//
// Security contract:
// - Strict PHC parsing.
// - Anti-DoS: verification refuses hashes with parameters wildly above configured maxima.
func VerifyPassword(passwordPlain string, encodedPHC string) (bool, error) {
	cfg, err := password.FromEnv()
	if err != nil {
		return false, err
	}

	// English comment:
	// identity baseline min length 8 (env can be stricter; baseline doesn't weaken it).
	if cfg.Policy.MinLength < 8 {
		cfg.Policy.MinLength = 8
	}
	if cfg.Policy.MaxLength <= 0 {
		cfg.Policy.MaxLength = 256
	}

	ok, err := cfg.Verify(encodedPHC, passwordPlain)
	if err != nil {
		if errors.Is(err, password.ErrInvalidHash) {
			return false, errors.New("invalid argon2id hash format")
		}
		return false, err
	}
	return ok, nil
}

func mergeIdentityParams(base password.Argon2idParams, p Argon2idParams) password.Argon2idParams {
	// English comment:
	// Only apply non-zero overrides to keep env/defaults as the canonical source.
	if p.MemoryKiB != 0 {
		base.MemoryKiB = p.MemoryKiB
	}
	if p.Time != 0 {
		base.Iterations = p.Time
	}
	if p.Threads != 0 {
		base.Parallelism = p.Threads
	}
	if p.SaltLen != 0 {
		base.SaltLength = p.SaltLen
	}
	if p.KeyLen != 0 {
		base.KeyLength = p.KeyLen
	}

	// Defensive minima (argon2 requires these to be sane).
	if base.Parallelism == 0 {
		base.Parallelism = 1
	}
	if base.Iterations == 0 {
		base.Iterations = 1
	}
	if base.MemoryKiB < 8*1024 {
		base.MemoryKiB = 8 * 1024
	}
	if base.SaltLength < 8 {
		base.SaltLength = 16
	}
	if base.KeyLength < 16 {
		base.KeyLength = 32
	}

	return base
}
