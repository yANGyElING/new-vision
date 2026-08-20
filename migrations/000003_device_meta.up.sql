-- device_name, manufacturer, device_type are metadata for the test console.
-- They are NOT synced to the access layer (Kamailio/Redis profile).
ALTER TABLE devices
    ADD COLUMN device_name VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN manufacturer VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN device_type VARCHAR(3) NOT NULL DEFAULT '';