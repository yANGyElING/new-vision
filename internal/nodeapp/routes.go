package nodeapp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"

	"github.com/new-vision-lab/new-vision/internal/audit"
	"github.com/new-vision-lab/new-vision/internal/authn"
	"github.com/new-vision-lab/new-vision/internal/authz"
	"github.com/new-vision-lab/new-vision/internal/identity"
	"github.com/new-vision-lab/new-vision/internal/nodeapp/access"
	"github.com/new-vision-lab/new-vision/internal/nodeapp/device"
	"github.com/new-vision-lab/new-vision/internal/nodeapp/siptest"
)

var accessIDPattern = regexp.MustCompile(`^[0-9]{20}$`)

// accessEndpoints is the read-only Access runtime surface exposed by the
// console (snapshot / events / ack).
type accessEndpoints struct {
	snapshot func(context.Context) (access.RuntimeSnapshot, error)
	poll     func(context.Context, int64, int) (access.PollResult, error)
	ack      func(context.Context, int64) error
}

// Routes wires the full HTTP surface: health (anonymous), auth, identity
// management, devices, access console, and the test-only SIP simulator.
//
// The scope wrapper attaches the caller's tenant and expanded visible region
// ids (subtree) to the request context before device handlers run.
func NewRoutes(
	mux *http.ServeMux,
	authnHandler *authn.Handler,
	authzMiddleware *authz.Middleware,
	identityHandler *identity.Handler,
	devices device.DeviceEndpoints,
	accessEP accessEndpoints,
	sip *siptest.SIPSimulator,
	regions identity.RegionRepository,
	users identity.UserRepository,
	auditWriter *audit.Writer,
) {
	scope := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			p := authn.PrincipalFrom(r.Context())
			if p == nil {
				writeAPIError(w, http.StatusUnauthorized, "unauthorized", "missing principal")
				return
			}
			regionIDs, err := visibleRegionIDs(r.Context(), regions, users, p)
			if err != nil {
				writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "scope resolution unavailable")
				return
			}
			ctx := device.WithScope(r.Context(), p.TenantID, regionIDs)
			next(w, r.WithContext(ctx))
		}
	}

	// Auth endpoints: login is anonymous, me requires authentication.
	mux.HandleFunc("POST /api/v1/auth/login", authnHandler.Login)
	mux.HandleFunc("GET /api/v1/auth/me", authzMiddleware.AuthenticatedOnly(authnHandler.Me))

	// Identity management (node_admin only, mapped to identity:manage).
	mux.HandleFunc("POST /api/v1/tenants", authzMiddleware.With("identity", "manage", identityHandler.CreateTenant))
	mux.HandleFunc("GET /api/v1/tenants", authzMiddleware.With("identity", "manage", identityHandler.ListTenants))
	mux.HandleFunc("PATCH /api/v1/tenants/{id}", authzMiddleware.With("identity", "manage", identityHandler.PatchTenant))
	mux.HandleFunc("POST /api/v1/regions", authzMiddleware.With("identity", "manage", identityHandler.CreateRegion))
	mux.HandleFunc("GET /api/v1/regions", authzMiddleware.With("identity", "manage", identityHandler.ListRegions))
	mux.HandleFunc("PATCH /api/v1/regions/{id}", authzMiddleware.With("identity", "manage", identityHandler.PatchRegion))
	mux.HandleFunc("DELETE /api/v1/regions/{id}", authzMiddleware.With("identity", "manage", identityHandler.DeleteRegion))
	mux.HandleFunc("POST /api/v1/users", authzMiddleware.With("identity", "manage", identityHandler.CreateUser))
	mux.HandleFunc("GET /api/v1/users", authzMiddleware.With("identity", "manage", identityHandler.ListUsers))
	mux.HandleFunc("GET /api/v1/users/{id}", authzMiddleware.With("identity", "manage", identityHandler.GetUser))
	mux.HandleFunc("PATCH /api/v1/users/{id}", authzMiddleware.With("identity", "manage", identityHandler.PatchUser))
	mux.HandleFunc("DELETE /api/v1/users/{id}", authzMiddleware.With("identity", "manage", identityHandler.DeleteUser))
	mux.HandleFunc("POST /api/v1/users/{id}/password", authzMiddleware.With("identity", "manage", identityHandler.SetUserPassword))
	mux.HandleFunc("GET /api/v1/roles", authzMiddleware.With("identity", "manage", identityHandler.ListRoles))

	// Device management (mapped to device:* permission points); scope wrapper
	// attaches tenant + visible regions for data filtering.
	deviceGuard := func(obj, act string, h http.HandlerFunc) http.HandlerFunc {
		return authzMiddleware.With(obj, act, scope(h))
	}
	deviceAudit := func(ctx context.Context, action, resourceID string, detail map[string]any) {
		recordAudit(ctx, auditWriter, action, "device", resourceID, detail)
	}
	device.RegisterRoutes(mux, devices, deviceGuard, deviceAudit)

	// Access console (read-only runtime inspection).
	accessGuard := func(obj, act string, h http.HandlerFunc) http.HandlerFunc {
		return authzMiddleware.With(obj, act, h)
	}
	mux.HandleFunc("GET /api/v1/access/snapshot", accessGuard("access", "view", func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := accessEP.snapshot(r.Context())
		if err != nil {
			writeAccessConsoleError(w, err)
			return
		}
		recordAudit(r.Context(), auditWriter, "access.view", "access", "snapshot", nil)
		writeJSON(w, http.StatusOK, snapshot)
	}))
	mux.HandleFunc("GET /api/v1/access/events", accessGuard("access", "events", func(w http.ResponseWriter, r *http.Request) {
		after, err := parseNonNegativeIntQuery(r, "after", 0)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		limit, err := parsePositiveIntQuery(r, "limit", 50)
		if err != nil || limit > 500 {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 500")
			return
		}
		result, err := accessEP.poll(r.Context(), after, limit)
		if err != nil {
			writeAccessConsoleError(w, err)
			return
		}
		recordAudit(r.Context(), auditWriter, "access.events", "access", "events", map[string]any{"after": after, "limit": limit})
		writeJSON(w, http.StatusOK, result)
	}))
	mux.HandleFunc("POST /api/v1/access/ack", accessGuard("access", "ack", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ThroughSequence int64 `json:"through_sequence"`
		}
		if err := decodeJSONBody(w, r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		if request.ThroughSequence < 0 {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "through_sequence must be non-negative")
			return
		}
		if err := accessEP.ack(r.Context(), request.ThroughSequence); err != nil {
			writeAccessConsoleError(w, err)
			return
		}
		recordAudit(r.Context(), auditWriter, "access.ack", "access", "ack", map[string]any{"through_sequence": request.ThroughSequence})
		writeJSON(w, http.StatusOK, map[string]string{"status": "acked"})
	}))

	// SIP test routes (test-only, node_admin via test:sip permission points).
	if sip != nil {
		sipGuard := func(obj, act string, h http.HandlerFunc) http.HandlerFunc {
			return authzMiddleware.With(obj, act, h)
		}
		mux.HandleFunc("POST /api/v1/test/sip/register", sipGuard("test:sip", "register", func(w http.ResponseWriter, r *http.Request) {
			accessID, ok := readAccessIDBody(w, r)
			if !ok {
				return
			}
			result, err := sip.Register(r.Context(), accessID, 3600)
			writeSIPTestResult(w, result, err)
		}))
		mux.HandleFunc("POST /api/v1/test/sip/keepalive", sipGuard("test:sip", "keepalive", func(w http.ResponseWriter, r *http.Request) {
			accessID, ok := readAccessIDBody(w, r)
			if !ok {
				return
			}
			result, err := sip.KeepAlive(r.Context(), accessID)
			writeSIPTestResult(w, result, err)
		}))
		mux.HandleFunc("POST /api/v1/test/sip/unregister", sipGuard("test:sip", "unregister", func(w http.ResponseWriter, r *http.Request) {
			accessID, ok := readAccessIDBody(w, r)
			if !ok {
				return
			}
			result, err := sip.Unregister(r.Context(), accessID)
			writeSIPTestResult(w, result, err)
		}))
	}
}

