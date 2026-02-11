package password

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Argon2idParams controls Argon2id hashing cost.
// MemoryKiB is in KiB as required by argon2.IDKey.
type Argon2idParams struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// Policy controls password validation and anti-DoS boundaries.
type Policy struct {
	MinLength int
	MaxLength int
	// If true, enable an extra, minimal weak-pattern rejection.
	RejectVeryWeak bool
}

// Config is the single configuration surface for this package.
type Config struct {
	Params Argon2idParams
	Policy Policy
}

// DefaultConfig returns a strong baseline suitable for a messaging system.
// Values are intentionally conservative and can be overridden via env.
func DefaultConfig() Config {
	// English comment:
	// CPU-aware parallelism avoids extreme settings on multi-core hosts while keeping a safe baseline.
	// We clamp to [1..4] to keep resource usage predictable in containers.
	threads := runtime.NumCPU()
	if threads <= 0 {
		threads = 1
	}
	if threads > 4 {
		threads = 4
	}

	return Config{
		Params: Argon2idParams{
			MemoryKiB:   64 * 1024,      // 64 MiB
			Iterations:  3,              // reasonable default for interactive logins
			Parallelism: uint8(threads), // #nosec G115 -- clamped to [1..4] above; safe conversion.
			SaltLength:  16,
			KeyLength:   32,
		},
		Policy: Policy{
			MinLength:      12,
			MaxLength:      256,
			RejectVeryWeak: false,
		},
	}
}

// FromEnv loads config from environment variables.
//
// Env surface:
// - ARC_PASSWORD_MIN_LEN
// - ARC_PASSWORD_MAX_LEN
// - ARC_PASSWORD_REJECT_VERY_WEAK (true/false)
// - ARC_ARGON2_MEMORY_KIB
// - ARC_ARGON2_ITERATIONS
// - ARC_ARGON2_PARALLELISM
// - ARC_ARGON2_SALT_LEN
// - ARC_ARGON2_KEY_LEN
func FromEnv() (Config, error) {
	cfg := DefaultConfig()

	if v, ok := os.LookupEnv("ARC_PASSWORD_MIN_LEN"); ok {
		n, err := atoiPositiveInt(v, 1, 1024)
		if err != nil {
			return Config{}, fmt.Errorf("ARC_PASSWORD_MIN_LEN: %w", err)
		}
		cfg.Policy.MinLength = n
	}

	if v, ok := os.LookupEnv("ARC_PASSWORD_MAX_LEN"); ok {
		n, err := atoiPositiveInt(v, 1, 4096)
		if err != nil {
			return Config{}, fmt.Errorf("ARC_PASSWORD_MAX_LEN: %w", err)
		}
		cfg.Policy.MaxLength = n
	}

	if v, ok := os.LookupEnv("ARC_PASSWORD_REJECT_VERY_WEAK"); ok {
		b, err := parseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("ARC_PASSWORD_REJECT_VERY_WEAK: %w", err)
		}
		cfg.Policy.RejectVeryWeak = b
	}

	if v, ok := os.LookupEnv("ARC_ARGON2_MEMORY_KIB"); ok {
		u, err := atou32(v, 8*1024, 1024*1024) // 8 MiB .. 1 GiB
		if err != nil {
			return Config{}, fmt.Errorf("ARC_ARGON2_MEMORY_KIB: %w", err)
		}
		cfg.Params.MemoryKiB = u
	}

	if v, ok := os.LookupEnv("ARC_ARGON2_ITERATIONS"); ok {
		u, err := atou32(v, 1, 20)
		if err != nil {
			return Config{}, fmt.Errorf("ARC_ARGON2_ITERATIONS: %w", err)
		}
		cfg.Params.Iterations = u
	}

	if v, ok := os.LookupEnv("ARC_ARGON2_PARALLELISM"); ok {
		u, err := atou32(v, 1, 64)
		if err != nil {
			return Config{}, fmt.Errorf("ARC_ARGON2_PARALLELISM: %w", err)
		}
		p, err := u32ToU8(u)
		if err != nil {
			return Config{}, fmt.Errorf("ARC_ARGON2_PARALLELISM: %w", err)
		}
		cfg.Params.Parallelism = p
	}

	if v, ok := os.LookupEnv("ARC_ARGON2_SALT_LEN"); ok {
		u, err := atou32(v, 8, 64)
		if err != nil {
			return Config{}, fmt.Errorf("ARC_ARGON2_SALT_LEN: %w", err)
		}
		cfg.Params.SaltLength = u
	}

	if v, ok := os.LookupEnv("ARC_ARGON2_KEY_LEN"); ok {
		u, err := atou32(v, 16, 64)
		if err != nil {
			return Config{}, fmt.Errorf("ARC_ARGON2_KEY_LEN: %w", err)
		}
		cfg.Params.KeyLength = u
	}

	// Final sanity.
	if cfg.Policy.MinLength > cfg.Policy.MaxLength {
		return Config{}, fmt.Errorf(
			"password policy invalid: min_len(%d) > max_len(%d)",
			cfg.Policy.MinLength,
			cfg.Policy.MaxLength,
		)
	}

	return cfg, nil
}

func atoiPositiveInt(s string, minVal, maxVal int) (int, error) {
	s = strings.TrimSpace(s)
	i64, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("not an integer")
	}

	i := int(i64)
	if i < minVal || i > maxVal {
		return 0, fmt.Errorf("out of range [%d..%d]", minVal, maxVal)
	}
	return i, nil
}

func atou32(s string, minVal, maxVal uint32) (uint32, error) {
	s = strings.TrimSpace(s)
	u64, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("not an unsigned integer")
	}

	u := uint32(u64)
	if u < minVal || u > maxVal {
		return 0, fmt.Errorf("out of range [%d..%d]", minVal, maxVal)
	}
	return u, nil
}

func u32ToU8(u uint32) (uint8, error) {
	// Explicit overflow guard to satisfy static analyzers and future changes.
	if u > math.MaxUint8 {
		return 0, fmt.Errorf("out of range [0..%d]", math.MaxUint8)
	}
	return uint8(u), nil
}

func parseBool(s string) (bool, error) {
	s = strings.TrimSpace(s)
	switch s {
	case "1", "true", "TRUE", "True", "yes", "YES", "Yes", "on", "ON", "On":
		return true, nil
	case "0", "false", "FALSE", "False", "no", "NO", "No", "off", "OFF", "Off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean")
	}
}
