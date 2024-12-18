-- 1_2.sql is a migration that adds an invalid table.

CREATE TABLE IF NOT EXISTS invalid (
	id BIGSERIAL PRIMARY KEY,
	time INVALIDTYPE
);
