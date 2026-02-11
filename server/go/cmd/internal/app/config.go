package app

import "time"

// Config contains all runtime configuration loaded from environment variables.
type Config struct {
	HTTPAddr string
	LogLevel string

	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int

	DatabaseURL string
	DBMaxConns  int32
	DBMinConns  int32

	// If true:
	// - /readyz returns 503 unless DB is configured and reachable.
	ReadinessRequireDB bool

	// Security policy:
	// If true, ARC_TOKEN_HMAC_KEY MUST be set (>= 32 bytes) and refresh-token hashing must be HMAC-based.
	RequireTokenHMAC bool
}

// LoadConfig loads Config from environment variables with defaults.
func LoadConfig() Config {
	return Config{
		HTTPAddr: EnvString("ARC_HTTP_ADDR", "0.0.0.0:8080"),
		LogLevel: EnvString("ARC_LOG_LEVEL", "info"),

		ReadHeaderTimeout: EnvDuration("ARC_HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		ReadTimeout:       EnvDuration("ARC_HTTP_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:      EnvDuration("ARC_HTTP_WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:       EnvDuration("ARC_HTTP_IDLE_TIMEOUT", 60*time.Second),

		MaxHeaderBytes: EnvInt("ARC_HTTP_MAX_HEADER_BYTES", 1<<20),

		DatabaseURL: EnvString("ARC_DATABASE_URL", ""),
		DBMaxConns:  EnvInt32("ARC_DB_MAX_CONNS", 10),
		DBMinConns:  EnvInt32("ARC_DB_MIN_CONNS", 0),

		ReadinessRequireDB: EnvBool("ARC_READINESS_REQUIRE_DB", false),

		RequireTokenHMAC: EnvBool("ARC_REQUIRE_TOKEN_HMAC", false),
	}
}
