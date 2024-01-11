-- 1_6.sql is a migration that renames `user` to `identity`.

-- Note that we don't need to rename underlying indexes/constraints. As Postgres
-- docs states:
--   "When renaming a constraint that has an underlying index, the index is renamed as well."
--   (See https://www.postgresql.org/docs/current/sql-altertable.html)

ALTER TABLE IF EXISTS users RENAME TO identities;
ALTER TABLE IF EXISTS users RENAME COLUMN username TO identity_name;

ALTER TABLE IF EXISTS cloud_credentials RENAME COLUMN owner_username TO owner_identity_name;
ALTER TABLE IF EXISTS cloud_defaults RENAME COLUMN username TO identity_name;
ALTER TABLE IF EXISTS models RENAME COLUMN owner_username TO owner_identity_name;
ALTER TABLE IF EXISTS application_offer_connections RENAME COLUMN username TO identity_name;
ALTER TABLE IF EXISTS user_model_defaults RENAME COLUMN username TO identity_name;

-- TODO (babakks): Do we need to rename these two instances as well?
ALTER TABLE IF EXISTS controllers RENAME COLUMN admin_user TO admin_identity_name;
ALTER TABLE IF EXISTS audit_log RENAME COLUMN user_tag TO identity_tag;

-- We don't need to rename columns in these tables, because they're already
-- dropped in an earlier migration:
--   - user_application_offer_access
--   - user_cloud_access
--   - user_model_access

UPDATE versions SET major=1, minor=6 WHERE component='jimmdb';
