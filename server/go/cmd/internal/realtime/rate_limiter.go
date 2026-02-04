package realtime

import (
	"sync"
	"time"
)

// RateLimiter is a per-connection sliding-window limiter.
type RateLimiter struct {
	mu     sync.Mutex
	events []time.Time
	limit  int
	window time.Duration
}

// NewRateLimiter constructs a RateLimiter with safe defaults when inputs are invalid.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	if limit <= 0 {
		limit = rateLimitEvents
	}
	if window <= 0 {
		window = rateLimitWindow
	}
	return &RateLimiter{
		events: make([]time.Time, 0, limit+8),
		limit:  limit,
		window: window,
	}
}

// Allow reports whether an event at time "now" should be permitted.
func (r *RateLimiter) Allow(now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	cut := now.Add(-r.window)
	dst := r.events[:0]
	for _, t := range r.events {
		if t.After(cut) {
			dst = append(dst, t)
		}
	}
	r.events = dst

	if len(r.events) >= r.limit {
		return false
	}
	r.events = append(r.events, now)
	return true
}
