// Package session implements Arc's session architecture (PR-005).
//
// It provides a multi-device session model with refresh-token rotation,
// reuse detection, and per-session/per-user revocation.
//
// Access tokens are issued as PASETO v4.public and are short-lived.
// Refresh tokens are opaque random strings and are stored hashed in Postgres
// (HMAC-SHA256 when ARC_TOKEN_HMAC_KEY is set; otherwise SHA-256 for dev/back-compat).
//
// Transport (HTTP/WS) integration is intentionally out of scope here.
package session
