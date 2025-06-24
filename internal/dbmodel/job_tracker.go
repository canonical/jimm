// Copyright 2025 Canonical.

package dbmodel

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// JobStatus represents the status of a job in the job tracker.
// The status can be one of the following:
// [statusPending], [StatusRunning], [StatusSuccessful], or [statusFailed].
// It is stored as a custom enum in the database.
// StatusFailed must be set using the [JobTrackerEntry.SetFailed] method.
type JobStatus string

const (
	StatusRunning    JobStatus = "running"
	StatusSuccessful JobStatus = "successful"
	StatusPending    JobStatus = "pending"
	StatusFailed     JobStatus = "failed"
)

// JobTrackerEntry represents a job that is being tracked within an instance of JIMM.
type JobTrackerEntry struct {
	// JobID is the unique identifier for the job. This is not to be set manually, please
	// use the constructor [NewJobTrackerEntry] instead.
	JobID uuid.UUID `gorm:"type:uuid;primaryKey"`
	// JobType is the type of the job, e.g., bootstrap, destroy, etc.
	JobType string `gorm:"type:varchar(128);not null"`
	// StopSignal signals whether the job should be stopped. This is set to true when the job is
	// requested to be stopped, but it does not mean the job has stopped yet.
	// This can be determined by the Status field.
	StopSignal bool `gorm:"not null;default:false"`
	// Status holds the current status of the job.
	Status JobStatus `gorm:"type:job_tracker_status;not null;default:'pending'"`
	// Error holds any error message associated with the job. Not to be set manually
	// and will only ever be set if the status is [StatusFailed].
	Error string `gorm:"type:text"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewJobTrackerEntry creates a new JobTrackerEntry with the given jobType.
// The Status is set to [statusPending] by default.
// And a new JobID is generated automatically.
func NewJobTrackerEntry(jobType string) (*JobTrackerEntry, error) {
	uuid, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	return &JobTrackerEntry{
		JobID:   uuid,
		JobType: jobType,
		Status:  StatusPending,
	}, nil
}

// SetFailed marks the job as failed and sets the error message.
func (j *JobTrackerEntry) SetFailed(err error) error {
	if err == nil {
		return errors.New("error cannot be nil")
	}

	j.Error = err.Error()
	j.Status = StatusFailed

	return nil
}

// SetRunning marks the job as running.
func (j *JobTrackerEntry) SetRunning() {
	j.Error = ""
	j.Status = StatusRunning
}

// SetSuccessful marks the job as successful.
func (j *JobTrackerEntry) SetSuccessful() {
	j.Error = ""
	j.Status = StatusSuccessful
}

// GetStatus returns the current status of the job.
func (j *JobTrackerEntry) GetStatus() JobStatus {
	return j.Status
}
