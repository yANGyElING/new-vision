package nodeapp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type syncRepo struct {
	device           Device
	pending          bool
	marked           []int64
	failed           []int
	listed           []Device
	reconciled       []ReconciledProfile
	currentVersions  map[string]int64
	syncStatuses     map[string]string
	processedThrough map[string]int64
}

func (r *syncRepo) Create(context.Context, CreateDeviceInput) (Device, error) {
	return r.device, nil
}
func (r *syncRepo) Get(context.Context, string) (Device, error)              { return r.device, nil }
func (r *syncRepo) SetEnabled(context.Context, string, bool) (Device, error) { return r.device, nil }
func (r *syncRepo) UpdateMeta(context.Context, string, *string, *string) (Device, error) {
	return r.device, nil
}
func (r *syncRepo) GetByAccessID(context.Context, string) (Device, error)    { return r.device, nil }
func (r *syncRepo) List(context.Context) ([]Device, error) {
	if r.listed != nil {
		return r.listed, nil
	}
	return []Device{r.device}, nil
}
func (r *syncRepo) NextPending(context.Context) (Device, bool, error) {
	return r.device, r.pending, nil
}
func (r *syncRepo) MarkSynced(_ context.Context, _ string, version int64) error {
	r.marked = append(r.marked, version)
	r.pending = false
	return nil
}
func (r *syncRepo) MarkReconciled(_ context.Context, profiles []ReconciledProfile) error {
	r.reconciled = append(r.reconciled, profiles...)
	if r.processedThrough == nil {
		r.processedThrough = make(map[string]int64)
	}
	for _, profile := range profiles {
		if profile.Version > r.processedThrough[profile.DeviceID] {
			r.processedThrough[profile.DeviceID] = profile.Version
		}
		if r.syncStatuses != nil && r.currentVersions[profile.DeviceID] <= profile.Version {
			r.syncStatuses[profile.DeviceID] = "synced"
		}
	}
	return nil
}
func (r *syncRepo) MarkFailed(_ context.Context, _ string, attempts int, _ time.Time, _ string) error {
	r.failed = append(r.failed, attempts)
	return nil
}
func (r *syncRepo) Delete(context.Context, string) error { return nil }

type syncAccess struct {
	result       ProfileResult
	err          error
	applied      []AccessProfile
	replace      func([]AccessProfile)
	snapshot     RuntimeSnapshot
	poll         func(int64) PollResult
	ackErrors    []error
	acknowledged []int64
	operations   *[]string
}

func (a *syncAccess) ApplyDeviceProfile(_ context.Context, profile AccessProfile) (ProfileResult, error) {
	a.applied = append(a.applied, profile)
	return a.result, a.err
}
func (a *syncAccess) RemoveDeviceProfile(context.Context, string, int64) (ProfileResult, error) {
	return ProfileResult{}, nil
}
func (a *syncAccess) ReplaceDeviceProfiles(_ context.Context, profiles []AccessProfile) error {
	if a.replace != nil {
		a.replace(profiles)
	}
	return nil
}
func (a *syncAccess) GetRuntimeSnapshot(context.Context) (RuntimeSnapshot, error) {
	if a.snapshot.AccessInstanceID != "" {
		return a.snapshot, nil
	}
	return RuntimeSnapshot{AccessInstanceID: "access-01", SessionEpoch: "epoch-1"}, nil
}
func (a *syncAccess) PollEvents(_ context.Context, after int64, _ int) (PollResult, error) {
	if a.poll != nil {
		return a.poll(after), nil
	}
	return PollResult{AccessInstanceID: "access-01", SessionEpoch: "epoch-1"}, nil
}
func (a *syncAccess) AckEvents(_ context.Context, through int64) error {
	a.acknowledged = append(a.acknowledged, through)
	if a.operations != nil {
		*a.operations = append(*a.operations, "ack")
	}
	if len(a.ackErrors) == 0 {
		return nil
	}
	err := a.ackErrors[0]
	a.ackErrors = a.ackErrors[1:]
	return err
}

type syncProjection struct {
	epoch      string
	sequence   int64
	applyCount int
	operations *[]string
}

