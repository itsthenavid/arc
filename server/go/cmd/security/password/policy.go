package password

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Validate checks password policy. It does not mutate input.
func (c Config) Validate(password string) error {
	// Count characters (runes), not bytes, to be user-friendly.
	n := utf8.RuneCountInString(password)

	if n < c.Policy.MinLength {
		return ErrPasswordTooShort
	}
	if n > c.Policy.MaxLength {
		return ErrPasswordTooLong
	}

	if c.Policy.RejectVeryWeak {
		if looksVeryWeak(password) {
			return ErrWeakPassword
		}
	}

	return nil
}

// looksVeryWeak is intentionally minimal and conservative.
// It is not a full zxcvbn-style estimator (non-goal).
func looksVeryWeak(pw string) bool {
	s := strings.TrimSpace(pw)
	if s == "" {
		return true
	}

	// Reject if all same char.
	allSame := true
	var first rune
	for i, r := range s {
		if i == 0 {
			first = r
			continue
		}
		if r != first {
			allSame = false
			break
		}
	}
	if allSame {
		return true
	}

	// Reject if it's only digits and short-ish (common PIN-like).
	onlyDigits := true
	for _, r := range s {
		if !unicode.IsDigit(r) {
			onlyDigits = false
			break
		}
	}
	if onlyDigits && utf8.RuneCountInString(s) < 12 {
		return true
	}

	// Reject common trivial patterns.
	lower := strings.ToLower(s)
	switch lower {
	case "password", "password123", "123456", "123456789", "qwerty", "qwerty123", "11111111":
		return true
	}

	return false
}
