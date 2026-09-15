// Package channel contains the catalog channel business: the channels
// read model (design D41-D51), the catalog event consumer that applies
// device catalog reports to PostgreSQL, and the read-only HTTP surface for
// the management and user pages.
package channel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/new-vision-lab/new-vision/internal/nodeapp/access"
	"github.com/new-vision-lab/new-vision/internal/nodeapp/device"
)

var (
	ErrInvalid  = errors.New("invalid channel input")
	ErrNotFound = errors.New("device not found")
)

// Channel is a stored channel row (management-page view, all columns).
type Channel struct {
	ID                  string     `json:"id"`
	DeviceID            string     `json:"device_id"`
	ChannelCode         string     `json:"channel_code"`
	ReportName          *string    `json:"report_name"`
	DisplayName         *string    `json:"display_name"`
	OrgUnitID           *string    `json:"org_unit_id,omitempty"`
	ReportedStatus      *string    `json:"reported_status"`
	Missing             bool       `json:"missing"`
	LastSeenInCatalogAt *time.Time `json:"last_seen_in_catalog_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// DeviceBrief is the device slice of the user-page channels response: the
// banner (sync three-state) and the offline group need device-level facts
// even for devices that reported no channels yet.
type DeviceBrief struct {
	ID              string     `json:"id"`
	DeviceName      string     `json:"device_name"`
	State           string     `json:"state"` // runtime online/offline
	CatalogState    string     `json:"catalog_state"`
	CatalogLastOkAt *time.Time `json:"catalog_last_ok_at"`
}

// ChannelView is a flat user-page channel row: display name resolved with
// COALESCE(display_name, report_name) (D42) and the resolved org anchor
// COALESCE(channels.org_unit_id, devices.org_unit_id) (D36).
type ChannelView struct {
	ID                  string     `json:"id"`
	DeviceID            string     `json:"device_id"`
	ChannelCode         string     `json:"channel_code"`
	Name                string     `json:"name"`
	ReportName          *string    `json:"report_name"`
	DisplayName         *string    `json:"display_name"`
	ReportedStatus      *string    `json:"reported_status"`
	Missing             bool       `json:"missing"`
	LastSeenInCatalogAt *time.Time `json:"last_seen_in_catalog_at"`
	OrgUnitID           *string    `json:"org_unit_id,omitempty"`
	OrgUnitName         *string    `json:"org_unit_name,omitempty"`
}

// CatalogDeviceResult is the response of GET /api/v1/devices/{id}/channels.
type CatalogDeviceResult struct {
	DeviceID         string     `json:"device_id"`
	CatalogState     string     `json:"catalog_state"`
	CatalogLastOkAt  *time.Time `json:"catalog_last_ok_at"`
	CatalogLastCount *int       `json:"catalog_last_count"`
	CatalogLastError *string    `json:"catalog_last_error,omitempty"`
	Channels         []Channel  `json:"channels"`
}

// CatalogListResult is the response of GET /api/v1/channels.
type CatalogListResult struct {
	Devices  []DeviceBrief `json:"devices"`
	Channels []ChannelView `json:"channels"`
}

// DeviceStateWriter is the device-table catalog state surface used by the
// consumer (implemented by the device repository).
type DeviceStateWriter interface {
	SetCatalogInProgress(context.Context, string) error
	SetCatalogOK(context.Context, string, int) error
	SetCatalogFailed(context.Context, string, int, int, string) error
}

// OrgTreeResolver expands an org unit id to its subtree (used for the
// org_unit_id filter on the user-page listing).
type OrgTreeResolver interface {
	SubtreeIDs(ctx context.Context, ids []string) ([]string, error)
}

// Repository is the channels table surface.
type Repository interface {
	ListByDevice(ctx context.Context, deviceID string) ([]Channel, error)
	ListVisible(ctx context.Context, tenantID string, allowedOrgUnitIDs []string, includeUnassigned bool, filterOrgUnitIDs []string) (CatalogListResult, error)
	ApplyResult(ctx context.Context, d device.Device, channels []access.CatalogChannel) error
}

type PostgresChannelRepository struct{ pool *pgxpool.Pool }

func NewPostgresChannelRepository(pool *pgxpool.Pool) *PostgresChannelRepository {
	return &PostgresChannelRepository{pool: pool}
}

const channelColumns = `id, device_id, channel_code, report_name, display_name, org_unit_id, reported_status, missing, last_seen_in_catalog_at, created_at, updated_at`

func scanChannel(row pgx.Row) (Channel, error) {
	var c Channel
	err := row.Scan(&c.ID, &c.DeviceID, &c.ChannelCode, &c.ReportName, &c.DisplayName, &c.OrgUnitID,
		&c.ReportedStatus, &c.Missing, &c.LastSeenInCatalogAt, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (r *PostgresChannelRepository) ListByDevice(ctx context.Context, deviceID string) ([]Channel, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+channelColumns+` FROM channels WHERE device_id = $1 ORDER BY channel_code`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	channels := make([]Channel, 0)
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, c)
	}
	return channels, rows.Err()
}

// ListVisible returns the user-page flat channel list plus device briefs for
// every visible device. Org visibility follows D36:
// COALESCE(channels.org_unit_id, devices.org_unit_id) must fall inside the
// caller's visible org set; full-visibility callers (node_admin / all_orgs)
// see the whole tenant. filterOrgUnitIDs (optional, already subtree-
// expanded) narrows the listing to those org units.
func (r *PostgresChannelRepository) ListVisible(ctx context.Context, tenantID string, allowedOrgUnitIDs []string, includeUnassigned bool, filterOrgUnitIDs []string) (CatalogListResult, error) {
	base := `
FROM devices d
LEFT JOIN channels c ON c.device_id = d.id
LEFT JOIN org_units ou ON ou.id = COALESCE(c.org_unit_id, d.org_unit_id)`
	where := " WHERE d.tenant_id = $1"
	args := []any{tenantID}
	if !includeUnassigned {
		if len(allowedOrgUnitIDs) == 0 {
			return CatalogListResult{Devices: []DeviceBrief{}, Channels: []ChannelView{}}, nil
		}
		args = append(args, allowedOrgUnitIDs)
		where += fmt.Sprintf(" AND COALESCE(c.org_unit_id, d.org_unit_id) = ANY($%d)", len(args))
	}
	if len(filterOrgUnitIDs) > 0 {
		args = append(args, filterOrgUnitIDs)
		where += fmt.Sprintf(" AND COALESCE(c.org_unit_id, d.org_unit_id) = ANY($%d)", len(args))
	}
	deviceRows, err := r.pool.Query(ctx, `SELECT DISTINCT d.id, d.device_name,
 COALESCE(d.catalog_state, 'never'), d.catalog_last_ok_at`+base+where+` ORDER BY d.id`, args...)
	if err != nil {
		return CatalogListResult{}, err
	}
	defer deviceRows.Close()
	devices := make([]DeviceBrief, 0)
	for deviceRows.Next() {
		var b DeviceBrief
		if err := deviceRows.Scan(&b.ID, &b.DeviceName, &b.CatalogState, &b.CatalogLastOkAt); err != nil {
			return CatalogListResult{}, err
		}
		b.State = "offline"
		devices = append(devices, b)
	}
	if err := deviceRows.Err(); err != nil {
		return CatalogListResult{}, err
	}
	channelRows, err := r.pool.Query(ctx, `SELECT c.id, c.device_id, c.channel_code, c.report_name, c.display_name,
 c.reported_status, c.missing, c.last_seen_in_catalog_at,
 COALESCE(c.org_unit_id, d.org_unit_id), ou.name`+base+where+` AND c.id IS NOT NULL ORDER BY d.device_access_id, c.channel_code`, args...)
	if err != nil {
		return CatalogListResult{}, err
	}
	defer channelRows.Close()
	channels := make([]ChannelView, 0)
	for channelRows.Next() {
		var v ChannelView
		if err := channelRows.Scan(&v.ID, &v.DeviceID, &v.ChannelCode, &v.ReportName, &v.DisplayName,
			&v.ReportedStatus, &v.Missing, &v.LastSeenInCatalogAt, &v.OrgUnitID, &v.OrgUnitName); err != nil {
			return CatalogListResult{}, err
		}
		v.Name = resolveName(v.DisplayName, v.ReportName)
		channels = append(channels, v)
	}
	return CatalogListResult{Devices: devices, Channels: channels}, channelRows.Err()
}

func resolveName(display, report *string) string {
	if display != nil && *display != "" {
		return *display
	}
	if report != nil {
		return *report
	}
	return ""
}

// ApplyResult applies one complete catalog report in a single transaction:
// upsert every reported channel (D41 identity = device_id + channel_code,
// D43 keep old values for fields the device did not report), revive missing
// channels, then mark channels absent from this report as missing (D45/D46).
func (r *PostgresChannelRepository) ApplyResult(ctx context.Context, d device.Device, channels []access.CatalogChannel) error {
	if len(channels) == 0 {
		return r.applyEmpty(ctx, d.ID)
	}
	codes := make([]string, len(channels))
	names := make([]string, len(channels))
	statuses := make([]string, len(channels))
	for i, ch := range channels {
		codes[i] = ch.Code
		names[i] = ch.Name
		statuses[i] = ch.Status
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `
INSERT INTO channels (tenant_id, device_id, channel_code, report_name, reported_status, missing, last_seen_in_catalog_at)
SELECT $1, $2, code, NULLIF(name, ''), NULLIF(status, ''), FALSE, now()
FROM unnest($3::text[], $4::text[], $5::text[]) AS t(code, name, status)
ON CONFLICT (device_id, channel_code) DO UPDATE SET
 report_name = COALESCE(EXCLUDED.report_name, channels.report_name),
 reported_status = COALESCE(EXCLUDED.reported_status, channels.reported_status),
 missing = FALSE,
 last_seen_in_catalog_at = now(),
 updated_at = now()`,
		d.TenantID, d.ID, codes, names, statuses); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE channels SET missing = TRUE, updated_at = now()
 WHERE device_id = $1 AND NOT (channel_code = ANY($2))`, d.ID, codes); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// applyEmpty handles a device that reports an empty catalog: every stored
