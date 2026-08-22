-- 000004_auth.up.sql
-- Permission architecture: tenants, regions, users, roles, scope, audit

CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    status VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE regions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id UUID REFERENCES regions(id) ON DELETE RESTRICT,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (parent_id, name)
);

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    username VARCHAR(64) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
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

CREATE TABLE user_region_scopes (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    region_id UUID NOT NULL REFERENCES regions(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT user_region_scopes_unique UNIQUE (user_id, region_id)
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

-- Seed default tenant and root region
INSERT INTO tenants (id, name) VALUES ('00000000-0000-0000-0000-000000000001', 'default');
INSERT INTO regions (id, name) VALUES ('00000000-0000-0000-0000-000000000002', 'root');

-- Add tenant_id and region_id to devices
ALTER TABLE devices
    ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001' REFERENCES tenants(id),
    ADD COLUMN region_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000002' REFERENCES regions(id);
CREATE INDEX devices_tenant_idx ON devices (tenant_id);
CREATE INDEX devices_region_idx ON devices (region_id);