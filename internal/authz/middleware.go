package authz

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/new-vision-lab/new-vision/internal/authn"
)

// Middleware wraps the protected route tree. It authenticates the bearer
// token (when present) and authorizes the mapped (obj, act) for each route.
// Roles are loaded from the identity store on every request (no cache) so
// role changes take effect immediately.
type Middleware struct {
	tokens    *authn.TokenManager
	loader    RoleLoader
	anonymous []string // exact path prefixes that skip authn entirely
}

func NewMiddleware(tokens *authn.TokenManager, loader RoleLoader, anonymous []string) *Middleware {
	return &Middleware{tokens: tokens, loader: loader, anonymous: anonymous}
}

// With returns a handler that runs authn + authz for the given (obj, act).
func (m *Middleware) With(obj, act string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if m.isAnonymous(r) {
			next(w, r)
			return
		}
		p, ok := m.authenticate(w, r)
		if !ok {
			return
		}
		ctx := authn.WithPrincipal(r.Context(), p)
		// Load live roles from the identity store.
		userRoles, err := m.loader(ctx, p.TenantID)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "service_unavailable", "authorization engine unavailable")
			return
		}
		roles := userRoles[p.UserID]
		if !Allow(roles, obj, act) {
			writeError(w, http.StatusForbidden, "forbidden", "insufficient permissions")
			return
		}
		next(w, r.WithContext(ctx))
	}
}

// AuthenticatedOnly allows any logged-in user through without a specific
// permission point (e.g. GET /api/v1/auth/me).
func (m *Middleware) AuthenticatedOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := m.authenticate(w, r)
		if !ok {
			return
		}
		next(w, r.WithContext(authn.WithPrincipal(r.Context(), p)))
	}
}

func (m *Middleware) isAnonymous(r *http.Request) bool {
	for _, prefix := range m.anonymous {
		if strings.HasPrefix(r.URL.Path, prefix) {
			return true
		}
	}
	return false
}

func (m *Middleware) authenticate(w http.ResponseWriter, r *http.Request) (*authn.Principal, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
		return nil, false
	}
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid authorization header")
		return nil, false
	}
	p, err := m.tokens.Parse(token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
		return nil, false
	}
	return p, true
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}

// RoleLoader reads the authoritative user-role assignments for a tenant from
// the identity store. It is called on every authorization to make role
// changes take effect immediately (no cache, no invalidation plumbing).
type RoleLoader func(ctx context.Context, tenantID string) (map[string][]string, error)