func (p *syncProjection) Get(context.Context, string) (*RuntimeState, error) { return nil, nil }
func (p *syncProjection) Apply(context.Context, string, RuntimeState) error {
	p.applyCount++
	if p.operations != nil {
		*p.operations = append(*p.operations, "apply")
	}
	return nil
}
func (p *syncProjection) Replace(context.Context, map[string]RuntimeState, RuntimeSnapshot) error {
	if p.operations != nil {
		*p.operations = append(*p.operations, "replace")
	}
	return nil
}
func (p *syncProjection) Cursor(context.Context, string) (string, int64, error) {
	if p.epoch == "" {
		return "epoch-1", p.sequence, nil
	}
	return p.epoch, p.sequence, nil
}
func (p *syncProjection) SetCursor(_ context.Context, _ string, epoch string, sequence int64) error {
	p.epoch = epoch
	p.sequence = sequence
	if p.operations != nil {
		*p.operations = append(*p.operations, "cursor")
	}
	return nil
}

func TestSyncOneUsesCurrentProfileVersion(t *testing.T) {
	repo := &syncRepo{pending: true, device: Device{ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", DeviceAccessID: "34020000001320000001", SIPUsername: "34020000001320000001", SIPRealm: "realm", DigestHA1: "0123456789abcdef0123456789abcdef", DigestAlgorithm: "MD5", Enabled: true, ProfileVersion: 4}}
	access := &syncAccess{result: ProfileResult{Status: "unchanged", Version: 4}}
	runner := NewSyncRunner(repo, access, &syncProjection{}, time.Second)
	if err := runner.syncOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(access.applied) != 1 || access.applied[0].Version != 4 || len(repo.marked) != 1 || repo.marked[0] != 4 {
		t.Fatalf("sync did not acknowledge current version: applied=%+v marked=%v", access.applied, repo.marked)
	}
}

func TestSyncOneRetryBackoffAttemptsAndNoCredentialDiagnostic(t *testing.T) {
	repo := &syncRepo{pending: true, device: Device{ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", DeviceAccessID: "34020000001320000001", SIPUsername: "34020000001320000001", SIPRealm: "realm", DigestHA1: "0123456789abcdef0123456789abcdef", Enabled: true, ProfileVersion: 1}}
	access := &syncAccess{err: errors.New("password=secret digest_ha1=0123456789abcdef0123456789abcdef")}
	runner := NewSyncRunner(repo, access, &syncProjection{}, time.Millisecond)
	if err := runner.syncOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	runner.syncOne(context.Background())
	if len(repo.failed) != 2 || repo.failed[0] != 1 || repo.failed[1] != 2 {
		t.Fatalf("attempts = %v", repo.failed)
	}
	if safeError(access.err) != "access synchronization failed" {
		t.Fatal("diagnostic was not redacted")
	}
}

func TestSyncOneStaleResultRequestsReconciliation(t *testing.T) {
	repo := &syncRepo{pending: true, device: Device{ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", DeviceAccessID: "34020000001320000001", SIPUsername: "34020000001320000001", SIPRealm: "realm", DigestHA1: "0123456789abcdef0123456789abcdef", Enabled: true, ProfileVersion: 2}}
	access := &syncAccess{result: ProfileResult{Status: "stale", Version: 3}}
	runner := NewSyncRunner(repo, access, &syncProjection{}, time.Second)
	if !errors.Is(runner.syncOne(context.Background()), ErrReconcile) {
		t.Fatal("stale result did not request reconciliation")
	}
	if len(repo.marked) != 0 {
		t.Fatal("stale result was marked synced")
	}
}

func TestReconcileAcknowledgesOnlyVersionsIncludedInReplacement(t *testing.T) {
	const (
		listedID  = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
		createdID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	)
	listed := Device{ID: listedID, DeviceAccessID: "34020000001320000001", SIPUsername: "34020000001320000001", ProfileVersion: 1}
	repo := &syncRepo{
		listed:          []Device{listed},
		currentVersions: map[string]int64{listedID: 1},
		syncStatuses:    map[string]string{listedID: "pending"},
	}
	access := &syncAccess{replace: func(profiles []AccessProfile) {
		if len(profiles) != 1 || profiles[0].Version != 1 {
			t.Fatalf("replacement profiles = %+v", profiles)
		}
		// These writes happen after List returned but before reconciliation bookkeeping.
		repo.currentVersions[listedID] = 2
		repo.currentVersions[createdID] = 1
		repo.syncStatuses[listedID] = "pending"
		repo.syncStatuses[createdID] = "pending"
	}}
	runner := NewSyncRunner(repo, access, &syncProjection{}, time.Second)
	if err := runner.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repo.reconciled) != 1 || repo.reconciled[0] != (ReconciledProfile{DeviceID: listedID, Version: 1}) {
		t.Fatalf("reconciled versions = %+v", repo.reconciled)
	}
	if repo.processedThrough[listedID] != 1 || repo.currentVersions[listedID] != 2 || repo.syncStatuses[listedID] != "pending" {
		t.Fatalf("updated device bookkeeping: current=%d processed=%d status=%s", repo.currentVersions[listedID], repo.processedThrough[listedID], repo.syncStatuses[listedID])
	}
	if _, ok := repo.processedThrough[createdID]; ok {
		t.Fatal("device created after List was marked reconciled")
	}
	if repo.syncStatuses[createdID] != "pending" {
		t.Fatalf("new device status = %s", repo.syncStatuses[createdID])
	}
}

func TestReconcilePersistsCursorBeforeAck(t *testing.T) {
	operations := []string{}
	ackFailure := errors.New("ack unavailable")
	repo := &syncRepo{listed: []Device{}}
	access := &syncAccess{
		snapshot:   RuntimeSnapshot{AccessInstanceID: "access-01", SessionEpoch: "epoch-1", LatestSequence: 7},
		ackErrors:  []error{ackFailure},
		operations: &operations,
	}
	projection := &syncProjection{operations: &operations}
	runner := NewSyncRunner(repo, access, projection, time.Second)
	if err := runner.reconcile(context.Background()); !errors.Is(err, ackFailure) {
		t.Fatalf("reconcile error = %v", err)
	}
	if projection.sequence != 7 || strings.Join(operations, ",") != "replace,cursor,ack" {
		t.Fatalf("cursor=%d operations=%v", projection.sequence, operations)
	}
}

func TestPollRetriesAckFromPersistedCursorWhenNoEventsRemain(t *testing.T) {
	operations := []string{}
	ackFailure := errors.New("ack unavailable")
	repo := &syncRepo{device: Device{ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", DeviceAccessID: "34020000001320000001"}}
	access := &syncAccess{
		snapshot:   RuntimeSnapshot{AccessInstanceID: "access-01", SessionEpoch: "epoch-1"},
		ackErrors:  []error{ackFailure, nil},
		operations: &operations,
	}
	access.poll = func(after int64) PollResult {
		result := PollResult{AccessInstanceID: "access-01", SessionEpoch: "epoch-1"}
		if after == 0 {
			result.Events = []AccessEvent{{
				EventID: "access-01:1", Sequence: 1, AccessInstanceID: "access-01", SessionEpoch: "epoch-1",
				Type: "registration_changed", DeviceAccessID: "34020000001320000001",
				Payload: AccessEventPayload{State: "online", Reason: "register"},
			}}
		}
		return result
	}
	projection := &syncProjection{epoch: "epoch-1", operations: &operations}
	runner := NewSyncRunner(repo, access, projection, time.Second)

	if err := runner.poll(context.Background()); !errors.Is(err, ackFailure) {
		t.Fatalf("first poll error = %v", err)
	}
	if projection.sequence != 1 || projection.applyCount != 1 || strings.Join(operations, ",") != "apply,cursor,ack" {
		t.Fatalf("after ACK failure: cursor=%d applies=%d operations=%v", projection.sequence, projection.applyCount, operations)
	}

	operations = operations[:0]
	if err := runner.poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if projection.applyCount != 1 || strings.Join(operations, ",") != "cursor,ack" {
		t.Fatalf("empty retry poll: applies=%d operations=%v", projection.applyCount, operations)
	}
	if len(access.acknowledged) != 2 || access.acknowledged[0] != 1 || access.acknowledged[1] != 1 {
		t.Fatalf("acknowledgements = %v", access.acknowledged)
	}
}
