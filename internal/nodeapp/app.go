package nodeapp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/new-vision-lab/new-vision/internal/audit"
	"github.com/new-vision-lab/new-vision/internal/authn"
	"github.com/new-vision-lab/new-vision/internal/authz"
	"github.com/new-vision-lab/new-vision/internal/identity"
	"github.com/new-vision-lab/new-vision/internal/nodeapp/access"
	"github.com/new-vision-lab/new-vision/internal/nodeapp/device"
	"github.com/new-vision-lab/new-vision/internal/nodeapp/siptest"
	"github.com/new-vision-lab/new-vision/internal/nodeapp/sync"
	"github.com/new-vision-lab/new-vision/internal/platform"
	"github.com/redis/go-redis/v9"
)

type App struct {
	Handler  http.Handler
	postgres *pgxpool.Pool
	redis    *redis.Client
	cancel   context.CancelFunc
}

func New(ctx context.Context, cfg Config, version string, logger *slog.Logger) (*App, error) {
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

	// Identity + audit + authn + authz.
	store := identity.NewStore(postgres)
	auditWriter := audit.NewWriter(postgres, logger)
	tokens, err := authn.NewTokenManager(cfg.JWTSecret, cfg.JWTTTL)
	if err != nil {
		postgres.Close()
		_ = redisClient.Close()
		cancel()
		return nil, fmt.Errorf("initialize token manager: %w", err)
	}
	authnHandler := authn.NewHandler(store.Tenants, store.Users, tokens, auditWriter)
	authzCache := authz.NewEnforcerCache(func(ctx context.Context, tenantID string) (map[string][]string, error) {
		users, err := store.Users.List(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		userRoles := make(map[string][]string, len(users))
		for _, u := range users {
			userRoles[u.ID] = u.Roles
		}
		return userRoles, nil
	})
	authzMiddleware := authz.NewMiddleware(tokens, authzCache, anonymousRoutes)
	identityHandler := identity.NewHandler(store, auditWriter, tokens.Hasher(), func(ctx context.Context) *identity.PrincipalInfo {
		p := authn.PrincipalFrom(ctx)
		if p == nil {
			return nil
		}
		return &identity.PrincipalInfo{UserID: p.UserID, TenantID: p.TenantID}
	}, func(tenantID string) {
		// Role assignments changed: drop the cached enforcer. The next
		// request rebuilds it from the authoritative user_roles table via
		// the cache's role loader, so new roles take effect immediately
		// without a restart.
		authzCache.Invalidate(tenantID)
	})

	// Device / access / sync / siptest.
	devices := device.NewPostgresDeviceRepository(postgres)
	projection := access.NewRedisProjection(redisClient)
	accessClient := access.NewAccessClient(cfg.AccessRPCURL, cfg.AccessRPCTimeout)
	deviceManager := device.NewDeviceManager(devices, projection)
	siptestSim := siptest.NewSIPSimulator(cfg.SIPHost, cfg.SIPPort, cfg.AccessRPCTimeout, devices)
	accessEP := accessEndpoints{
		snapshot: accessClient.GetRuntimeSnapshot,
		poll:     accessClient.PollEvents,
		ack:      accessClient.AckEvents,
	}

	// Route assembly.
	healthMux := newHandler(postgres.Ping, func(ctx context.Context) error {
		return redisClient.Ping(ctx).Err()
	}, cfg.HealthTimeout, metrics)
	NewRoutes(healthMux, authnHandler, authzMiddleware, identityHandler, deviceManager, accessEP, siptestSim, store.Regions, store.Users, auditWriter)
	app := &App{
		Handler:  healthMux,
		postgres: postgres,
		redis:    redisClient,
		cancel:   cancel,
	}

	// Seed the initial admin user first, then warm the authz cache: the
	// enforcer must be built after the admin exists so its roles are
	// included (the lazy loader covers later tenants either way).
	if err := seedAdmin(ctx, store, tokens.Hasher(), cfg.SeedAdminPassword); err != nil {
		app.Close()
		return nil, fmt.Errorf("seed admin: %w", err)
	}
	if err := loadAuthzRoles(ctx, store, authzCache); err != nil {
		app.Close()
		return nil, fmt.Errorf("load authorization roles: %w", err)
	}

	go sync.NewSyncRunner(devices, accessClient, projection, cfg.AccessPollInterval).Run(ctx)
	return app, nil
}

func (a *App) Close() {
	if a.cancel != nil {
		a.cancel()
	}
	a.postgres.Close()
	_ = a.redis.Close()
}

// anonymousRoutes are prefixes that skip authn entirely.
var anonymousRoutes = []string{
	"/livez",
	"/readyz",
	"/api/health",
	"/metrics",
	"/api/v1/auth/login",
}

// loadAuthzRoles populates the authz enforcer cache for all tenants.
func loadAuthzRoles(ctx context.Context, store *identity.Store, cache *authz.EnforcerCache) error {
	tenants, err := store.Tenants.List(ctx)
	if err != nil {
		return err
	}
	for _, tenant := range tenants {
		users, err := store.Users.List(ctx, tenant.ID)
		if err != nil {
			return err
		}
		userRoles := make(map[string][]string, len(users))
		for _, u := range users {
			userRoles[u.ID] = u.Roles
		}
		if err := cache.Load(tenant.ID, userRoles); err != nil {
			return err
		}
	}
	return nil
}

// seedAdmin creates the initial node_admin user when the users table is empty
// and NV_SEED_ADMIN_PASSWORD is configured. It belongs to the default tenant
// (created by migration 000004) and the root region.
func seedAdmin(ctx context.Context, store *identity.Store, hasher interface{ Hash(string) (string, error) }, password string) error {
	if password == "" {
		return nil
	}
	tenant, err := store.Tenants.GetByName(ctx, "default")
	if err != nil {
		return fmt.Errorf("default tenant missing: %w", err)
	}
	existing, err := store.Users.List(ctx, tenant.ID)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	hash, err := hasher.Hash(password)
	if err != nil {
		return err
	}
	rootRegion, err := store.Regions.Get(ctx, "00000000-0000-0000-0000-000000000002")
	if err != nil {
		return fmt.Errorf("root region missing: %w", err)
	}
	_, err = store.Users.Create(ctx, identity.CreateUserInput{
		TenantID:    tenant.ID,
		Username:    "admin",
		Password:    password,
		DisplayName: "Platform Admin",
		Roles:       []string{identity.RoleNodeAdmin},
		RegionIDs:   []string{rootRegion.ID},
	}, hash)
	if err != nil {
		return err
	}
	return nil
}
