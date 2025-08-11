// Copyright 2025 Canonical.

// bootstrap package provides functionality to manage the bootstrap process
// for controllers in JIMM.
package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuclistore"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

// Store defines the store methods required by the manager.
type Store interface {
	QueryBootstrapLog(ctx context.Context, jobId uuid.UUID, offset int) (loggies []string, nextOffsetValue int, err error)

	// BootstrapJob store methods:

	LockBootstrap(ctx context.Context, ttl time.Duration) error
	GetController(ctx context.Context, controller *dbmodel.Controller) (err error)
	AddBootstrapLog(ctx context.Context, jobId uuid.UUID, logLine string) (err error)
	UnlockBootstrap(ctx context.Context) error
}

// JobTracker interface defines the methods required for job tracking.
type JobTracker interface {
	// GetJob retrieves a job entry by its ID.
	GetJob(ctx context.Context, jobId uuid.UUID) (dbmodel.JobTrackerEntry, error)

	// StopJob stops a job by its ID.
	StopJob(ctx context.Context, jobId uuid.UUID) error
}

// JujuManager defines the juju manager methods required by the job.
type JujuManager interface {
	AddController(ctx context.Context, user *openfga.User, ctl *dbmodel.Controller, creds juju.ControllerCreds) error
}

// BinaryStore defines the binary store methods required by the job.
type BinaryStore interface {
	Get(ctx context.Context, spec jujuclistore.JujuBinarySpec) (*jujuclistore.Binary, error)
}

type bootstrapManager struct {
	authSvc *openfga.OFGAClient

	store       Store
	jobtracker  JobTracker
	jujuManager JujuManager
	binaryStore BinaryStore
}

// NewBootstrapManager creates a new BootstrapManager instance.
// TODO(ale8k): Remove authSvc later, it isn't used.
func NewBootstrapManager(
	authSvc *openfga.OFGAClient,
	store Store,
	jobtracker JobTracker,
	jujuManager JujuManager,
	binaryStore BinaryStore,
) (*bootstrapManager, error) {
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
		store:       store,
		authSvc:     authSvc,
		jobtracker:  jobtracker,
		jujuManager: jujuManager,
		binaryStore: binaryStore,
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

// StartBootstrap starts a bootstrap job with the provided parameters.
func (b *bootstrapManager) StartBootstrap(ctx context.Context, user *openfga.User, params BootstrapParams) (string, error) {
	const op = errors.Op("jimm.StartBootstrap")

	err := params.validate()
	if err != nil {
		return "", errors.E(op, fmt.Errorf("invalid bootstrap parameters: %v", err))
	}

	return "", errors.E(op, "not implemented")
}

// StopBootstrap stops a bootstrap job by its ID.
func (b *bootstrapManager) StopBootstrap(ctx context.Context, user *openfga.User, jobId uuid.UUID) error {
	const op = errors.Op("jimm.StopBootstrap")

	if user == nil {
		return errors.E(op, "user cannot be nil")
	}

	if jobId == uuid.Nil {
		return errors.E(op, "job ID cannot be nil")
	}

	err := b.jobtracker.StopJob(ctx, jobId)
	if err != nil {
		return errors.E(op, "failed to stop job", err)
	}

	return nil
}
