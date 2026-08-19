package nodeapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const maxDeviceRequestBytes = 64 << 10

type RuntimeReader interface {
	Get(context.Context, string) (*RuntimeState, error)
}

type DeviceManager struct {
	repository DeviceRepository
	runtime    RuntimeReader
}

func NewDeviceManager(repository DeviceRepository, runtime RuntimeReader) *DeviceManager {
	return &DeviceManager{repository: repository, runtime: runtime}
}

func (m *DeviceManager) Create(ctx context.Context, input CreateDeviceInput) (Device, error) {
	if err := input.Validate(); err != nil {
		return Device{}, err
	}
	return m.repository.Create(ctx, input, DeriveHA1(input.SIPUsername, input.SIPRealm, input.Password))
}

func (m *DeviceManager) Get(ctx context.Context, id string) (Device, error) {
	device, err := m.repository.Get(ctx, id)
	if err != nil {
		return Device{}, err
	}
	if m.runtime != nil {
		device.Runtime, err = m.runtime.Get(ctx, device.ID)
		if err != nil {
			return Device{}, err
		}
	}
	return device, nil
}

func (m *DeviceManager) SetEnabled(ctx context.Context, id string, enabled bool) (Device, error) {
	return m.repository.SetEnabled(ctx, id, enabled)
}

type DeviceEndpoints interface {
	Create(context.Context, CreateDeviceInput) (Device, error)
	Get(context.Context, string) (Device, error)
	SetEnabled(context.Context, string, bool) (Device, error)
}

func registerDeviceRoutes(mux *http.ServeMux, service DeviceEndpoints) {
	mux.HandleFunc("POST /internal/v1/devices", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			DeviceAccessID string `json:"device_access_id"`
			SIPUsername    string `json:"sip_username"`
			SIPRealm       string `json:"sip_realm"`
			Password       string `json:"password"`
			Enabled        *bool  `json:"enabled"`
		}
		if err := decodeJSONBody(w, r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		if request.Enabled == nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "enabled is required")
			return
		}
		device, err := service.Create(r.Context(), CreateDeviceInput{
			DeviceAccessID: request.DeviceAccessID, SIPUsername: request.SIPUsername,
			SIPRealm: request.SIPRealm, Password: request.Password, Enabled: *request.Enabled,
		})
		if err != nil {
			writeDeviceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, device)
	})
	mux.HandleFunc("GET /internal/v1/devices/{id}", func(w http.ResponseWriter, r *http.Request) {
		device, err := service.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeDeviceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, device)
	})
	mux.HandleFunc("PATCH /internal/v1/devices/{id}", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Enabled *bool `json:"enabled"`
		}
		if err := decodeJSONBody(w, r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		if request.Enabled == nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "enabled is required")
			return
		}
		device, err := service.SetEnabled(r.Context(), r.PathValue("id"), *request.Enabled)
		if err != nil {
			writeDeviceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, device)
	})
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

func writeDeviceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		writeAPIError(w, http.StatusBadRequest, "invalid_device", err.Error())
	case errors.Is(err, ErrConflict):
		writeAPIError(w, http.StatusConflict, "device_exists", "device_access_id already exists")
	case errors.Is(err, ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "device_not_found", "device not found")
	default:
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "device storage is unavailable")
	}
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func validateEvent(event AccessEvent) error {
	if event.Sequence <= 0 || event.EventID == "" || event.AccessInstanceID == "" || event.SessionEpoch == "" || !accessIDPattern.MatchString(event.DeviceAccessID) {
		return fmt.Errorf("invalid access event envelope")
	}
	if event.Type != "registration_changed" || (event.Payload.State != "online" && event.Payload.State != "offline") {
		return fmt.Errorf("unsupported access event")
	}
	return nil
}
