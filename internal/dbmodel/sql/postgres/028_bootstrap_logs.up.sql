-- Adds a table for storing bootstrap logs

CREATE TABLE  IF NOT EXISTS bootstrap_logs(
	job_id UUID NOT NULL REFERENCES job_tracker_entries(job_id) ON DELETE CASCADE,
	line_number INT NOT NULL, 
	log_line TEXT NOT NULL, 
	PRIMARY KEY (job_id, line_number)
);
