package password

import "testing"

func BenchmarkHash_DefaultConfig(b *testing.B) {
	cfg := DefaultConfig()
	pw := "this is a strong password 123!"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cfg.Hash(pw)
		if err != nil {
			b.Fatalf("Hash error: %v", err)
		}
	}
}

func BenchmarkVerify_DefaultConfig(b *testing.B) {
	cfg := DefaultConfig()
	pw := "this is a strong password 123!"
	h, err := cfg.Hash(pw)
	if err != nil {
		b.Fatalf("Hash error: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ok, err := cfg.Verify(h, pw)
		if err != nil || !ok {
			b.Fatalf("Verify failed: ok=%v err=%v", ok, err)
		}
	}
}
