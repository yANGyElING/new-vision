package identity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"

	"github.com/new-vision-lab/new-vision/internal/audit"
)

// Handler exposes tenant/region/user/role management.
// Every endpoint requires the caller to be a node_admin (enforced by the
// authz middleware mapping routes to identity:manage).
type Handler struct {
	store        *Store
	audit        *audit.Writer
	hasher       interface{ Hash(string) (string, error) }
	principal    func(context.Context) *PrincipalInfo
	onRoleChanged func(tenantID string)
}

func NewHandler(store *Store, audit *audit.Writer, hasher interface{ Hash(string) (string, error) }, principal func(context.Context) *PrincipalInfo, onRoleChanged func(tenantID string)) *Handler {
	return &Handler{store: store, audit: audit, hasher: hasher, principal: principal, onRoleChanged: onRoleChanged}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func decodeBody(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("request body must be valid JSON with only supported fields")
	}
	// ensure no trailing data
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func mapIdentityError(w http.ResponseWriter, err error) {
	var invalidErr *InvalidError
	switch {
	case errors.As(err, &invalidErr):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "resource already exists")
	case errors.Is(err, ErrInUse):
		writeError(w, http.StatusConflict, "in_use", "resource is referenced by other records")
	case errors.Is(err, ErrNoPermission):
		writeError(w, http.StatusForbidden, "forbidden", "insufficient permissions")
	default:
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "identity storage is unavailable")
	}
}

func (h *Handler) p(r *http.Request) *PrincipalInfo {
	return h.principal(r.Context())
}

// isNodeAdmin reports whether the principal currently holds node_admin.
// It reads the live roles from the store (not from a token claim) so role
// changes take effect immediately.
func (h *Handler) isNodeAdmin(ctx context.Context, p *PrincipalInfo) (bool, error) {
	user, err := h.store.Users.Get(ctx, p.TenantID, p.UserID)
	if err != nil {
		return false, err
	}
	for _, role := range user.Roles {
		if role == RoleNodeAdmin {
			return true, nil
		}
	}
	return false, nil
}

// tenantScope resolves the effective tenant for a management operation.
// Callers stay in their own tenant unless a different tenant_id is requested
// and the caller is a node_admin.
func (h *Handler) tenantScope(r *http.Request, p *PrincipalInfo) (string, error) {
	requested := r.URL.Query().Get("tenant_id")
	if requested == "" || requested == p.TenantID {
		return p.TenantID, nil
	}
	nodeAdmin, err := h.isNodeAdmin(r.Context(), p)
	if err != nil {
		return "", err
	}
	if !nodeAdmin {
		return "", ErrNoPermission
	}
	return requested, nil
}

func (h *Handler) roleChanged(tenantID string) {
	if h.onRoleChanged != nil {
		h.onRoleChanged(tenantID)
	}
}

// --- tenants ---

func (h *Handler) CreateTenant(w http.ResponseWriter, r *http.Request) {
	var in CreateTenantInput
	if err := decodeBody(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := in.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	tenant, err := h.store.Tenants.Create(r.Context(), in)
	if err != nil {
		mapIdentityError(w, err)
		return
	}
	h.auditEntry(r, "identity.tenant.create", "tenant", tenant.ID, audit.ResultSuccess, nil)
	writeJSON(w, http.StatusCreated, tenant)
}

func (h *Handler) ListTenants(w http.ResponseWriter, r *http.Request) {
	tenants, err := h.store.Tenants.List(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "identity storage is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, tenants)
}

func (h *Handler) PatchTenant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		Status *string `json:"status"`
	}
	if err := decodeBody(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if in.Status == nil || (*in.Status != "active" && *in.Status != "disabled") {
		writeError(w, http.StatusBadRequest, "invalid_request", "status must be active or disabled")
		return
	}
	tenant, err := h.store.Tenants.SetStatus(r.Context(), id, *in.Status)
	if err != nil {
		mapIdentityError(w, err)
		return
	}
	h.auditEntry(r, "identity.tenant.update", "tenant", id, audit.ResultSuccess, map[string]string{"status": *in.Status})
	writeJSON(w, http.StatusOK, tenant)
}

// --- regions ---

func (h *Handler) CreateRegion(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ParentID string `json:"parent_id"`
		Name     string `json:"name"`
	}
	if err := decodeBody(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if in.Name == "" || len(in.Name) > 255 {
		writeError(w, http.StatusBadRequest, "invalid_request", "name must be non-empty and at most 255 bytes")
		return
	}
	region, err := h.store.Regions.Create(r.Context(), in.ParentID, in.Name)
	if err != nil {
		mapIdentityError(w, err)
		return
	}
	h.auditEntry(r, "identity.region.create", "region", region.ID, audit.ResultSuccess, map[string]string{"name": in.Name})
	writeJSON(w, http.StatusCreated, region)
}

func (h *Handler) ListRegions(w http.ResponseWriter, r *http.Request) {
	tree, err := h.store.Regions.Tree(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "identity storage is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, tree)
}

func (h *Handler) PatchRegion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		Name *string `json:"name"`
	}
	if err := decodeBody(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if in.Name == nil || *in.Name == "" || len(*in.Name) > 255 {
		writeError(w, http.StatusBadRequest, "invalid_request", "name must be non-empty and at most 255 bytes")
		return
	}
	region, err := h.store.Regions.UpdateName(r.Context(), id, *in.Name)
	if err != nil {
		mapIdentityError(w, err)
		return
	}
	h.auditEntry(r, "identity.region.update", "region", id, audit.ResultSuccess, map[string]string{"name": *in.Name})
	writeJSON(w, http.StatusOK, region)
}

func (h *Handler) DeleteRegion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.Regions.Delete(r.Context(), id); err != nil {
		mapIdentityError(w, err)
		return
	}
	h.auditEntry(r, "identity.region.delete", "region", id, audit.ResultSuccess, nil)
	w.WriteHeader(http.StatusNoContent)
}

