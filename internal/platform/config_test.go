package platform

import (
	"strings"
	"testing"
	"time"
)

func TestLoadHTTPConfigDefaults(t *testing.T) {
	cfg, err := LoadHTTPConfig(mapLookup(nil))
	if err != nil {
		t.Fatalf("LoadHTTPConfig() error = %v", err)
	}
	if cfg.Addr != ":8080" || cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("LoadHTTPConfig() = %+v", cfg)
	}
}

func TestLoadHTTPConfigRejectsInvalidValues(t *testing.T) {
	tests := []map[string]string{
		{"NV_HTTP_ADDR": "8080"},
		{"NV_LOG_LEVEL": "trace"},
		{"NV_SHUTDOWN_TIMEOUT": "0s"},
	}
	for _, values := range tests {
		if _, err := LoadHTTPConfig(mapLookup(values)); err == nil {
			t.Fatalf("LoadHTTPConfig(%v) expected error", values)
		}
	}
}

func TestRequiredDoesNotEchoValue(t *testing.T) {
	secret := "do-not-print-this"
	_, err := Port(mapLookup(map[string]string{"SECRET_PORT": secret}), "SECRET_PORT", 1)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("error must reject and redact input, got %v", err)
	}
}

func mapLookup(values map[string]string) LookupEnv {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
