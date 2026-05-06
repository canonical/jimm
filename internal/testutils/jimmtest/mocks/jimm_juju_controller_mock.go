// Copyright 2025 Canonical.

package mocks

import (
	"context"

	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// ControllerService is an implementation of the jujuapi.ControllerService interface.
type ControllerService struct {
	AddController_                     func(ctx context.Context, u *openfga.User, ctl *dbmodel.Controller, creds juju.ControllerCreds) error
	ControllerDetailsForModel_         func(ctx context.Context, modelUUID string) (juju.ControllerConnectionDetails, error)
	ControllerDetailsForIncomingModel_ func(ctx context.Context, modelUUID string) (juju.ControllerConnectionDetails, error)
	ControllerInfo_                    func(ctx context.Context, name string) (*dbmodel.Controller, error)
	EarliestControllerVersion_         func(ctx context.Context) (version.Number, error)
	ListControllers_                   func(ctx context.Context, user *openfga.User) ([]dbmodel.Controller, error)
	RemoveController_                  func(ctx context.Context, user *openfga.User, controllerName string, force bool) error
	SetControllerDeprecated_           func(ctx context.Context, user *openfga.User, controllerName string, deprecated bool) error
	ControllerConfig_                  func(ctx context.Context, user *openfga.User, controllerName string) (jujucontroller.Config, error)
}

func (j *ControllerService) AddController(ctx context.Context, u *openfga.User, ctl *dbmodel.Controller, creds juju.ControllerCreds) error {
	if j.AddController_ == nil {
		return errors.New("not implemented")
	}
	return j.AddController_(ctx, u, ctl, creds)
}

func (j *ControllerService) ControllerDetailsForModel(ctx context.Context, modelUUID string) (juju.ControllerConnectionDetails, error) {
	if j.ControllerDetailsForModel_ == nil {
		return juju.ControllerConnectionDetails{}, errors.New("not implemented")
	}
	return j.ControllerDetailsForModel_(ctx, modelUUID)
}

func (j *ControllerService) ControllerDetailsForIncomingModel(ctx context.Context, modelUUID string) (juju.ControllerConnectionDetails, error) {
	if j.ControllerDetailsForIncomingModel_ == nil {
		return juju.ControllerConnectionDetails{}, errors.New("not implemented")
	}
	return j.ControllerDetailsForIncomingModel_(ctx, modelUUID)
}

func (j *ControllerService) ControllerInfo(ctx context.Context, name string) (*dbmodel.Controller, error) {
	if j.ControllerInfo_ == nil {
		return nil, errors.New("not implemented")
	}
	return j.ControllerInfo_(ctx, name)
}

func (j *ControllerService) EarliestControllerVersion(ctx context.Context) (version.Number, error) {
	if j.EarliestControllerVersion_ == nil {
		return version.Number{}, errors.New("not implemented")
	}
	return j.EarliestControllerVersion_(ctx)
}

func (j *ControllerService) ListControllers(ctx context.Context, user *openfga.User) ([]dbmodel.Controller, error) {
	if j.ListControllers_ == nil {
		return nil, errors.New("not implemented")
	}
	return j.ListControllers_(ctx, user)
}

func (j *ControllerService) RemoveController(ctx context.Context, user *openfga.User, controllerName string, force bool) error {
	if j.RemoveController_ == nil {
		return errors.New("not implemented")
	}
	return j.RemoveController_(ctx, user, controllerName, force)
}

func (j *ControllerService) SetControllerDeprecated(ctx context.Context, user *openfga.User, controllerName string, deprecated bool) error {
	if j.SetControllerDeprecated_ == nil {
		return errors.New("not implemented")
	}
	return j.SetControllerDeprecated_(ctx, user, controllerName, deprecated)
}

func (j *ControllerService) ControllerConfig(ctx context.Context, user *openfga.User, controllerName string) (jujucontroller.Config, error) {
	if j.ControllerConfig_ == nil {
		return jujucontroller.Config{}, errors.New("not implemented")
	}
	return j.ControllerConfig_(ctx, user, controllerName)
}
