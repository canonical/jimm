// Copyright 2024 Canonical.
package jimm

import (
	"context"

	"github.com/juju/names/v5"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

/**
* Authorisation utilities
**/

// checkJimmAdmin checks if the user is a JIMM admin.
func (j *JIMM) checkJimmAdmin(user *openfga.User) error {
	if !user.JimmAdmin {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}
	return nil
}

// checkAdminAccess checks if the user is an admin of the controller.
func (j *JIMM) checkControllerAdminAccess(ctx context.Context, user *openfga.User, controller *dbmodel.Controller) error {
	isAdministrator, err := openfga.IsAdministrator(ctx, user, controller.ResourceTag())
	if err != nil {
		return err
	}
	if !isAdministrator {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}
	return nil
}

/**
* General utility
**/

// getController gets the controller from the database by name.
func (j *JIMM) getControllerByName(ctx context.Context, controllerName string) (*dbmodel.Controller, error) {
	controller := dbmodel.Controller{Name: controllerName}
	err := j.Database.GetController(ctx, &controller)
	if err != nil {
		return nil, errors.E(errors.CodeNotFound, "controller not found")
	}
	return &controller, nil
}

// dialController dials a controller.
func (j *JIMM) dialController(ctx context.Context, ctl *dbmodel.Controller) (API, error) {
	api, err := j.dial(ctx, ctl, names.ModelTag{})
	if err != nil {
		zapctx.Error(ctx, "failed to dial the controller", zaputil.Error(err))
		return nil, err
	}
	return api, nil
}

// dialModel dials a model.
func (j *JIMM) dialModel(ctx context.Context, ctl *dbmodel.Controller, mt names.ModelTag) (API, error) {
	api, err := j.dial(ctx, ctl, mt)
	if err != nil {
		zapctx.Error(ctx, "failed to dial the controller", zaputil.Error(err))
		return nil, err
	}
	return api, nil
}
