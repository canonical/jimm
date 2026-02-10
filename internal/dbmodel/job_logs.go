// Copyright 2025 Canonical.

package dbmodel

import (
	"errors"
)

// JobLog represents a log entry for a job.
type JobLog struct {
	// JobID is the unique identifier for the job. References a [JobTrackerEntry].
	JobID int64 `gorm:"type:bigint;not null;primaryKey"`
	// LineNumber is the line number of a running job. It is used to offset the log lines
	// when fetching logs.
	LineNumber int `gorm:"not null;primaryKey"`
	// LogLine is an actual log line from the job.
	LogLine string `gorm:"type:text;not null"`
}

// NewJobLog creates a new JobLog with the given jobId, lineNumber, and logLine.
// It returns an error if any of the parameters are invalid.
func NewJobLog(jobId int64, lineNumber int, logLine string) (*JobLog, error) {
	res := &JobLog{}

	if lineNumber < 0 {
		return res, errors.New("line number must be non-negative")
	}
	if logLine == "" {
		return res, errors.New("log line cannot be empty")
	}

	res.JobID = jobId
	res.LineNumber = lineNumber
	res.LogLine = logLine
	return res, nil
}
