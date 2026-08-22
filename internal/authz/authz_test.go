package authz

import (
	"context"
	"testing"

	"github.com/new-vision-lab/new-vision/internal/authn"
)

func TestAuthorizeLazyLoadUsesRoleLoader(t *testing.T) {
	// Regression: a cache miss must build the enforcer with the tenant's
	// live user roles. Building an enforcer with no user bindings (as
	// happened before the loader existed) denied every request, including
	// the seeded node_admin.
	roles := map[string][]string{"user-1": {"node_admin"}}
	cache := NewEnforcerCache(func(ctx context.Context, tenantID string) (map[string][]string, error) {
		return roles, nil
	})

	p := &authn.Principal{UserID: "user-1", TenantID: "tenant-1"}
	ok, err := cache.Authorize(context.Background(), p, ObjIdentity, ActManage)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if !ok {
		t.Fatal("node_admin should be allowed identity:manage via lazy-loaded roles")
	}
}

func TestAuthorizeUnknownUserDenied(t *testing.T) {
	cache := NewEnforcerCache(func(ctx context.Context, tenantID string) (map[string][]string, error) {
		return map[string][]string{"user-1": {"viewer"}}, nil
	})
	p := &authn.Principal{UserID: "user-2", TenantID: "tenant-1"}
	ok, err := cache.Authorize(context.Background(), p, ObjDevice, ActView)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if ok {
		t.Fatal("user without roles must be denied")
	}
}

func TestInvalidateReloadsFromLoader(t *testing.T) {
	roles := map[string][]string{"user-1": {"viewer"}}
	cache := NewEnforcerCache(func(ctx context.Context, tenantID string) (map[string][]string, error) {
		return roles, nil
	})
	p := &authn.Principal{UserID: "user-1", TenantID: "tenant-1"}

	if ok, _ := cache.Authorize(context.Background(), p, ObjDevice, ActCreate); ok {
		t.Fatal("viewer must not create devices")
	}
	// Role assignment changes in the store: Invalidate must make the next
	// request pick the new roles up through the loader.
	roles["user-1"] = []string{"tenant_admin"}
	cache.Invalidate("tenant-1")
	if ok, err := cache.Authorize(context.Background(), p, ObjDevice, ActCreate); err != nil || !ok {
		t.Fatalf("tenant_admin should create devices after invalidate: ok=%v err=%v", ok, err)
	}
}

func TestAuthorizeNilPrincipalDenied(t *testing.T) {
	cache := NewEnforcerCache(nil)
	ok, err := cache.Authorize(context.Background(), nil, ObjDevice, ActView)
	if err != nil || ok {
		t.Fatalf("nil principal must be denied without error: ok=%v err=%v", ok, err)
	}
}
