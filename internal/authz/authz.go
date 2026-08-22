package authz

import (
	"context"
	"errors"
	"sync"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"

	"github.com/new-vision-lab/new-vision/internal/authn"
)

// RoleLoader reads the authoritative user-role assignments for a tenant from
// the identity store. It is called lazily whenever a tenant's enforcer is
// missing from the cache, so the first request after startup (or after an
// Invalidate outside the identity handlers) sees real roles.
type RoleLoader func(ctx context.Context, tenantID string) (map[string][]string, error)

// EnforcerCache lazily builds and caches one Casbin enforcer per tenant.
// node-app is a single-instance deployment, so a process-local cache is
// sufficient; multi-instance deployments would need an invalidation
// broadcast. The cache is intentionally kept simple (mutex-guarded map).
//
// Policies live in memory only: the authoritative source is the identity
// business tables (user_roles), and the permission matrix is a constant.
type EnforcerCache struct {
	mu        sync.RWMutex
	enforcers map[string]*casbin.Enforcer
	loader    RoleLoader
}

func NewEnforcerCache(loader RoleLoader) *EnforcerCache {
	return &EnforcerCache{enforcers: map[string]*casbin.Enforcer{}, loader: loader}
}

// Load sets the role->permission matrix for a tenant from the given
// user-role map (userID -> roles). It rebuilds the tenant's enforcer.
func (c *EnforcerCache) Load(tenantID string, userRoles map[string][]string) error {
	e, err := buildEnforcer(tenantID, userRoles)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.enforcers[tenantID] = e
	c.mu.Unlock()
	return nil
}

// Invalidate drops the cached enforcer for a tenant.
func (c *EnforcerCache) Invalidate(tenantID string) {
	c.mu.Lock()
	delete(c.enforcers, tenantID)
	c.mu.Unlock()
}

// Authorize checks whether the principal may perform act on obj within
// its tenant domain. A cache miss loads the tenant's user roles through
// the configured RoleLoader, so enforcers always reflect the user_roles
// table even when they were never populated explicitly.
func (c *EnforcerCache) Authorize(ctx context.Context, p *authn.Principal, obj, act string) (bool, error) {
	if p == nil {
		return false, nil
	}
	e, err := c.enforcer(ctx, p.TenantID)
	if err != nil {
		return false, err
	}
	return e.Enforce(p.UserID, p.TenantID, obj, act)
}

// enforcer returns the cached enforcer for tenantID. On a miss it builds one
// from the permission matrix plus the tenant's live user roles (via the
// RoleLoader; a nil loader yields an enforcer with no user bindings). It
// never builds while holding the lock; the built enforcer is installed under
// the write lock so concurrent callers see one winner.
func (c *EnforcerCache) enforcer(ctx context.Context, tenantID string) (*casbin.Enforcer, error) {
	c.mu.RLock()
	e, ok := c.enforcers[tenantID]
	c.mu.RUnlock()
	if ok {
		return e, nil
	}
	// Build outside the lock: slow path only.
	userRoles := map[string][]string{}
	if c.loader != nil {
		loaded, err := c.loader(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		userRoles = loaded
	}
	e, err := buildEnforcer(tenantID, userRoles)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.enforcers[tenantID]; ok {
		return existing, nil
	}
	c.enforcers[tenantID] = e
	return e, nil
}

func buildEnforcer(tenantID string, userRoles map[string][]string) (*casbin.Enforcer, error) {
	m, err := model.NewModelFromString(CasbinModel)
	if err != nil {
		return nil, err
	}
	e, err := casbin.NewEnforcer(m)
	if err != nil {
		return nil, err
	}
	for role, objects := range rolePermissions {
		for obj, actions := range objects {
			for _, act := range actions {
				if _, err := e.AddPolicy(role, tenantID, obj, act); err != nil {
					return nil, err
				}
			}
		}
	}
	for userID, roles := range userRoles {
		for _, role := range roles {
			if !validRole(role) {
				continue
			}
			if _, err := e.AddGroupingPolicy(userID, role, tenantID); err != nil {
				return nil, err
			}
		}
	}
	if err := e.BuildRoleLinks(); err != nil {
		return nil, err
	}
	return e, nil
}

func validRole(role string) bool {
	switch role {
	case "node_admin", "tenant_admin", "operator", "viewer":
		return true
	}
	return false
}

// ErrNotLoaded is returned when a tenant enforcer has not been initialized.
var ErrNotLoaded = errors.New("authz: enforcer not loaded")
