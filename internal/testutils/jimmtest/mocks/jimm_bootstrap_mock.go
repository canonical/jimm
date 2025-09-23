// Copyright 2025 Canonical.

package mocks

import (
	"context"

	"github.com/google/uuid"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

type BootstapManager struct {
	GetJobInfo_        func(ctx context.Context, user *openfga.User, jobId uuid.UUID, offset int) (params.GetJobInfoResponse, error)
	StopJob_           func(ctx context.Context, user *openfga.User, jobId uuid.UUID) error
	StartBootstrapJob_ func(ctx context.Context, user *openfga.User, params bootstrap.BootstrapParams) (string, error)
}

func (b *BootstapManager) GetJobInfo(ctx context.Context, user *openfga.User, jobId uuid.UUID, offset int) (params.GetJobInfoResponse, error) {
	if b.GetJobInfo_ == nil {
		return params.GetJobInfoResponse{}, errors.E(errors.CodeNotImplemented)
	}
	return b.GetJobInfo_(ctx, user, jobId, offset)
}

func (b *BootstapManager) StopJob(ctx context.Context, user *openfga.User, jobId uuid.UUID) error {
	if b.StopJob_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return b.StopJob_(ctx, user, jobId)
}

func (b *BootstapManager) StartBootstrapJob(ctx context.Context, user *openfga.User, params bootstrap.BootstrapParams) (string, error) {
	if b.StartBootstrapJob_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return b.StartBootstrapJob_(ctx, user, params)
}
