-- v.sql initialises the versions table
-- This will be run unconditionally at application startup, so must be
-- idempotent.

CREATE TABLE IF NOT EXISTS versions (
	component TEXT NOT NULL PRIMARY KEY,
	major INTEGER NOT NULL,
	minor INTEGER NOT NULL
);
