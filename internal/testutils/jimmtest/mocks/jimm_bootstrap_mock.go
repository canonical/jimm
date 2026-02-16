// Copyright 2025 Canonical.

package mocks

import (
	"context"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

type BootstapManager struct {
	GetJobInfo_                func(ctx context.Context, user *openfga.User, jobId int64, offset int) (params.GetBootstrapInfoResponse, error)
	StopJob_                   func(ctx context.Context, user *openfga.User, jobId int64) error
	StartBootstrapJob_         func(ctx context.Context, user *openfga.User, params bootstrap.BootstrapParams) (int64, error)
	StartDestroyControllerJob_ func(ctx context.Context, user *openfga.User, params bootstrap.DestroyControllerParams) (int64, error)
	WaitForJobCompletion_      func(ctx context.Context, jobId int64, config bootstrap.WaitConfig) error
	BootstrapController_       func(ctx context.Context, p bootstrap.RunBootstrapArgs, cmdFactory bootstrap.CommandFactory, user *openfga.User) error
	DestroyController_         func(ctx context.Context, p bootstrap.RunDestroyControllerArgs, cmdFactory bootstrap.CommandFactory, user *openfga.User) error
}

func (b *BootstapManager) GetJobInfo(ctx context.Context, user *openfga.User, jobId int64, offset int) (params.GetBootstrapInfoResponse, error) {
	if b.GetJobInfo_ == nil {
		return params.GetBootstrapInfoResponse{}, errors.E(errors.CodeNotImplemented)
	}
	return b.GetJobInfo_(ctx, user, jobId, offset)
}

func (b *BootstapManager) StopJob(ctx context.Context, user *openfga.User, jobId int64) error {
	if b.StopJob_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return b.StopJob_(ctx, user, jobId)
}

func (b *BootstapManager) StartBootstrapJob(ctx context.Context, user *openfga.User, params bootstrap.BootstrapParams) (int64, error) {
	if b.StartBootstrapJob_ == nil {
		return 0, errors.E(errors.CodeNotImplemented)
	}
	return b.StartBootstrapJob_(ctx, user, params)
}

func (b *BootstapManager) StartDestroyControllerJob(ctx context.Context, user *openfga.User, params bootstrap.DestroyControllerParams) (int64, error) {
	if b.StartDestroyControllerJob_ == nil {
		return 0, errors.E(errors.CodeNotImplemented)
	}
	return b.StartDestroyControllerJob_(ctx, user, params)
}

func (b *BootstapManager) WaitForJobCompletion(ctx context.Context, jobId int64, config bootstrap.WaitConfig) error {
	if b.WaitForJobCompletion_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return b.WaitForJobCompletion_(ctx, jobId, config)
}

func (b *BootstapManager) BootstrapController(ctx context.Context, p bootstrap.RunBootstrapArgs, cmdFactory bootstrap.CommandFactory, user *openfga.User) error {
	if b.BootstrapController_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return b.BootstrapController_(ctx, p, cmdFactory, user)
}

func (b *BootstapManager) DestroyController(ctx context.Context, p bootstrap.RunDestroyControllerArgs, cmdFactory bootstrap.CommandFactory, user *openfga.User) error {
	if b.DestroyController_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return b.DestroyController_(ctx, p, cmdFactory, user)
}
