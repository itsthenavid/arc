package authapi

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config controls auth API behavior and security defaults.
type Config struct {
	InviteOnly    bool
	InviteTTL     time.Duration
	InviteMaxTTL  time.Duration
	TrustProxy    bool
	MaxBodyBytes  int64
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
		InviteOnly:             envBool("ARC_AUTH_INVITE_ONLY", true),
		InviteTTL:              envDuration("ARC_AUTH_INVITE_TTL", 7*24*time.Hour),
		InviteMaxTTL:           envDuration("ARC_AUTH_INVITE_TTL_MAX", 30*24*time.Hour),
		TrustProxy:             envBool("ARC_AUTH_TRUST_PROXY", false),
		MaxBodyBytes:           envInt64("ARC_AUTH_MAX_BODY_BYTES", 1<<20), // 1 MiB
		LoginIPMax:             envInt("ARC_AUTH_LOGIN_IP_MAX", 20),
		LoginIPWindow:          envDuration("ARC_AUTH_LOGIN_IP_WINDOW", 5*time.Minute),
		LoginUserMax:           envInt("ARC_AUTH_LOGIN_USER_MAX", 5),
		LoginUserWindow:        envDuration("ARC_AUTH_LOGIN_USER_WINDOW", 15*time.Minute),
		LockoutShortThreshold:  envInt("ARC_AUTH_LOGIN_LOCKOUT_SHORT_THRESHOLD", 5),
		LockoutShortDuration:   envDuration("ARC_AUTH_LOGIN_LOCKOUT_SHORT_DURATION", 5*time.Minute),
		LockoutLongThreshold:   envInt("ARC_AUTH_LOGIN_LOCKOUT_LONG_THRESHOLD", 10),
		LockoutLongDuration:    envDuration("ARC_AUTH_LOGIN_LOCKOUT_LONG_DURATION", 30*time.Minute),
		LockoutSevereThreshold: envInt("ARC_AUTH_LOGIN_LOCKOUT_SEVERE_THRESHOLD", 20),
		LockoutSevereDuration:  envDuration("ARC_AUTH_LOGIN_LOCKOUT_SEVERE_DURATION", 2*time.Hour),
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

	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 1 << 20
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
