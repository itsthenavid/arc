// Package password provides password hashing and verification utilities for Arc.
//
// It implements Argon2id hashing using a PHC-like encoded string format and includes:
// - Configurable Argon2id parameters (via environment variables)
// - Password policy validation
// - Strict hash decoding and verification with anti-DoS bounds
//
// Security notes:
// - Hash strings are treated as untrusted input during Verify and are validated accordingly.
// - Verification refuses hashes with parameters that exceed reasonable bounds.
package password
