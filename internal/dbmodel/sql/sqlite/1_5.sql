-- 1_5.sql is a migration that removes adds a column necessary for model migrations

ALTER TABLE models ADD COLUMN migration_controller_id INTEGER REFERENCES controllers (id);

UPDATE versions SET major=1, minor=5 WHERE component='jimmdb';
