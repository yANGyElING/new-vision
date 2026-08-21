package nodeapp

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	accessIDPattern   = regexp.MustCompile(`^[0-9]{20}$`)
	centerCodePattern = regexp.MustCompile(`^[0-9]{8}$`)
	uuidPattern       = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	ErrInvalid        = errors.New("invalid device input")
	ErrConflict       = errors.New("device already exists")
	ErrNotFound       = errors.New("device not found")
)

type Device struct {
	ID                  string        `json:"id"`
	DeviceAccessID      string        `json:"device_access_id"`
	DeviceName          string        `json:"device_name"`
	Manufacturer        string        `json:"manufacturer"`
	DeviceType          string        `json:"device_type"`
	SIPUsername         string        `json:"sip_username"`
	SIPRealm            string        `json:"sip_realm"`
	DigestAlgorithm     string        `json:"digest_algorithm"`
	DigestHA1           string        `json:"-"`
	Enabled             bool          `json:"enabled"`
	ProfileVersion      int64         `json:"profile_version"`
	AccessSyncStatus    string        `json:"access_sync_status"`
	AccessSyncedVersion *int64        `json:"access_synced_version"`
	CreatedAt           time.Time     `json:"created_at"`
	UpdatedAt           time.Time     `json:"updated_at"`
	Runtime             *RuntimeState `json:"runtime,omitempty"`
}

type CreateDeviceInput struct {
	CenterCode   string `json:"center_code"`
	DeviceType   string `json:"device_type"`
	DeviceName   string `json:"device_name"`
	Manufacturer string `json:"manufacturer"`
	SIPRealm     string `json:"sip_realm"`
	Password     string `json:"password"`
	Enabled      bool   `json:"enabled"`
}

// GB/T 28181 device type codes (position 11-13 of the 20-digit access id).
// The creation entry point picks the type, so the user never types it.
const (
	DeviceTypeIPC    = "132" // network camera
	DeviceTypeNVR    = "118"
	DeviceTypeDVR    = "111"
	DeviceTypeServer = "200" // center signaling server
)

func validDeviceType(t string) bool {
	switch t {
	case DeviceTypeIPC, DeviceTypeNVR, DeviceTypeDVR, DeviceTypeServer:
		return true
	}
	return false
}

func (in CreateDeviceInput) Validate() error {
	if !centerCodePattern.MatchString(in.CenterCode) {
		return fmt.Errorf("%w: center_code must contain exactly 8 digits", ErrInvalid)
	}
	if !validDeviceType(in.DeviceType) {
		return fmt.Errorf("%w: device_type must be one of 132 (IPC), 118 (NVR), 111 (DVR), 200 (server)", ErrInvalid)
	}
	if in.DeviceName == "" || len(in.DeviceName) > 255 {
		return fmt.Errorf("%w: device_name must be non-empty and at most 255 bytes", ErrInvalid)
	}
	if in.Manufacturer == "" || len(in.Manufacturer) > 255 {
		return fmt.Errorf("%w: manufacturer must be non-empty and at most 255 bytes", ErrInvalid)
	}
	if in.SIPRealm == "" || strings.TrimSpace(in.SIPRealm) != in.SIPRealm || len(in.SIPRealm) > 255 {
		return fmt.Errorf("%w: sip_realm must be non-empty, unpadded, and at most 255 bytes", ErrInvalid)
	}
	for i := 0; i < len(in.SIPRealm); i++ {
		if in.SIPRealm[i] < 0x20 || in.SIPRealm[i] == 0x7f {
			return fmt.Errorf("%w: sip_realm contains a control character", ErrInvalid)
		}
	}
	if in.Password == "" || len(in.Password) > 256 {
		return fmt.Errorf("%w: password must be between 1 and 256 bytes", ErrInvalid)
	}
	return nil
}

// accessIDPrefix builds the fixed 14-digit prefix of the GB/T 28181 code:
// center(8) + industry(2, fixed 00) + type(3) + network(1, fixed 0). The
// 6-digit sequence is allocated per prefix by the repository.
func (in CreateDeviceInput) accessIDPrefix() string {
	return in.CenterCode + "00" + in.DeviceType + "0"
}

func DeriveHA1(username, realm, password string) string {
	sum := md5.Sum([]byte(username + ":" + realm + ":" + password))
	return hex.EncodeToString(sum[:])
}

func (d Device) AccessProfile() AccessProfile {
	return AccessProfile{
		DeviceAccessID:  d.DeviceAccessID,
		SIPUsername:     d.SIPUsername,
		SIPRealm:        d.SIPRealm,
		DigestAlgorithm: d.DigestAlgorithm,
		DigestHA1:       d.DigestHA1,
		Enabled:         d.Enabled,
		Version:         d.ProfileVersion,
	}
}

