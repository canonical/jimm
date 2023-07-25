-- 0_1.sql is a migration that adds a secrets table.

CREATE TABLE IF NOT EXISTS secrets (
	id BIGSERIAL PRIMARY KEY,
	time TIMESTAMP WITH TIME ZONE,
	type TEXT NOT NULL,
	tag  TEXT NOT NULL,
	data JSONB
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_secret_name ON secrets (type, tag);

UPDATE versions SET major=1, minor=2 WHERE component='jimmdb';
