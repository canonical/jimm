-- 1_4.sql is a migration that removes unused tables and columns previously used for access checks.

DROP TABLE IF EXISTS user_application_offer_access;
DROP TABLE IF EXISTS user_cloud_access;
DROP TABLE IF EXISTS user_model_access;

UPDATE versions SET major=1, minor=4 WHERE component='jimmdb';
