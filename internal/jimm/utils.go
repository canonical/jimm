// Copyright 2024 Canonical.
package jimm

import (
	"context"
	"fmt"
	"strings"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

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

// getController gets the controller from the database by name.
func (j *JIMM) getControllerByName(ctx context.Context, controllerName string) (*dbmodel.Controller, error) {
	controller := dbmodel.Controller{Name: controllerName}
	err := j.Database.GetController(ctx, &controller)
	if err != nil {
		return nil, errors.E(errors.CodeNotFound, "controller not found")
	}
	return &controller, nil
}

// checkReservedCloudNames checks if the tag intended to be added to JIMM
// is a reserved name.
func (j *JIMM) checkReservedCloudNames(tag names.CloudTag) error {
	reservedNames := j.ReservedCloudNames
	if len(reservedNames) == 0 {
		reservedNames = DefaultReservedCloudNames
	}
	for _, n := range reservedNames {
		if tag.Id() == n {
			return errors.E(errors.CodeAlreadyExists, fmt.Sprintf("cloud %q already exists", tag.Id()))
		}
	}
	return nil
}

// validateCloudRegion validates that the cloud region:
//
// - Exists
// - The user can add models using this cloud
// - The host cloud region is set
// - The controller we wish to add a cloud to is in the region
func (j *JIMM) validateCloudRegion(ctx context.Context, user *openfga.User, cloud jujuparams.Cloud, controllerName string) error {
	if cloud.HostCloudRegion == "" {
		return nil
	}

	parts := strings.SplitN(cloud.HostCloudRegion, "/", 2)
	if len(parts) != 2 || parts[0] == "" {
		return errors.E(errors.CodeIncompatibleClouds, fmt.Sprintf("cloud host region %q has invalid cloud/region format", cloud.HostCloudRegion))
	}

	region, err := j.Database.FindRegion(ctx, parts[0], parts[1])
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(errors.CodeIncompatibleClouds, fmt.Sprintf("unable to find cloud/region %q", cloud.HostCloudRegion))
		}
		return err
	}

	allowedAddModel, err := user.IsAllowedAddModel(ctx, region.Cloud.ResourceTag())
	if err != nil {
		return err
	}
	if !allowedAddModel {
		return errors.E(errors.CodeUnauthorized, fmt.Sprintf("missing access to %q", cloud.HostCloudRegion))
	}

	if region.Cloud.HostCloudRegion != "" {
		return errors.E(errors.CodeIncompatibleClouds, fmt.Sprintf("cloud already hosted %q", cloud.HostCloudRegion))
	}

	for _, rc := range region.Controllers {
		if rc.Controller.Name == controllerName {
			return nil
		}
	}
	return errors.E(errors.CodeNotFound, "controller not found")
}
