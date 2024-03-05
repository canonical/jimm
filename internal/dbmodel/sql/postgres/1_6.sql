-- 1_6.sql is a migration that adds access tokens to the user table
-- and is a migration that renames `user` to `identity`.
ALTER TABLE users ADD COLUMN access_token TEXT;
ALTER TABLE users ADD COLUMN refresh_token TEXT;

-- Note that we don't need to rename underlying indexes/constraints. As Postgres
-- docs states:
--   "When renaming a constraint that has an underlying index, the index is renamed as well."
--   (See https://www.postgresql.org/docs/current/sql-altertable.html)

-- Renaming tables/columns.
ALTER TABLE IF EXISTS users RENAME TO identities;
ALTER TABLE IF EXISTS identities RENAME COLUMN username TO name;
ALTER TABLE IF EXISTS cloud_credentials RENAME COLUMN owner_username TO owner_identity_name;
ALTER TABLE IF EXISTS cloud_defaults RENAME COLUMN username TO identity_name;
ALTER TABLE IF EXISTS models RENAME COLUMN owner_username TO owner_identity_name;
ALTER TABLE IF EXISTS application_offer_connections RENAME COLUMN username TO identity_name;
ALTER TABLE IF EXISTS user_model_defaults RENAME TO identity_model_defaults;
ALTER TABLE IF EXISTS identity_model_defaults RENAME COLUMN username TO identity_name;
ALTER TABLE IF EXISTS controllers RENAME COLUMN admin_user TO admin_identity_name;
ALTER TABLE IF EXISTS audit_log RENAME COLUMN user_tag TO identity_tag;

-- Renaming indexes:
ALTER INDEX IF EXISTS users_username_key RENAME TO identities_name_key;
ALTER INDEX IF EXISTS users_pkey RENAME TO identities_pkey;
ALTER INDEX IF EXISTS idx_users_deleted_at RENAME TO idx_identities_deleted_at;
ALTER INDEX IF EXISTS models_controller_id_owner_username_name_key RENAME TO models_controller_id_owner_identity_name_name_key;
ALTER INDEX IF EXISTS user_model_defaults_username_key RENAME TO user_model_defaults_identity_name_key;
ALTER INDEX IF EXISTS user_model_defaults_username_fkey RENAME TO user_model_defaults_identity_name_fkey;
ALTER INDEX IF EXISTS cloud_credentials_cloud_name_owner_username_name_key RENAME TO cloud_credentials_cloud_name_owner_identity_name_name_key;
ALTER INDEX IF EXISTS cloud_defaults_username_cloud_id_region_key RENAME TO cloud_defaults_identity_name_cloud_id_region_key;
ALTER INDEX IF EXISTS idx_audit_log_user_tag RENAME TO idx_audit_log_identity_tag;

-- We don't need to rename columns in these tables, because they're already
-- dropped in an earlier migration:
--   - user_application_offer_access
--   - user_cloud_access
--   - user_model_access

UPDATE versions SET major=1, minor=6 WHERE component='jimmdb';
