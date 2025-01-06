-- removes unused tables and columns previously used for access checks.

DROP TABLE IF EXISTS user_application_offer_access;
DROP TABLE IF EXISTS user_cloud_access;
DROP TABLE IF EXISTS user_model_access;
ALTER TABLE users DROP COLUMN controller_access;
ALTER TABLE users DROP COLUMN audit_log_access;
