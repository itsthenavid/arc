package session

import (
	"testing"
	"time"

	paseto "aidanwoods.dev/go-paseto"
)

func TestLoadConfigFromEnv_MissingSecretKey(t *testing.T) {
	t.Setenv("ARC_PASETO_V4_SECRET_KEY_HEX", "")
	_, err := LoadConfigFromEnv()
	if err != ErrConfig {
		t.Fatalf("expected ErrConfig on missing secret, got %v", err)
	}
}

func TestLoadConfigFromEnv_InvalidDurations(t *testing.T) {
	secret := paseto.NewV4AsymmetricSecretKey()
	t.Setenv("ARC_PASETO_V4_SECRET_KEY_HEX", secret.ExportHex())
	t.Setenv("ARC_AUTH_ACCESS_TTL", "-5m")
	_, err := LoadConfigFromEnv()
	if err != ErrConfig {
		t.Fatalf("expected ErrConfig for negative duration, got %v", err)
	}
}

func TestLoadConfigFromEnv_InvalidRefreshTokenBytes(t *testing.T) {
	secret := paseto.NewV4AsymmetricSecretKey()
	t.Setenv("ARC_PASETO_V4_SECRET_KEY_HEX", secret.ExportHex())
	t.Setenv("ARC_AUTH_REFRESH_TOKEN_BYTES", "16")
	_, err := LoadConfigFromEnv()
	if err != ErrConfig {
		t.Fatalf("expected ErrConfig for small refresh bytes, got %v", err)
	}
}

func TestLoadConfigFromEnv_InvalidNativeTTLOrder(t *testing.T) {
	secret := paseto.NewV4AsymmetricSecretKey()
	t.Setenv("ARC_PASETO_V4_SECRET_KEY_HEX", secret.ExportHex())
	t.Setenv("ARC_AUTH_REFRESH_TTL_NATIVE", "24h")
	t.Setenv("ARC_AUTH_REFRESH_TTL_NATIVE_SHORT", "72h")
	_, err := LoadConfigFromEnv()
	if err != ErrConfig {
		t.Fatalf("expected ErrConfig for native ttl order, got %v", err)
	}
}

func TestLoadConfigFromEnv_Valid(t *testing.T) {
	secret := paseto.NewV4AsymmetricSecretKey()
	t.Setenv("ARC_PASETO_V4_SECRET_KEY_HEX", secret.ExportHex())
	t.Setenv("ARC_AUTH_ISSUER", "arc-test")
	t.Setenv("ARC_AUTH_ACCESS_TTL", "10m")
	t.Setenv("ARC_AUTH_REFRESH_TTL_WEB", "48h")
	t.Setenv("ARC_AUTH_REFRESH_TTL_NATIVE", "720h")
	t.Setenv("ARC_AUTH_REFRESH_TTL_NATIVE_SHORT", "168h")
	t.Setenv("ARC_AUTH_CLOCK_SKEW", "20s")
	t.Setenv("ARC_AUTH_REFRESH_TOKEN_BYTES", "32")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Issuer != "arc-test" {
		t.Fatalf("issuer mismatch: %q", cfg.Issuer)
	}
	if cfg.AccessTokenTTL != 10*time.Minute {
		t.Fatalf("access ttl mismatch: %v", cfg.AccessTokenTTL)
	}
	if cfg.RefreshTTLWeb != 48*time.Hour {
		t.Fatalf("refresh web ttl mismatch: %v", cfg.RefreshTTLWeb)
	}
	if cfg.RefreshTTLNative != 720*time.Hour {
		t.Fatalf("refresh native ttl mismatch: %v", cfg.RefreshTTLNative)
	}
	if cfg.RefreshTTLNativeShort != 168*time.Hour {
		t.Fatalf("refresh native short ttl mismatch: %v", cfg.RefreshTTLNativeShort)
	}
	if cfg.ClockSkew != 20*time.Second {
		t.Fatalf("clock skew mismatch: %v", cfg.ClockSkew)
	}
	if cfg.RefreshTokenBytes != 32 {
		t.Fatalf("refresh token bytes mismatch: %d", cfg.RefreshTokenBytes)
	}
}
