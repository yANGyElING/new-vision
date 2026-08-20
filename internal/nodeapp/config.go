package nodeapp

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/new-vision-lab/new-vision/internal/platform"
)

const maxHealthTimeout = 9 * time.Second

type Config struct {
	HTTP               platform.HTTPConfig
	HealthTimeout      time.Duration
	PostgresHost       string
	PostgresPort       int
	PostgresDatabase   string
	PostgresUser       string
	PostgresPassword   string
	PostgresSSLMode    string
	RedisHost          string
	RedisPort          int
	RedisUsername      string
	RedisPassword      string
	RedisDatabase      int
	AccessRPCURL       string
	AccessRPCTimeout   time.Duration
	AccessPollInterval time.Duration
	AccessInstanceID   string
	SIPHost            string
	SIPPort            int
}

func LoadConfig(lookup platform.LookupEnv) (Config, error) {
	httpConfig, err := platform.LoadHTTPConfig(lookup)
	if err != nil {
		return Config{}, err
	}
	healthTimeout, err := platform.Duration(lookup, "NV_HEALTH_TIMEOUT", time.Second, maxHealthTimeout)
	if err != nil {
		return Config{}, err
	}

	postgresHost, err := platform.Required(lookup, "NV_POSTGRES_HOST")
	if err != nil {
		return Config{}, err
	}
	postgresPort, err := platform.Port(lookup, "NV_POSTGRES_PORT", 5432)
	if err != nil {
		return Config{}, err
	}
	postgresDatabase, err := platform.Required(lookup, "NV_POSTGRES_DB")
	if err != nil {
		return Config{}, err
	}
	postgresUser, err := platform.Required(lookup, "NV_POSTGRES_USER")
	if err != nil {
		return Config{}, err
	}
	postgresPassword, err := platform.Required(lookup, "NV_POSTGRES_PASSWORD")
	if err != nil {
		return Config{}, err
	}
	postgresSSLMode := valueOrDefault(lookup, "NV_POSTGRES_SSLMODE", "disable")
	if !validSSLMode(postgresSSLMode) {
		return Config{}, fmt.Errorf("NV_POSTGRES_SSLMODE must be one of disable, allow, prefer, require, verify-ca, verify-full")
	}

	redisHost, err := platform.Required(lookup, "NV_REDIS_HOST")
	if err != nil {
		return Config{}, err
	}
	redisPort, err := platform.Port(lookup, "NV_REDIS_PORT", 6379)
	if err != nil {
		return Config{}, err
	}
	redisUsername := valueOrDefault(lookup, "NV_REDIS_USERNAME", "nodeapp")
	redisPassword, err := platform.Required(lookup, "NV_REDIS_PASSWORD")
	if err != nil {
		return Config{}, err
	}
	redisDatabase, err := platform.NonNegativeInt(lookup, "NV_REDIS_DB", 0)
	if err != nil {
		return Config{}, err
	}

	accessRPCURL := valueOrDefault(lookup, "NV_ACCESS_RPC_URL", "http://node-access:8090/rpc")
	if !validAccessRPCURL(accessRPCURL) {
		return Config{}, fmt.Errorf("NV_ACCESS_RPC_URL must be an http URL with no credentials")
	}
	accessRPCTimeout, err := platform.Duration(lookup, "NV_ACCESS_RPC_TIMEOUT", 3*time.Second, 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	accessPollInterval, err := platform.Duration(lookup, "NV_ACCESS_POLL_INTERVAL", time.Second, time.Minute)
	if err != nil {
		return Config{}, err
	}
	accessInstanceID := valueOrDefault(lookup, "NV_ACCESS_INSTANCE_ID", "access-01")
	if accessInstanceID == "" || len(accessInstanceID) > 128 || strings.TrimSpace(accessInstanceID) != accessInstanceID {
		return Config{}, fmt.Errorf("NV_ACCESS_INSTANCE_ID must be an unpadded value no longer than 128 bytes")
	}

	sipHost := valueOrDefault(lookup, "NV_SIP_HOST", "node-access")
	if strings.TrimSpace(sipHost) == "" || len(sipHost) > 255 {
		return Config{}, fmt.Errorf("NV_SIP_HOST must be a non-empty value no longer than 255 bytes")
	}
	sipPort, err := platform.Port(lookup, "NV_SIP_PORT", 5060)
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTP: httpConfig, HealthTimeout: healthTimeout,
		PostgresHost: postgresHost, PostgresPort: postgresPort, PostgresDatabase: postgresDatabase,
		PostgresUser: postgresUser, PostgresPassword: postgresPassword, PostgresSSLMode: postgresSSLMode,
		RedisHost: redisHost, RedisPort: redisPort, RedisUsername: redisUsername,
		RedisPassword: redisPassword, RedisDatabase: redisDatabase,
		AccessRPCURL: accessRPCURL, AccessRPCTimeout: accessRPCTimeout,
		AccessPollInterval: accessPollInterval, AccessInstanceID: accessInstanceID,
		SIPHost: sipHost, SIPPort: sipPort,
	}, nil
}

func valueOrDefault(lookup platform.LookupEnv, key, defaultValue string) string {
	if value, ok := lookup(key); ok && value != "" {
		return value
	}
	return defaultValue
}

func validAccessRPCURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "http" && parsed.Host != "" && parsed.User == nil
}

func validSSLMode(value string) bool {
	switch value {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
		return true
	default:
		return false
	}
}
