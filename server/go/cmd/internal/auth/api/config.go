package authapi

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config controls auth API behavior and security defaults.
type Config struct {
	InviteOnly       bool
	InviteTTL        time.Duration
	InviteMaxTTL     time.Duration
	InviteMaxUses    int
	InviteMaxUsesMax int
	TrustProxy       bool
	MaxBodyBytes     int64

	// Optional web transport mode:
	// refresh token in HttpOnly cookie + CSRF double-submit enforcement on refresh.
	WebRefreshCookieEnabled bool
	RefreshCookieName       string
	CSRFCookieName          string
	CSRFHeaderName          string
	CookieSecure            bool
	CookieSameSite          http.SameSite
	CookieDomain            string
	CookiePath              string

	LoginIPMax    int
	LoginIPWindow time.Duration

	LoginUserMax    int
	LoginUserWindow time.Duration

	LockoutShortThreshold  int
	LockoutShortDuration   time.Duration
	LockoutLongThreshold   int
	LockoutLongDuration    time.Duration
	LockoutSevereThreshold int
	LockoutSevereDuration  time.Duration
}

// LoadConfigFromEnv loads auth config from environment variables with safe defaults.
func LoadConfigFromEnv() Config {
	cfg := Config{
		InviteOnly:              envBool("ARC_AUTH_INVITE_ONLY", true),
		InviteTTL:               envDuration("ARC_AUTH_INVITE_TTL", 7*24*time.Hour),
		InviteMaxTTL:            envDuration("ARC_AUTH_INVITE_TTL_MAX", 30*24*time.Hour),
		InviteMaxUses:           envInt("ARC_AUTH_INVITE_MAX_USES", 1),
		InviteMaxUsesMax:        envInt("ARC_AUTH_INVITE_MAX_USES_MAX", 50),
		TrustProxy:              envBool("ARC_AUTH_TRUST_PROXY", false),
		MaxBodyBytes:            envInt64("ARC_AUTH_MAX_BODY_BYTES", 1<<20), // 1 MiB
		WebRefreshCookieEnabled: envBool("ARC_AUTH_WEB_COOKIE_MODE", false),
		RefreshCookieName:       envString("ARC_AUTH_REFRESH_COOKIE_NAME", "arc_refresh_token"),
		CSRFCookieName:          envString("ARC_AUTH_CSRF_COOKIE_NAME", "arc_csrf_token"),
		CSRFHeaderName:          envString("ARC_AUTH_CSRF_HEADER_NAME", "X-CSRF-Token"),
		CookieSecure:            envBool("ARC_AUTH_COOKIE_SECURE", true),
		CookieSameSite:          parseSameSite(envString("ARC_AUTH_COOKIE_SAMESITE", "lax")),
		CookieDomain:            strings.TrimSpace(os.Getenv("ARC_AUTH_COOKIE_DOMAIN")),
		CookiePath:              envString("ARC_AUTH_COOKIE_PATH", "/"),
		LoginIPMax:              envInt("ARC_AUTH_LOGIN_IP_MAX", 20),
		LoginIPWindow:           envDuration("ARC_AUTH_LOGIN_IP_WINDOW", 5*time.Minute),
		LoginUserMax:            envInt("ARC_AUTH_LOGIN_USER_MAX", 5),
		LoginUserWindow:         envDuration("ARC_AUTH_LOGIN_USER_WINDOW", 15*time.Minute),
		LockoutShortThreshold:   envInt("ARC_AUTH_LOGIN_LOCKOUT_SHORT_THRESHOLD", 5),
		LockoutShortDuration:    envDuration("ARC_AUTH_LOGIN_LOCKOUT_SHORT_DURATION", 5*time.Minute),
		LockoutLongThreshold:    envInt("ARC_AUTH_LOGIN_LOCKOUT_LONG_THRESHOLD", 10),
		LockoutLongDuration:     envDuration("ARC_AUTH_LOGIN_LOCKOUT_LONG_DURATION", 30*time.Minute),
		LockoutSevereThreshold:  envInt("ARC_AUTH_LOGIN_LOCKOUT_SEVERE_THRESHOLD", 20),
		LockoutSevereDuration:   envDuration("ARC_AUTH_LOGIN_LOCKOUT_SEVERE_DURATION", 2*time.Hour),
	}

	// Clamp TTLs to keep them sensible.
	if cfg.InviteTTL <= 0 {
		cfg.InviteTTL = 7 * 24 * time.Hour
	}
	if cfg.InviteMaxTTL <= 0 {
		cfg.InviteMaxTTL = 30 * 24 * time.Hour
	}
	if cfg.InviteTTL > cfg.InviteMaxTTL {
		cfg.InviteTTL = cfg.InviteMaxTTL
	}
	if cfg.InviteMaxUses <= 0 {
		cfg.InviteMaxUses = 1
	}
	if cfg.InviteMaxUsesMax <= 0 {
		cfg.InviteMaxUsesMax = 50
	}
	if cfg.InviteMaxUses > cfg.InviteMaxUsesMax {
		cfg.InviteMaxUses = cfg.InviteMaxUsesMax
	}

	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 1 << 20
	}
	if strings.TrimSpace(cfg.RefreshCookieName) == "" {
		cfg.RefreshCookieName = "arc_refresh_token"
	}
	if strings.TrimSpace(cfg.CSRFCookieName) == "" {
		cfg.CSRFCookieName = "arc_csrf_token"
	}
	if strings.TrimSpace(cfg.CSRFHeaderName) == "" {
		cfg.CSRFHeaderName = "X-CSRF-Token"
	}
	if strings.TrimSpace(cfg.CookiePath) == "" {
		cfg.CookiePath = "/"
	}
	if cfg.CSRFCookieName == cfg.RefreshCookieName {
		cfg.CSRFCookieName = "arc_csrf_token"
	}
	// SameSite=None cookies are ignored by modern browsers unless Secure=true.
	if cfg.CookieSameSite == http.SameSiteNoneMode {
		cfg.CookieSecure = true
	}
	if cfg.LoginIPMax <= 0 {
		cfg.LoginIPMax = 20
	}
	if cfg.LoginUserMax <= 0 {
		cfg.LoginUserMax = 5
	}

	return cfg
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func envInt64(key string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func envDuration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

func envString(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func parseSameSite(v string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	case "default":
		return http.SameSiteDefaultMode
	case "lax":
		fallthrough
	default:
		return http.SameSiteLaxMode
	}
}
