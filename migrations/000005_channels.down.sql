-- 000005_channels.down.sql
ALTER TABLE devices
    DROP COLUMN IF EXISTS catalog_state,
    DROP COLUMN IF EXISTS catalog_last_query_at,
    DROP COLUMN IF EXISTS catalog_last_ok_at,
    DROP COLUMN IF EXISTS catalog_last_count,
    DROP COLUMN IF EXISTS catalog_last_error,
    DROP COLUMN IF EXISTS catalog_received,
    DROP COLUMN IF EXISTS catalog_total;

DROP TABLE IF EXISTS channels;
DROP INDEX IF EXISTS org_units_id_tenant_idx;
