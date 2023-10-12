// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"
	"fmt"
	"strings"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
)

func (j *JIMM) getUserCloudAccess(ctx context.Context, user *openfga.User, cloud names.CloudTag) (string, error) {
	accessLevel := user.GetCloudAccess(ctx, cloud)
	if accessLevel == ofganames.NoRelation {
		everyoneTag := names.NewUserTag(auth.Everyone)
		everyone := openfga.NewUser(
			&dbmodel.User{
				Username: everyoneTag.Id(),
			},
			j.OpenFGAClient,
		)
		accessLevel = everyone.GetCloudAccess(ctx, cloud)
	}
	return ToCloudAccessString(accessLevel), nil
}

// GetCloud retrieves the cloud for the given cloud tag. If the cloud
// cannot be found then an error with the code CodeNotFound is
// returned. If the user does not have permission to view the cloud then an
// error with a code of CodeUnauthorized is returned. If the user only has
// add-model access to the cloud then the returned Users field will only
// contain the authentcated user.
func (j *JIMM) GetCloud(ctx context.Context, u *openfga.User, tag names.CloudTag) (dbmodel.Cloud, error) {
	const op = errors.Op("jimm.GetCloud")

	var cl dbmodel.Cloud
	cl.SetTag(tag)

	if err := j.Database.GetCloud(ctx, &cl); err != nil {
		return cl, errors.E(op, err)
	}

	accessLevel, err := j.getUserCloudAccess(ctx, u, tag)
	if err != nil {
		if err != nil {
			return dbmodel.Cloud{}, errors.E(op, err)
		}
	}

	switch accessLevel {
	case "":
		return dbmodel.Cloud{}, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	case "admin":
		return cl, nil
	default:
		// at this point the user must have add-model permission on the cloud
		cl.Users = []dbmodel.UserCloudAccess{{
			Username: u.Username,
			User:     *u.User,
			Access:   "add-model",
		}}
		return cl, nil
	}
}

// ForEachUserCloud iterates through all of the clouds a user has access to
// calling the given function for each cloud. If the user has admin level
// access to the cloud then the provided cloud will include all user
// information, otherwise it will just include the authenticated user. If
// the authenticated user is a controller superuser and the all flag is
// true then f will be called with all clouds known to JIMM. If f returns
// an error then iteration will stop immediately and the error will be
// returned unchanged. The given function should not update the database.
func (j *JIMM) ForEachUserCloud(ctx context.Context, user *openfga.User, f func(*dbmodel.Cloud) error) error {
	const op = errors.Op("jimm.ForEachUserCloud")

	clouds, err := j.Database.GetClouds(ctx)
	if err != nil {
		return errors.E(op, err, "cannot load clouds")
	}
	seen := make(map[string]bool, len(clouds))
	for _, cloud := range clouds {
		userAccess := ToCloudAccessString(user.GetCloudAccess(ctx, cloud.ResourceTag()))
		if userAccess == "" {
			// If user does not have access to the cloud,
			// we skip this cloud.
			continue
		}
		// If user is not a cloud admin they will only see themselves as users.
		if userAccess != "admin" {
			cloud.Users = []dbmodel.UserCloudAccess{{
				Username: user.Username,
				User:     *user.User,
				Access:   userAccess,
			}}
		}
		if err := f(&cloud); err != nil {
			return err
		}
		seen[cloud.Name] = true
	}

	// Also include "public" clouds
	everyoneDB := dbmodel.User{
		Username: auth.Everyone,
	}
	everyone := openfga.NewUser(&everyoneDB, j.OpenFGAClient)

	for _, cloud := range clouds {
		if seen[cloud.Name] {
			continue
		}
		userAccess := ToCloudAccessString(everyone.GetCloudAccess(ctx, cloud.ResourceTag()))
		if userAccess == "" {
			// if user does not have access to the cloud,
			// we skip this cloud
			continue
		}
		// For public clouds a user can only ever see themselves.
		cloud.Users = []dbmodel.UserCloudAccess{{
			Username: user.Username,
			User:     *user.User,
			Access:   userAccess,
		}}
		if err := f(&cloud); err != nil {
			return err
		}
	}

	return nil
}

