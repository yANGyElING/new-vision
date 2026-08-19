package nodeapp

import (
	"strings"
	"testing"

	"github.com/new-vision-lab/new-vision/internal/platform"
)

func TestLoadConfig(t *testing.T) {
	cfg, err := LoadConfig(platform.LookupEnv(mapLookup(validEnvironment())))
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.PostgresHost != "postgres" || cfg.RedisDatabase != 0 {
		t.Fatalf("LoadConfig() = %+v", cfg)
	}
}

func TestLoadConfigRejectsMissingAndInvalidValuesWithoutSecrets(t *testing.T) {
	secret := "secret-that-must-not-appear"
	tests := []map[string]string{
		without(validEnvironment(), "NV_POSTGRES_HOST"),
		with(validEnvironment(), "NV_POSTGRES_PORT", secret),
		with(validEnvironment(), "NV_POSTGRES_SSLMODE", secret),
		with(validEnvironment(), "NV_REDIS_DB", "-1"),
	}
	for _, values := range tests {
		_, err := LoadConfig(platform.LookupEnv(mapLookup(values)))
		if err == nil {
			t.Fatalf("LoadConfig(%v) expected error", values)
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaked input: %v", err)
		}
	}
}

func validEnvironment() map[string]string {
	return map[string]string{
		"NV_POSTGRES_HOST":     "postgres",
		"NV_POSTGRES_DB":       "new_vision",
		"NV_POSTGRES_USER":     "new_vision",
		"NV_POSTGRES_PASSWORD": "public-local-password",
		"NV_REDIS_HOST":        "redis",
		"NV_REDIS_PASSWORD":    "public-local-password",
	}
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func without(values map[string]string, key string) map[string]string {
	delete(values, key)
	return values
}

func with(values map[string]string, key, value string) map[string]string {
	values[key] = value
	return values
}
