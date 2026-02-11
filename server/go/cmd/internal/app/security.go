package app

import (
	"errors"

	"arc/cmd/security/token"
)

// ValidateSecurityConfig enforces Arc's security policy at startup.
//
// English comment:
// - Fail-fast is intentional: silently falling back to weaker crypto in production is unacceptable.
// - Enforcement is end-to-end by validating the same module that performs hashing (security/token).
func ValidateSecurityConfig(cfg Config) error {
	if !cfg.RequireTokenHMAC {
		return nil
	}

	// English comment:
	// - Minimum 32 bytes recommended for HMAC-SHA256 secret.
	// - We measure bytes (not runes) because the key is used as raw bytes.
	if _, err := token.HMACKeyFromEnv(32); err != nil {
		switch {
		case errors.Is(err, token.ErrHMACKeyMissing):
			return errors.New("security policy: ARC_REQUIRE_TOKEN_HMAC=true but ARC_TOKEN_HMAC_KEY is missing")
		case errors.Is(err, token.ErrHMACKeyTooShort):
			return errors.New("security policy: ARC_REQUIRE_TOKEN_HMAC=true but ARC_TOKEN_HMAC_KEY is too short (min 32 bytes)")
		default:
			return err
		}
	}

	// Extra hard assertion: hashing must be HMAC-enabled in this runtime.
	// (This guards against accidental future changes that reintroduce a SHA fallback under policy.)
	if !token.HMACEnabled() {
		return errors.New("security policy: ARC_REQUIRE_TOKEN_HMAC=true but token hasher is not in HMAC mode")
	}

	return nil
}
