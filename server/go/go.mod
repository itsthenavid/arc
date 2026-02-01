module arc

go 1.23

require github.com/coder/websocket v1.8.14

replace (
	arc/server/go/internal/realtime => ./internal/realtime
	arc/server/go/shared/contracts => ../shared/contracts
)
