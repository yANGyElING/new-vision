package channel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	"github.com/new-vision-lab/new-vision/internal/nodeapp/device"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Endpoints is the device-facing surface the channel routes need: device
// visibility checks (management page) and channel listing.
type Endpoints interface {
	EnsureVisible(ctx context.Context, tenantID string, orgUnitIDs []string, includeUnassigned bool, id string) (device.Device, error)
}

// Service wires the channel repository, the device lookup and the runtime
// projection (device online state for the user page).
type Service struct {
	repository Repository
	devices    Endpoints
	runtime    device.RuntimeReader
	orgs       OrgTreeResolver
}

func NewService(repository Repository, devices Endpoints, runtime device.RuntimeReader, orgs OrgTreeResolver) *Service {
	return &Service{repository: repository, devices: devices, runtime: runtime, orgs: orgs}
}

// DeviceChannels serves GET /api/v1/devices/{id}/channels (management page:
// full channel columns plus the device catalog state).
func (s *Service) DeviceChannels(ctx context.Context, tenantID string, orgUnitIDs []string, includeUnassigned bool, deviceID string) (CatalogDeviceResult, error) {
	d, err := s.devices.EnsureVisible(ctx, tenantID, orgUnitIDs, includeUnassigned, deviceID)
	if err != nil {
		return CatalogDeviceResult{}, err
	}
	channels, err := s.repository.ListByDevice(ctx, d.ID)
	if err != nil {
		return CatalogDeviceResult{}, err
	}
	return CatalogDeviceResult{
		DeviceID:         d.ID,
		CatalogState:     d.CatalogState,
		CatalogLastOkAt:  d.CatalogLastOkAt,
		CatalogLastCount: d.CatalogLastCount,
		CatalogLastError: d.CatalogLastError,
		Channels:         channels,
	}, nil
}

// ListVisible serves GET /api/v1/channels (user page: flat channel list
// filtered by the caller's data scope; filterOrgUnitID optionally narrows to
// one org subtree).
func (s *Service) ListVisible(ctx context.Context, tenantID string, orgUnitIDs []string, includeUnassigned bool, filterOrgUnitID string) (CatalogListResult, error) {
	var filterIDs []string
	if filterOrgUnitID != "" {
		if s.orgs == nil {
			return CatalogListResult{}, errors.New("org resolver unavailable")
		}
		subtree, err := s.orgs.SubtreeIDs(ctx, []string{filterOrgUnitID})
		if err != nil {
			return CatalogListResult{}, err
		}
		filterIDs = subtree
	}
	result, err := s.repository.ListVisible(ctx, tenantID, orgUnitIDs, includeUnassigned, filterIDs)
	if err != nil {
		return CatalogListResult{}, err
	}
	// Attach device runtime state (D44/D49: an offline device greys out its
	// whole channel group on the user page — presentation-level only).
	if s.runtime != nil && len(result.Devices) > 0 {
		ids := make([]string, len(result.Devices))
		for i := range result.Devices {
			ids[i] = result.Devices[i].ID
		}
		states, err := s.runtime.GetMany(ctx, ids)
		if err != nil {
			return CatalogListResult{}, err
		}
		for i := range result.Devices {
			if state, ok := states[result.Devices[i].ID]; ok && state != nil && state.State == "online" {
				result.Devices[i].State = "online"
			} else {
				result.Devices[i].State = "offline"
			}
		}
	}
	return result, nil
}

// RegisterRoutes mounts the read-only channel surface. The guard enforces
// the channel:view permission point; the scope wrapper (nodeapp wiring)
// attaches tenant + visible org units to the request context.
func RegisterRoutes(mux *http.ServeMux, service *Service, guard func(obj, act string, h http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/v1/devices/{id}/channels", guard("channel", "view", func(w http.ResponseWriter, r *http.Request) {
		result, err := service.DeviceChannels(r.Context(), tenantID(r), orgUnitIDs(r), includeUnassigned(r), r.PathValue("id"))
		if err != nil {
			writeChannelError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}))
	mux.HandleFunc("GET /api/v1/channels", guard("channel", "view", func(w http.ResponseWriter, r *http.Request) {
		orgFilter := r.URL.Query().Get("org_unit_id")
		if orgFilter != "" && !uuidPattern.MatchString(orgFilter) {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "org_unit_id must be a valid UUID")
			return
		}
		result, err := service.ListVisible(r.Context(), tenantID(r), orgUnitIDs(r), includeUnassigned(r), orgFilter)
		if err != nil {
			writeChannelError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}))
}

func tenantID(r *http.Request) string {
	return device.TenantIDFrom(r.Context())
}

func orgUnitIDs(r *http.Request) []string {
	return device.OrgUnitIDsFrom(r.Context())
}

func includeUnassigned(r *http.Request) bool {
	return device.IncludeUnassignedFrom(r.Context())
}

func writeChannelError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, device.ErrInvalid):
		writeAPIError(w, http.StatusBadRequest, "invalid_channel", err.Error())
	case errors.Is(err, device.ErrNoAccess):
		writeAPIError(w, http.StatusForbidden, "forbidden", "device is outside your scope")
	case errors.Is(err, device.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "device_not_found", "device not found")
	default:
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "channel storage is unavailable")
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
