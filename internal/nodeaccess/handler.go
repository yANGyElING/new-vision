package nodeaccess

import (
	"encoding/json"
	"net/http"

	"github.com/new-vision-lab/new-vision/internal/platform"
)

type Health struct {
	Service string            `json:"service"`
	Status  string            `json:"status"`
	Checks  map[string]string `json:"checks"`
}

func NewHandler(version string) (http.Handler, error) {
	metrics, err := platform.MetricsHandler("node-access", version)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"service": "node-access", "status": "alive"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, Health{Service: "node-access", Status: "ready", Checks: map[string]string{}})
	})
	mux.Handle("GET /metrics", metrics)
	return mux, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
