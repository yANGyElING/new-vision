-- 000004_auth.up.sql
-- Permission architecture: tenants, org_units, users, roles, scopes, audit

CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    status VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Org unit tree: per-tenant, no seed root, user-defined hierarchy.
-- parent_id is nullable for top-level nodes.
CREATE TABLE org_units (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    parent_id UUID REFERENCES org_units(id) ON DELETE RESTRICT,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX org_units_tenant_idx ON org_units (tenant_id);

-- Sibling-name uniqueness (including root-level): COALESCE fixes the
-- PostgreSQL NULL-not-equal-to-any-value issue for parent_id IS NULL.
CREATE UNIQUE INDEX org_units_sibling_name_idx
    ON org_units (tenant_id, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'::uuid), name);

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    username VARCHAR(64) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    all_orgs BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT users_tenant_username UNIQUE (tenant_id, username)
);

CREATE TABLE user_roles (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(32) NOT NULL CHECK (role IN ('node_admin', 'tenant_admin', 'operator', 'viewer')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT user_roles_unique UNIQUE (user_id, role)
);

CREATE TABLE user_org_scopes (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_unit_id UUID NOT NULL REFERENCES org_units(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT user_org_scopes_unique UNIQUE (user_id, org_unit_id)
);

CREATE TABLE audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor_user_id UUID,
    tenant_id UUID,
    action VARCHAR(64) NOT NULL,
    resource_type VARCHAR(64) NOT NULL,
    resource_id VARCHAR(128),
    result VARCHAR(16) NOT NULL CHECK (result IN ('success', 'denied', 'error')),
    ip_addr INET,
    detail JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_created_idx ON audit_logs (created_at DESC);
CREATE INDEX audit_logs_actor_idx ON audit_logs (actor_user_id, created_at DESC);

-- Seed default tenant (no seed org tree — users build it from scratch)
INSERT INTO tenants (id, name) VALUES ('00000000-0000-0000-0000-000000000001', 'default');

-- Add tenant_id and org_unit_id to devices (org_unit_id nullable = unassigned).
-- tenant_id is added nullable first, backfilled to the default tenant, then
-- set NOT NULL so the migration succeeds when devices already contain rows.
ALTER TABLE devices
    ADD COLUMN tenant_id UUID REFERENCES tenants(id),
    ADD COLUMN org_unit_id UUID REFERENCES org_units(id);
UPDATE devices SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
ALTER TABLE devices ALTER COLUMN tenant_id SET NOT NULL;
CREATE INDEX devices_tenant_idx ON devices (tenant_id);
CREATE INDEX devices_org_idx ON devices (org_unit_id);
