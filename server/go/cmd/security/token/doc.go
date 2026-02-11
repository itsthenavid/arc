// Package token provides token hashing primitives for Arc.
//
// It is the single source of truth for refresh-token hashing behavior.
//
// Design goals:
// - Default dev/back-compat mode: SHA-256(token) when no HMAC key is configured.
// - Production-enforced mode: HMAC-SHA256(token, key) when policy requires it.
// - Stable 64-char hex output for storage and constant-time comparison.
//
// Environment:
// - ARC_TOKEN_HMAC_KEY: when set, enables HMAC mode.
// Policy:
//   - If RequireTokenHMAC=true, callers MUST enforce a minimum key size (>= 32 bytes)
//     and MUST use HMAC (no SHA fallback).
package token
