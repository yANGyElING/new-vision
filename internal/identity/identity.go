package identity

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OrgUnit is a node in the per-tenant organization tree. Top-level nodes
// have a nil ParentID; the tree starts empty (no seed root) and is built
// by users. It is the data-scope anchor: users get scopes pointing at org
// units (subtree expansion), devices optionally point at an org unit.
type OrgUnit struct {
	ID        string     `json:"id"`
	TenantID  string     `json:"tenant_id"`
	ParentID  *string    `json:"parent_id,omitempty"`
	Name      string     `json:"name"`
	CreatedAt time.Time  `json:"created_at"`
	Children  []*OrgUnit `json:"children,omitempty"`
}

type User struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	Username     string    `json:"username"`
	DisplayName  string    `json:"display_name"`
	PasswordHash string    `json:"-"`
	Status       string    `json:"status"`
	AllOrgs      bool      `json:"all_orgs"`
	Roles        []string  `json:"roles"`
	OrgIDs       []string  `json:"org_ids"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CreateTenantInput struct {
	Name string `json:"name"`
}

func (in CreateTenantInput) Validate() error {
	if in.Name == "" || len(in.Name) > 255 {
		return invalid("tenant name must be non-empty and at most 255 bytes")
	}
	return nil
}

// CreateUserInput creates a user in the caller's tenant by default. A
// node_admin may set TenantID to create a user in another tenant (enforced
// by the handler). Roles, OrgIDs and AllOrgs may be empty: a user can be
// created first and have roles/org scopes assigned later via Update.
type CreateUserInput struct {
	TenantID    string   `json:"tenant_id"`
	Username    string   `json:"username"`
	Password    string   `json:"password"`
	DisplayName string   `json:"display_name"`
	AllOrgs     bool     `json:"all_orgs"`
	Roles       []string `json:"roles"`
	OrgIDs      []string `json:"org_ids"`
}

func (in CreateUserInput) Validate() error {
	if in.Username == "" || len(in.Username) > 64 {
		return invalid("username must be non-empty and at most 64 bytes")
	}
	if in.Password == "" || len(in.Password) > 256 {
		return invalid("password must be between 1 and 256 bytes")
	}
	if len(in.DisplayName) > 255 {
		return invalid("display_name must be at most 255 bytes")
	}
	for _, role := range in.Roles {
		if !ValidRole(role) {
			return invalid("unknown role: " + role)
		}
	}
	return nil
}

type UpdateUserInput struct {
	DisplayName *string  `json:"display_name"`
	Status      *string  `json:"status"`
	AllOrgs     *bool    `json:"all_orgs"`
	Roles       []string `json:"roles"`
	OrgIDs      []string `json:"org_ids"`
}

const (
	RoleNodeAdmin      = "node_admin"
	RoleTenantAdmin    = "tenant_admin"
	RoleOperator       = "operator"
	RoleViewer         = "viewer"
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
)

func ValidRole(role string) bool {
	switch role {
	case RoleNodeAdmin, RoleTenantAdmin, RoleOperator, RoleViewer:
		return true
	}
	return false
}

// AllRoles returns the fixed role names in a stable order.
func AllRoles() []string {
	return []string{RoleNodeAdmin, RoleTenantAdmin, RoleOperator, RoleViewer}
}

// PrincipalInfo is the minimal caller identity that management handlers
// need. It is populated by the nodeapp wiring from authn.Principal, keeping
// this package free of an authn dependency (authn imports identity).
type PrincipalInfo struct {
	UserID   string
	TenantID string
}

type TenantRepository interface {
	Create(context.Context, CreateTenantInput) (Tenant, error)
	List(context.Context) ([]Tenant, error)
	Get(context.Context, string) (Tenant, error)
	GetByName(context.Context, string) (Tenant, error)
	SetStatus(context.Context, string, string) (Tenant, error)
}

type OrgUnitRepository interface {
	Create(context.Context, string, *string, string) (OrgUnit, error)
	Tree(context.Context, string) ([]*OrgUnit, error)
	Get(context.Context, string, string) (OrgUnit, error)
	UpdateName(context.Context, string, string, string) (OrgUnit, error)
	UpdateParent(context.Context, string, string, *string) (OrgUnit, error)
	UpdateNameAndParent(context.Context, string, string, string, *string) (OrgUnit, error)
	Delete(context.Context, string, string) error
	SubtreeIDs(context.Context, []string) ([]string, error)
}

type UserRepository interface {
	Create(context.Context, CreateUserInput, string) (User, error)
	List(context.Context, string) ([]User, error)
	Get(context.Context, string, string) (User, error)
	GetByUsername(context.Context, string, string) (User, error)
	Update(context.Context, string, string, UpdateUserInput) (User, error)
	Delete(context.Context, string, string) error
	SetPassword(context.Context, string, string, string) error
}

type Store struct {
	Tenants  TenantRepository
	OrgUnits OrgUnitRepository
	Users    UserRepository
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		Tenants:  NewPostgresTenantRepository(pool),
		OrgUnits: NewPostgresOrgUnitRepository(pool),
		Users:    NewPostgresUserRepository(pool),
	}
}
