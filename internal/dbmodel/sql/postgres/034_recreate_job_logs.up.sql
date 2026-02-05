--- Drop and recreate job_logs to match the current JobLog model.
---
--- The table was originally introduced as bootstrap_logs (028) with a UUID job_id
--- referencing job_tracker_entries, then renamed to job_logs (033). The new job
--- identifier used by JIMM for these logs is River's job ID (BIGINT).

DROP TABLE IF EXISTS job_logs;

CREATE TABLE IF NOT EXISTS job_logs (
    job_id BIGINT NOT NULL,
    line_number INT NOT NULL,
    log_line TEXT NOT NULL,
    PRIMARY KEY (job_id, line_number)
);
