package jimmtest

import (
	"context"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/pkg/api/params"
	jujuparams "github.com/juju/juju/rpc/params"
)

// ControllerService is an implementation of the jujuapi.ControllerService interface.
type ControllerService struct {
	AddController_           func(ctx context.Context, u *openfga.User, ctl *dbmodel.Controller) error
	ControllerInfo_          func(ctx context.Context, name string) (params.ControllerInfo, error)
	GetControllerConfig_     func(ctx context.Context, u *dbmodel.Identity) (*dbmodel.ControllerConfig, error)
	ListControllers_         func(ctx context.Context, user *openfga.User) ([]dbmodel.Controller, error)
	RemoveController_        func(ctx context.Context, user *openfga.User, controllerName string, force bool) error
	SetControllerConfig_     func(ctx context.Context, u *openfga.User, args jujuparams.ControllerConfigSet) error
	SetControllerDeprecated_ func(ctx context.Context, user *openfga.User, controllerName string, deprecated bool) error
}

func (j *ControllerService) AddController(ctx context.Context, u *openfga.User, ctl *dbmodel.Controller) error {
	if j.AddController_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.AddController_(ctx, u, ctl)
}

func (j *ControllerService) ControllerInfo(ctx context.Context, name string) (params.ControllerInfo, error) {
	if j.ControllerInfo_ == nil {
		return params.ControllerInfo{}, errors.E(errors.CodeNotImplemented)
	}
	return j.ControllerInfo_(ctx, name)
}

func (j *ControllerService) GetControllerConfig(ctx context.Context, u *dbmodel.Identity) (*dbmodel.ControllerConfig, error) {
	if j.GetControllerConfig_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.GetControllerConfig_(ctx, u)
}

func (j *ControllerService) ListControllers(ctx context.Context, user *openfga.User) ([]dbmodel.Controller, error) {
	if j.ListControllers_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ListControllers_(ctx, user)
}

func (j *ControllerService) RemoveController(ctx context.Context, user *openfga.User, controllerName string, force bool) error {
	if j.RemoveController_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RemoveController_(ctx, user, controllerName, force)
}

func (j *ControllerService) SetControllerConfig(ctx context.Context, u *openfga.User, args jujuparams.ControllerConfigSet) error {
	if j.SetControllerConfig_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.SetControllerConfig_(ctx, u, args)
}

func (j *ControllerService) SetControllerDeprecated(ctx context.Context, user *openfga.User, controllerName string, deprecated bool) error {
	if j.SetControllerDeprecated_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.SetControllerDeprecated_(ctx, user, controllerName, deprecated)
}
