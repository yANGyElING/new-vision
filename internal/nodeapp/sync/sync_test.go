package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/new-vision-lab/new-vision/internal/nodeapp/access"
	"github.com/new-vision-lab/new-vision/internal/nodeapp/device"
)

type fakeRepo struct {
	device     device.Device
	pending    bool
	marked     []int64
	failed     int
	listed     []device.Device
	reconciled []device.ReconciledProfile
}

func (r *fakeRepo) NextPending(context.Context) (device.Device, bool, error) {
	return r.device, r.pending, nil
}
func (r *fakeRepo) MarkSynced(_ context.Context, _ string, version int64) error {
	r.marked = append(r.marked, version)
	r.pending = false
	return nil
}
func (r *fakeRepo) MarkReconciled(_ context.Context, profiles []device.ReconciledProfile) error {
	r.reconciled = append(r.reconciled, profiles...)
	return nil
}
func (r *fakeRepo) MarkFailed(context.Context, string, time.Duration, string) error {
	r.failed++
	return nil
}
func (r *fakeRepo) GetByAccessID(context.Context, string) (device.Device, error) {
	return r.device, nil
}
func (r *fakeRepo) List(context.Context) ([]device.Device, error) {
	if r.listed != nil {
		return r.listed, nil
	}
	return []device.Device{r.device}, nil
}

type fakeAccess struct {
	applied   int
	replaced  int
	removeErr error
}

func (f *fakeAccess) ApplyDeviceProfile(_ context.Context, profile access.AccessProfile) (access.ProfileResult, error) {
	f.applied++
	return access.ProfileResult{Status: "applied", Version: profile.Version}, nil
}
func (f *fakeAccess) RemoveDeviceProfile(context.Context, string, int64) (access.ProfileResult, error) {
	return access.ProfileResult{}, f.removeErr
}
func (f *fakeAccess) ReplaceDeviceProfiles(context.Context, []access.AccessProfile) error {
	f.replaced++
	return nil
}
func (f *fakeAccess) GetRuntimeSnapshot(context.Context) (access.RuntimeSnapshot, error) {
	return access.RuntimeSnapshot{AccessInstanceID: "access-01", SessionEpoch: "11111111-1111-4111-8111-111111111111"}, nil
}
func (f *fakeAccess) PollEvents(context.Context, int64, int) (access.PollResult, error) {
	return access.PollResult{AccessInstanceID: "access-01", SessionEpoch: "11111111-1111-4111-8111-111111111111"}, nil
}
func (f *fakeAccess) AckEvents(context.Context, int64) error { return nil }

type fakeProjection struct {
	states map[string]access.RuntimeState
}

func (p *fakeProjection) Get(context.Context, string) (*access.RuntimeState, error) {
	return nil, nil
}
func (p *fakeProjection) GetMany(context.Context, []string) (map[string]*access.RuntimeState, error) {
	return nil, nil
}
func (p *fakeProjection) Apply(_ context.Context, id string, state access.RuntimeState) error {
	if p.states == nil {
		p.states = map[string]access.RuntimeState{}
	}
	p.states[id] = state
	return nil
}
func (p *fakeProjection) Replace(_ context.Context, states map[string]access.RuntimeState) error {
	p.states = states
	return nil
}
func (p *fakeProjection) Cursor(context.Context, string) (string, int64, error) {
	return "11111111-1111-4111-8111-111111111111", 0, nil
}
func (p *fakeProjection) SetCursor(context.Context, string, string, int64) error { return nil }

type fakeCatalog struct {
	progress []access.CatalogProgressPayload
	results  []access.CatalogResultPayload
}

func (c *fakeCatalog) ApplyProgress(_ context.Context, _ device.Device, p access.CatalogProgressPayload) error {
	c.progress = append(c.progress, p)
	return nil
}
func (c *fakeCatalog) ApplyResult(_ context.Context, _ device.Device, r access.CatalogResultPayload) error {
	c.results = append(c.results, r)
	return nil
}

