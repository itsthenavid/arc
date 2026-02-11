package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2idParams defines Argon2id hashing parameters for password hashing.
// These values must be chosen carefully to balance security and performance.
type Argon2idParams struct {
	MemoryKiB uint32
	Time      uint32
	Threads   uint8
	SaltLen   uint32
	KeyLen    uint32
}

// DefaultArgon2idParams are safe modern defaults for interactive logins.
// MemoryKiB=65536 => 64 MiB
func DefaultArgon2idParams() Argon2idParams {
	// English comment:
	// Use a CPU-aware thread count to avoid instability on low-core machines/containers.
	// Keep a conservative cap of 4 while enforcing minimum 1.
	threads := runtime.NumCPU()
	if threads <= 0 {
		threads = 1
	}
	if threads > 4 {
		threads = 4
	}

	return Argon2idParams{
		MemoryKiB: 65536,
		Time:      1,
		Threads:   uint8(threads), // #nosec G115 -- threads is clamped to [1..4] above; safe conversion.
		SaltLen:   16,
		KeyLen:    32,
	}
}

// HashPassword returns a PHC-style Argon2id hash string.
func HashPassword(password string, p Argon2idParams) (string, error) {
	if len(password) < 8 {
		return "", errors.New("password too short")
	}

	// English comment:
	// Defensive normalization of threads to ensure argon2.IDKey does not receive 0.
	if p.Threads == 0 {
		p.Threads = 1
	}

	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	key := argon2.IDKey([]byte(password), salt, p.Time, p.MemoryKiB, p.Threads, p.KeyLen)

	b64 := base64.RawStdEncoding
	saltB64 := b64.EncodeToString(salt)
	keyB64 := b64.EncodeToString(key)

	// PHC format:
	// $argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		p.MemoryKiB, p.Time, p.Threads, saltB64, keyB64,
	), nil
}

// VerifyPassword checks a password against a PHC Argon2id hash.
func VerifyPassword(password string, encoded string) (bool, error) {
	p, salt, hash, err := parsePHCArgon2id(encoded)
	if err != nil {
		return false, err
	}

	hashLen := len(hash)
	if hashLen > 4294967295 { // max uint32
		return false, errors.New("hash length exceeds maximum")
	}

	// English comment:
	// Argon2 parameters are embedded in the PHC string; enforce threads >= 1 defensively.
	if p.Threads == 0 {
		p.Threads = 1
	}

	key := argon2.IDKey([]byte(password), salt, p.Time, p.MemoryKiB, p.Threads, uint32(hashLen))
	if subtle.ConstantTimeCompare(key, hash) == 1 {
		return true, nil
	}
	return false, nil
}

func parsePHCArgon2id(s string) (Argon2idParams, []byte, []byte, error) {
	// expected: $argon2id$v=19$m=...,t=...,p=...$salt$hash
	parts := strings.Split(s, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return Argon2idParams{}, nil, nil, errors.New("invalid argon2id hash format")
	}
	if parts[2] != "v=19" {
		return Argon2idParams{}, nil, nil, errors.New("unsupported argon2 version")
	}

	paramPart := parts[3]
	params := DefaultArgon2idParams()

	kv := strings.Split(paramPart, ",")
	for _, item := range kv {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		pair := strings.SplitN(item, "=", 2)
		if len(pair) != 2 {
			return Argon2idParams{}, nil, nil, errors.New("invalid argon2 params")
		}
		k := pair[0]
		v := pair[1]
		switch k {
		case "m":
			n, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				return Argon2idParams{}, nil, nil, err
			}
			params.MemoryKiB = uint32(n)
		case "t":
			n, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				return Argon2idParams{}, nil, nil, err
			}
			params.Time = uint32(n)
		case "p":
			n, err := strconv.ParseUint(v, 10, 8)
			if err != nil {
				return Argon2idParams{}, nil, nil, err
			}
			if n == 0 {
				return Argon2idParams{}, nil, nil, errors.New("invalid argon2 threads (p=0)")
			}
			params.Threads = uint8(n)
		default:
			return Argon2idParams{}, nil, nil, errors.New("unknown argon2 param")
		}
	}

	b64 := base64.RawStdEncoding

	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return Argon2idParams{}, nil, nil, err
	}
	hash, err := b64.DecodeString(parts[5])
	if err != nil {
		return Argon2idParams{}, nil, nil, err
	}

	if len(salt) < 8 || len(hash) < 16 {
		return Argon2idParams{}, nil, nil, errors.New("invalid argon2 payload")
	}

	return params, salt, hash, nil
}
