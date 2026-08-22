package nodeapp

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type Probe func(context.Context) error

type Health struct {
	Service string            `json:"service"`
	Status  string            `json:"status"`
	Checks  map[string]string `json:"checks"`
}

type Handler struct {
	postgres Probe
	redis    Probe
	timeout  time.Duration
	metrics  http.Handler
}

func NewHandler(postgres, redis Probe, timeout time.Duration, metrics http.Handler) http.Handler {
	return newHandler(postgres, redis, timeout, metrics)
}

func newHandler(postgres, redis Probe, timeout time.Duration, metrics http.Handler) *http.ServeMux {
	h := &Handler{postgres: postgres, redis: redis, timeout: timeout, metrics: metrics}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", h.live)
	mux.HandleFunc("GET /readyz", h.ready)
	mux.HandleFunc("GET /api/health", h.ready)
	mux.Handle("GET /metrics", metrics)
	return mux
}

func (h *Handler) live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"service": "node-app", "status": "alive"})
}

func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	checks := map[string]string{"postgres": "down", "redis": "down"}
	if h.postgres(ctx) == nil {
		checks["postgres"] = "up"
	}
	if h.redis(ctx) == nil {
		checks["redis"] = "up"
	}

	statusCode := http.StatusOK
	status := "ready"
	if checks["postgres"] != "up" || checks["redis"] != "up" {
		statusCode = http.StatusServiceUnavailable
		status = "not_ready"
	}
	writeJSON(w, statusCode, Health{Service: "node-app", Status: status, Checks: checks})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
