package nodeapp

import (
	"context"
	"crypto/md5"
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
	accessIDPattern = regexp.MustCompile(`^[0-9]{20}$`)
	uuidPattern     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	ErrInvalid      = errors.New("invalid device input")
	ErrConflict     = errors.New("device already exists")
	ErrNotFound     = errors.New("device not found")
)

type Device struct {
	ID                  string        `json:"id"`
	DeviceAccessID      string        `json:"device_access_id"`
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
	DeviceAccessID string `json:"device_access_id"`
	SIPUsername    string `json:"sip_username"`
	SIPRealm       string `json:"sip_realm"`
	Password       string `json:"password"`
	Enabled        bool   `json:"enabled"`
}

func (in CreateDeviceInput) Validate() error {
	if !accessIDPattern.MatchString(in.DeviceAccessID) {
		return fmt.Errorf("%w: device_access_id must contain exactly 20 digits", ErrInvalid)
	}
	if in.SIPUsername != in.DeviceAccessID {
		return fmt.Errorf("%w: sip_username must equal device_access_id", ErrInvalid)
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
	Create(context.Context, CreateDeviceInput, string) (Device, error)
	Get(context.Context, string) (Device, error)
	SetEnabled(context.Context, string, bool) (Device, error)
	GetByAccessID(context.Context, string) (Device, error)
	List(context.Context) ([]Device, error)
	NextPending(context.Context) (Device, bool, error)
	MarkSynced(context.Context, string, int64) error
	MarkReconciled(context.Context, []ReconciledProfile) error
	MarkFailed(context.Context, string, int, time.Time, string) error
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

const deviceColumns = `id, device_access_id, sip_username, sip_realm, digest_algorithm, digest_ha1,
 enabled, profile_version, access_sync_status, access_synced_version, created_at, updated_at`

func scanDevice(row pgx.Row) (Device, error) {
	var d Device
	err := row.Scan(&d.ID, &d.DeviceAccessID, &d.SIPUsername, &d.SIPRealm, &d.DigestAlgorithm, &d.DigestHA1,
		&d.Enabled, &d.ProfileVersion, &d.AccessSyncStatus, &d.AccessSyncedVersion, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func (r *PostgresDeviceRepository) Create(ctx context.Context, in CreateDeviceInput, ha1 string) (Device, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Device{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	d, err := scanDevice(tx.QueryRow(ctx, `INSERT INTO devices
 (device_access_id, sip_username, sip_realm, digest_ha1, enabled)
 VALUES ($1,$2,$3,$4,$5) RETURNING `+deviceColumns,
		in.DeviceAccessID, in.SIPUsername, in.SIPRealm, ha1, in.Enabled))
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

func (r *PostgresDeviceRepository) MarkFailed(ctx context.Context, id string, attempts int, next time.Time, message string) error {
	if len(message) > 1000 {
		message = message[:1000]
	}
	_, err := r.pool.Exec(ctx, `UPDATE access_profile_outbox SET attempt_count=$2, next_attempt_at=$3, last_error=$4
 WHERE device_id=$1 AND processed_at IS NULL`, id, attempts, next, message)
	return err
}
