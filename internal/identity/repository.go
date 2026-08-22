package identity

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound    = errors.New("identity: not found")
	ErrConflict    = errors.New("identity: already exists")
	ErrInUse       = errors.New("identity: in use")
	ErrInvalid     = errors.New("identity: invalid input")
	ErrNoPermission = errors.New("identity: permission denied")
)

// InvalidError carries a validation message for the handler layer.
type InvalidError struct{ msg string }

func (e *InvalidError) Error() string { return e.msg }

func invalid(msg string) error { return &InvalidError{msg: "identity: " + msg} }

type PostgresTenantRepository struct{ pool *pgxpool.Pool }

func NewPostgresTenantRepository(pool *pgxpool.Pool) *PostgresTenantRepository {
	return &PostgresTenantRepository{pool: pool}
}

const tenantColumns = `id, name, status, created_at, updated_at`

func scanTenant(row pgx.Row) (Tenant, error) {
	var t Tenant
	err := row.Scan(&t.ID, &t.Name, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (r *PostgresTenantRepository) Create(ctx context.Context, in CreateTenantInput) (Tenant, error) {
	t, err := scanTenant(r.pool.QueryRow(ctx,
		`INSERT INTO tenants (name) VALUES ($1) RETURNING `+tenantColumns, in.Name))
	if err != nil {
		if isUniqueViolation(err) {
			return Tenant{}, ErrConflict
		}
		return Tenant{}, err
	}
	return t, nil
}

func (r *PostgresTenantRepository) List(ctx context.Context) ([]Tenant, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+tenantColumns+` FROM tenants ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tenants := []Tenant{}
	for rows.Next() {
		t, err := scanTenant(rows)
		if err != nil {
			return nil, err
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

func (r *PostgresTenantRepository) Get(ctx context.Context, id string) (Tenant, error) {
	t, err := scanTenant(r.pool.QueryRow(ctx,
		`SELECT `+tenantColumns+` FROM tenants WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Tenant{}, ErrNotFound
	}
	return t, err
}

func (r *PostgresTenantRepository) GetByName(ctx context.Context, name string) (Tenant, error) {
	t, err := scanTenant(r.pool.QueryRow(ctx,
		`SELECT `+tenantColumns+` FROM tenants WHERE name = $1`, name))
	if errors.Is(err, pgx.ErrNoRows) {
		return Tenant{}, ErrNotFound
	}
	return t, err
}

func (r *PostgresTenantRepository) SetStatus(ctx context.Context, id, status string) (Tenant, error) {
	t, err := scanTenant(r.pool.QueryRow(ctx,
		`UPDATE tenants SET status = $2, updated_at = now() WHERE id = $1 RETURNING `+tenantColumns, id, status))
	if errors.Is(err, pgx.ErrNoRows) {
		return Tenant{}, ErrNotFound
	}
	return t, err
}

type PostgresRegionRepository struct{ pool *pgxpool.Pool }

func NewPostgresRegionRepository(pool *pgxpool.Pool) *PostgresRegionRepository {
	return &PostgresRegionRepository{pool: pool}
}

const regionColumns = `id, parent_id, name, created_at`

func scanRegion(row pgx.Row) (Region, error) {
	var r Region
	var parentID *string
	err := row.Scan(&r.ID, &parentID, &r.Name, &r.CreatedAt)
	r.ParentID = parentID
	return r, err
}

func (r *PostgresRegionRepository) Create(ctx context.Context, parentID, name string) (Region, error) {
	var parent any
	if parentID == "" {
		parent = nil
	} else {
		parent = parentID
	}
	region, err := scanRegion(r.pool.QueryRow(ctx,
		`INSERT INTO regions (parent_id, name) VALUES ($1, $2) RETURNING `+regionColumns, parent, name))
	if err != nil {
		if isUniqueViolation(err) {
			return Region{}, ErrConflict
		}
		return Region{}, err
	}
	return region, nil
}

func (r *PostgresRegionRepository) Get(ctx context.Context, id string) (Region, error) {
	region, err := scanRegion(r.pool.QueryRow(ctx,
		`SELECT `+regionColumns+` FROM regions WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Region{}, ErrNotFound
	}
	return region, err
}

func (r *PostgresRegionRepository) UpdateName(ctx context.Context, id, name string) (Region, error) {
	region, err := scanRegion(r.pool.QueryRow(ctx,
		`UPDATE regions SET name = $2 WHERE id = $1 RETURNING `+regionColumns, id, name))
	if errors.Is(err, pgx.ErrNoRows) {
		return Region{}, ErrNotFound
	}
	if err != nil && isUniqueViolation(err) {
		return Region{}, ErrConflict
	}
	return region, err
}

func (r *PostgresRegionRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM regions WHERE id = $1`, id)
	if err != nil {
		if isForeignKeyViolation(err) {
			return ErrInUse
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Tree loads all regions once and assembles the forest in memory.
func (r *PostgresRegionRepository) Tree(ctx context.Context) ([]*Region, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+regionColumns+` FROM regions ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[string]*Region{}
	var roots []*Region
	for rows.Next() {
		region, err := scanRegion(rows)
		if err != nil {
			return nil, err
		}
		node := &Region{ID: region.ID, ParentID: region.ParentID, Name: region.Name, CreatedAt: region.CreatedAt}
		byID[node.ID] = node
		if node.ParentID == nil {
			roots = append(roots, node)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, node := range byID {
		if node.ParentID != nil {
			if parent, ok := byID[*node.ParentID]; ok {
				parent.Children = append(parent.Children, node)
			}
		}
	}
	return roots, nil
}

// SubtreeIDs returns the region id and all descendant ids via recursive CTE.
func (r *PostgresRegionRepository) SubtreeIDs(ctx context.Context, regionID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		WITH RECURSIVE subtree AS (
			SELECT id FROM regions WHERE id = $1
			UNION ALL
			SELECT c.id FROM regions c JOIN subtree s ON c.parent_id = s.id
		)
		SELECT id FROM subtree`, regionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, ErrNotFound
	}
	return ids, rows.Err()
}

type PostgresUserRepository struct{ pool *pgxpool.Pool }

func NewPostgresUserRepository(pool *pgxpool.Pool) *PostgresUserRepository {
	return &PostgresUserRepository{pool: pool}
}

const userColumns = `id, tenant_id, username, password_hash, display_name, status, created_at, updated_at`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.TenantID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (r *PostgresUserRepository) loadRoles(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT role FROM user_roles WHERE user_id = $1 ORDER BY role`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *PostgresUserRepository) loadRegionScopes(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT region_id FROM user_region_scopes WHERE user_id = $1 ORDER BY region_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *PostgresUserRepository) hydrate(ctx context.Context, u User) (User, error) {
	roles, err := r.loadRoles(ctx, u.ID)
	if err != nil {
		return User{}, err
	}
	regionIDs, err := r.loadRegionScopes(ctx, u.ID)
	if err != nil {
		return User{}, err
	}
	u.Roles = roles
	u.RegionIDs = regionIDs
	return u, nil
}

func (r *PostgresUserRepository) Create(ctx context.Context, in CreateUserInput, passwordHash string) (User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	u, err := scanUser(tx.QueryRow(ctx,
		`INSERT INTO users (tenant_id, username, password_hash, display_name)
		 VALUES ($1, $2, $3, $4) RETURNING `+userColumns,
		in.TenantID, in.Username, passwordHash, in.DisplayName))
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrConflict
		}
		return User{}, err
	}
	if err := replaceRoles(ctx, tx, u.ID, in.Roles); err != nil {
		return User{}, err
	}
	if err := replaceRegionScopes(ctx, tx, u.ID, in.RegionIDs); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}
	return r.hydrate(ctx, u)
}

func (r *PostgresUserRepository) List(ctx context.Context, tenantID string) ([]User, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+userColumns+` FROM users WHERE tenant_id = $1 ORDER BY username`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range users {
		users[i], err = r.hydrate(ctx, users[i])
		if err != nil {
			return nil, err
		}
	}
	return users, nil
}

func (r *PostgresUserRepository) Get(ctx context.Context, tenantID, id string) (User, error) {
	u, err := scanUser(r.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE tenant_id = $1 AND id = $2`, tenantID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	return r.hydrate(ctx, u)
}

func (r *PostgresUserRepository) GetByUsername(ctx context.Context, tenantID, username string) (User, error) {
	u, err := scanUser(r.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE tenant_id = $1 AND username = $2`, tenantID, username))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	return r.hydrate(ctx, u)
}

func (r *PostgresUserRepository) Update(ctx context.Context, tenantID, id string, in UpdateUserInput) (User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var displayName, status any
	if in.DisplayName != nil {
		displayName = *in.DisplayName
	}
	if in.Status != nil {
		status = *in.Status
	}
	u, err := scanUser(tx.QueryRow(ctx,
		`UPDATE users SET
			display_name = COALESCE($3, display_name),
			status = COALESCE($4, status),
			updated_at = now()
		 WHERE tenant_id = $1 AND id = $2 RETURNING `+userColumns,
		tenantID, id, displayName, status))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	if in.Roles != nil {
		if err := replaceRoles(ctx, tx, u.ID, in.Roles); err != nil {
			return User{}, err
		}
	}
	if in.RegionIDs != nil {
		if err := replaceRegionScopes(ctx, tx, u.ID, in.RegionIDs); err != nil {
			return User{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}
	return r.hydrate(ctx, u)
}

func (r *PostgresUserRepository) Delete(ctx context.Context, tenantID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM users WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresUserRepository) SetPassword(ctx context.Context, tenantID, id, passwordHash string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET password_hash = $3, updated_at = now() WHERE tenant_id = $1 AND id = $2`,
		tenantID, id, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type execer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func replaceRoles(ctx context.Context, tx execer, userID string, roles []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, userID); err != nil {
		return err
	}
	for _, role := range roles {
		if _, err := tx.Exec(ctx,
			`INSERT INTO user_roles (user_id, role) VALUES ($1, $2) ON CONFLICT (user_id, role) DO NOTHING`,
			userID, role); err != nil {
			return err
		}
	}
	return nil
}

func replaceRegionScopes(ctx context.Context, tx execer, userID string, regionIDs []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM user_region_scopes WHERE user_id = $1`, userID); err != nil {
		return err
	}
	for _, regionID := range regionIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO user_region_scopes (user_id, region_id) VALUES ($1, $2) ON CONFLICT (user_id, region_id) DO NOTHING`,
			userID, regionID); err != nil {
			return err
		}
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
