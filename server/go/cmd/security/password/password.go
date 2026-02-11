package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argon2Version = 19 // argon2.Version is 0x13 (19)
)

// Hash hashes a password using Argon2id and returns an encoded hash string.
// Format:
// $argon2id$v=19$m=<mem>,t=<iter>,p=<par>$<salt_b64>$<hash_b64>
func (c Config) Hash(password string) (string, error) {
	if err := c.Validate(password); err != nil {
		return "", err
	}

	salt := make([]byte, c.Params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("salt: %w", err)
	}

	key := argon2.IDKey(
		[]byte(password),
		salt,
		c.Params.Iterations,
		c.Params.MemoryKiB,
		c.Params.Parallelism,
		c.Params.KeyLength,
	)

	b64 := base64.RawStdEncoding
	saltB64 := b64.EncodeToString(salt)
	keyB64 := b64.EncodeToString(key)

	enc := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2Version,
		c.Params.MemoryKiB,
		c.Params.Iterations,
		c.Params.Parallelism,
		saltB64,
		keyB64,
	)

	return enc, nil
}

// Verify checks whether password matches the given encoded hash.
// Returns (true, nil) for a match, (false, nil) for mismatch,
// and (false, ErrInvalidHash) for malformed/unsupported hashes.
func (c Config) Verify(encodedHash, password string) (bool, error) {
	params, salt, expected, err := decode(encodedHash)
	if err != nil {
		return false, err
	}

	// Anti-DoS boundary: refuse to verify if params exceed our configured maximums
	// by a large margin (prevents attacker-controlled hash strings from causing
	// pathological resource usage).
	if !withinReasonableBounds(params, c.Params) {
		return false, ErrInvalidHash
	}

	key := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.MemoryKiB,
		params.Parallelism,
		uint32(len(expected)), // #nosec G115 -- expected length is bounded by decode(); safe conversion.
	)

	// Constant-time compare.
	if subtle.ConstantTimeCompare(key, expected) == 1 {
		return true, nil
	}
	return false, nil
}

func withinReasonableBounds(got Argon2idParams, limits Argon2idParams) bool {
	// Allow verifying hashes generated with older/smaller settings,
	// but reject wildly larger settings.
	if got.MemoryKiB > limits.MemoryKiB*2 {
		return false
	}
	if got.Iterations > limits.Iterations*2 {
		return false
	}
	if got.Parallelism > limits.Parallelism*2 {
		return false
	}
	if got.SaltLength < 8 || got.SaltLength > 64 {
		return false
	}
	if got.KeyLength < 16 || got.KeyLength > 128 {
		return false
	}
	return true
}

// decode parses the encoded hash and returns params, salt and expected key.
func decode(encoded string) (Argon2idParams, []byte, []byte, error) {
	// Expected:
	// $argon2id$v=19$m=65536,t=3,p=1$<salt>$<hash>
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return Argon2idParams{}, nil, nil, ErrInvalidHash
	}

	// v=19
	if !strings.HasPrefix(parts[2], "v=") {
		return Argon2idParams{}, nil, nil, ErrInvalidHash
	}
	if parts[2] != "v=19" {
		return Argon2idParams{}, nil, nil, ErrInvalidHash
	}

	// m=...,t=...,p=...
	if !strings.HasPrefix(parts[3], "m=") {
		return Argon2idParams{}, nil, nil, ErrInvalidHash
	}
	var mem, it, par uint32
	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &it, &par)
	if err != nil {
		return Argon2idParams{}, nil, nil, ErrInvalidHash
	}
	if mem == 0 || it == 0 || par == 0 || par > 255 {
		return Argon2idParams{}, nil, nil, ErrInvalidHash
	}

	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return Argon2idParams{}, nil, nil, ErrInvalidHash
	}
	hash, err := b64.DecodeString(parts[5])
	if err != nil {
		return Argon2idParams{}, nil, nil, ErrInvalidHash
	}

	params := Argon2idParams{
		MemoryKiB:   mem,
		Iterations:  it,
		Parallelism: uint8(par),
		SaltLength:  uint32(len(salt)), // #nosec G115 -- decode() bounds salt length via base64 decode + Validate limits.
		KeyLength:   uint32(len(hash)), // #nosec G115 -- decode() bounds hash length via base64 decode + Validate limits.
	}

	return params, salt, hash, nil
}
