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
	device       device.Device
	pending      bool
	marked       []int64
	failed       int
	listed       []device.Device
	reconciled   []device.ReconciledProfile
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

func TestReconcilePushesProfilesAndMarksReconciled(t *testing.T) {
	repo := &fakeRepo{device: device.Device{ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", DeviceAccessID: "34020000001320000001", ProfileVersion: 3}}
	acc := &fakeAccess{}
	proj := &fakeProjection{}
	runner := NewSyncRunner(repo, acc, proj, time.Second)
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
	runner := NewSyncRunner(repo, acc, proj, time.Second)
	if err := runner.syncOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if acc.applied != 1 || len(repo.marked) != 1 || repo.marked[0] != 3 {
		t.Fatalf("applied=%d marked=%v", acc.applied, repo.marked)
	}
}

func TestSyncOneNoPending(t *testing.T) {
	repo := &fakeRepo{device: device.Device{}, pending: false}
	runner := NewSyncRunner(repo, &fakeAccess{}, &fakeProjection{}, time.Second)
	if err := runner.syncOne(context.Background()); !errors.Is(err, ErrNoPending) {
		t.Fatalf("err = %v, want ErrNoPending", err)
	}
}

func TestPollAppliesRuntimeState(t *testing.T) {
	repo := &fakeRepo{device: device.Device{ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", DeviceAccessID: "34020000001320000001"}}
	acc := &fakeAccess{}
	proj := &fakeProjection{}
	runner := NewSyncRunner(repo, acc, proj, time.Second)
	if err := runner.poll(context.Background()); err != nil {
		t.Fatal(err)
	}
}