func TestReconcilePushesProfilesAndMarksReconciled(t *testing.T) {
	repo := &fakeRepo{device: device.Device{ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", DeviceAccessID: "34020000001320000001", ProfileVersion: 3}}
	acc := &fakeAccess{}
	proj := &fakeProjection{}
	runner := NewSyncRunner(repo, acc, proj, &fakeCatalog{}, time.Second)
	if err := runner.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if acc.replaced != 1 {
		t.Fatalf("replace calls = %d", acc.replaced)
	}
	if len(repo.reconciled) != 1 || repo.reconciled[0].Version != 3 {
		t.Fatalf("reconciled = %+v", repo.reconciled)
	}
}

func TestSyncOneAppliesAndMarksSynced(t *testing.T) {
	repo := &fakeRepo{device: device.Device{ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", DeviceAccessID: "34020000001320000001", ProfileVersion: 3}, pending: true}
	acc := &fakeAccess{}
	proj := &fakeProjection{}
	runner := NewSyncRunner(repo, acc, proj, &fakeCatalog{}, time.Second)
	if err := runner.syncOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if acc.applied != 1 || len(repo.marked) != 1 || repo.marked[0] != 3 {
		t.Fatalf("applied=%d marked=%v", acc.applied, repo.marked)
	}
}

func TestSyncOneNoPending(t *testing.T) {
	repo := &fakeRepo{device: device.Device{}, pending: false}
	runner := NewSyncRunner(repo, &fakeAccess{}, &fakeProjection{}, &fakeCatalog{}, time.Second)
	if err := runner.syncOne(context.Background()); !errors.Is(err, ErrNoPending) {
		t.Fatalf("err = %v, want ErrNoPending", err)
	}
}

func TestPollAppliesRuntimeState(t *testing.T) {
	repo := &fakeRepo{device: device.Device{ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", DeviceAccessID: "34020000001320000001"}}
	acc := &fakeAccess{}
	proj := &fakeProjection{}
	runner := NewSyncRunner(repo, acc, proj, &fakeCatalog{}, time.Second)
	if err := runner.poll(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// The poll loop must dispatch catalog events to the catalog consumer with
// the same gap-checked sequencing as registration events, and reject
// unknown payload shapes instead of silently dropping them.
func TestApplyEventDispatchesCatalogEvents(t *testing.T) {
	d := device.Device{ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", DeviceAccessID: "34020000001320000001"}
	repo := &fakeRepo{device: d}
	acc := &fakeAccess{}
	proj := &fakeProjection{}
	catalog := &fakeCatalog{}
	runner := NewSyncRunner(repo, acc, proj, catalog, time.Second)

	progress := access.AccessEvent{Type: "catalog.progress", Payload: []byte(`{"sn":1,"received":4,"total":8}`)}
	if err := runner.applyEvent(context.Background(), d, progress); err != nil {
		t.Fatal(err)
	}
	result := access.AccessEvent{Type: "catalog.result", Payload: []byte(`{"ok":true,"channels":[{"code":"34020000001320000001","name":"大门口","status":"ON"}]}`)}
	if err := runner.applyEvent(context.Background(), d, result); err != nil {
		t.Fatal(err)
	}
	failed := access.AccessEvent{Type: "catalog.result", Payload: []byte(`{"ok":false,"error":"timeout","received":26,"total":32}`)}
	if err := runner.applyEvent(context.Background(), d, failed); err != nil {
		t.Fatal(err)
	}
	if len(catalog.progress) != 1 || catalog.progress[0].Received != 4 || catalog.progress[0].Total != 8 {
		t.Fatalf("progress = %+v", catalog.progress)
	}
	if len(catalog.results) != 2 {
		t.Fatalf("results = %+v", catalog.results)
	}
	if !catalog.results[0].OK || len(catalog.results[0].Channels) != 1 || catalog.results[0].Channels[0].Name != "大门口" {
		t.Fatalf("ok result = %+v", catalog.results[0])
	}
	if catalog.results[1].OK || catalog.results[1].Received != 26 || catalog.results[1].Error != "timeout" {
		t.Fatalf("failed result = %+v", catalog.results[1])
	}

	badPayload := access.AccessEvent{Type: "catalog.progress", Payload: []byte(`{"sn":0,"received":1,"total":1}`)}
	if err := runner.applyEvent(context.Background(), d, badPayload); err == nil {
		t.Fatal("invalid progress payload accepted")
	}
	unknown := access.AccessEvent{Type: "something_else", Payload: []byte(`{}`)}
	if err := runner.applyEvent(context.Background(), d, unknown); err == nil {
		t.Fatal("unknown event type accepted")
	}
}
