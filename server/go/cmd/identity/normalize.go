package identity

import "strings"

// NormalizeUsername performs case-insensitive canonicalization.
// Note: for now we only trim + lower-case. Additional rules (unicode confusables)
// can be added later behind a versioned policy.
func NormalizeUsername(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// NormalizeEmail performs case-insensitive canonicalization.
func NormalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
