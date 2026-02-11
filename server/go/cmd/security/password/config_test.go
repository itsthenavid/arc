package password

import (
	"os"
	"testing"
)

func TestFromEnv_Defaults(t *testing.T) {
	// Ensure env is clean for this test.
	clearEnv := []string{
		"ARC_PASSWORD_MIN_LEN",
		"ARC_PASSWORD_MAX_LEN",
		"ARC_PASSWORD_REJECT_VERY_WEAK",
		"ARC_ARGON2_MEMORY_KIB",
		"ARC_ARGON2_ITERATIONS",
		"ARC_ARGON2_PARALLELISM",
		"ARC_ARGON2_SALT_LEN",
		"ARC_ARGON2_KEY_LEN",
	}
	for _, k := range clearEnv {
		_ = os.Unsetenv(k)
	}

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv error: %v", err)
	}

	def := DefaultConfig()
	if cfg.Policy.MinLength != def.Policy.MinLength {
		t.Fatalf("min length mismatch")
	}
	if cfg.Params.MemoryKiB != def.Params.MemoryKiB {
		t.Fatalf("memory mismatch")
	}
}

func TestFromEnv_Override(t *testing.T) {
	t.Setenv("ARC_PASSWORD_MIN_LEN", "10")
	t.Setenv("ARC_PASSWORD_MAX_LEN", "200")
	t.Setenv("ARC_PASSWORD_REJECT_VERY_WEAK", "true")
	t.Setenv("ARC_ARGON2_MEMORY_KIB", "32768")
	t.Setenv("ARC_ARGON2_ITERATIONS", "4")
	t.Setenv("ARC_ARGON2_PARALLELISM", "2")
	t.Setenv("ARC_ARGON2_SALT_LEN", "24")
	t.Setenv("ARC_ARGON2_KEY_LEN", "32")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv error: %v", err)
	}

	if cfg.Policy.MinLength != 10 || cfg.Policy.MaxLength != 200 || !cfg.Policy.RejectVeryWeak {
		t.Fatalf("policy override failed: %+v", cfg.Policy)
	}
	if cfg.Params.MemoryKiB != 32768 || cfg.Params.Iterations != 4 || cfg.Params.Parallelism != 2 {
		t.Fatalf("argon2 override failed: %+v", cfg.Params)
	}
	if cfg.Params.SaltLength != 24 || cfg.Params.KeyLength != 32 {
		t.Fatalf("len override failed: %+v", cfg.Params)
	}
}

func TestFromEnv_InvalidMinMax(t *testing.T) {
	t.Setenv("ARC_PASSWORD_MIN_LEN", "20")
	t.Setenv("ARC_PASSWORD_MAX_LEN", "10")

	_, err := FromEnv()
	if err == nil {
		t.Fatalf("expected error")
	}
}
