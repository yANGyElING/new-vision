package nodeapp

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type AccessRuntimeProjection interface {
	RuntimeReader
	Apply(context.Context, string, RuntimeState) error
	Replace(context.Context, map[string]RuntimeState) error
	Cursor(context.Context, string) (string, int64, error)
	SetCursor(context.Context, string, string, int64) error
}

type SyncRunner struct {
	repository DeviceRepository
	access     AccessAPI
	projection AccessRuntimeProjection
	interval   time.Duration
}

func NewSyncRunner(repository DeviceRepository, access AccessAPI, projection AccessRuntimeProjection, interval time.Duration) *SyncRunner {
	if interval <= 0 {
		interval = time.Second
	}
	return &SyncRunner{repository: repository, access: access, projection: projection, interval: interval}
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
	profiles := make([]AccessProfile, 0, len(devices))
	reconciledProfiles := make([]ReconciledProfile, 0, len(devices))
	byAccessID := make(map[string]string, len(devices))
	for _, device := range devices {
		profiles = append(profiles, device.AccessProfile())
		reconciledProfiles = append(reconciledProfiles, ReconciledProfile{DeviceID: device.ID, Version: device.ProfileVersion})
		byAccessID[device.DeviceAccessID] = device.ID
	}
	if err = s.access.ReplaceDeviceProfiles(ctx, profiles); err != nil {
		return err
	}
	// Read the snapshot after the atomic replacement so events generated while removing stale registrations are included.
	snapshot, err := s.access.GetRuntimeSnapshot(ctx)
	if err != nil {
		return err
	}
	if err = s.repository.MarkReconciled(ctx, reconciledProfiles); err != nil {
		return err
	}
	states := make(map[string]RuntimeState, len(snapshot.Registrations))
	for _, registration := range snapshot.Registrations {
		if id, ok := byAccessID[registration.DeviceAccessID]; ok {
			states[id] = RuntimeState{State: registration.State, Reason: registration.Reason,
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
		return s.access.AckEvents(ctx, snapshot.LatestSequence)
	}
	return nil
}

func (s *SyncRunner) syncOne(ctx context.Context) error {
	device, ok, err := s.repository.NextPending(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoPending
	}
	profileResult, err := s.access.ApplyDeviceProfile(ctx, device.AccessProfile())
	if err != nil {
		if IsRPCErrorCode(err, "PROFILE_VERSION_CONFLICT") {
			return ErrReconcile
		}
		return s.repository.MarkFailed(ctx, device.ID, s.interval, safeError(err))
	}
	if profileResult.Status != "applied" && profileResult.Status != "unchanged" {
		return ErrReconcile
	}
	if profileResult.Version != 0 && profileResult.Version != device.ProfileVersion {
		return ErrReconcile
	}
	return s.repository.MarkSynced(ctx, device.ID, device.ProfileVersion)
}

func (s *SyncRunner) poll(ctx context.Context) error {
	instance, epoch, cursor, err := s.currentCursor(ctx)
	if err != nil {
		return err
	}
	result, err := s.access.PollEvents(ctx, cursor, 500)
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
		device, lookupErr := s.repository.GetByAccessID(ctx, event.DeviceAccessID)
		if lookupErr != nil && !errors.Is(lookupErr, ErrNotFound) {
			return lookupErr
		}
		if lookupErr == nil {
			state := RuntimeState{State: event.Payload.State, Reason: event.Payload.Reason,
				RemoteAddress: event.Payload.RemoteAddress, ExpiresAt: event.Payload.ExpiresAt,
				LastSeen: event.Payload.LastSeen, SessionEpoch: event.SessionEpoch}
			if err = s.projection.Apply(ctx, device.ID, state); err != nil {
				return err
			}
		}
		next = event.Sequence
	}
	if err = s.projection.SetCursor(ctx, result.AccessInstanceID, result.SessionEpoch, next); err != nil {
		return err
	}
	if next > 0 {
		return s.access.AckEvents(ctx, next)
	}
	return nil
}

func (s *SyncRunner) currentCursor(ctx context.Context) (string, string, int64, error) {
	// The instance and epoch come from Access; the persisted cursor determines whether reconciliation is needed.
	snapshot, err := s.access.GetRuntimeSnapshot(ctx)
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
