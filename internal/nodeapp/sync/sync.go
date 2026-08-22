// Package sync synchronises device profiles to the node-access layer and
// polls runtime events. Behavior is unchanged from the pre-refactor nodeapp
// package; only the package location moved.
package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/new-vision-lab/new-vision/internal/nodeapp/access"
	"github.com/new-vision-lab/new-vision/internal/nodeapp/device"
)

// DeviceRepository is the subset of the device repository needed by sync.
type DeviceRepository interface {
	NextPending(context.Context) (device.Device, bool, error)
	MarkSynced(context.Context, string, int64) error
	MarkReconciled(context.Context, []device.ReconciledProfile) error
	MarkFailed(context.Context, string, time.Duration, string) error
	GetByAccessID(context.Context, string) (device.Device, error)
	List(context.Context) ([]device.Device, error)
}

// AccessRuntimeProjection is the runtime state projection interface used
// by the sync subsystem.
type AccessRuntimeProjection interface {
	access.RuntimeReader
	Apply(context.Context, string, access.RuntimeState) error
	Replace(context.Context, map[string]access.RuntimeState) error
	Cursor(context.Context, string) (string, int64, error)
	SetCursor(context.Context, string, string, int64) error
}

type SyncRunner struct {
	repository DeviceRepository
	accessAPI  access.AccessAPI
	projection AccessRuntimeProjection
	interval   time.Duration
}

func NewSyncRunner(repository DeviceRepository, accessAPI access.AccessAPI, projection AccessRuntimeProjection, interval time.Duration) *SyncRunner {
	if interval <= 0 {
		interval = time.Second
	}
	return &SyncRunner{repository: repository, accessAPI: accessAPI, projection: projection, interval: interval}
}

