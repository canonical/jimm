-- 1_14.sql is a migration simplifies application offers
DROP INDEX IF EXISTS idx_application_offer_connections_deleted_at;
DROP INDEX IF EXISTS idx_application_offer_remote_endpoints_deleted_at;
DROP INDEX IF EXISTS idx_application_offer_remote_spaces_deleted_at;
DROP INDEX IF EXISTS idx_user_application_offer_access_deleted_at;
DROP TABLE IF EXISTS application_offer_connections;
DROP TABLE IF EXISTS application_offer_remote_endpoints;
DROP TABLE IF EXISTS application_offer_remote_spaces;
DROP TABLE IF EXISTS user_application_offer_access;
ALTER TABLE application_offers ALTER COLUMN application_name DROP NOT NULL;
ALTER TABLE application_offers ALTER COLUMN application_description DROP NOT NULL;
ALTER TABLE application_offers ALTER COLUMN charm_url DROP NOT NULL;
ALTER TABLE application_offers DROP COLUMN IF EXISTS application_name;
ALTER TABLE application_offers DROP COLUMN IF EXISTS application_description;
ALTER TABLE application_offers DROP COLUMN IF EXISTS bindings;
ALTER TABLE application_offers DROP COLUMN IF EXISTS charm_url;

UPDATE versions SET major=1, minor=14 WHERE component='jimmdb';

