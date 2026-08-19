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

func NewHandler(postgres, redis Probe, timeout time.Duration, metrics http.Handler, devices ...DeviceEndpoints) http.Handler {
	h := &Handler{postgres: postgres, redis: redis, timeout: timeout, metrics: metrics}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", h.live)
	mux.HandleFunc("GET /readyz", h.ready)
	mux.HandleFunc("GET /api/health", h.ready)
	mux.Handle("GET /metrics", metrics)
	if len(devices) > 0 && devices[0] != nil {
		registerDeviceRoutes(mux, devices[0])
	}
	return mux
}

func (h *Handler) live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"service": "node-app", "status": "alive"})
}

func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	type result struct {
		name string
		up   bool
	}
	results := make(chan result, 2)
	for name, probe := range map[string]Probe{"postgres": h.postgres, "redis": h.redis} {
		go func() {
			results <- result{name: name, up: probe(ctx) == nil}
		}()
	}

	checks := map[string]string{"postgres": "down", "redis": "down"}
	for completed := 0; completed < 2; completed++ {
		select {
		case probeResult := <-results:
			if probeResult.up {
				checks[probeResult.name] = "up"
			}
		case <-ctx.Done():
			completed = 2
		}
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
