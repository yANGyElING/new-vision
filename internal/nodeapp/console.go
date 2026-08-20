package nodeapp

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

// ConsoleDeps wires the public test console surface: device management
// (registered separately as /api/v1/devices), read-only Access runtime
// inspection, and the test-only SIP simulator.
type ConsoleDeps struct {
	Devices DeviceEndpoints
	Access  AccessAPI
	SIP     *SIPSimulator
}

func registerConsoleRoutes(mux *http.ServeMux, deps ConsoleDeps) {
	if deps.Devices != nil {
		registerPublicDeviceRoutes(mux, deps.Devices)
	}
	if deps.Access != nil {
		registerAccessConsoleRoutes(mux, deps.Access)
	}
	if deps.SIP != nil {
		registerSIPTestRoutes(mux, deps.SIP)
	}
}

func registerAccessConsoleRoutes(mux *http.ServeMux, access AccessAPI) {
	mux.HandleFunc("GET /api/v1/access/snapshot", func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := access.GetRuntimeSnapshot(r.Context())
		if err != nil {
			writeAccessConsoleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
	})
	mux.HandleFunc("GET /api/v1/access/events", func(w http.ResponseWriter, r *http.Request) {
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
		result, err := access.PollEvents(r.Context(), after, limit)
		if err != nil {
			writeAccessConsoleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("POST /api/v1/access/ack", func(w http.ResponseWriter, r *http.Request) {
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
		if err := access.AckEvents(r.Context(), request.ThroughSequence); err != nil {
			writeAccessConsoleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "acked"})
	})
}

func registerSIPTestRoutes(mux *http.ServeMux, sim *SIPSimulator) {
	mux.HandleFunc("POST /api/v1/test/sip/register", func(w http.ResponseWriter, r *http.Request) {
		accessID, ok := readAccessIDBody(w, r)
		if !ok {
			return
		}
		result, err := sim.Register(r.Context(), accessID, 3600)
		writeSIPTestResult(w, result, err)
	})
	mux.HandleFunc("POST /api/v1/test/sip/keepalive", func(w http.ResponseWriter, r *http.Request) {
		accessID, ok := readAccessIDBody(w, r)
		if !ok {
			return
		}
		result, err := sim.KeepAlive(r.Context(), accessID)
		writeSIPTestResult(w, result, err)
	})
	mux.HandleFunc("POST /api/v1/test/sip/unregister", func(w http.ResponseWriter, r *http.Request) {
		accessID, ok := readAccessIDBody(w, r)
		if !ok {
			return
		}
		result, err := sim.Unregister(r.Context(), accessID)
		writeSIPTestResult(w, result, err)
	})
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

func writeSIPTestResult(w http.ResponseWriter, result SIPTestResult, err error) {
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalid):
			writeAPIError(w, http.StatusBadRequest, "invalid_device", err.Error())
		case errors.Is(err, ErrNotFound):
			writeAPIError(w, http.StatusNotFound, "device_not_found", "device not found")
		default:
			writeAPIError(w, http.StatusServiceUnavailable, "sip_test_failed", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeAccessConsoleError(w http.ResponseWriter, err error) {
	var rpcErr *RPCError
	if errors.As(err, &rpcErr) {
		writeAPIError(w, http.StatusBadGateway, "access_rpc_error", rpcErr.Error())
		return
	}
	writeAPIError(w, http.StatusServiceUnavailable, "access_unavailable", safeError(err))
}

func parseNonNegativeIntQuery(r *http.Request, name string, defaultValue int64) (int64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
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
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}
