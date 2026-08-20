package nodeapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDeriveHA1(t *testing.T) {
	if got := DeriveHA1("alice", "example", "secret"); got != "a110383056f556b818bd7026fed7451b" {
		// The expected value is calculated independently from the implementation contract.
		t.Fatalf("HA1 = %s", got)
	}
}

func TestCreateDeviceValidation(t *testing.T) {
	valid := CreateDeviceInput{CenterCode: "34020000", DeviceType: DeviceTypeIPC, DeviceName: "东门摄像机", Manufacturer: "海康威视", SIPRealm: "3402000000", Password: "secret", Enabled: true}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	for name, input := range map[string]CreateDeviceInput{
		"short center code":   validWith(valid, func(v *CreateDeviceInput) { v.CenterCode = "3402000" }),
		"non-digit center":    validWith(valid, func(v *CreateDeviceInput) { v.CenterCode = "3402000a" }),
		"unknown device type": validWith(valid, func(v *CreateDeviceInput) { v.DeviceType = "999" }),
		"empty name":          validWith(valid, func(v *CreateDeviceInput) { v.DeviceName = "" }),
		"empty manufacturer":  validWith(valid, func(v *CreateDeviceInput) { v.Manufacturer = "" }),
		"empty realm":         validWith(valid, func(v *CreateDeviceInput) { v.SIPRealm = "" }),
		"control in realm":    validWith(valid, func(v *CreateDeviceInput) { v.SIPRealm = "realm\x00suffix" }),
		"empty password":      validWith(valid, func(v *CreateDeviceInput) { v.Password = "" }),
	} {
		if err := input.Validate(); err == nil {
			t.Errorf("%s accepted invalid input", name)
		}
	}
	if got := valid.accessIDPrefix(); got != "34020000001320" {
		t.Errorf("accessIDPrefix = %s, want 34020000001320", got)
	}
}

func validWith(input CreateDeviceInput, change func(*CreateDeviceInput)) CreateDeviceInput {
	change(&input)
	return input
}

type endpointStub struct{ device Device }

func (s *endpointStub) Create(context.Context, CreateDeviceInput) (Device, error) {
	return s.device, nil
}
func (s *endpointStub) Get(context.Context, string) (Device, error) { return s.device, nil }
func (s *endpointStub) SetEnabled(context.Context, string, bool) (Device, error) {
	return s.device, nil
}
func (s *endpointStub) List(context.Context) ([]Device, error) { return []Device{s.device}, nil }
func (s *endpointStub) Delete(context.Context, string) error   { return nil }

func TestAccessClientDecodesOfflineEventWithoutExpiresAt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"` + request.ID + `","result":{"access_instance_id":"access-01","session_epoch":"11111111-1111-4111-8111-111111111111","latest_sequence":1,"events":[{"event_id":"access-01:1","sequence":1,"access_instance_id":"access-01","session_epoch":"11111111-1111-4111-8111-111111111111","type":"registration_changed","occurred_at":"2026-08-18T15:00:00Z","device_access_id":"34020000001320000001","payload":{"state":"offline","reason":"unregister","remote_address":"192.0.2.1:5060","last_seen":"2026-08-18T15:00:00Z"}}]}}`))
	}))
	defer server.Close()

	result, err := NewAccessClient(server.URL, time.Second).PollEvents(context.Background(), 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 1 || !result.Events[0].Payload.ExpiresAt.IsZero() {
		t.Fatalf("offline event did not decode as an absent optional expiry: %#v", result.Events)
	}
}

func TestDeviceAPIDoesNotLeakCredentials(t *testing.T) {
	stub := &endpointStub{device: Device{ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", DeviceAccessID: "34020000001320000001", DeviceName: "东门摄像机", Manufacturer: "海康威视", DeviceType: DeviceTypeIPC, SIPUsername: "34020000001320000001", SIPRealm: "3402000000", DigestAlgorithm: "MD5", Enabled: true, ProfileVersion: 1, AccessSyncStatus: "pending", CreatedAt: time.Now(), UpdatedAt: time.Now()}}
	server := httptest.NewServer(NewHandler(func(context.Context) error { return nil }, func(context.Context) error { return nil }, time.Second, http.NotFoundHandler(), stub))
	defer server.Close()
	body := `{"center_code":"34020000","device_type":"132","device_name":"东门摄像机","manufacturer":"海康威视","sip_realm":"3402000000","password":"super-secret","enabled":true}`
	response, err := http.Post(server.URL+"/internal/v1/devices", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var data map[string]any
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(data)
	if strings.Contains(string(encoded), "super-secret") || strings.Contains(string(encoded), "digest_ha1") {
		t.Fatalf("response leaked credential material: %s", encoded)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestAccessRPCClientApply(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.JSONRPC != "2.0" || request.Method != "access.v1.applyDeviceProfile" || request.ID == "" {
			t.Fatalf("request = %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"status": "applied", "version": 3}})
	}))
	defer server.Close()
	client := NewAccessClient(server.URL, time.Second)
	result, err := client.ApplyDeviceProfile(context.Background(), AccessProfile{DeviceAccessID: "34020000001320000001", Version: 3})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "applied" || result.Version != 3 {
		t.Fatalf("result = %+v", result)
	}
}
