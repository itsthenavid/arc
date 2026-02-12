package api

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func (h *Handler) checkLoginIPThrottle(ctx context.Context, ip net.IP, now time.Time) (bool, time.Duration, error) {
	if ip == nil || h.cfg.LoginIPMax <= 0 {
		return false, 0, nil
	}
	cut := now.Add(-h.cfg.LoginIPWindow)
	count, err := countLoginFailuresByIP(ctx, h.pool, ip, cut)
	if err != nil {
		return false, 0, err
	}
	if count >= h.cfg.LoginIPMax {
		return true, h.cfg.LoginIPWindow, nil
	}
	return false, 0, nil
}

func (h *Handler) checkLoginUserThrottle(ctx context.Context, userID string, now time.Time) (bool, time.Duration, error) {
	if stringsTrim(userID) == "" {
		return false, 0, nil
	}
	cut := now.Add(-h.cfg.LoginUserWindow)
	count, err := countLoginFailuresByUser(ctx, h.pool, userID, cut)
	if err != nil {
		return false, 0, err
	}

	// Progressive lockout thresholds.
	switch {
	case h.cfg.LockoutSevereThreshold > 0 && count >= h.cfg.LockoutSevereThreshold:
		return true, h.cfg.LockoutSevereDuration, nil
	case h.cfg.LockoutLongThreshold > 0 && count >= h.cfg.LockoutLongThreshold:
		return true, h.cfg.LockoutLongDuration, nil
	case h.cfg.LockoutShortThreshold > 0 && count >= h.cfg.LockoutShortThreshold:
		return true, h.cfg.LockoutShortDuration, nil
	default:
		return false, 0, nil
	}
}

func writeRateLimited(w http.ResponseWriter, retryAfter time.Duration) {
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.FormatInt(int64(retryAfter.Seconds()), 10))
	}
	writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts")
}

// ---- audit queries ----

func countLoginFailuresByIP(ctx context.Context, pool *pgxpool.Pool, ip net.IP, since time.Time) (int, error) {
	if pool == nil || ip == nil {
		return 0, nil
	}
	var n int
	err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM arc.audit_log
		WHERE action = 'auth.login.failed'
		  AND ip = $1
		  AND created_at >= $2
	`, ip.String(), since).Scan(&n)
	return n, err
}

func countLoginFailuresByUser(ctx context.Context, pool *pgxpool.Pool, userID string, since time.Time) (int, error) {
	if pool == nil || stringsTrim(userID) == "" {
		return 0, nil
	}
	var n int
	err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM arc.audit_log
		WHERE action = 'auth.login.failed'
		  AND user_id = $1
		  AND created_at >= $2
	`, userID, since).Scan(&n)
	return n, err
}

func stringsTrim(s string) string {
	return strings.TrimSpace(s)
}
