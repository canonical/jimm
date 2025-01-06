-- adds a column necessary for model migrations

ALTER TABLE models ADD COLUMN migration_controller_id INTEGER REFERENCES controllers (id);
