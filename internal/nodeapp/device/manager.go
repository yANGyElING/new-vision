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

func (m *DeviceManager) SetOrgUnit(ctx context.Context, id string, orgUnitID *string) (Device, error) {
	return m.repository.SetOrgUnit(ctx, id, orgUnitID)
}

// List returns devices visible to the given tenant and org set.
func (m *DeviceManager) List(ctx context.Context, tenantID string, orgUnitIDs []string, includeUnassigned bool) ([]Device, error) {
	devices, err := m.repository.ListByTenant(ctx, tenantID, orgUnitIDs, includeUnassigned)
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

// EnsureVisible checks that the device belongs to the caller's tenant and is
// inside the allowed org set. Full-visibility callers (includeUnassigned,
// i.e. node_admin / all_orgs) see every device in their tenant, including
// unassigned ones. Scoped callers see only devices whose org unit is inside
// the allowed set; unassigned devices (org_unit_id NULL) are never visible
// to scoped callers. It returns ErrNoAccess when not visible.
func (m *DeviceManager) EnsureVisible(ctx context.Context, tenantID string, orgUnitIDs []string, includeUnassigned bool, id string) (Device, error) {
	device, err := m.repository.Get(ctx, id)
	if err != nil {
		return Device{}, err
	}
	if device.TenantID != tenantID {
		return Device{}, ErrNoAccess
	}
	if includeUnassigned {
		return device, nil
	}
	if device.OrgUnitID == nil {
		return Device{}, ErrNoAccess
	}
	for _, orgUnitID := range orgUnitIDs {
		if orgUnitID == *device.OrgUnitID {
			return device, nil
		}
	}
	return Device{}, ErrNoAccess
}
