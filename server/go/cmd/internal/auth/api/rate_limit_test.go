package authapi

import (
	"testing"
	"time"
)

func TestEvaluateWindowThrottle(t *testing.T) {
	now := time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC)

	failures := []time.Time{
		now.Add(-1 * time.Minute),
		now.Add(-2 * time.Minute),
		now.Add(-6 * time.Minute),
	}

	blocked, retry := evaluateWindowThrottle(now, failures, 2, 5*time.Minute)
	if !blocked {
		t.Fatalf("expected window throttle to block")
	}
	if retry != 3*time.Minute {
		t.Fatalf("expected retry=3m, got %v", retry)
	}

	blocked, retry = evaluateWindowThrottle(now, failures, 3, 5*time.Minute)
	if blocked {
		t.Fatalf("expected window throttle to allow")
	}
	if retry != 0 {
		t.Fatalf("expected retry=0, got %v", retry)
	}
}

func TestEvaluateProgressiveLockout_ShortTier(t *testing.T) {
	now := time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC)
	failures := []time.Time{
		now.Add(-30 * time.Second),
		now.Add(-1 * time.Minute),
		now.Add(-2 * time.Minute),
		now.Add(-3 * time.Minute),
		now.Add(-4 * time.Minute),
	}

	blocked, retry := evaluateProgressiveLockout(now, failures, []lockoutTier{
		{Threshold: 20, Duration: 2 * time.Hour},
		{Threshold: 10, Duration: 30 * time.Minute},
		{Threshold: 5, Duration: 5 * time.Minute},
	})
	if !blocked {
		t.Fatalf("expected short-tier lockout")
	}
	if retry != 4*time.Minute+30*time.Second {
		t.Fatalf("unexpected retry duration: %v", retry)
	}
}

func TestEvaluateProgressiveLockout_ClearsAfterDuration(t *testing.T) {
	now := time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC)
	failures := []time.Time{
		now.Add(-6 * time.Minute),
		now.Add(-7 * time.Minute),
		now.Add(-8 * time.Minute),
		now.Add(-9 * time.Minute),
		now.Add(-10 * time.Minute),
	}

	blocked, retry := evaluateProgressiveLockout(now, failures, []lockoutTier{
		{Threshold: 5, Duration: 5 * time.Minute},
	})
	if blocked {
		t.Fatalf("expected lockout to clear, retry=%v", retry)
	}
	if retry != 0 {
		t.Fatalf("expected retry=0, got %v", retry)
	}
}

func TestEvaluateProgressiveLockout_SevereTierWins(t *testing.T) {
	now := time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC)
	failures := make([]time.Time, 0, 20)
	for i := 0; i < 20; i++ {
		failures = append(failures, now.Add(-time.Duration(i+1)*time.Minute))
	}

	blocked, retry := evaluateProgressiveLockout(now, failures, []lockoutTier{
		{Threshold: 20, Duration: 2 * time.Hour},
		{Threshold: 10, Duration: 30 * time.Minute},
		{Threshold: 5, Duration: 5 * time.Minute},
	})
	if !blocked {
		t.Fatalf("expected severe-tier lockout")
	}

	want := failures[0].Add(2 * time.Hour).Sub(now)
	if retry != want {
		t.Fatalf("expected retry=%v, got %v", want, retry)
	}
}
