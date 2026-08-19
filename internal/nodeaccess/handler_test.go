package nodeaccess

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReady(t *testing.T) {
	handler, err := NewHandler("test")
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var health Health
	if err := json.Unmarshal(response.Body.Bytes(), &health); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if health.Service != "node-access" || health.Status != "ready" || len(health.Checks) != 0 {
		t.Fatalf("health = %+v", health)
	}
}
