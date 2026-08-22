package device

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const maxDeviceRequestBytes = 64 << 10

// DeviceEndpoints is the public device management surface.
type DeviceEndpoints interface {
	Create(context.Context, CreateDeviceInput) (Device, error)
	Get(context.Context, string) (Device, error)
	SetEnabled(context.Context, string, bool) (Device, error)
	UpdateMeta(context.Context, string, *string, *string) (Device, error)
	List(context.Context, string, []string) ([]Device, error)
	Delete(context.Context, string) error
	EnsureVisible(context.Context, string, []string, string) (Device, error)
}

// AuditRecorder reports a successful device operation for the audit log.
// The nodeapp wiring supplies the implementation (audit.Writer + principal).
type AuditRecorder func(ctx context.Context, action, resourceID string, detail map[string]any)

// RegisterRoutes mounts /api/v1/devices. The guard function (provided by the
// nodeapp wiring) enforces the permission point mapping for each route; the
// audit recorder logs successful device operations.
func RegisterRoutes(mux *http.ServeMux, service DeviceEndpoints, guard func(obj, act string, h http.HandlerFunc) http.HandlerFunc, audit AuditRecorder) {
	mux.HandleFunc("GET /api/v1/devices", guard("device", "view", func(w http.ResponseWriter, r *http.Request) {
		devices, err := service.List(r.Context(), tenantID(r), regionIDs(r))
		if err != nil {
			writeDeviceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, devices)
	}))
	mux.HandleFunc("POST /api/v1/devices", guard("device", "create", func(w http.ResponseWriter, r *http.Request) {
		request, ok := decodeCreateDeviceBody(w, r)
		if !ok {
			return
		}
		// The device must be created inside a region the caller can see;
		// otherwise it would become an invisible orphan in the list.
		if !regionAllowed(regionIDs(r), request.RegionID) {
			writeAPIError(w, http.StatusForbidden, "forbidden", "region is outside your scope")
			return
		}
		request.TenantID = tenantID(r)
		device, err := service.Create(r.Context(), request)
		if err != nil {
			writeDeviceError(w, err)
			return
		}
		if audit != nil {
			audit(r.Context(), "device.create", device.ID, map[string]any{"device_access_id": device.DeviceAccessID})
		}
		writeJSON(w, http.StatusCreated, device)
	}))
	mux.HandleFunc("GET /api/v1/devices/{id}", guard("device", "view", func(w http.ResponseWriter, r *http.Request) {
		device, err := service.EnsureVisible(r.Context(), tenantID(r), regionIDs(r), r.PathValue("id"))
		if err != nil {
			writeDeviceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, device)
	}))
	mux.HandleFunc("PATCH /api/v1/devices/{id}", func(w http.ResponseWriter, r *http.Request) {
		// Decode once, then dispatch to the correct permission point: enabling
		// requires device:enable (operator), metadata edits require
		// device:update (tenant_admin and above).
		request := struct {
			Enabled      *bool   `json:"enabled"`
			DeviceName   *string `json:"device_name"`
			Manufacturer *string `json:"manufacturer"`
		}{}
		if err := decodeJSONBody(w, r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		if request.Enabled == nil && request.DeviceName == nil && request.Manufacturer == nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "at least one of enabled, device_name, manufacturer is required")
			return
		}
		id := r.PathValue("id")
		if request.Enabled != nil {
			guard("device", "enable", func(w http.ResponseWriter, r *http.Request) {
				if _, err := service.EnsureVisible(r.Context(), tenantID(r), regionIDs(r), id); err != nil {
					writeDeviceError(w, err)
					return
				}
				device, err := service.SetEnabled(r.Context(), id, *request.Enabled)
				if err != nil {
					writeDeviceError(w, err)
					return
				}
				if audit != nil {
					audit(r.Context(), "device.enable", id, map[string]any{"enabled": *request.Enabled})
				}
				writeJSON(w, http.StatusOK, device)
			})(w, r)
			return
		}
		guard("device", "update", func(w http.ResponseWriter, r *http.Request) {
			if _, err := service.EnsureVisible(r.Context(), tenantID(r), regionIDs(r), id); err != nil {
				writeDeviceError(w, err)
				return
			}
			if request.DeviceName != nil && *request.DeviceName == "" {
				writeAPIError(w, http.StatusBadRequest, "invalid_request", "device_name must be non-empty")
				return
			}
			if request.Manufacturer != nil && *request.Manufacturer == "" {
				writeAPIError(w, http.StatusBadRequest, "invalid_request", "manufacturer must be non-empty")
				return
			}
			device, err := service.UpdateMeta(r.Context(), id, request.DeviceName, request.Manufacturer)
			if err != nil {
				writeDeviceError(w, err)
				return
			}
			if audit != nil {
				audit(r.Context(), "device.update", id, nil)
			}
			writeJSON(w, http.StatusOK, device)
		})(w, r)
	})
	mux.HandleFunc("DELETE /api/v1/devices/{id}", guard("device", "delete", func(w http.ResponseWriter, r *http.Request) {
		if _, err := service.EnsureVisible(r.Context(), tenantID(r), regionIDs(r), r.PathValue("id")); err != nil {
			writeDeviceError(w, err)
			return
		}
		if err := service.Delete(r.Context(), r.PathValue("id")); err != nil {
			writeDeviceError(w, err)
			return
		}
		if audit != nil {
			audit(r.Context(), "device.delete", r.PathValue("id"), nil)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

// regionAllowed reports whether regionID is inside the allowed region set
// (the caller's visible region subtree, possibly empty). An empty set denies
// everything.
func regionAllowed(allowed []string, regionID string) bool {
	for _, id := range allowed {
		if id == regionID {
			return true
		}
	}
	return false
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxDeviceRequestBytes)
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

func decodeCreateDeviceBody(w http.ResponseWriter, r *http.Request) (CreateDeviceInput, bool) {
	var request struct {
		RegionID     string `json:"region_id"`
		CenterCode   string `json:"center_code"`
		DeviceType   string `json:"device_type"`
		DeviceName   string `json:"device_name"`
		Manufacturer string `json:"manufacturer"`
		SIPRealm     string `json:"sip_realm"`
		Password     string `json:"password"`
		Enabled      *bool  `json:"enabled"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return CreateDeviceInput{}, false
	}
	if request.RegionID == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "region_id is required")
		return CreateDeviceInput{}, false
	}
	if request.Enabled == nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "enabled is required")
		return CreateDeviceInput{}, false
	}
	return CreateDeviceInput{
		RegionID:     request.RegionID,
		CenterCode:   request.CenterCode,
		DeviceType:   request.DeviceType,
		DeviceName:   request.DeviceName,
		Manufacturer: request.Manufacturer,
		SIPRealm:     request.SIPRealm,
		Password:     request.Password,
		Enabled:      *request.Enabled,
	}, true
}

func writeDeviceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		writeAPIError(w, http.StatusBadRequest, "invalid_device", err.Error())
	case errors.Is(err, ErrConflict):
		writeAPIError(w, http.StatusConflict, "device_exists", "device_access_id already exists")
	case errors.Is(err, ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "device_not_found", "device not found")
	case errors.Is(err, ErrNoAccess):
		writeAPIError(w, http.StatusForbidden, "forbidden", "device is outside your scope")
	default:
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "device storage is unavailable")
	}
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// tenantID and regionIDs read the caller scope from the request context.
// The nodeapp wiring attaches them via WithScope after authentication.
func tenantID(r *http.Request) string {
	ctx := r.Context()
	if v, ok := ctx.Value(tenantKey{}).(string); ok {
		return v
	}
	return ""
}

func regionIDs(r *http.Request) []string {
	ctx := r.Context()
	if v, ok := ctx.Value(regionKey{}).([]string); ok {
		return v
	}
	return nil
}

// WithScope attaches the caller tenant and expanded visible region ids to
// the context so device handlers can filter by data scope.
func WithScope(ctx context.Context, tenantID string, regionIDs []string) context.Context {
	ctx = context.WithValue(ctx, tenantKey{}, tenantID)
	ctx = context.WithValue(ctx, regionKey{}, regionIDs)
	return ctx
}

type tenantKey struct{}
type regionKey struct{}
