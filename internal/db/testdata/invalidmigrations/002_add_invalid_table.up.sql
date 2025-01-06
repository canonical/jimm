-- adds an invalid table.

CREATE TABLE IF NOT EXISTS invalid (
	id BIGSERIAL PRIMARY KEY,
	time INVALIDTYPE
);
