package device

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
		t.Fatalf("HA1 = %s", got)
	}
}

func TestCreateDeviceValidation(t *testing.T) {
	valid := CreateDeviceInput{
		RegionID: "00000000-0000-0000-0000-000000000002",
		CenterCode: "34020000", DeviceType: DeviceTypeIPC,
		DeviceName: "东门摄像机", Manufacturer: "海康威视",
		SIPRealm: "3402000000", Password: "secret", Enabled: true,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	for name, input := range map[string]CreateDeviceInput{
		"missing region":     validWith(valid, func(v *CreateDeviceInput) { v.RegionID = "" }),
		"bad region":         validWith(valid, func(v *CreateDeviceInput) { v.RegionID = "not-a-uuid" }),
		"short center code":  validWith(valid, func(v *CreateDeviceInput) { v.CenterCode = "3402000" }),
		"non-digit center":   validWith(valid, func(v *CreateDeviceInput) { v.CenterCode = "3402000a" }),
		"unknown device type": validWith(valid, func(v *CreateDeviceInput) { v.DeviceType = "999" }),
		"empty name":         validWith(valid, func(v *CreateDeviceInput) { v.DeviceName = "" }),
		"empty manufacturer": validWith(valid, func(v *CreateDeviceInput) { v.Manufacturer = "" }),
		"empty realm":        validWith(valid, func(v *CreateDeviceInput) { v.SIPRealm = "" }),
		"control in realm":   validWith(valid, func(v *CreateDeviceInput) { v.SIPRealm = "realm\x00suffix" }),
		"empty password":     validWith(valid, func(v *CreateDeviceInput) { v.Password = "" }),
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

type endpointStub struct {
	device Device
	protectedIDs []string
}

func (s *endpointStub) Create(context.Context, CreateDeviceInput) (Device, error) {
	return s.device, nil
}
func (s *endpointStub) Get(context.Context, string) (Device, error) { return s.device, nil }
func (s *endpointStub) SetEnabled(context.Context, string, bool) (Device, error) {
	return s.device, nil
}
func (s *endpointStub) UpdateMeta(context.Context, string, *string, *string) (Device, error) {
	return s.device, nil
}
func (s *endpointStub) List(context.Context, string, []string) ([]Device, error) {
	return []Device{s.device}, nil
}
func (s *endpointStub) Delete(context.Context, string) error { return nil }
func (s *endpointStub) EnsureVisible(context.Context, string, []string, string) (Device, error) {
	return s.device, nil
}

func TestDeviceAPIDoesNotLeakCredentials(t *testing.T) {
	stub := &endpointStub{
		device: Device{
			ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			TenantID: "00000000-0000-0000-0000-000000000001",
			RegionID: "00000000-0000-0000-0000-000000000002",
			DeviceAccessID: "34020000001320000001",
			DeviceName: "东门摄像机", Manufacturer: "海康威视",
			DeviceType: DeviceTypeIPC, SIPUsername: "34020000001320000001",
			SIPRealm: "3402000000", DigestAlgorithm: "MD5",
			Enabled: true, ProfileVersion: 1, AccessSyncStatus: "pending",
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		},
	}
	mux := http.NewServeMux()
	guard := func(obj, act string, h http.HandlerFunc) http.HandlerFunc { return h }
	RegisterRoutes(mux, stub, guard, nil)
	// The region-visibility check reads the scope injected by the wiring; the
	// stub request must carry the same scope so creation is allowed.
	scoped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := WithScope(r.Context(), "00000000-0000-0000-0000-000000000001", []string{"00000000-0000-0000-0000-000000000002"})
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
	server := httptest.NewServer(scoped)
	defer server.Close()

	body := `{"region_id":"00000000-0000-0000-0000-000000000002","center_code":"34020000","device_type":"132","device_name":"东门摄像机","manufacturer":"海康威视","sip_realm":"3402000000","password":"super-secret","enabled":true}`
	response, err := http.Post(server.URL+"/api/v1/devices", "application/json", strings.NewReader(body))
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