package nodeapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHealthReady(t *testing.T) {
	handler := NewHandler(successProbe, successProbe, time.Second, http.NotFoundHandler())
	response := request(t, handler, "/api/health")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	assertHealth(t, response, "ready", map[string]string{"postgres": "up", "redis": "up"})
}

func TestHealthDegraded(t *testing.T) {
	handler := NewHandler(successProbe, func(context.Context) error { return errors.New("unavailable") }, time.Second, http.NotFoundHandler())
	response := request(t, handler, "/readyz")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	assertHealth(t, response, "not_ready", map[string]string{"postgres": "up", "redis": "down"})
}

func TestHealthTimeout(t *testing.T) {
	blocked := func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	handler := NewHandler(successProbe, blocked, 10*time.Millisecond, http.NotFoundHandler())
	response := request(t, handler, "/api/health")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestLivenessDoesNotProbeDependencies(t *testing.T) {
	var calls atomic.Int32
	probe := func(context.Context) error {
		calls.Add(1)
		return nil
	}
	handler := NewHandler(probe, probe, time.Second, http.NotFoundHandler())
	response := request(t, handler, "/livez")
	if response.Code != http.StatusOK || calls.Load() != 0 {
		t.Fatalf("status = %d, probe calls = %d", response.Code, calls.Load())
	}
}

func TestHealthRejectsOtherMethods(t *testing.T) {
	handler := NewHandler(successProbe, successProbe, time.Second, http.NotFoundHandler())
	req := httptest.NewRequest(http.MethodPost, "/api/health", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", response.Code)
	}
}

func successProbe(context.Context) error { return nil }

func request(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func assertHealth(t *testing.T, response *httptest.ResponseRecorder, status string, checks map[string]string) {
	t.Helper()
	var health Health
	if err := json.Unmarshal(response.Body.Bytes(), &health); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if health.Service != "node-app" || health.Status != status {
		t.Fatalf("health = %+v", health)
	}
	for name, want := range checks {
		if health.Checks[name] != want {
			t.Fatalf("checks[%q] = %q, want %q", name, health.Checks[name], want)
		}
	}
}
