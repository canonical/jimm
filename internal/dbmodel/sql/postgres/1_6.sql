-- 1_6.sql is a migration that adds access tokens to the user table

ALTER TABLE users ADD COLUMN access_token TEXT;

UPDATE versions SET major=1, minor=6 WHERE component='jimmdb';