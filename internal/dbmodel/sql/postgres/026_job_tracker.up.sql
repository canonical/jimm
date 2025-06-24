-- adds tracker status enum and job tracker table

CREATE TYPE job_tracker_status AS ENUM (
    'pending',
    'running',
    'successful',
    'failed'
);

CREATE TABLE IF NOT EXISTS job_tracker_entries (
    job_id UUID PRIMARY KEY,
    job_type VARCHAR(128) NOT NULL,
    stop_signal BOOLEAN NOT NULL DEFAULT FALSE,
    status job_tracker_status NOT NULL DEFAULT 'pending',
    error TEXT,

    created_at TIMESTAMP,
    updated_at TIMESTAMP
);
