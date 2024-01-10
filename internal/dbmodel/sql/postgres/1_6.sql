-- 1_6.sql is a migration that adds service_account table and adds a reference column to cloud_credentials table.

CREATE TABLE IF NOT EXISTS service_accounts (
    id BIGSERIAL PRIMARY KEY,
	client_id TEXT NOT NULL UNIQUE,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	display_name TEXT NOT NULL,
	last_login TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_service_accounts_deleted_at ON service_accounts (deleted_at);

ALTER TABLE cloud_credentials
    ALTER COLUMN owner_username DROP NOT NULL,
    ADD COLUMN owner_client_id TEXT NULL REFERENCES service_accounts (client_id),
    ADD UNIQUE (cloud_name, owner_client_id, name),
    ADD CHECK (owner_username IS NOT NULL OR owner_client_id IS NOT NULL);


UPDATE versions SET major=1, minor=6 WHERE component='jimmdb';