// ForEachCloud iterates through each cloud known to JIMM calling the given
// function. If f returns an error then iteration stops immediately and the
// error is returned unmodified. If the given user is not a controller
// superuser then an error with the code CodeUnauthorized is returned. The
// given function should not update the database.
func (j *JIMM) ForEachCloud(ctx context.Context, user *openfga.User, f func(*dbmodel.Cloud) error) error {
	const op = errors.Op("jimm.ForEachCloud")

	isControllerAdmin, err := openfga.IsAdministrator(ctx, user, j.ResourceTag())
	if err != nil {
		return errors.E(op, err)
	}
	if !isControllerAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	clds, err := j.Database.GetClouds(ctx)
	if err != nil {
		return errors.E(op, "cannot load clouds", err)
	}

	for i := range clds {
		if err := f(&clds[i]); err != nil {
			return err
		}
	}
	return nil
}

// DefaultReservedCloudNames contains a list of cloud names that are used
// with public (or similar) clouds that cannot be used for the name of a
// hosted cloud.
var DefaultReservedCloudNames = []string{
	"aks",
	"aws",
	"aws-china",
	"aws-gov",
	"azure",
	"azure-china",
	"cloudsigma",
	"ecs",
	"eks",
	"google",
	"joyent",
	"localhost",
	"oracle",
	"oracle-classic",
	"oracle-compute",
	"rackspace",
}

