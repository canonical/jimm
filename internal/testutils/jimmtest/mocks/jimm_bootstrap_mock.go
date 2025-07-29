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
	GetBootstrapStatusAndLogs_ func(ctx context.Context, user *openfga.User, jobId uuid.UUID, offset int) (params.BootstrapStatusResponse, error)
	StartBootstrap_            func(ctx context.Context, user *openfga.User, params bootstrap.BootstrapParams) (string, error)
	StopBootstrap_             func(ctx context.Context, user *openfga.User, jobId uuid.UUID) error
}

func (b *BootstapManager) GetBootstrapStatusAndLogs(ctx context.Context, user *openfga.User, jobId uuid.UUID, offset int) (params.BootstrapStatusResponse, error) {
	if b.GetBootstrapStatusAndLogs_ == nil {
		return params.BootstrapStatusResponse{}, errors.E(errors.CodeNotImplemented)
	}
	return b.GetBootstrapStatusAndLogs_(ctx, user, jobId, offset)
}

func (b *BootstapManager) StartBootstrap(ctx context.Context, user *openfga.User, params bootstrap.BootstrapParams) (string, error) {
	if b.StartBootstrap_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return b.StartBootstrap_(ctx, user, params)
}

func (b *BootstapManager) StopBootstrap(ctx context.Context, user *openfga.User, jobId uuid.UUID) error {
	if b.StopBootstrap_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return b.StopBootstrap_(ctx, user, jobId)
}
