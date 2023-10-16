-- 1_4 is a migration that deletes all rows with soft deleted rows 
DELETE FROM cloud_regions WHERE deleted_at IS NOT NULL;
DELETE FROM controllers WHERE deleted_at IS NOT NULL;
DELETE FROM cloud_region_controller_priorities WHERE deleted_at IS NOT NULL;
DELETE FROM users WHERE deleted_at IS NOT NULL;
DELETE FROM cloud_credentials WHERE deleted_at IS NOT NULL;
DELETE FROM cloud_defaults WHERE deleted_at IS NOT NULL;
DELETE FROM application_offer_connections WHERE deleted_at IS NOT NULL;
DELETE FROM application_offer_remote_endpoints WHERE deleted_at IS NOT NULL;
DELETE FROM application_offer_remote_spaces WHERE deleted_at IS NOT NULL;
DELETE FROM user_application_offer_access WHERE deleted_at IS NOT NULL;
DELETE FROM user_cloud_access WHERE deleted_at IS NOT NULL;
DELETE FROM user_model_access WHERE deleted_at IS NOT NULL;
DELETE FROM user_model_defaults WHERE deleted_at IS NOT NULL;
DELETE FROM controller_configs WHERE deleted_at IS NOT NULL;
DELETE FROM groups WHERE deleted_at IS NOT NULL;


ALTER TABLE cloud_regions DROP COLUMN deleted_at;
ALTER TABLE controllers DROP COLUMN deleted_at; 
ALTER TABLE cloud_region_controller_priorities DROP COLUMN deleted_at; 
ALTER TABLE users DROP COLUMN deleted_at; 
ALTER TABLE cloud_credentials  DROP COLUMN deleted_at; 
ALTER TABLE cloud_defaults  DROP COLUMN deleted_at; 
ALTER TABLE application_offer_connections  DROP COLUMN deleted_at; 
ALTER TABLE application_offer_remote_endpoints  DROP COLUMN deleted_at; 
ALTER TABLE application_offer_remote_spaces  DROP COLUMN deleted_at; 
ALTER TABLE user_application_offer_access  DROP COLUMN deleted_at; 
ALTER TABLE user_cloud_access  DROP COLUMN deleted_at; 
ALTER TABLE user_model_access  DROP COLUMN deleted_at; 
ALTER TABLE user_model_defaults  DROP COLUMN deleted_at; 
ALTER TABLE controller_configs  DROP COLUMN deleted_at; 
ALTER TABLE groups  DROP COLUMN deleted_at; 