// --- users ---

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	p := h.p(r)
	if p == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing principal")
		return
	}
	var in CreateUserInput
	if err := decodeBody(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	// A non-node_admin may only create users in their own tenant; node_admin
	// may specify a different tenant_id in the request body.
	if in.TenantID != "" && in.TenantID != p.TenantID {
		nodeAdmin, err := h.isNodeAdmin(r.Context(), p)
		if err != nil {
			mapIdentityError(w, err)
			return
		}
		if !nodeAdmin {
			writeError(w, http.StatusForbidden, "forbidden", "insufficient permissions")
			return
		}
	} else {
		in.TenantID = p.TenantID
	}
	if err := in.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	passwordHash, err := h.hasher.Hash(in.Password)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "password hashing failed")
		return
	}
	user, err := h.store.Users.Create(r.Context(), in, passwordHash)
	if err != nil {
		mapIdentityError(w, err)
		return
	}
	h.roleChanged(in.TenantID)
	h.auditEntry(r, "identity.user.create", "user", user.ID, audit.ResultSuccess, map[string]string{"username": user.Username})
	writeJSON(w, http.StatusCreated, user)
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	p := h.p(r)
	if p == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing principal")
		return
	}
	tenantID, err := h.tenantScope(r, p)
	if err != nil {
		mapIdentityError(w, err)
		return
	}
	users, err := h.store.Users.List(r.Context(), tenantID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "identity storage is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	p := h.p(r)
	if p == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing principal")
		return
	}
	tenantID, err := h.tenantScope(r, p)
	if err != nil {
		mapIdentityError(w, err)
		return
	}
	user, err := h.store.Users.Get(r.Context(), tenantID, r.PathValue("id"))
	if err != nil {
		mapIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) PatchUser(w http.ResponseWriter, r *http.Request) {
	p := h.p(r)
	if p == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing principal")
		return
	}
	id := r.PathValue("id")
	tenantID, err := h.tenantScope(r, p)
	if err != nil {
		mapIdentityError(w, err)
		return
	}
	var in UpdateUserInput
	if err := decodeBody(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if in.DisplayName != nil && len(*in.DisplayName) > 255 {
		writeError(w, http.StatusBadRequest, "invalid_request", "display_name must be at most 255 bytes")
		return
	}
	if in.Status != nil && *in.Status != UserStatusActive && *in.Status != UserStatusDisabled {
		writeError(w, http.StatusBadRequest, "invalid_request", "status must be active or disabled")
		return
	}
	for _, role := range in.Roles {
		if !ValidRole(role) {
			writeError(w, http.StatusBadRequest, "invalid_request", "unknown role: "+role)
			return
		}
	}
	user, err := h.store.Users.Update(r.Context(), tenantID, id, in)
	if err != nil {
		mapIdentityError(w, err)
		return
	}
	if in.Roles != nil {
		h.roleChanged(tenantID)
	}
	h.auditEntry(r, "identity.user.update", "user", id, audit.ResultSuccess, map[string]string{"roles": stringsJoin(in.Roles)})
	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	p := h.p(r)
	if p == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing principal")
		return
	}
	id := r.PathValue("id")
	if id == p.UserID {
		writeError(w, http.StatusBadRequest, "invalid_request", "cannot delete your own account")
		return
	}
	tenantID, err := h.tenantScope(r, p)
	if err != nil {
		mapIdentityError(w, err)
		return
	}
	if err := h.store.Users.Delete(r.Context(), tenantID, id); err != nil {
		mapIdentityError(w, err)
		return
	}
	h.roleChanged(tenantID)
	h.auditEntry(r, "identity.user.delete", "user", id, audit.ResultSuccess, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) SetUserPassword(w http.ResponseWriter, r *http.Request) {
	p := h.p(r)
	if p == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing principal")
		return
	}
	id := r.PathValue("id")
	tenantID, err := h.tenantScope(r, p)
	if err != nil {
		mapIdentityError(w, err)
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if err := decodeBody(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if in.Password == "" || len(in.Password) > 256 {
		writeError(w, http.StatusBadRequest, "invalid_request", "password must be between 1 and 256 bytes")
		return
	}
	passwordHash, err := h.hasher.Hash(in.Password)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "password hashing failed")
		return
	}
	if err := h.store.Users.SetPassword(r.Context(), tenantID, id, passwordHash); err != nil {
		mapIdentityError(w, err)
		return
	}
	h.auditEntry(r, "identity.user.set_password", "user", id, audit.ResultSuccess, nil)
	w.WriteHeader(http.StatusNoContent)
}

// --- roles ---

func (h *Handler) ListRoles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"roles": AllRoles(),
	})
}

func (h *Handler) auditEntry(r *http.Request, action, resourceType, resourceID, result string, detail any) {
	p := h.p(r)
	var actorID, tenantID *string
	if p != nil {
		actorID = &p.UserID
		tenantID = &p.TenantID
	}
	h.audit.Record(r.Context(), audit.Entry{
		ActorUserID:  actorID,
		TenantID:     tenantID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Result:       result,
		IPAddr:       remoteIP(r),
		Detail:       detail,
	})
}

func remoteIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(host)
}

func stringsJoin(items []string) string {
	out := ""
	for i, item := range items {
		if i > 0 {
			out += ","
		}
		out += item
	}
	return out
}