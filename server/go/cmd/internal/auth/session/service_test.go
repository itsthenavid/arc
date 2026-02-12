package session

import (
	"testing"
	"time"

	paseto "aidanwoods.dev/go-paseto"
)

func TestPasetoV4_IssueAndVerify(t *testing.T) {
	secret := paseto.NewV4AsymmetricSecretKey()
	cfg := DefaultConfig()
	cfg.PasetoV4SecretKeyHex = secret.ExportHex()

	mgr, err := NewPasetoV4PublicManager(cfg)
	if err != nil {
		t.Fatalf("NewPasetoV4PublicManager: %v", err)
	}

	now := time.Now().UTC()
	tok, exp, err := mgr.Issue("01HZZZZZZZZZZZZZZZZZZZZZZZ", "01HYYYYYYYYYYYYYYYYYYYYYYYY", now)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if !exp.After(now) {
		t.Fatalf("expected exp after now")
	}

	claims, err := mgr.Verify(tok, now.Add(1*time.Second))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.UserID == "" || claims.SessionID == "" {
		t.Fatalf("missing claims")
	}
}
