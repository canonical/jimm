-- 1_8.sql is a migration that adds a tls_hostname column to the controller table.
ALTER TABLE controllers ADD COLUMN tls_hostname TEXT;

UPDATE versions SET major=1, minor=8 WHERE component='jimmdb';
