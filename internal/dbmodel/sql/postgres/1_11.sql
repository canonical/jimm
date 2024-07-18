-- 1_11.sql is a migration that enforces uniqueness on URLs in application offers.
ALTER TABLE application_offers ADD UNIQUE (url);

UPDATE versions SET major=1, minor=11 WHERE component='jimmdb';
