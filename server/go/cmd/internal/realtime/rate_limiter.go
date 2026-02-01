package realtime

import (
	"sync"
	"time"
)

type RateLimiter struct {
	mu     sync.Mutex
	events []time.Time
}

func (r *RateLimiter) Allow(now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	cut := now.Add(-rateLimitWindow)
	dst := r.events[:0]
	for _, t := range r.events {
		if t.After(cut) {
			dst = append(dst, t)
		}
	}
	r.events = dst

	if len(r.events) >= rateLimitEvents {
		return false
	}
	r.events = append(r.events, now)
	return true
}
