// Copyright 2025 Canonical.

package bootstrap

import (
	"context"

	"github.com/google/uuid"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

// JobTracker interface defines the methods required for job tracking.
type JobTracker interface {
	// GetJob retrieves a job entry by its ID.
	GetJob(ctx context.Context, jobId uuid.UUID) (dbmodel.JobTrackerEntry, error)
}

type Store interface {
	QueryBootstrapLog(ctx context.Context, jobId uuid.UUID, offset int) (loggies []string, nextOffsetValue int, err error)
}

type bootstrapManager struct {
	jobtracker JobTracker
	store      Store
	authSvc    *openfga.OFGAClient
}

// NewBootstrapManager creates a new BootstrapManager instance.
func NewBootstrapManager(store Store, authSvc *openfga.OFGAClient, jobtracker JobTracker) (*bootstrapManager, error) {
	if store == nil {
		return nil, errors.E("store cannot be nil")
	}
	if authSvc == nil {
		return nil, errors.E("authorisation service cannot be nil")
	}
	if jobtracker == nil {
		return nil, errors.E("job tracker cannot be nil")
	}
	return &bootstrapManager{
		store:      store,
		authSvc:    authSvc,
		jobtracker: jobtracker,
	}, nil
}

// GetBootstrapStatusAndLogs retrieves the status and logs of a bootstrap job.
// It requires the user to be an admin and returns the status, error message, logs,
// and a watermark for pagination.
func (b *bootstrapManager) GetBootstrapStatusAndLogs(ctx context.Context, _ *openfga.User, jobId uuid.UUID, offset int) (params.BootstrapStatusResponse, error) {
	const op = errors.Op("jimm.GetBootstrapStatusAndLogs")

	job, err := b.jobtracker.GetJob(ctx, jobId)
	if err != nil {
		return params.BootstrapStatusResponse{}, errors.E(op, "failed to get job status", err)
	}

	loggies, newOffset, err := b.store.QueryBootstrapLog(ctx, jobId, offset)
	if err != nil {
		return params.BootstrapStatusResponse{}, errors.E(op, "failed to query bootstrap logs", err)
	}
	return params.BootstrapStatusResponse{
		Status:    params.JobStatus(job.Status),
		Error:     job.Error,
		Logs:      loggies,
		Watermark: newOffset,
	}, nil
}
