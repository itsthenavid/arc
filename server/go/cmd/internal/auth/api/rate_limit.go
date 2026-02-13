package authapi

import (
	"context"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func (h *Handler) checkLoginIPThrottle(ctx context.Context, ip net.IP, now time.Time) (bool, time.Duration, error) {
	if ip == nil || h.cfg.LoginIPMax <= 0 || h.cfg.LoginIPWindow <= 0 {
		return false, 0, nil
	}
	cut := now.Add(-h.cfg.LoginIPWindow)
	failures, err := recentLoginFailureTimesByIP(ctx, h.pool, ip, cut, h.cfg.LoginIPMax)
	if err != nil {
		return false, 0, err
	}

	blocked, retryAfter := evaluateWindowThrottle(now, failures, h.cfg.LoginIPMax, h.cfg.LoginIPWindow)
	return blocked, retryAfter, nil
}

func (h *Handler) checkLoginIdentifierThrottle(ctx context.Context, identifier string, now time.Time) (bool, time.Duration, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return false, 0, nil
	}

	limit := maxInt(
		h.cfg.LoginUserMax,
		h.cfg.LockoutShortThreshold,
		h.cfg.LockoutLongThreshold,
		h.cfg.LockoutSevereThreshold,
	)
	lookback := maxDuration(
		h.cfg.LoginUserWindow,
		h.cfg.LockoutShortDuration,
		h.cfg.LockoutLongDuration,
		h.cfg.LockoutSevereDuration,
	)
	if limit <= 0 || lookback <= 0 {
		return false, 0, nil
	}

	failures, err := recentLoginFailureTimesByIdentifier(ctx, h.pool, identifier, now.Add(-lookback), limit)
	if err != nil {
		return false, 0, err
	}

	// Strongest lockout tier wins.
	if blocked, retryAfter := evaluateProgressiveLockout(now, failures, []lockoutTier{
		{Threshold: h.cfg.LockoutSevereThreshold, Duration: h.cfg.LockoutSevereDuration},
		{Threshold: h.cfg.LockoutLongThreshold, Duration: h.cfg.LockoutLongDuration},
		{Threshold: h.cfg.LockoutShortThreshold, Duration: h.cfg.LockoutShortDuration},
	}); blocked {
		return true, retryAfter, nil
	}

	blocked, retryAfter := evaluateWindowThrottle(now, failures, h.cfg.LoginUserMax, h.cfg.LoginUserWindow)
	return blocked, retryAfter, nil
}

func writeRateLimited(w http.ResponseWriter, retryAfter time.Duration) {
	writeRateLimitedError(w, retryAfter, "rate_limited", "too many attempts")
}

func writeRateLimitedError(w http.ResponseWriter, retryAfter time.Duration, code string, msg string) {
	if secs := retryAfterSeconds(retryAfter); secs > 0 {
		w.Header().Set("Retry-After", strconv.FormatInt(secs, 10))
	}
	writeError(w, http.StatusTooManyRequests, code, msg)
}

func retryAfterSeconds(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	// Retry-After uses whole seconds. Round up to avoid "0" for sub-second windows.
	return int64(math.Ceil(d.Seconds()))
}

type lockoutTier struct {
	Threshold int
	Duration  time.Duration
}

// evaluateWindowThrottle checks if there are at least limit failures inside [now-window, now].
// failures must be sorted DESC by created_at.
func evaluateWindowThrottle(now time.Time, failures []time.Time, limit int, window time.Duration) (bool, time.Duration) {
	if limit <= 0 || window <= 0 || len(failures) < limit {
		return false, 0
	}
	nth := failures[limit-1]
	retryAfter := nth.Add(window).Sub(now)
	if retryAfter <= 0 {
		return false, 0
	}
	return true, retryAfter
}

// evaluateProgressiveLockout applies strongest-tier-first lockout rules.
// failures must be sorted DESC by created_at.
func evaluateProgressiveLockout(now time.Time, failures []time.Time, tiers []lockoutTier) (bool, time.Duration) {
	if len(failures) == 0 {
		return false, 0
	}

	last := failures[0]
	for _, tier := range tiers {
		if tier.Threshold <= 0 || tier.Duration <= 0 || len(failures) < tier.Threshold {
			continue
		}

		// Threshold is considered met only if the Nth latest failure still falls
		// inside this tier duration window.
		nth := failures[tier.Threshold-1]
		if nth.Add(tier.Duration).Sub(now) <= 0 {
			continue
		}

		retryAfter := last.Add(tier.Duration).Sub(now)
		if retryAfter > 0 {
			return true, retryAfter
		}
	}
	return false, 0
}

func maxInt(vals ...int) int {
	out := 0
	for _, v := range vals {
		if v > out {
			out = v
		}
	}
	return out
}

func maxDuration(vals ...time.Duration) time.Duration {
	var out time.Duration
	for _, v := range vals {
		if v > out {
			out = v
		}
	}
	return out
}

// ---- audit queries ----

func recentLoginFailureTimesByIP(ctx context.Context, pool *pgxpool.Pool, ip net.IP, since time.Time, limit int) ([]time.Time, error) {
	if pool == nil || ip == nil || limit <= 0 {
		return nil, nil
	}

	rows, err := pool.Query(ctx, `
		SELECT created_at
		FROM arc.audit_log
		WHERE action = 'auth.login.failed'
		  AND ip = $1
		  AND created_at >= $2
		ORDER BY created_at DESC
		LIMIT $3
	`, ip.String(), since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]time.Time, 0, limit)
	for rows.Next() {
		var ts time.Time
		if err := rows.Scan(&ts); err != nil {
			return nil, err
		}
		out = append(out, ts)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func recentLoginFailureTimesByIdentifier(ctx context.Context, pool *pgxpool.Pool, identifier string, since time.Time, limit int) ([]time.Time, error) {
	if pool == nil || strings.TrimSpace(identifier) == "" || limit <= 0 {
		return nil, nil
	}

	rows, err := pool.Query(ctx, `
		SELECT created_at
		FROM arc.audit_log
		WHERE action = 'auth.login.failed'
		  AND meta ->> 'identifier' = $1
		  AND created_at >= $2
		ORDER BY created_at DESC
		LIMIT $3
	`, identifier, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]time.Time, 0, limit)
	for rows.Next() {
		var ts time.Time
		if err := rows.Scan(&ts); err != nil {
			return nil, err
		}
		out = append(out, ts)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
