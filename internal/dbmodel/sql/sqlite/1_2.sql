-- 0_1.sql is a migration that adds a secrets table.

CREATE TABLE IF NOT EXISTS secrets (
	id INTEGER PRIMARY KEY,
	time DATETIME,
	type TEXT NOT NULL,
	tag  TEXT NOT NULL,
	data BLOB
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_secret_name ON secrets (type, tag);

UPDATE versions SET major=1, minor=2 WHERE component='jimmdb';