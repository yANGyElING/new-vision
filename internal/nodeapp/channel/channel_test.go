package channel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/new-vision-lab/new-vision/internal/nodeapp/access"
	"github.com/new-vision-lab/new-vision/internal/nodeapp/device"
)

type fakeRepo struct {
	device  device.Device
	applied [][]access.CatalogChannel
	listed  []Channel
	visible CatalogListResult
	listErr error
}

func (r *fakeRepo) ListByDevice(context.Context, string) ([]Channel, error) {
	return r.listed, nil
}

func (r *fakeRepo) ListVisible(context.Context, string, []string, bool, []string) (CatalogListResult, error) {
	return r.visible, r.listErr
}

func (r *fakeRepo) ApplyResult(_ context.Context, d device.Device, channels []access.CatalogChannel) error {
	r.applied = append(r.applied, channels)
	r.device = d
	return nil
}

type fakeDeviceState struct {
	inProgress []string
	ok         map[string]int
	failed     map[string][3]any // id -> {received, total, message}
}

func (s *fakeDeviceState) SetCatalogInProgress(_ context.Context, id string) error {
	s.inProgress = append(s.inProgress, id)
	return nil
}

func (s *fakeDeviceState) SetCatalogOK(_ context.Context, id string, count int) error {
	if s.ok == nil {
		s.ok = map[string]int{}
	}
	s.ok[id] = count
	return nil
}

func (s *fakeDeviceState) SetCatalogFailed(_ context.Context, id string, received, total int, message string) error {
	if s.failed == nil {
		s.failed = map[string][3]any{}
	}
	s.failed[id] = [3]any{received, total, message}
	return nil
}

