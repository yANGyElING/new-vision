package authn

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"

	"github.com/new-vision-lab/new-vision/internal/audit"
	"github.com/new-vision-lab/new-vision/internal/identity"
)

type APIError struct {
	Status int
	Code   string
	Msg    string
}

func (e *APIError) Error() string { return e.Msg }

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// Handler exposes the login and me endpoints.
type Handler struct {
	tenants identity.TenantRepository
	users   identity.UserRepository
	tokens  *TokenManager
	audit   *audit.Writer
}

func NewHandler(tenants identity.TenantRepository, users identity.UserRepository, tokens *TokenManager, audit *audit.Writer) *Handler {
	return &Handler{tenants: tenants, users: users, tokens: tokens, audit: audit}
}

type loginRequest struct {
	Tenant   string `json:"tenant"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string       `json:"token"`
	ExpiresAt string       `json:"expires_at"`
	User      identity.User `json:"user"`
	Roles     []string     `json:"roles"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	tenant, err := h.tenants.GetByName(r.Context(), req.Tenant)
	if errors.Is(err, identity.ErrNotFound) {
		writeAPIError(w, http.StatusUnauthorized, "unauthorized", "invalid tenant or credentials")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "identity storage is unavailable")
		return
	}
	if tenant.Status != "active" {
		writeAPIError(w, http.StatusUnauthorized, "unauthorized", "tenant is disabled")
		return
	}
	user, err := h.users.GetByUsername(r.Context(), tenant.ID, req.Username)
	if errors.Is(err, identity.ErrNotFound) {
		h.auditLogin(r, tenant.ID, &req.Username, "invalid credentials")
		writeAPIError(w, http.StatusUnauthorized, "unauthorized", "invalid tenant or credentials")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "identity storage is unavailable")
		return
	}
	if user.Status != "active" {
		h.auditLogin(r, tenant.ID, &req.Username, "user disabled")
		writeAPIError(w, http.StatusUnauthorized, "unauthorized", "user is disabled")
		return
	}
	if !h.tokens.Hasher().Verify(user.PasswordHash, req.Password) {
		h.auditLogin(r, tenant.ID, &req.Username, "invalid credentials")
		writeAPIError(w, http.StatusUnauthorized, "unauthorized", "invalid tenant or credentials")
		return
	}
	token, expiresAt, err := h.tokens.Issue(user)
	if err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "token issuance failed")
		return
	}
	h.auditLogin(r, tenant.ID, &req.Username, "success")
	writeJSON(w, http.StatusOK, loginResponse{
		Token:     token,
		ExpiresAt: expiresAt.Format("2006-01-02T15:04:05Z07:00"),
		User:      user,
		Roles:     user.Roles,
	})
}

func (h *Handler) auditLogin(r *http.Request, tenantID string, username *string, outcome string) {
	result := audit.ResultSuccess
	if outcome != "success" {
		result = audit.ResultDenied
	}
	h.audit.Record(r.Context(), audit.Entry{
		ActorUserID:  nil,
		TenantID:     &tenantID,
		Action:       "auth.login",
		ResourceType: "user",
		ResourceID:   usernameOrEmpty(username),
		Result:       result,
		IPAddr:       remoteIP(r),
		Detail:       map[string]string{"username": usernameOrEmpty(username), "outcome": outcome},
	})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	if p == nil {
		writeAPIError(w, http.StatusUnauthorized, "unauthorized", "missing principal")
		return
	}
	user, err := h.users.Get(r.Context(), p.TenantID, p.UserID)
	if errors.Is(err, identity.ErrNotFound) {
		writeAPIError(w, http.StatusUnauthorized, "unauthorized", "user no longer exists")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "identity storage is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":         user,
		"roles":        user.Roles,
		"region_scopes": user.RegionIDs,
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
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

func usernameOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func remoteIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(host)
}
