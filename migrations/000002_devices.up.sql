CREATE TABLE devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_access_id VARCHAR(20) NOT NULL UNIQUE,
    sip_username VARCHAR(20) NOT NULL,
    sip_realm VARCHAR(255) NOT NULL,
    digest_algorithm VARCHAR(10) NOT NULL DEFAULT 'MD5',
    digest_ha1 CHAR(32) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    profile_version BIGINT NOT NULL DEFAULT 1,
    access_sync_status VARCHAR(16) NOT NULL DEFAULT 'pending',
    access_synced_version BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT devices_access_id_format CHECK (device_access_id ~ '^[0-9]{20}$'),
    CONSTRAINT devices_sip_username_format CHECK (sip_username ~ '^[0-9]{20}$'),
    CONSTRAINT devices_sip_username_matches_access_id CHECK (sip_username = device_access_id),
    CONSTRAINT devices_digest_algorithm CHECK (digest_algorithm = 'MD5'),
    CONSTRAINT devices_digest_ha1_format CHECK (digest_ha1 ~ '^[0-9a-f]{32}$'),
    CONSTRAINT devices_profile_version_positive CHECK (profile_version > 0),
    CONSTRAINT devices_sync_status CHECK (access_sync_status IN ('pending', 'synced')),
    CONSTRAINT devices_synced_version_valid CHECK (access_synced_version IS NULL OR access_synced_version > 0)
);

CREATE TABLE access_profile_outbox (
    id BIGSERIAL PRIMARY KEY,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    profile_version BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ,
    last_error VARCHAR(1000),
    CONSTRAINT access_profile_outbox_version_positive CHECK (profile_version > 0),
    CONSTRAINT access_profile_outbox_attempt_nonnegative CHECK (attempt_count >= 0)
);
CREATE INDEX access_profile_outbox_pending_idx ON access_profile_outbox (next_attempt_at, id) WHERE processed_at IS NULL;
CREATE INDEX access_profile_outbox_device_idx ON access_profile_outbox (device_id, profile_version);
