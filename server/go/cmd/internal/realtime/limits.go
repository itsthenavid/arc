package realtime

import "time"

// Security/performance limits.
// Keep these aligned with docs/spec/realtime-v1.md (and PR policies).
const (
	// Max bytes per websocket frame read (hard limit).
	maxFrameBytes = 64 << 10 // 64 KiB

	// Max message text length (runes).
	maxMessageChars = 4000
)

const (
	// Heartbeat defaults (can be overridden by env in ws_gateway.go).
	heartbeatInterval = 25 * time.Second
	heartbeatTimeout  = 5 * time.Second

	// Per-connection rate limits (events per window).
	rateLimitEvents = 120
	rateLimitWindow = 10 * time.Second
)