func testDevice() device.Device {
	return device.Device{ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", TenantID: "00000000-0000-0000-0000-000000000001", DeviceAccessID: "34020000001180000001"}
}

// Scenario coverage for the consumer (D45/D46/D50): progress only marks the
// device; ok writes channels then flips state; failure keeps channel data
// (no repository write) and records the error. An invalid channel list is
// rejected as a failure, never written.
func TestConsumerScenarios(t *testing.T) {
	ctx := context.Background()
	d := testDevice()
	repo := &fakeRepo{}
	states := &fakeDeviceState{}
	consumer := NewConsumer(repo, states)

	// progress → state marker only
	if err := consumer.ApplyProgress(ctx, d, access.CatalogProgressPayload{SN: 1, Received: 4, Total: 8}); err != nil {
		t.Fatal(err)
	}
	if len(states.inProgress) != 1 || states.inProgress[0] != d.ID || len(repo.applied) != 0 {
		t.Fatalf("progress side effects: %+v %+v", states.inProgress, repo.applied)
	}

	// ok result → channels applied, then device marked ok with the count
	result := access.CatalogResultPayload{OK: true, Channels: []access.CatalogChannel{
		{Code: "34020000001320000001", Name: "大门口", Status: "ON"},
		{Code: "34020000001320000002", Name: "泵房", Status: "OFF"},
	}}
	if err := consumer.ApplyResult(ctx, d, result); err != nil {
		t.Fatal(err)
	}
	if len(repo.applied) != 1 || len(repo.applied[0]) != 2 {
		t.Fatalf("applied = %+v", repo.applied)
	}
	if states.ok[d.ID] != 2 {
		t.Fatalf("ok count = %d", states.ok[d.ID])
	}

	// failed result → no channel write, error recorded with received/total
	failed := access.CatalogResultPayload{OK: false, Error: "timeout", Received: 26, Total: 32}
	if err := consumer.ApplyResult(ctx, d, failed); err != nil {
		t.Fatal(err)
	}
	if len(repo.applied) != 1 {
		t.Fatalf("failed result must not touch channels: %+v", repo.applied)
	}
	got := states.failed[d.ID]
	if got[0] != 26 || got[1] != 32 || got[2] != "timeout" {
		t.Fatalf("failed state = %+v", got)
	}
}

// An ok result whose channel list violates the identity rules must fail
// before any write, so a malformed report can never partially land.
func TestConsumerRejectsInvalidChannels(t *testing.T) {
	ctx := context.Background()
	d := testDevice()
	repo := &fakeRepo{}
	states := &fakeDeviceState{}
	consumer := NewConsumer(repo, states)

	invalid := access.CatalogResultPayload{OK: true, Channels: []access.CatalogChannel{
		{Code: "34020000001320000001", Name: "a", Status: "ON"},
		{Code: "34020000001320000001", Name: "duplicate", Status: "ON"},
	}}
	if err := consumer.ApplyResult(ctx, d, invalid); err == nil {
		t.Fatal("duplicate channel code accepted")
	}
	if len(repo.applied) != 0 || len(states.ok) != 0 {
		t.Fatal("invalid report must not write anything")
	}
}

// The user-page list must attach device runtime state: an online device
// flips its brief to online (D44/D49 presentation rule).
func TestListVisibleAttachesRuntimeState(t *testing.T) {
	online := &access.RuntimeState{State: "online"}
	service := NewService(&fakeRepo{visible: CatalogListResult{
		Devices: []DeviceBrief{
			{ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", DeviceName: "nvr-1", CatalogState: "ok"},
			{ID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", DeviceName: "nvr-2", CatalogState: "ok"},
		},
		Channels: []ChannelView{{ID: "1", DeviceID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Name: "大门口"}},
	}}, nil, stubRuntime{states: map[string]*access.RuntimeState{
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa": online,
	}}, nil)

	result, err := service.ListVisible(context.Background(), "t", nil, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Devices[0].State != "online" || result.Devices[1].State != "offline" {
		t.Fatalf("device states = %+v", result.Devices)
	}
	if result.Channels[0].Name != "大门口" {
		t.Fatalf("channel = %+v", result.Channels[0])
	}
}

type stubRuntime struct {
	states map[string]*access.RuntimeState
}

func (s stubRuntime) Get(_ context.Context, id string) (*access.RuntimeState, error) {
	return s.states[id], nil
}

func (s stubRuntime) GetMany(_ context.Context, ids []string) (map[string]*access.RuntimeState, error) {
	out := make(map[string]*access.RuntimeState, len(ids))
	for _, id := range ids {
		if state, ok := s.states[id]; ok {
			out[id] = state
		}
	}
	return out, nil
}

// The device-channels endpoint must map scope/lookup failures to the same
// error contract as the device surface, and the scope values must flow from
// the request context into the service.
func TestDeviceChannelsEndpoint(t *testing.T) {
	d := testDevice()
	d.CatalogState = "ok"
	count := 8
	d.CatalogLastCount = &count
	name := "大门口"
	repo := &fakeRepo{listed: []Channel{{ID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", DeviceID: d.ID, ChannelCode: "34020000001320000001", ReportName: &name}}}
	service := NewService(repo, stubEndpoints{device: d}, nil, nil)
	mux := http.NewServeMux()
	guard := func(obj, act string, h http.HandlerFunc) http.HandlerFunc { return h }
	RegisterRoutes(mux, service, guard)
	scoped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := device.WithScope(r.Context(), d.TenantID, nil, true)
		mux.ServeHTTP(w, r.WithContext(ctx))
	})

	recorder := httptest.NewRecorder()
	scoped.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/devices/"+d.ID+"/channels", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var result CatalogDeviceResult
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.CatalogState != "ok" || *result.CatalogLastCount != 8 || len(result.Channels) != 1 || *result.Channels[0].ReportName != "大门口" {
		t.Fatalf("result = %+v", result)
	}

	forbidden := NewService(repo, stubEndpoints{err: device.ErrNoAccess}, nil, nil)
	mux2 := http.NewServeMux()
	RegisterRoutes(mux2, forbidden, guard)
	recorder = httptest.NewRecorder()
	scoped2 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := device.WithScope(r.Context(), d.TenantID, nil, false)
		mux2.ServeHTTP(w, r.WithContext(ctx))
	})
	scoped2.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/devices/"+d.ID+"/channels", nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("out-of-scope status = %d", recorder.Code)
	}
}

// org_unit_id must be a valid UUID; anything else is a 400 rather than a
// silent full listing.
func TestChannelsEndpointRejectsBadOrgFilter(t *testing.T) {
	service := NewService(&fakeRepo{visible: CatalogListResult{Devices: []DeviceBrief{}, Channels: []ChannelView{}}}, nil, nil, nil)
	mux := http.NewServeMux()
	guard := func(obj, act string, h http.HandlerFunc) http.HandlerFunc { return h }
	RegisterRoutes(mux, service, guard)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/channels?org_unit_id=not-a-uuid", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
}

type stubEndpoints struct {
	device device.Device
	err    error
}

func (s stubEndpoints) EnsureVisible(_ context.Context, _ string, _ []string, _ bool, id string) (device.Device, error) {
	if s.err != nil {
		return device.Device{}, s.err
	}
	return s.device, nil
}

// The device-channels endpoint must map scope/lookup failures to the same
// error contract as the device surface, and the scope values must flow from
// the request context into the service.
