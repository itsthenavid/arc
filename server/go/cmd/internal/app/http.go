package app

import (
	"net/http"
	"time"

	"arc/cmd/internal/auth/api"
	"arc/cmd/internal/realtime"

	"github.com/jackc/pgx/v5/pgxpool"
)

func registerHTTP(
	mux *http.ServeMux,
	log Logger,
	cfg Config,
	dbPool *pgxpool.Pool,
	dbEnabled bool,
	ws *realtime.WSGateway,
	auth *api.Handler,
) {
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if cfg.ReadinessRequireDB && !dbEnabled {
			http.Error(w, "db not configured", http.StatusServiceUnavailable)
			return
		}

		if dbEnabled && dbPool != nil {
			if err := PingDB(r.Context(), dbPool, 2*time.Second); err != nil {
				http.Error(w, "db not ready", http.StatusServiceUnavailable)
				log.Info("readyz.db.not_ready", "err", err)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})

	if auth != nil {
		auth.Register(mux)
	}

	mux.HandleFunc("/ws", ws.HandleWS)
}
