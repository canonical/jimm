-- 1_1.sql initialises an empty database.

CREATE TABLE IF NOT EXISTS test (
	id BIGSERIAL PRIMARY KEY,
	time TIMESTAMP WITH TIME ZONE
);