func (s *SyncRunner) Run(ctx context.Context) {
	retry := time.NewTicker(s.interval)
	defer retry.Stop()
	reconciled := false
	for {
		if !reconciled {
			if err := s.reconcile(ctx); err != nil {
				reconciled = false
			} else {
				reconciled = true
			}
		} else {
			if err := s.syncOne(ctx); errors.Is(err, ErrReconcile) {
				reconciled = false
			}
			if reconciled {
				if err := s.poll(ctx); err != nil {
					reconciled = false
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-retry.C:
		}
	}
}

var ErrNoPending = errors.New("no pending profile")
var ErrReconcile = errors.New("access reconciliation required")

func (s *SyncRunner) reconcile(ctx context.Context) error {
	devices, err := s.repository.List(ctx)
	if err != nil {
		return err
	}
	profiles := make([]access.AccessProfile, 0, len(devices))
	reconciledProfiles := make([]device.ReconciledProfile, 0, len(devices))
	byAccessID := make(map[string]string, len(devices))
	for _, d := range devices {
		profiles = append(profiles, d.AccessProfile())
		reconciledProfiles = append(reconciledProfiles, device.ReconciledProfile{DeviceID: d.ID, Version: d.ProfileVersion})
		byAccessID[d.DeviceAccessID] = d.ID
	}
	if err = s.accessAPI.ReplaceDeviceProfiles(ctx, profiles); err != nil {
		return err
	}
	snapshot, err := s.accessAPI.GetRuntimeSnapshot(ctx)
	if err != nil {
		return err
	}
	if err = s.repository.MarkReconciled(ctx, reconciledProfiles); err != nil {
		return err
	}
	states := make(map[string]access.RuntimeState, len(snapshot.Registrations))
	for _, registration := range snapshot.Registrations {
		if id, ok := byAccessID[registration.DeviceAccessID]; ok {
			states[id] = access.RuntimeState{State: registration.State, Reason: registration.Reason,
				RemoteAddress: registration.RemoteAddress, ExpiresAt: registration.ExpiresAt,
				LastSeen: registration.LastSeen, SessionEpoch: snapshot.SessionEpoch}
		}
	}
	if err = s.projection.Replace(ctx, states); err != nil {
		return err
	}
	if err = s.projection.SetCursor(ctx, snapshot.AccessInstanceID, snapshot.SessionEpoch, snapshot.LatestSequence); err != nil {
		return err
	}
	if snapshot.LatestSequence > 0 {
		return s.accessAPI.AckEvents(ctx, snapshot.LatestSequence)
	}
	return nil
}

func (s *SyncRunner) syncOne(ctx context.Context) error {
	d, ok, err := s.repository.NextPending(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoPending
	}
	profileResult, err := s.accessAPI.ApplyDeviceProfile(ctx, d.AccessProfile())
	if err != nil {
		if access.IsRPCErrorCode(err, "PROFILE_VERSION_CONFLICT") {
			return ErrReconcile
		}
		return s.repository.MarkFailed(ctx, d.ID, s.interval, safeError(err))
	}
	if profileResult.Status != "applied" && profileResult.Status != "unchanged" {
		return ErrReconcile
	}
	if profileResult.Version != 0 && profileResult.Version != d.ProfileVersion {
		return ErrReconcile
	}
	return s.repository.MarkSynced(ctx, d.ID, d.ProfileVersion)
}

func (s *SyncRunner) poll(ctx context.Context) error {
	instance, epoch, cursor, err := s.currentCursor(ctx)
	if err != nil {
		return err
	}
	result, err := s.accessAPI.PollEvents(ctx, cursor, 500)
	if err != nil {
		return err
	}
	if result.AccessInstanceID == "" || result.SessionEpoch == "" {
		return errors.New("access returned incomplete event cursor")
	}
	if instance != "" && result.AccessInstanceID != instance {
		return errors.New("access instance changed")
	}
	if epoch != "" && result.SessionEpoch != epoch {
		return errors.New("access session epoch changed")
	}
	next := cursor
	for _, event := range result.Events {
		if err := validateEvent(event); err != nil {
			return err
		}
		if event.Sequence != next+1 {
			return fmt.Errorf("access event cursor gap at %d", next+1)
		}
		d, lookupErr := s.repository.GetByAccessID(ctx, event.DeviceAccessID)
		if lookupErr != nil {
			next = event.Sequence
			continue
		}
		state := access.RuntimeState{State: event.Payload.State, Reason: event.Payload.Reason,
			RemoteAddress: event.Payload.RemoteAddress, ExpiresAt: event.Payload.ExpiresAt,
			LastSeen: event.Payload.LastSeen, SessionEpoch: event.SessionEpoch}
		if err = s.projection.Apply(ctx, d.ID, state); err != nil {
			return err
		}
		next = event.Sequence
	}
	if err = s.projection.SetCursor(ctx, result.AccessInstanceID, result.SessionEpoch, next); err != nil {
		return err
	}
	if next > 0 {
		return s.accessAPI.AckEvents(ctx, next)
	}
	return nil
}

func (s *SyncRunner) currentCursor(ctx context.Context) (string, string, int64, error) {
	snapshot, err := s.accessAPI.GetRuntimeSnapshot(ctx)
	if err != nil {
		return "", "", 0, err
	}
	instance := snapshot.AccessInstanceID
	epoch, cursor, err := s.projection.Cursor(ctx, instance)
	return instance, epoch, cursor, err
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	return "access synchronization failed"
}

func validateEvent(event access.AccessEvent) error {
	if event.Sequence <= 0 || event.EventID == "" || event.AccessInstanceID == "" || event.SessionEpoch == "" || !isValidAccessID(event.DeviceAccessID) {
		return fmt.Errorf("invalid access event envelope")
	}
	if event.Type != "registration_changed" || (event.Payload.State != "online" && event.Payload.State != "offline") {
		return fmt.Errorf("unsupported access event")
	}
	return nil
}

func isValidAccessID(id string) bool {
	if len(id) != 20 {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}