type DeviceRepository interface {
	Create(context.Context, CreateDeviceInput) (Device, error)
	Get(context.Context, string) (Device, error)
	SetEnabled(context.Context, string, bool) (Device, error)
	UpdateMeta(context.Context, string, *string, *string) (Device, error)
	GetByAccessID(context.Context, string) (Device, error)
	List(context.Context) ([]Device, error)
	NextPending(context.Context) (Device, bool, error)
	MarkSynced(context.Context, string, int64) error
	MarkReconciled(context.Context, []ReconciledProfile) error
	MarkFailed(context.Context, string, time.Duration, string) error
	Delete(context.Context, string) error
}

type ReconciledProfile struct {
	DeviceID string
	Version  int64
}

type PostgresDeviceRepository struct{ pool *pgxpool.Pool }

func NewPostgresDeviceRepository(pool *pgxpool.Pool) *PostgresDeviceRepository {
	return &PostgresDeviceRepository{pool: pool}
}

const deviceColumns = `id, device_access_id, device_name, manufacturer, device_type, sip_username, sip_realm, digest_algorithm, digest_ha1,
 enabled, profile_version, access_sync_status, access_synced_version, created_at, updated_at`

func scanDevice(row pgx.Row) (Device, error) {
	var d Device
	err := row.Scan(&d.ID, &d.DeviceAccessID, &d.DeviceName, &d.Manufacturer, &d.DeviceType,
		&d.SIPUsername, &d.SIPRealm, &d.DigestAlgorithm, &d.DigestHA1,
		&d.Enabled, &d.ProfileVersion, &d.AccessSyncStatus, &d.AccessSyncedVersion, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func (r *PostgresDeviceRepository) Create(ctx context.Context, in CreateDeviceInput) (Device, error) {
	accessID, err := r.allocateAccessID(ctx, in.accessIDPrefix())
	if err != nil {
		return Device{}, err
	}
	ha1 := DeriveHA1(accessID, in.SIPRealm, in.Password)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Device{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	d, err := scanDevice(tx.QueryRow(ctx, `INSERT INTO devices
 (device_access_id, device_name, manufacturer, device_type, sip_username, sip_realm, digest_ha1, enabled)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+deviceColumns,
		accessID, in.DeviceName, in.Manufacturer, in.DeviceType, accessID, in.SIPRealm, ha1, in.Enabled))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Device{}, ErrConflict
		}
		return Device{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO access_profile_outbox (device_id, profile_version) VALUES ($1,$2)`, d.ID, d.ProfileVersion); err != nil {
		return Device{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Device{}, err
	}
	return d, nil
}

// allocateAccessID returns the next 20-digit GB/T 28181 code for the given
// 14-digit prefix (center+industry+type+network). It picks the largest
// existing sequence for that prefix and increments it, starting at 1 when
// the prefix is unused. Concurrent creators may collide on the UNIQUE
// device_access_id constraint and must retry on ErrConflict.
func (r *PostgresDeviceRepository) allocateAccessID(ctx context.Context, prefix string) (string, error) {
	var maxSeq sql.NullInt64
	if err := r.pool.QueryRow(ctx,
		`SELECT MAX(CAST(SUBSTRING(device_access_id FROM 15 FOR 6) AS INTEGER)) FROM devices WHERE device_access_id LIKE $1 || '%'`,
		prefix).Scan(&maxSeq); err != nil {
		return "", err
	}
	next := int64(1)
	if maxSeq.Valid {
		next = maxSeq.Int64 + 1
	}
	if next > 999999 {
		return "", fmt.Errorf("%w: sequence exhausted for prefix %s", ErrInvalid, prefix)
	}
	return prefix + fmt.Sprintf("%06d", next), nil
}

func (r *PostgresDeviceRepository) Get(ctx context.Context, id string) (Device, error) {
	if !uuidPattern.MatchString(id) {
		return Device{}, ErrNotFound
	}
	d, err := scanDevice(r.pool.QueryRow(ctx, `SELECT `+deviceColumns+` FROM devices WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	return d, err
}

func (r *PostgresDeviceRepository) GetByAccessID(ctx context.Context, accessID string) (Device, error) {
	d, err := scanDevice(r.pool.QueryRow(ctx, `SELECT `+deviceColumns+` FROM devices WHERE device_access_id=$1`, accessID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	return d, err
}

func (r *PostgresDeviceRepository) SetEnabled(ctx context.Context, id string, enabled bool) (Device, error) {
	if !uuidPattern.MatchString(id) {
		return Device{}, ErrNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Device{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	d, err := scanDevice(tx.QueryRow(ctx, `SELECT `+deviceColumns+` FROM devices WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	if err != nil {
		return Device{}, err
	}
	if d.Enabled == enabled {
		if err = tx.Commit(ctx); err != nil {
			return Device{}, err
		}
		return d, nil
	}
	d, err = scanDevice(tx.QueryRow(ctx, `UPDATE devices SET enabled=$2, profile_version=profile_version+1,
 access_sync_status='pending', updated_at=now() WHERE id=$1 RETURNING `+deviceColumns, id, enabled))
	if err != nil {
		return Device{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO access_profile_outbox (device_id, profile_version) VALUES ($1,$2)`, d.ID, d.ProfileVersion); err != nil {
		return Device{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Device{}, err
	}
	return d, nil
}

func (r *PostgresDeviceRepository) UpdateMeta(ctx context.Context, id string, name, manufacturer *string) (Device, error) {
	if !uuidPattern.MatchString(id) {
		return Device{}, ErrNotFound
	}
	if name == nil && manufacturer == nil {
		return Device{}, fmt.Errorf("%w: no fields to update", ErrInvalid)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Device{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	d, err := scanDevice(tx.QueryRow(ctx, `SELECT `+deviceColumns+` FROM devices WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	if err != nil {
		return Device{}, err
	}
	// Metadata changes do NOT touch profile_version or the outbox: they are
	// management-only fields and are never synced to the access layer.
	if name != nil {
		d.DeviceName = *name
	}
	if manufacturer != nil {
		d.Manufacturer = *manufacturer
	}
	d, err = scanDevice(tx.QueryRow(ctx, `UPDATE devices SET device_name=$2, manufacturer=$3, updated_at=now()
 WHERE id=$1 RETURNING `+deviceColumns, id, d.DeviceName, d.Manufacturer))
	if err != nil {
		return Device{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Device{}, err
	}
	return d, nil
}

func (r *PostgresDeviceRepository) List(ctx context.Context) ([]Device, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+deviceColumns+` FROM devices ORDER BY device_access_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	devices := make([]Device, 0)
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

func (r *PostgresDeviceRepository) Delete(ctx context.Context, id string) error {
	if !uuidPattern.MatchString(id) {
		return ErrNotFound
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM devices WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresDeviceRepository) NextPending(ctx context.Context) (Device, bool, error) {
	d, err := scanDevice(r.pool.QueryRow(ctx, `SELECT `+deviceColumns+` FROM devices d
 WHERE EXISTS (SELECT 1 FROM access_profile_outbox o WHERE o.device_id=d.id AND o.processed_at IS NULL AND o.next_attempt_at <= now())
 ORDER BY (SELECT min(o.id) FROM access_profile_outbox o WHERE o.device_id=d.id AND o.processed_at IS NULL) LIMIT 1`))
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, false, nil
	}
	return d, err == nil, err
}

func (r *PostgresDeviceRepository) MarkSynced(ctx context.Context, id string, version int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `UPDATE devices SET access_synced_version=GREATEST(COALESCE(access_synced_version,0),$2),
 access_sync_status=CASE WHEN profile_version <= $2 THEN 'synced' ELSE 'pending' END, updated_at=now() WHERE id=$1`, id, version); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE access_profile_outbox SET processed_at=now(), last_error=NULL WHERE device_id=$1 AND profile_version <= $2 AND processed_at IS NULL`, id, version); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresDeviceRepository) MarkReconciled(ctx context.Context, profiles []ReconciledProfile) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, profile := range profiles {
		if _, err = tx.Exec(ctx, `UPDATE devices SET access_synced_version=GREATEST(COALESCE(access_synced_version,0),$2),
 access_sync_status=CASE WHEN profile_version <= GREATEST(COALESCE(access_synced_version,0),$2) THEN 'synced' ELSE 'pending' END,
 updated_at=now() WHERE id=$1`, profile.DeviceID, profile.Version); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE access_profile_outbox SET processed_at=now(), last_error=NULL
 WHERE device_id=$1 AND profile_version <= $2 AND processed_at IS NULL`, profile.DeviceID, profile.Version); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresDeviceRepository) MarkFailed(ctx context.Context, id string, interval time.Duration, message string) error {
	if len(message) > 1000 {
		message = message[:1000]
	}
	// attempt_count persists across restarts and drives the exponential backoff;
	// all SET expressions see the pre-update row, so the multiplier uses the
	// count before this failure (first failure doubles nothing, then 2x, 4x, ...).
	_, err := r.pool.Exec(ctx, `UPDATE access_profile_outbox
 SET attempt_count = attempt_count + 1,
     next_attempt_at = now() + LEAST($2::bigint * (1 << LEAST(attempt_count, 15)) * interval '1 microsecond', interval '1 minute'),
     last_error = $3
 WHERE device_id = $1 AND processed_at IS NULL`, id, interval.Microseconds(), message)
	return err
}
