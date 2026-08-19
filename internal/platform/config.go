package platform

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"
)

type LookupEnv func(string) (string, bool)

type HTTPConfig struct {
	Addr            string
	LogLevel        slog.Level
	ShutdownTimeout time.Duration
}

func LoadHTTPConfig(lookup LookupEnv) (HTTPConfig, error) {
	addr := valueOrDefault(lookup, "NV_HTTP_ADDR", ":8080")
	if err := validateAddress(addr); err != nil {
		return HTTPConfig{}, fmt.Errorf("NV_HTTP_ADDR must be a valid listen address")
	}

	levelText := valueOrDefault(lookup, "NV_LOG_LEVEL", "info")
	var level slog.Level
	if err := level.UnmarshalText([]byte(levelText)); err != nil || (level != slog.LevelDebug && level != slog.LevelInfo && level != slog.LevelWarn && level != slog.LevelError) {
		return HTTPConfig{}, fmt.Errorf("NV_LOG_LEVEL must be one of debug, info, warn, error")
	}

	shutdownTimeout, err := Duration(lookup, "NV_SHUTDOWN_TIMEOUT", 10*time.Second, 5*time.Minute)
	if err != nil {
		return HTTPConfig{}, err
	}

	return HTTPConfig{Addr: addr, LogLevel: level, ShutdownTimeout: shutdownTimeout}, nil
}

func Required(lookup LookupEnv, key string) (string, error) {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func Port(lookup LookupEnv, key string, defaultValue int) (int, error) {
	text := valueOrDefault(lookup, key, strconv.Itoa(defaultValue))
	value, err := strconv.Atoi(text)
	if err != nil || value < 1 || value > 65535 {
		return 0, fmt.Errorf("%s must be an integer between 1 and 65535", key)
	}
	return value, nil
}

func NonNegativeInt(lookup LookupEnv, key string, defaultValue int) (int, error) {
	text := valueOrDefault(lookup, key, strconv.Itoa(defaultValue))
	value, err := strconv.Atoi(text)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", key)
	}
	return value, nil
}

func Duration(lookup LookupEnv, key string, defaultValue, maximum time.Duration) (time.Duration, error) {
	text := valueOrDefault(lookup, key, defaultValue.String())
	value, err := time.ParseDuration(text)
	if err != nil || value <= 0 || value > maximum {
		return 0, fmt.Errorf("%s must be a positive duration no greater than %s", key, maximum)
	}
	return value, nil
}

func valueOrDefault(lookup LookupEnv, key, defaultValue string) string {
	if value, ok := lookup(key); ok && strings.TrimSpace(value) != "" {
		return value
	}
	return defaultValue
}

func validateAddress(addr string) error {
	_, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid port")
	}
	return nil
}
