-- 1_3.sql is a migration that adds an controller table.

CREATE TABLE IF NOT EXISTS controller (
	id BIGSERIAL PRIMARY KEY,
	name TEXT
);
