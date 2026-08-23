package device

import (
	"context"
	"encoding/json"
	"errors"
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
	orgID := "00000000-0000-0000-0000-000000000002"
	valid := CreateDeviceInput{
		OrgUnitID:  &orgID,
		CenterCode: "34020000", DeviceType: DeviceTypeIPC,
		DeviceName: "东门摄像机", Manufacturer: "海康威视",
		SIPRealm: "3402000000", Password: "secret", Enabled: true,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	// No org unit is valid: devices can be created unassigned (D10).
	var unassigned = CreateDeviceInput{
		CenterCode: "34020000", DeviceType: DeviceTypeIPC,
		DeviceName: "未归位摄像机", Manufacturer: "海康威视",
		SIPRealm: "3402000000", Password: "secret", Enabled: true,
	}
	if err := unassigned.Validate(); err != nil {
		t.Fatalf("unassigned input rejected: %v", err)
	}
	badOrg := "not-a-uuid"
	for name, input := range map[string]CreateDeviceInput{
		"bad org":            validWith(valid, func(v *CreateDeviceInput) { v.OrgUnitID = &badOrg }),
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
func (s *endpointStub) SetOrgUnit(context.Context, string, *string) (Device, error) {
	return s.device, nil
}
func (s *endpointStub) List(context.Context, string, []string, bool) ([]Device, error) {
	return []Device{s.device}, nil
}
func (s *endpointStub) Delete(context.Context, string) error { return nil }
func (s *endpointStub) EnsureVisible(context.Context, string, []string, bool, string) (Device, error) {
	return s.device, nil
}

func TestDeviceAPIDoesNotLeakCredentials(t *testing.T) {
	orgID := "00000000-0000-0000-0000-000000000002"
	stub := &endpointStub{
		device: Device{
			ID:           "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			TenantID:     "00000000-0000-0000-0000-000000000001",
			OrgUnitID:    &orgID,
			DeviceAccessID: "34020000001320000001",
			DeviceName:   "东门摄像机", Manufacturer: "海康威视",
			DeviceType: DeviceTypeIPC, SIPUsername: "34020000001320000001",
			SIPRealm: "3402000000", DigestAlgorithm: "MD5",
			Enabled: true, ProfileVersion: 1, AccessSyncStatus: "pending",
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		},
	}
	mux := http.NewServeMux()
	guard := func(obj, act string, h http.HandlerFunc) http.HandlerFunc { return h }
	RegisterRoutes(mux, stub, guard, nil)
	// The org-visibility check reads the scope injected by the wiring; the
	// stub request must carry the same scope so creation is allowed.
	scoped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := WithScope(r.Context(), "00000000-0000-0000-0000-000000000001", []string{"00000000-0000-0000-0000-000000000002"}, false)
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
	server := httptest.NewServer(scoped)
	defer server.Close()

	body := `{"org_unit_id":"00000000-0000-0000-0000-000000000002","center_code":"34020000","device_type":"132","device_name":"东门摄像机","manufacturer":"海康威视","sip_realm":"3402000000","password":"super-secret","enabled":true}`
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

// repoStub is a minimal DeviceRepository whose Get returns one fixed device.
// The embedded interface satisfies the remaining methods; only Get is used
// by the manager's EnsureVisible.
type repoStub struct {
	DeviceRepository
	device Device
}

func (s *repoStub) Get(ctx context.Context, id string) (Device, error) {
	if id != s.device.ID {
		return Device{}, ErrNotFound
	}
	return s.device, nil
}

// TestEnsureVisibleFullVisibility guards the critical scope semantics:
// node_admin / all_orgs callers carry an EMPTY org id set plus
// includeUnassigned=true and must still see every device in their tenant,
// including assigned ones. Scoped callers must not see unassigned devices.
func TestEnsureVisibleFullVisibility(t *testing.T) {
	const tenantID = "00000000-0000-0000-0000-000000000001"
	orgID := "00000000-0000-0000-0000-000000000002"
	assigned := Device{
		ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", TenantID: tenantID,
		OrgUnitID: &orgID, DeviceAccessID: "34020000001320000001",
	}
	unassigned := Device{
		ID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", TenantID: tenantID,
		OrgUnitID: nil, DeviceAccessID: "34020000001320000002",
	}

	tcases := map[string]struct {
		device            Device
		orgIDs            []string
		includeUnassigned bool
		wantVisible       bool
	}{
		// Finding 1: full visibility with empty org set must see assigned devices.
		"node_admin sees assigned device": {
			device: assigned, orgIDs: nil, includeUnassigned: true, wantVisible: true,
		},
		"node_admin sees unassigned device": {
			device: unassigned, orgIDs: nil, includeUnassigned: true, wantVisible: true,
		},
		"scoped user sees device in scope": {
			device: assigned, orgIDs: []string{orgID}, includeUnassigned: false, wantVisible: true,
		},
		"scoped user cannot see unassigned device": {
			device: unassigned, orgIDs: []string{orgID}, includeUnassigned: false, wantVisible: false,
		},
		"empty scopes deny assigned device": {
			device: assigned, orgIDs: nil, includeUnassigned: false, wantVisible: false,
		},
	}

	for name, tc := range tcases {
		t.Run(name, func(t *testing.T) {
			m := NewDeviceManager(&repoStub{device: tc.device}, nil)
			_, err := m.EnsureVisible(context.Background(), tenantID, tc.orgIDs, tc.includeUnassigned, tc.device.ID)
			if tc.wantVisible && err != nil {
				t.Fatalf("expected visible, got %v", err)
			}
			if !tc.wantVisible && !errors.Is(err, ErrNoAccess) {
				t.Fatalf("expected ErrNoAccess, got %v", err)
			}
		})
	}
}