// AddCloudToController adds the cloud defined by the given tag and
// cloud to the JAAS system. The cloud will be created on the specified
// controller running on the requested host cloud-region and the cloud
// created there. If the controller does not host the cloud-regions
// an error with code of CodeNotFound will be returned. If the given
// user does not have add-model access to JAAS then an error with a code of
// CodeUnauthorized will be returned (please note this differs from juju
// which requires admin controller access to create clouds). If the
// requested cloud cannot be created on this JAAS system an error with a
// code of CodeIncompatibleClouds will be returned. If there is an error
// returned by the controller when creating the cloud then that error code
// will be preserved.
func (j *JIMM) AddCloudToController(ctx context.Context, user *openfga.User, controllerName string, tag names.CloudTag, cloud jujuparams.Cloud) error {
	const op = errors.Op("jimm.AddCloudToController")

	controller := dbmodel.Controller{
		Name: controllerName,
	}
	err := j.Database.GetController(ctx, &controller)
	if err != nil {
		return errors.E(op, errors.CodeNotFound, "controller not found")
	}

	// NOTE (alesstimec) Previously the code checked:
	//  u.ControllerAccess != "login" && u.ControllerAccess != "superuser"
	// I have changed this to require the user to be controller administrator
	// which contradicts the godoc of this method. We will need to
	// reconsider which is the right approach and either change this
	// check or the godoc.
	isAdministrator, err := openfga.IsAdministrator(ctx, user, controller.ResourceTag())
	if err != nil {
		return errors.E(op, err)
	}

	if !isAdministrator {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	// Ensure the new cloud could not mask the name of a known public cloud.
	reservedNames := j.ReservedCloudNames
	if len(reservedNames) == 0 {
		reservedNames = DefaultReservedCloudNames
	}
	for _, n := range reservedNames {
		if tag.Id() == n {
			return errors.E(op, errors.CodeAlreadyExists, fmt.Sprintf("cloud %q already exists", tag.Id()))
		}
	}

	if cloud.HostCloudRegion != "" {
		parts := strings.SplitN(cloud.HostCloudRegion, "/", 2)
		if len(parts) != 2 || parts[0] == "" {
			return errors.E(op, errors.CodeIncompatibleClouds, fmt.Sprintf("unsupported cloud host region %q", cloud.HostCloudRegion))
		}
		region, err := j.Database.FindRegion(ctx, parts[0], parts[1])
		if err != nil {
			if errors.ErrorCode(err) == errors.CodeNotFound {
				return errors.E(op, err, errors.CodeIncompatibleClouds, fmt.Sprintf("unsupported cloud host region %q", cloud.HostCloudRegion))
			}
			return errors.E(op, err)
		}
		allowedAddModel, err := user.IsAllowedAddModel(ctx, region.Cloud.ResourceTag())
		if err != nil {
			return errors.E(op, err)
		}

		if !allowedAddModel {
			return errors.E(op, errors.CodeIncompatibleClouds, fmt.Sprintf("unsupported cloud host region %q", cloud.HostCloudRegion))
		}

		if region.Cloud.HostCloudRegion != "" {
			// Do not support creating a new cloud on an already hosted
			// cloud.
			return errors.E(op, errors.CodeIncompatibleClouds, fmt.Sprintf("unsupported cloud host region %q", cloud.HostCloudRegion))
		}

		found := false
		for _, rc := range region.Controllers {
			if rc.Controller.Name == controllerName {
				found = true
				break
			}
		}
		if !found {
			return errors.E(op, errors.CodeNotFound, "controller not found")
		}
	}

	var dbCloud dbmodel.Cloud
	dbCloud.FromJujuCloud(cloud)
	dbCloud.Name = tag.Id()
	// TODO (alesstimec) remove as this will no longer be needed.
	dbCloud.Users = []dbmodel.UserCloudAccess{{
		User:   *user.User,
		Access: "admin",
	}}

	ccloud, err := j.addControllerCloud(ctx, &controller, user.ResourceTag(), tag, cloud)
	if err != nil {
		return errors.E(op, err)
	}

	dbCloud.FromJujuCloud(*ccloud)
	for i := range dbCloud.Regions {
		dbCloud.Regions[i].Controllers = []dbmodel.CloudRegionControllerPriority{{
			ControllerID: controller.ID,
			Priority:     dbmodel.CloudRegionControllerPrioritySupported,
		}}
	}
	if err := j.Database.AddCloud(ctx, &dbCloud); err != nil {
		return errors.E(op, err)
	}

	err = j.OpenFGAClient.AddCloudController(ctx, dbCloud.ResourceTag(), controller.ResourceTag())
	if err != nil {
		zapctx.Error(
			ctx,
			"failed to add controller relation between controller and cloud",
			zap.String("controller", controller.ResourceTag().Id()),
			zap.String("cloud", dbCloud.ResourceTag().Id()),
			zap.Error(err),
		)
	}

	return nil
}

// AddHostedCloud adds the cloud defined by the given tag and cloud to the
// JAAS system. The cloud will be created on a controller running on the
// requested host cloud-region and the cloud created there. If the given
// user does not have add-model access to JAAS then an error with a code of
// CodeUnauthorized will be returned (please note this differs from juju
// which requires admin controller access to create clouds). If the
// requested cloud cannot be created on this JAAS system an error with a
// code of CodeIncompatibleClouds will be returned. If there is an error
// returned by the controller when creating the cloud then that error code
// will be preserved.
func (j *JIMM) AddHostedCloud(ctx context.Context, user *openfga.User, tag names.CloudTag, cloud jujuparams.Cloud) error {
	const op = errors.Op("jimm.AddHostedCloud")

	// NOTE (alesstimec) The default JIMM access right for every user is
	// "login". Previously the code checked:
	//  u.ControllerAccess != "login" && u.ControllerAccess != "superuser"
	// so this check was removed.

	// Ensure the new cloud could not mask the name of a known public cloud.
	reservedNames := j.ReservedCloudNames
	if len(reservedNames) == 0 {
		reservedNames = DefaultReservedCloudNames
	}
	for _, n := range reservedNames {
		if tag.Id() == n {
			return errors.E(op, errors.CodeAlreadyExists, fmt.Sprintf("cloud %q already exists", tag.Id()))
		}
	}

	// Validate that the requested cloud is valid.
	if cloud.Type != "kubernetes" {
		return errors.E(op, errors.CodeIncompatibleClouds, fmt.Sprintf("unsupported cloud type %q", cloud.Type))
	}
	if cloud.HostCloudRegion == "" {
		return errors.E(op, errors.CodeCloudRegionRequired, "cloud host region not specified")
	}
	parts := strings.SplitN(cloud.HostCloudRegion, "/", 2)
	if len(parts) != 2 || parts[0] == "" {
		return errors.E(op, errors.CodeIncompatibleClouds, fmt.Sprintf("unsupported cloud host region %q", cloud.HostCloudRegion))
	}
	region, err := j.Database.FindRegion(ctx, parts[0], parts[1])
	if errors.ErrorCode(err) == errors.CodeNotFound {
		return errors.E(op, err, errors.CodeIncompatibleClouds, fmt.Sprintf("unsupported cloud host region %q", cloud.HostCloudRegion))
	} else if err != nil {
		return errors.E(op, err)
	}

	allowedAddModel, err := user.IsAllowedAddModel(ctx, region.Cloud.ResourceTag())
	if err != nil {
		return errors.E(op, err)
	}
	if !allowedAddModel {
		everyone := openfga.NewUser(&dbmodel.User{Username: auth.Everyone}, j.OpenFGAClient)
		everyonewAllowedAddModel, err := everyone.IsAllowedAddModel(ctx, region.Cloud.ResourceTag())
		if err != nil {
			return errors.E(op, err)
		}
		if !everyonewAllowedAddModel {
			return errors.E(op, errors.CodeIncompatibleClouds, fmt.Sprintf("unsupported cloud host region %q", cloud.HostCloudRegion))
		}
	}

	if region.Cloud.HostCloudRegion != "" {
		// Do not support creating a new cloud on an already hosted
		// cloud.
		return errors.E(op, errors.CodeIncompatibleClouds, fmt.Sprintf("unsupported cloud host region %q", cloud.HostCloudRegion))
	}

	// Create the cloud locally, to reserve the name.
	var dbCloud dbmodel.Cloud
	dbCloud.FromJujuCloud(cloud)
	dbCloud.Name = tag.Id()
	// NOTE (alesstimec) this will no longer be needed.
	dbCloud.Users = []dbmodel.UserCloudAccess{{
		User:   *user.User,
		Access: "admin",
	}}
	if err := j.Database.AddCloud(ctx, &dbCloud); err != nil {
		return errors.E(op, err)
	}

	// Create the cloud on a host.
	shuffleRegionControllers(region.Controllers)
	controller := region.Controllers[0].Controller

	ccloud, err := j.addControllerCloud(ctx, &controller, user.ResourceTag(), tag, cloud)
	if err != nil {
		// TODO(mhilton) remove the added cloud if adding it to the controller failed.
		return errors.E(op, err)
	}
	// Update the cloud in the database.
	dbCloud.FromJujuCloud(*ccloud)
	zapctx.Debug(ctx, "received cloud info from controller", zap.Any("cloud", dbCloud))
	for i := range dbCloud.Regions {
		dbCloud.Regions[i].Controllers = []dbmodel.CloudRegionControllerPriority{{
			ControllerID: controller.ID,
			Priority:     dbmodel.CloudRegionControllerPrioritySupported,
		}}
	}
	zapctx.Debug(ctx, "received cloud info from controller", zap.Any("cloud", dbCloud))

	if err := j.Database.UpdateCloud(ctx, &dbCloud); err != nil {
		// At this point the cloud has been created on the
		// controller and we know something about it. Trying to
		// undo that will probably make things worse.
		return errors.E(op, err)
	}

	err = j.OpenFGAClient.AddCloudController(ctx, dbCloud.ResourceTag(), controller.ResourceTag())
	if err != nil {
		zapctx.Error(
			ctx,
			"failed to add controller relation between controller and cloud",
			zap.String("controller", controller.ResourceTag().Id()),
			zap.String("cloud", dbCloud.ResourceTag().Id()),
			zap.Error(err),
		)
	}
	err = user.SetCloudAccess(ctx, dbCloud.ResourceTag(), ofganames.AdministratorRelation)
	if err != nil {
		zapctx.Error(
			ctx,
			"failed to add user as cloud admin",
			zap.String("user", user.Username),
			zap.String("cloud", dbCloud.ResourceTag().Id()),
			zap.Error(err),
		)
	}
	return nil
}

func randomController() func(controllers []dbmodel.CloudRegionControllerPriority) (dbmodel.Controller, error) {
	return func(controllers []dbmodel.CloudRegionControllerPriority) (dbmodel.Controller, error) {
		shuffleRegionControllers(controllers)
		return controllers[0].Controller, nil
	}
}

func findController(controllerName string) func(controllers []dbmodel.CloudRegionControllerPriority) (dbmodel.Controller, error) {
	return func(controllers []dbmodel.CloudRegionControllerPriority) (dbmodel.Controller, error) {
		for _, crp := range controllers {
			crp := crp
			if crp.Controller.Name == controllerName {
				return crp.Controller, nil
			}
		}
		return dbmodel.Controller{}, errors.E("controller not found", errors.CodeNotFound)
	}
}

// addControllerCloud creates the hosted cloud defined by the given tag and
// jujuparams cloud definition. Admin access to the cloud will be granted
// to the user identified by the given user tag. On success
// addControllerCloud returns the definition of the cloud retrieved from
// the controller.
func (j *JIMM) addControllerCloud(ctx context.Context, ctl *dbmodel.Controller, ut names.UserTag, tag names.CloudTag, cloud jujuparams.Cloud) (*jujuparams.Cloud, error) {
	const op = errors.Op("jimm.addControllerCloud")

	api, err := j.dial(ctx, ctl, names.ModelTag{})
	if err != nil {
		return nil, errors.E(op, err)
	}
	defer api.Close()
	if err := api.AddCloud(ctx, tag, cloud); err != nil {
		return nil, errors.E(op, err)
	}
	// TODO (alesstimec) This will no longer be needed.
	if err := api.GrantCloudAccess(ctx, tag, ut, "admin"); err != nil {
		return nil, errors.E(op, err)
	}
	var result jujuparams.Cloud
	if err := api.Cloud(ctx, tag, &result); err != nil {
		return nil, errors.E(op, err)
	}

	return &result, nil
}

// doCloudAdmin is a simple wrapper that provides the common parts of cloud
// administration commands. doCloudAdmin finds the cloud with the given tag
// and validates that the given user has admin access to the cloud.
// doCloudAdmin then connects to the controller hosting the cloud and calls
// the given function with the cloud and API connection to perform the
// operation specific commands. If the cloud cannot be found then an error
// with the code CodeNotFound is returned. If the given user does not have
// admin access to the cloud then an error with the code CodeUnauthorized
// is returned. If there is an error connecting to the controller hosting
// the cloud then the returned error will have the same code as the error
// returned from the dial operation. If the given function returns an error
// that error will be returned with the code unmasked.
func (j *JIMM) doCloudAdmin(ctx context.Context, u *openfga.User, ct names.CloudTag, f func(*dbmodel.Cloud, API) error) error {
	const op = errors.Op("jimm.doCloudAdmin")

	var c dbmodel.Cloud
	c.SetTag(ct)

	if err := j.Database.GetCloud(ctx, &c); err != nil {
		return errors.E(op, err)
	}

	isCloudAdministrator, err := openfga.IsAdministrator(ctx, u, c.ResourceTag())
	if err != nil {
		return errors.E(op, err)
	}
	if !isCloudAdministrator {
		// If the user doesn't have admin access on the cloud return
		// an unauthorized error.
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	// Ensure we always have at least 1 region for the cloud with at least 1 controller
	// managing that region.
	if len(c.Regions) < 1 || len(c.Regions[0].Controllers) < 1 {
		zapctx.Error(ctx, "number of regions available in cloud", zap.String("cloud", c.Name), zap.Int("regions", len(c.Regions)))
		if len(c.Regions) > 0 {
			zapctx.Error(ctx, "number of controllers available for cloud/region", zap.Int("controllers", len(c.Regions[0].Controllers)))
		}
		return errors.E(op, fmt.Sprintf("cloud administration not available for %s", ct.Id()))
	}
	api, err := j.dial(ctx, &c.Regions[0].Controllers[0].Controller, names.ModelTag{})
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()
	if err := f(&c, api); err != nil {
		return errors.E(op, err)
	}
	return nil
}

// GrantCloudAccess grants the given access level on the given cloud to the
// given user. If the cloud is not found then an error with the code
// CodeNotFound is returned. If the authenticated user does not have admin
// access to the cloud then an error with the code CodeUnauthorized is
// returned.
func (j *JIMM) GrantCloudAccess(ctx context.Context, user *openfga.User, ct names.CloudTag, ut names.UserTag, access string) error {
	const op = errors.Op("jimm.GrantCloudAccess")

	targetRelation, err := ToCloudRelation(access)
	if err != nil {
		zapctx.Debug(
			ctx,
			"failed to recognize given access",
			zaputil.Error(err),
			zap.String("access", string(access)),
		)
		return errors.E(op, errors.CodeBadRequest, fmt.Sprintf("failed to recognize given access: %q", access), err)
	}

	err = j.doCloudAdmin(ctx, user, ct, func(_ *dbmodel.Cloud, _ API) error {
		targetUser := &dbmodel.User{}
		targetUser.SetTag(ut)
		if err := j.Database.GetUser(ctx, targetUser); err != nil {
			return err
		}
		targetOfgaUser := openfga.NewUser(targetUser, j.OpenFGAClient)

		currentRelation := targetOfgaUser.GetCloudAccess(ctx, ct)
		switch targetRelation {
		case ofganames.CanAddModelRelation:
			switch currentRelation {
			case ofganames.NoRelation:
				break
			default:
				return nil
			}
		case ofganames.AdministratorRelation:
			switch currentRelation {
			case ofganames.NoRelation, ofganames.CanAddModelRelation:
				break
			default:
				return nil
			}
		}

		if err := targetOfgaUser.SetCloudAccess(ctx, ct, targetRelation); err != nil {
			return errors.E(err, op, "failed to set cloud access")
		}
		return nil
	})

	if err != nil {
		zapctx.Error(
			ctx,
			"failed to grant cloud access",
			zaputil.Error(err),
			zap.String("targetUser", string(ut.Id())),
			zap.String("cloud", string(ct.Id())),
			zap.String("access", string(access)),
		)
		return errors.E(op, err)
	}
	return nil
}

// RevokeCloudAccess revokes the given access level on the given cloud from
// the given user. If the cloud is not found then an error with the code
// CodeNotFound is returned. If the authenticated user does not have admin
// access to the cloud then an error with the code CodeUnauthorized is
// returned.
func (j *JIMM) RevokeCloudAccess(ctx context.Context, user *openfga.User, ct names.CloudTag, ut names.UserTag, access string) error {
	const op = errors.Op("jimm.RevokeCloudAccess")

	targetRelation, err := ToCloudRelation(access)
	if err != nil {
		zapctx.Debug(
			ctx,
			"failed to recognize given access",
			zaputil.Error(err),
			zap.String("access", string(access)),
		)
		return errors.E(op, errors.CodeBadRequest, fmt.Sprintf("failed to recognize given access: %q", access), err)
	}

	err = j.doCloudAdmin(ctx, user, ct, func(_ *dbmodel.Cloud, _ API) error {
		targetUser := &dbmodel.User{}
		targetUser.SetTag(ut)
		if err := j.Database.GetUser(ctx, targetUser); err != nil {
			return err
		}
		targetOfgaUser := openfga.NewUser(targetUser, j.OpenFGAClient)

		currentRelation := targetOfgaUser.GetCloudAccess(ctx, ct)

		var relationsToRevoke []openfga.Relation
		switch targetRelation {
		case ofganames.CanAddModelRelation:
			switch currentRelation {
			case ofganames.NoRelation:
				return nil
			default:
				// If we're revoking "add-model" access, in addition to the "add-model" relation, we should also revoke the
				// "admin" relation. That's because having an "admin" relation indirectly grants the "add-model" permission
				// to the user.
				relationsToRevoke = []openfga.Relation{
					ofganames.CanAddModelRelation,
					ofganames.AdministratorRelation,
				}
			}
		case ofganames.AdministratorRelation:
			switch currentRelation {
			case ofganames.NoRelation, ofganames.CanAddModelRelation:
				return nil
			default:
				relationsToRevoke = []openfga.Relation{
					ofganames.AdministratorRelation,
				}
			}
		}

		if err := targetOfgaUser.UnsetCloudAccess(ctx, ct, relationsToRevoke...); err != nil {
			return errors.E(err, op, "failed to unset cloud access")
		}
		return nil
	})

	if err != nil {
		zapctx.Error(
			ctx,
			"failed to revoke cloud access",
			zaputil.Error(err),
			zap.String("targetUser", string(ut.Id())),
			zap.String("cloud", string(ct.Id())),
			zap.String("access", string(access)),
		)
		return errors.E(op, err)
	}
	return nil
}

// RemoveCloud removes the given cloud from JAAS If the cloud is not found
// then an error with the code CodeNotFound is returned. If the
// authenticated user does not have admin access to the cloud then an error
// with the code CodeUnauthorized is returned. If the RemoveClouds API call
// retuns an error the error code is not masked.
func (j *JIMM) RemoveCloud(ctx context.Context, u *openfga.User, ct names.CloudTag) error {
	const op = errors.Op("jimm.RemoveCloud")

	err := j.doCloudAdmin(ctx, u, ct, func(c *dbmodel.Cloud, api API) error {
		// Note: JIMM doesn't attempt to determine if the cloud is
		// used by any models before attempting to remove it. JIMM
		// relies on the controller failing the RemoveClouds API
		// request if the cloud is in use.
		if err := api.RemoveCloud(ctx, ct); err != nil {
			return err
		}

		if err := j.Database.DeleteCloud(ctx, c); err != nil {
			return errors.E(op, err, "cannot update database after updating controller")
		}

		if err := j.OpenFGAClient.RemoveCloud(ctx, ct); err != nil {
			zapctx.Error(ctx, "failed to remove cloud from openfga", zap.String("cloud", ct.Id()), zap.Error(err))
		}
		return nil
	})
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// UpdateCloud updates the cloud with the given name on all controllers
// that host the cloud. If the given user is not a controller superuser or
// an admin on the cloud an error is returned with a code of
// CodeUnauthorized. If the cloud with the given name cannot be found then
// an error with the code CodeNotFound is returned.
func (j *JIMM) UpdateCloud(ctx context.Context, u *openfga.User, ct names.CloudTag, cloud jujuparams.Cloud) error {
	const op = errors.Op("jimm.UpdateCloud")

	var c dbmodel.Cloud
	c.SetTag(ct)

	if err := j.Database.GetCloud(ctx, &c); err != nil {
		return errors.E(op, err)
	}
	cloudAccess, err := j.getUserCloudAccess(ctx, u, c.ResourceTag())
	if err != nil {
		return errors.E(op, err)
	}
	if cloudAccess != "admin" {
		// If the user doesn't have admin access on the cloud return
		// an unauthorized error.
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	var controllers []dbmodel.Controller
	seen := make(map[uint]bool)
	for _, r := range c.Regions {
		for _, ctl := range r.Controllers {
			if seen[ctl.ControllerID] {
				continue
			}
			seen[ctl.ControllerID] = true
			controllers = append(controllers, ctl.Controller)
		}
	}

	err = j.forEachController(ctx, controllers, func(ctl *dbmodel.Controller, api API) error {
		return api.UpdateCloud(ctx, ct, cloud)
	})
	if err != nil {
		return errors.E(op, err)
	}

	// Update the local database with the updated cloud definition. We
	// do this in a transaction so that the local view cannot finish in
	// an inconsistent state.
	err = j.Database.Transaction(func(db *db.Database) error {

		var c dbmodel.Cloud
		c.SetTag(ct)
		if err := db.GetCloud(ctx, &c); err != nil {
			return err
		}
		c.FromJujuCloud(cloud)
		for i := range c.Regions {
			if len(c.Regions[i].Controllers) == 0 {
				for _, ctl := range controllers {
					c.Regions[i].Controllers = append(c.Regions[i].Controllers, dbmodel.CloudRegionControllerPriority{
						Controller: ctl,
						Priority:   dbmodel.CloudRegionControllerPrioritySupported,
					})
				}
			}
		}
		return db.UpdateCloud(ctx, &c)
	})

	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// RemoveCloudFromController removes the given cloud from the JAAS controller.
// If the cloud or the controller are not found then an error with the code
// CodeNotFound is returned. If the authenticated user does not have admin
// access to the cloud then an error with the code CodeUnauthorized is returned.
// If the RemoveClouds API call retuns an error the error code is not masked.
func (j *JIMM) RemoveCloudFromController(ctx context.Context, u *openfga.User, controllerName string, ct names.CloudTag) error {
	const op = errors.Op("jimm.RemoveCloudFromController")

	var cloud dbmodel.Cloud
	cloud.SetTag(ct)

	if err := j.Database.GetCloud(ctx, &cloud); err != nil {
		return errors.E(op, err)
	}

	isAdministrator, err := openfga.IsAdministrator(ctx, u, ct)
	if err != nil {
		return errors.E(op, err, errors.CodeUnauthorized, "unauthorized")
	}
	if !isAdministrator {
		// If the user doesn't have admin access on the cloud return
		// an unauthorized error.
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	controllers := make(map[string]dbmodel.Controller)
	for _, cr := range cloud.Regions {
		for _, rc := range cr.Controllers {
			controllers[rc.Controller.Name] = rc.Controller
		}
	}

	controller, ok := controllers[controllerName]
	if !ok {
		return errors.E(op, "cloud not hosted by controller", errors.CodeNotFound)
	}

	api, err := j.dial(ctx, &controller, names.ModelTag{})
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()

	// Note: JIMM doesn't attempt to determine if the cloud is
	// used by any models before attempting to remove it. JIMM
	// relies on the controller failing the RemoveClouds API
	// request if the cloud is in use.
	if err := api.RemoveCloud(ctx, ct); err != nil {
		return errors.E(op, err)
	}

	delete(controllers, controllerName)

	// if this was the only cloud controller, we delete the cloud
	if len(controllers) == 0 {
		if err := j.Database.DeleteCloud(ctx, &cloud); err != nil {
			return errors.E(op, err, "failed to delete cloud after updating controller")
		}
		return nil
	}

	// otherwise we need to update the cloud by removing the controller
	// from cloud regions
	for _, cr := range cloud.Regions {
		for _, crp := range cr.Controllers {
			crp := crp
			if err := j.Database.DeleteCloudRegionControllerPriority(ctx, &crp); err != nil {
				return errors.E(op, err, "cannot update database after updating controller")
			}
		}
	}

	if err := j.OpenFGAClient.RemoveCloud(ctx, ct); err != nil {
		zapctx.Error(ctx, "failed to remove cloud", zap.String("cloud", ct.String()), zap.Error(err))
	}

	return nil
}
