package device

import (
	"context"
	"errors"

	"github.com/new-vision-lab/new-vision/internal/nodeapp/access"
)

// RuntimeReader and RuntimeRemover come from the access package; the manager
// enriches devices with runtime state.
type RuntimeReader = access.RuntimeReader
type RuntimeRemover interface {
	Remove(context.Context, string) error
}

type DeviceManager struct {
	repository DeviceRepository
	runtime    RuntimeReader
	remover    RuntimeRemover
}

func NewDeviceManager(repository DeviceRepository, runtime RuntimeReader) *DeviceManager {
	manager := &DeviceManager{repository: repository, runtime: runtime}
	if remover, ok := runtime.(RuntimeRemover); ok {
		manager.remover = remover
	}
	return manager
}

func (m *DeviceManager) Create(ctx context.Context, in CreateDeviceInput) (Device, error) {
	if err := in.Validate(); err != nil {
		return Device{}, err
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		device, err := m.repository.Create(ctx, in)
		if err == nil {
			return device, nil
		}
		if !errors.Is(err, ErrConflict) {
			return Device{}, err
		}
		lastErr = err
	}
	return Device{}, lastErr
}

func (m *DeviceManager) Get(ctx context.Context, id string) (Device, error) {
	device, err := m.repository.Get(ctx, id)
	if err != nil {
		return Device{}, err
	}
	if m.runtime != nil {
		device.Runtime, err = m.runtime.Get(ctx, device.ID)
		if err != nil {
			return Device{}, err
		}
	}
	return device, nil
}

func (m *DeviceManager) SetEnabled(ctx context.Context, id string, enabled bool) (Device, error) {
	return m.repository.SetEnabled(ctx, id, enabled)
}

func (m *DeviceManager) UpdateMeta(ctx context.Context, id string, name, manufacturer *string) (Device, error) {
	return m.repository.UpdateMeta(ctx, id, name, manufacturer)
}

// List returns devices visible to the given tenant and region set.
func (m *DeviceManager) List(ctx context.Context, tenantID string, regionIDs []string) ([]Device, error) {
	devices, err := m.repository.ListByTenant(ctx, tenantID, regionIDs)
	if err != nil {
		return nil, err
	}
	if m.runtime != nil {
		ids := make([]string, len(devices))
		for i := range devices {
			ids[i] = devices[i].ID
		}
		states, getErr := m.runtime.GetMany(ctx, ids)
		if getErr != nil {
			return nil, getErr
		}
		for i := range devices {
			devices[i].Runtime = states[devices[i].ID]
		}
	}
	return devices, nil
}

func (m *DeviceManager) Delete(ctx context.Context, id string) error {
	if err := m.repository.Delete(ctx, id); err != nil {
		return err
	}
	if m.remover != nil {
		return m.remover.Remove(ctx, id)
	}
	return nil
}

// EnsureVisible checks that the device belongs to the caller's tenant and a
// region in the allowed set. It returns ErrNoAccess when not visible.
func (m *DeviceManager) EnsureVisible(ctx context.Context, tenantID string, regionIDs []string, id string) (Device, error) {
	device, err := m.repository.Get(ctx, id)
	if err != nil {
		return Device{}, err
	}
	if device.TenantID != tenantID {
		return Device{}, ErrNoAccess
	}
	for _, regionID := range regionIDs {
		if regionID == device.RegionID {
			return device, nil
		}
	}
	return Device{}, ErrNoAccess
}
