--- Drop the bootstrap lock table and the job tracker table/type.
--- With the move to River queue, concurrency control for bootstrapping is now handled
--- by River, so the bootstrap lock and job tracker tables are no longer needed.

DROP TABLE IF EXISTS bootstrap_locks;
DROP TABLE IF EXISTS job_tracker_entries;
DROP TYPE IF EXISTS job_tracker_status;
