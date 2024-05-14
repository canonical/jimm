-- 1_7.sql is a migration that adds a UUID column to the identity table.
ALTER TABLE groups ADD COLUMN uuid TEXT NOT NULL UNIQUE;

UPDATE versions SET major=1, minor=7 WHERE component='jimmdb';
