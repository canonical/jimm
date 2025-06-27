// Copyright 2025 Canonical.

package dbmodel

import (
	"errors"

	"github.com/google/uuid"
)

// BootstrapLog represents a log entry for a bootstrap job.
type BootstrapLog struct {
	// JobID is the unique identifier for the job. References a [JobTrackerEntry].
	JobID uuid.UUID `gorm:"type:uuid;not null;primaryKey"`
	// LineNumber is the line number of a running bootstrap. It is used to offset the log lines
	// when fetching logs.
	LineNumber int `gorm:"not null;primaryKey"`
	// LogLine is an actual log line from the bootstrap job.
	LogLine string `gorm:"type:text;not null"`

	Job JobTrackerEntry `gorm:"constraint:OnDelete:CASCADE;foreignKey:JobID;references:JobID"`
}

// NewBootstrapLog creates a new BootstrapLog with the given jobId, lineNumber, and logLine.
// It returns an error if any of the parameters are invalid.
func NewBootstrapLog(jobId uuid.UUID, lineNumber int, logLine string) (*BootstrapLog, error) {
	res := &BootstrapLog{}

	if jobId == uuid.Nil {
		return res, errors.New("job id cannot be nil")
	}
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