// channel becomes missing (the device no longer owns any of them).
func (r *PostgresChannelRepository) applyEmpty(ctx context.Context, deviceID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE channels SET missing = TRUE, updated_at = now() WHERE device_id = $1`, deviceID)
	return err
}

// Consumer applies catalog access events to PostgreSQL. It implements the
// consumer interface expected by the sync runner.
type Consumer struct {
	repositories Repository
	devices      DeviceStateWriter
}

func NewConsumer(repositories Repository, devices DeviceStateWriter) *Consumer {
	return &Consumer{repositories: repositories, devices: devices}
}

// ApplyProgress handles catalog.progress: state marker + query timestamp
// only (D50 — the numbers ride the SSE stream in a later task).
func (c *Consumer) ApplyProgress(ctx context.Context, d device.Device, progress access.CatalogProgressPayload) error {
	return c.devices.SetCatalogInProgress(ctx, d.ID)
}

// ApplyResult handles catalog.result: ok writes channels then flips the
// device to ok; failure only records the error and keeps all channel data.
// The channel list is event-stream input, so identity rules are validated
// here, before any write.
func (c *Consumer) ApplyResult(ctx context.Context, d device.Device, result access.CatalogResultPayload) error {
	if !result.OK {
		return c.devices.SetCatalogFailed(ctx, d.ID, result.Received, result.Total, result.Error)
	}
	if err := validateChannels(result.Channels); err != nil {
		return err
	}
	if err := c.repositories.ApplyResult(ctx, d, result.Channels); err != nil {
		return err
	}
	return c.devices.SetCatalogOK(ctx, d.ID, len(result.Channels))
}

// validateChannels enforces the channel identity rules (D41): non-empty
// code bounded to the column width, unique within one report.
func validateChannels(channels []access.CatalogChannel) error {
	seen := make(map[string]bool, len(channels))
	for _, ch := range channels {
		if ch.Code == "" || len(ch.Code) > 20 {
			return fmt.Errorf("%w: bad channel code %q", ErrInvalid, ch.Code)
		}
		if seen[ch.Code] {
			return fmt.Errorf("%w: duplicate channel code %q", ErrInvalid, ch.Code)
		}
		seen[ch.Code] = true
	}
	return nil
}