// visibleRegionIDs resolves the principal's region scopes to the full set of
// visible region ids (each scoped region expanded to its subtree).
func visibleRegionIDs(ctx context.Context, regions identity.RegionRepository, users identity.UserRepository, p *authn.Principal) ([]string, error) {
	user, err := users.Get(ctx, p.TenantID, p.UserID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, regionID := range user.RegionIDs {
		subtree, err := regions.SubtreeIDs(ctx, regionID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, subtree...)
	}
	return ids, nil
}

func parseNonNegativeIntQuery(r *http.Request, name string, defaultValue int64) (int64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, errors.New(name + " must be a non-negative integer")
	}
	return value, nil
}

func parsePositiveIntQuery(r *http.Request, name string, defaultValue int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, errors.New(name + " must be a positive integer")
	}
	return value, nil
}

func writeAccessConsoleError(w http.ResponseWriter, err error) {
	var rpcErr *access.RPCError
	if errors.As(err, &rpcErr) {
		writeAPIError(w, http.StatusBadGateway, "access_rpc_error", rpcErr.Error())
		return
	}
	writeAPIError(w, http.StatusServiceUnavailable, "access_unavailable", "access is unavailable")
}

func writeSIPTestResult(w http.ResponseWriter, result siptest.SIPTestResult, err error) {
	if err != nil {
		switch {
		case errors.Is(err, device.ErrInvalid):
			writeAPIError(w, http.StatusBadRequest, "invalid_device", err.Error())
		case errors.Is(err, device.ErrNotFound):
			writeAPIError(w, http.StatusNotFound, "device_not_found", "device not found")
		default:
			writeAPIError(w, http.StatusServiceUnavailable, "sip_test_failed", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func readAccessIDBody(w http.ResponseWriter, r *http.Request) (string, bool) {
	var request struct {
		DeviceAccessID string `json:"device_access_id"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return "", false
	}
	if !accessIDPattern.MatchString(request.DeviceAccessID) {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "device_access_id must contain exactly 20 digits")
		return "", false
	}
	return request.DeviceAccessID, true
}

const maxRouteRequestBodyBytes = 64 << 10

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRouteRequestBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("request body must be valid JSON with only supported fields")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

// recordAudit writes a best-effort audit entry for the current principal.
func recordAudit(ctx context.Context, writer *audit.Writer, action, resourceType, resourceID string, detail any) {
	if writer == nil {
		return
	}
	p := authn.PrincipalFrom(ctx)
	if p == nil {
		return
	}
	writer.Record(ctx, audit.Entry{
		ActorUserID:  &p.UserID,
		TenantID:     &p.TenantID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Result:       audit.ResultSuccess,
		Detail:       detail,
	})
}
