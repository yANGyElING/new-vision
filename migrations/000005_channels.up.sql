-- 000005_channels.up.sql
-- Catalog channel model (design D41-D51): channels table + device catalog sync state.

-- org_units needs a unique (id, tenant_id) index for the I3 composite foreign
-- key on channels (tenant must match on both ends of the org reference).
CREATE UNIQUE INDEX IF NOT EXISTS org_units_id_tenant_idx ON org_units (id, tenant_id);

CREATE TABLE channels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    channel_code VARCHAR(20) NOT NULL,
    report_name TEXT,
    display_name TEXT,
    org_unit_id UUID NULL,
    reported_status VARCHAR(8),
    missing BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_in_catalog_at TIMESTAMPTZ,
    -- D41: channel identity is (device_id, channel_code); the internal id is
    -- only for foreign-key references.
    CONSTRAINT channels_device_code UNIQUE (device_id, channel_code),
    -- I3: org reference must stay inside the owning tenant.
    CONSTRAINT channels_org_tenant_fk FOREIGN KEY (org_unit_id, tenant_id)
        REFERENCES org_units (id, tenant_id)
);

CREATE INDEX channels_tenant_idx ON channels (tenant_id);
CREATE INDEX channels_device_idx ON channels (device_id);
CREATE INDEX channels_org_idx ON channels (org_unit_id);

-- Device catalog sync state (D50/D51 four states: never / in_progress / ok / failed).
ALTER TABLE devices
    ADD COLUMN catalog_state VARCHAR(16) NOT NULL DEFAULT 'never'
        CHECK (catalog_state IN ('never', 'in_progress', 'ok', 'failed')),
    ADD COLUMN catalog_last_query_at TIMESTAMPTZ,
    ADD COLUMN catalog_last_ok_at TIMESTAMPTZ,
    ADD COLUMN catalog_last_count INTEGER,
    ADD COLUMN catalog_last_error TEXT,
    ADD COLUMN catalog_received INTEGER,
    ADD COLUMN catalog_total INTEGER;
