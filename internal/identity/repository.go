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

type PostgresOrgUnitRepository struct{ pool *pgxpool.Pool }

func NewPostgresOrgUnitRepository(pool *pgxpool.Pool) *PostgresOrgUnitRepository {
	return &PostgresOrgUnitRepository{pool: pool}
}

const orgUnitColumns = `id, tenant_id, parent_id, name, created_at`

func scanOrgUnit(row pgx.Row) (OrgUnit, error) {
	var o OrgUnit
	var parentID *string
	err := row.Scan(&o.ID, &o.TenantID, &parentID, &o.Name, &o.CreatedAt)
	o.ParentID = parentID
	return o, err
}

func (r *PostgresOrgUnitRepository) Create(ctx context.Context, tenantID string, parentID *string, name string) (OrgUnit, error) {
	orgUnit, err := scanOrgUnit(r.pool.QueryRow(ctx,
		`INSERT INTO org_units (tenant_id, parent_id, name) VALUES ($1, $2, $3) RETURNING `+orgUnitColumns,
		tenantID, parentID, name))
	if err != nil {
		if isUniqueViolation(err) {
			return OrgUnit{}, ErrConflict
		}
		return OrgUnit{}, err
	}
	return orgUnit, nil
}

func (r *PostgresOrgUnitRepository) Get(ctx context.Context, tenantID, id string) (OrgUnit, error) {
	orgUnit, err := scanOrgUnit(r.pool.QueryRow(ctx,
		`SELECT `+orgUnitColumns+` FROM org_units WHERE tenant_id = $1 AND id = $2`, tenantID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return OrgUnit{}, ErrNotFound
	}
	return orgUnit, err
}

func (r *PostgresOrgUnitRepository) UpdateName(ctx context.Context, tenantID, id, name string) (OrgUnit, error) {
	orgUnit, err := scanOrgUnit(r.pool.QueryRow(ctx,
		`UPDATE org_units SET name = $3 WHERE tenant_id = $1 AND id = $2 RETURNING `+orgUnitColumns,
		tenantID, id, name))
	if errors.Is(err, pgx.ErrNoRows) {
		return OrgUnit{}, ErrNotFound
	}
	if err != nil && isUniqueViolation(err) {
		return OrgUnit{}, ErrConflict
	}
	return orgUnit, err
}

// UpdateParent moves an org unit (with its whole subtree) to a new parent,
// or to the root when newParentID is nil. It rejects:
//   - moving a node under itself or one of its descendants (cycle),
//   - a parent that belongs to a different tenant,
//   - a sibling-name conflict at the target location.
func (r *PostgresOrgUnitRepository) UpdateParent(ctx context.Context, tenantID, id string, newParentID *string) (OrgUnit, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return OrgUnit{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the moved node; fail fast when it does not exist in this tenant.
	orgUnit, err := scanOrgUnit(tx.QueryRow(ctx,
		`SELECT `+orgUnitColumns+` FROM org_units WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, tenantID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return OrgUnit{}, ErrNotFound
	}
	if err != nil {
		return OrgUnit{}, err
	}

	// Same parent is a no-op.
	if (orgUnit.ParentID == nil && newParentID == nil) ||
		(orgUnit.ParentID != nil && newParentID != nil && *orgUnit.ParentID == *newParentID) {
		return orgUnit, tx.Commit(ctx)
	}

	if newParentID != nil {
		// The new parent must exist in the same tenant.
		parent, err := scanOrgUnit(tx.QueryRow(ctx,
			`SELECT `+orgUnitColumns+` FROM org_units WHERE tenant_id = $1 AND id = $2`, tenantID, *newParentID))
		if errors.Is(err, pgx.ErrNoRows) {
			return OrgUnit{}, ErrNotFound
		}
		if err != nil {
			return OrgUnit{}, err
		}
		// Cycle guard: the new parent must not be the node itself or any
		// descendant of it.
		var ancestor bool
		if err := tx.QueryRow(ctx, `
			WITH RECURSIVE subtree AS (
				SELECT id FROM org_units WHERE id = $1
				UNION ALL
				SELECT c.id FROM org_units c JOIN subtree s ON c.parent_id = s.id
			)
			SELECT EXISTS (SELECT 1 FROM subtree WHERE id = $2)`, id, *newParentID).Scan(&ancestor); err != nil {
			return OrgUnit{}, err
		}
		if ancestor {
			return OrgUnit{}, invalid("cannot move an org unit under itself or one of its descendants")
		}
		_ = parent
	}

	orgUnit, err = scanOrgUnit(tx.QueryRow(ctx,
		`UPDATE org_units SET parent_id = $3 WHERE tenant_id = $1 AND id = $2 RETURNING `+orgUnitColumns,
		tenantID, id, newParentID))
	if err != nil {
		if isUniqueViolation(err) {
			return OrgUnit{}, ErrConflict
		}
		return OrgUnit{}, err
	}
	return orgUnit, tx.Commit(ctx)
}

// UpdateNameAndParent renames an org unit and moves it to a new parent in a
// single transaction so a failed move (cycle, cross-tenant parent, sibling
// conflict) never leaves a half-applied rename.
func (r *PostgresOrgUnitRepository) UpdateNameAndParent(ctx context.Context, tenantID, id, name string, newParentID *string) (OrgUnit, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return OrgUnit{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the moved node; fail fast when it does not exist in this tenant.
	orgUnit, err := scanOrgUnit(tx.QueryRow(ctx,
		`SELECT `+orgUnitColumns+` FROM org_units WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, tenantID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return OrgUnit{}, ErrNotFound
	}
	if err != nil {
		return OrgUnit{}, err
	}

	if newParentID != nil {
		// The new parent must exist in the same tenant.
		if _, err := scanOrgUnit(tx.QueryRow(ctx,
			`SELECT `+orgUnitColumns+` FROM org_units WHERE tenant_id = $1 AND id = $2`, tenantID, *newParentID)); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return OrgUnit{}, ErrNotFound
			}
			return OrgUnit{}, err
		}
		// Cycle guard: the new parent must not be the node itself or any
		// descendant of it.
		var ancestor bool
		if err := tx.QueryRow(ctx, `
			WITH RECURSIVE subtree AS (
				SELECT id FROM org_units WHERE id = $1
				UNION ALL
				SELECT c.id FROM org_units c JOIN subtree s ON c.parent_id = s.id
			)
			SELECT EXISTS (SELECT 1 FROM subtree WHERE id = $2)`, id, *newParentID).Scan(&ancestor); err != nil {
			return OrgUnit{}, err
		}
		if ancestor {
			return OrgUnit{}, invalid("cannot move an org unit under itself or one of its descendants")
		}
	}

	orgUnit, err = scanOrgUnit(tx.QueryRow(ctx,
		`UPDATE org_units SET name = $3, parent_id = $4 WHERE tenant_id = $1 AND id = $2 RETURNING `+orgUnitColumns,
		tenantID, id, name, newParentID))
	if err != nil {
		if isUniqueViolation(err) {
			return OrgUnit{}, ErrConflict
		}
		return OrgUnit{}, err
	}
	return orgUnit, tx.Commit(ctx)
}

func (r *PostgresOrgUnitRepository) Delete(ctx context.Context, tenantID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM org_units WHERE tenant_id = $1 AND id = $2`, tenantID, id)
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

// Tree loads all org units of one tenant once and assembles the forest in
// memory.
func (r *PostgresOrgUnitRepository) Tree(ctx context.Context, tenantID string) ([]*OrgUnit, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+orgUnitColumns+` FROM org_units WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[string]*OrgUnit{}
	var roots []*OrgUnit
	for rows.Next() {
		orgUnit, err := scanOrgUnit(rows)
		if err != nil {
			return nil, err
		}
		node := &OrgUnit{ID: orgUnit.ID, TenantID: orgUnit.TenantID, ParentID: orgUnit.ParentID, Name: orgUnit.Name, CreatedAt: orgUnit.CreatedAt}
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

// SubtreeIDs returns all org unit ids in the subtrees rooted at the given
// ids via a single recursive CTE (the anchors plus every descendant).
// It returns an empty slice when inputs are empty; unknown anchor ids are
// silently ignored (callers resolve visibility elsewhere).
func (r *PostgresOrgUnitRepository) SubtreeIDs(ctx context.Context, orgUnitIDs []string) ([]string, error) {
	if len(orgUnitIDs) == 0 {
		return []string{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		WITH RECURSIVE subtree AS (
			SELECT id FROM org_units WHERE id = ANY($1)
			UNION ALL
			SELECT c.id FROM org_units c JOIN subtree s ON c.parent_id = s.id
		)
		SELECT id FROM subtree`, orgUnitIDs)
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

type PostgresUserRepository struct{ pool *pgxpool.Pool }

func NewPostgresUserRepository(pool *pgxpool.Pool) *PostgresUserRepository {
	return &PostgresUserRepository{pool: pool}
}

const userColumns = `id, tenant_id, username, password_hash, display_name, status, all_orgs, created_at, updated_at`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.TenantID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Status, &u.AllOrgs, &u.CreatedAt, &u.UpdatedAt)
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

func (r *PostgresUserRepository) loadOrgScopes(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT org_unit_id FROM user_org_scopes WHERE user_id = $1 ORDER BY org_unit_id`, userID)
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
	orgIDs, err := r.loadOrgScopes(ctx, u.ID)
	if err != nil {
		return User{}, err
	}
	u.Roles = roles
	u.OrgIDs = orgIDs
	return u, nil
}

func (r *PostgresUserRepository) Create(ctx context.Context, in CreateUserInput, passwordHash string) (User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	u, err := scanUser(tx.QueryRow(ctx,
		`INSERT INTO users (tenant_id, username, password_hash, display_name, all_orgs)
		 VALUES ($1, $2, $3, $4, $5) RETURNING `+userColumns,
		in.TenantID, in.Username, passwordHash, in.DisplayName, in.AllOrgs))
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrConflict
		}
		return User{}, err
	}
	if err := replaceRoles(ctx, tx, u.ID, in.Roles); err != nil {
		return User{}, err
	}
	if err := replaceOrgScopes(ctx, tx, u.ID, in.OrgIDs); err != nil {
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
	var displayName, status, allOrgs any
	if in.DisplayName != nil {
		displayName = *in.DisplayName
	}
	if in.Status != nil {
		status = *in.Status
	}
	if in.AllOrgs != nil {
		allOrgs = *in.AllOrgs
	}
	u, err := scanUser(tx.QueryRow(ctx,
		`UPDATE users SET
			display_name = COALESCE($3, display_name),
			status = COALESCE($4, status),
			all_orgs = COALESCE($5, all_orgs),
			updated_at = now()
		 WHERE tenant_id = $1 AND id = $2 RETURNING `+userColumns,
		tenantID, id, displayName, status, allOrgs))
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
	if in.OrgIDs != nil {
		if err := replaceOrgScopes(ctx, tx, u.ID, in.OrgIDs); err != nil {
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

func replaceOrgScopes(ctx context.Context, tx execer, userID string, orgUnitIDs []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM user_org_scopes WHERE user_id = $1`, userID); err != nil {
		return err
	}
	for _, orgUnitID := range orgUnitIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO user_org_scopes (user_id, org_unit_id) VALUES ($1, $2) ON CONFLICT (user_id, org_unit_id) DO NOTHING`,
			userID, orgUnitID); err != nil {
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
