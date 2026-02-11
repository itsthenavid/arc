package password

import "testing"

func TestHashAndVerify_OK(t *testing.T) {
	cfg := DefaultConfig()

	h, err := cfg.Hash("this is a strong password 123!")
	if err != nil {
		t.Fatalf("Hash error: %v", err)
	}

	ok, err := cfg.Verify(h, "this is a strong password 123!")
	if err != nil {
		t.Fatalf("Verify error: %v", err)
	}
	if !ok {
		t.Fatalf("expected match")
	}
}

func TestVerify_WrongPassword(t *testing.T) {
	cfg := DefaultConfig()

	h, err := cfg.Hash("this is a strong password 123!")
	if err != nil {
		t.Fatalf("Hash error: %v", err)
	}

	ok, err := cfg.Verify(h, "wrong password")
	if err != nil {
		t.Fatalf("Verify error: %v", err)
	}
	if ok {
		t.Fatalf("expected mismatch")
	}
}

func TestValidate_MinMax(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Policy.MinLength = 12
	cfg.Policy.MaxLength = 16

	if err := cfg.Validate("short"); err != ErrPasswordTooShort {
		t.Fatalf("expected ErrPasswordTooShort, got %v", err)
	}

	if err := cfg.Validate("this password is definitely too long"); err != ErrPasswordTooLong {
		t.Fatalf("expected ErrPasswordTooLong, got %v", err)
	}

	if err := cfg.Validate("goodpassw0rd!"); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestVerify_InvalidHash(t *testing.T) {
	cfg := DefaultConfig()

	ok, err := cfg.Verify("not-a-hash", "whatever")
	if err != ErrInvalidHash {
		t.Fatalf("expected ErrInvalidHash, got %v", err)
	}
	if ok {
		t.Fatalf("expected false")
	}
}

func TestPolicy_RejectVeryWeak(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Policy.RejectVeryWeak = true
	cfg.Policy.MinLength = 8

	if err := cfg.Validate("password"); err != ErrWeakPassword {
		t.Fatalf("expected ErrWeakPassword, got %v", err)
	}
	if err := cfg.Validate("11111111"); err != ErrWeakPassword {
		t.Fatalf("expected ErrWeakPassword, got %v", err)
	}
	if err := cfg.Validate("a-very-ok-pass"); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}
