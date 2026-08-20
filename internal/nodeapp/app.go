package nodeapp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/new-vision-lab/new-vision/internal/platform"
	"github.com/redis/go-redis/v9"
)

type App struct {
	Handler  http.Handler
	postgres *pgxpool.Pool
	redis    *redis.Client
	cancel   context.CancelFunc
}

func New(ctx context.Context, cfg Config, version string) (*App, error) {
	postgresConfig, err := pgxpool.ParseConfig("sslmode=" + cfg.PostgresSSLMode)
	if err != nil {
		return nil, fmt.Errorf("initialize PostgreSQL configuration: %w", err)
	}
	postgresConfig.ConnConfig.Host = cfg.PostgresHost
	postgresConfig.ConnConfig.Port = uint16(cfg.PostgresPort)
	postgresConfig.ConnConfig.Database = cfg.PostgresDatabase
	postgresConfig.ConnConfig.User = cfg.PostgresUser
	postgresConfig.ConnConfig.Password = cfg.PostgresPassword

	postgres, err := pgxpool.NewWithConfig(ctx, postgresConfig)
	if err != nil {
		return nil, fmt.Errorf("initialize PostgreSQL client: %w", err)
	}
	redisClient := redis.NewClient(&redis.Options{
		Addr:     net.JoinHostPort(cfg.RedisHost, strconv.Itoa(cfg.RedisPort)),
		Username: cfg.RedisUsername,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDatabase,
	})
	metrics, err := platform.MetricsHandler("node-app", version)
	if err != nil {
		postgres.Close()
		_ = redisClient.Close()
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	devices := NewPostgresDeviceRepository(postgres)
	projection := NewRedisProjection(redisClient)
	access := NewAccessClient(cfg.AccessRPCURL, cfg.AccessRPCTimeout)
	deviceManager := NewDeviceManager(devices, projection)
	sipSim := NewSIPSimulator(cfg.SIPHost, cfg.SIPPort, cfg.AccessRPCTimeout, devices)
	app := &App{
		Handler: NewConsoleHandler(postgres.Ping, func(ctx context.Context) error {
			return redisClient.Ping(ctx).Err()
		}, cfg.HealthTimeout, metrics, ConsoleDeps{
			Devices: deviceManager,
			Access:  access,
			SIP:     sipSim,
		}),
		postgres: postgres,
		redis:    redisClient,
		cancel:   cancel,
	}
	go NewSyncRunner(devices, access, projection, cfg.AccessPollInterval).Run(ctx)
	return app, nil
}

func (a *App) Close() {
	if a.cancel != nil {
		a.cancel()
	}
	a.postgres.Close()
	_ = a.redis.Close()
}
