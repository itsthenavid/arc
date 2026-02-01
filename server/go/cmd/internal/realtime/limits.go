package realtime

import "time"

const (
	maxFrameBytes     = 64 * 1024
	maxMessageChars   = 4000
	rateLimitEvents   = 20
	rateLimitWindow   = 10 * time.Second
	heartbeatInterval = 25 * time.Second
	heartbeatTimeout  = 5 * time.Second
)
