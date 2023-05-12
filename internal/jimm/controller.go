// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/juju/version"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/openfga"
	ofganames "github.com/CanonicalLtd/jimm/internal/openfga/names"
)

// AddController adds the specified controller to JIMM. Only
// controller-admin level users may add new controllers. If the user adding
// the controller is not authorized then an error with a code of
// CodeUnauthorized will be returned. If there already exists a controller
// with the same name as the controller being added then an error with a
// code of CodeAlreadyExists will be returned. If the controller cannot be
// contacted then an error with a code of CodeConnectionFailed will be
// returned.
func (j *JIMM) AddController(ctx context.Context, u *openfga.User, ctl *dbmodel.Controller) error {
	const op = errors.Op("jimm.AddController")

	isJIMMAdmin, err := openfga.IsAdministrator(ctx, u, j.ResourceTag())
	if err != nil {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	if !isJIMMAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	api, err := j.dial(ctx, ctl, names.ModelTag{})
	if err != nil {
		zapctx.Error(ctx, "failed to dial the controller", zaputil.Error(err))
		return errors.E(op, err, "failed to dial the controller")
	}
	defer api.Close()

	var ms jujuparams.ModelSummary
	if err := api.ControllerModelSummary(ctx, &ms); err != nil {
		zapctx.Error(ctx, "failed to get model summary", zaputil.Error(err))
		return errors.E(op, err, "failed to get model summary")
	}
	ct, err := names.ParseCloudTag(ms.CloudTag)
	if err != nil {
		return errors.E(op, err, "failed to parse the cloud tag")
	}
	ctl.CloudName = ct.Id()
	ctl.CloudRegion = ms.CloudRegion
	// TODO(mhilton) add the controller model?

	clouds, err := api.Clouds(ctx)
	if err != nil {
		return errors.E(op, err, "failed to fetch controller clouds")
	}

	var dbClouds []dbmodel.Cloud
	for tag, cld := range clouds {
		ctx := zapctx.WithFields(ctx, zap.Stringer("tag", tag))

		var cloud dbmodel.Cloud
		cloud.FromJujuCloud(cld)
		cloud.Name = tag.Id()

		// If this cloud is not the one used by the controller model then
		// it is only available to a subset of users.
		if tag.String() != ms.CloudTag {
			var err error
			cloud.Users, err = cloudUsers(ctx, api, tag)
			if err != nil {
				// If there is an error getting the users, log the failure
				// but carry on, this will prevent anyone trying to add a
				// cloud with the same name. The user access can be fixed
				// later.
				zapctx.Error(ctx, "cannot get cloud users", zap.Error(err))
			}
		} else {
			cloud.Users = []dbmodel.UserCloudAccess{{
				Username: auth.Everyone,
				User: dbmodel.User{
					Username: auth.Everyone,
				},
				Access: "add-model",
			}}
		}
		dbClouds = append(dbClouds, cloud)
	}

	credentialsStored := false
	if j.CredentialStore != nil {
		err := j.CredentialStore.PutControllerCredentials(ctx, ctl.Name, ctl.AdminUser, ctl.AdminPassword)
		if err != nil {
			return errors.E(op, err, "failed to store controller credentials")
		}
		credentialsStored = true
	}

	err = j.Database.Transaction(func(tx *db.Database) error {
		for i := range dbClouds {
			cloud := dbmodel.Cloud{
				Name: dbClouds[i].Name,
			}
			if err := tx.GetCloud(ctx, &cloud); err != nil {
				if errors.ErrorCode(err) != errors.CodeNotFound {
					zapctx.Error(ctx, "failed to fetch the cloud", zaputil.Error(err), zap.String("cloud-name", dbClouds[i].Name))
					return err
				}
				err := tx.AddCloud(ctx, &dbClouds[i])
				if err != nil && errors.ErrorCode(err) != errors.CodeAlreadyExists {
					zapctx.Error(ctx, "failed to add cloud", zaputil.Error(err))
					return err
				}
				if err := tx.GetCloud(ctx, &cloud); err != nil {
					zapctx.Error(ctx, "failed to fetch the cloud", zaputil.Error(err), zap.String("cloud-name", dbClouds[i].Name))
					return err
				}
			}
			for _, reg := range dbClouds[i].Regions {
				if cloud.Region(reg.Name).ID != 0 {
					continue
				}
				reg.CloudName = cloud.Name
				if err := tx.AddCloudRegion(ctx, &reg); err != nil {
					zapctx.Error(ctx, "failed to add cloud region", zaputil.Error(err))
					return err
				}
				cloud.Regions = append(cloud.Regions, reg)
			}
			for _, uca := range dbClouds[i].Users {
				if cloud.UserAccess(&uca.User) != "" {
					continue
				}
				uca.Username = uca.User.Username
				uca.CloudName = cloud.Name
				if err := tx.UpdateUserCloudAccess(ctx, &uca); err != nil {
					zapctx.Error(ctx, "failed to update user cloud access", zaputil.Error(err))
					return err
				}
				cloud.Users = append(cloud.Users, uca)
			}
			for _, cr := range dbClouds[i].Regions {
				reg := cloud.Region(cr.Name)
				priority := dbmodel.CloudRegionControllerPrioritySupported
				if cloud.Name == ctl.CloudName && cr.Name == ctl.CloudRegion {
					priority = dbmodel.CloudRegionControllerPriorityDeployed
				}
				ctl.CloudRegions = append(ctl.CloudRegions, dbmodel.CloudRegionControllerPriority{
					CloudRegion: reg,
					Priority:    uint(priority),
				})
			}
		}
		// if we already stored controller credentials in CredentialStore
		// we should not store them plain text in JIMM's DB.
		if credentialsStored {
			ctl.AdminUser = ""
			ctl.AdminPassword = ""
		}
		if err := tx.AddController(ctx, ctl); err != nil {
			if errors.ErrorCode(err) == errors.CodeAlreadyExists {
				zapctx.Error(ctx, "failed to add controller", zaputil.Error(err))
				return errors.E(op, err, fmt.Sprintf("controller %q already exists", ctl.Name))
			}
			zapctx.Error(ctx, "failed to add controller", zaputil.Error(err))
			return err
		}
		return nil
	})

	if err != nil {
		return errors.E(op, err)
	}

	for _, cloud := range dbClouds {
		// Add controller relation between the cloud and the added controller.
		err = j.OpenFGAClient.AddCloudController(ctx, cloud.ResourceTag(), ctl.ResourceTag())
		if err != nil {
			zapctx.Error(
				ctx,
				"failed to add controller relation between controller and cloud",
				zap.String("controller", ctl.ResourceTag().Id()),
				zap.String("cloud", cloud.ResourceTag().Id()),
				zap.Error(err),
			)
		}

		for _, uca := range cloud.Users {
			cloudUser := openfga.NewUser(
				&dbmodel.User{
					Username: uca.Username,
				},
				j.OpenFGAClient,
			)
			relation, err := ToCloudRelation(uca.Access)
			if err != nil {
				zapctx.Error(
					ctx,
					"failed to parse user cloud access",
					zap.String("user", uca.Username),
					zap.String("access", uca.Access),
					zap.Error(err),
				)
			} else {
				if err := cloudUser.SetCloudAccess(ctx, cloud.ResourceTag(), relation); err != nil {
					zapctx.Error(
						ctx,
						"failed to set cloud access",
						zap.String("user", uca.Username),
						zap.String("access", uca.Access),
						zap.Error(err),
					)
				}
			}
		}
	}

	// Finally add a controller relation between JIMM and the added controller.
	err = j.OpenFGAClient.AddController(ctx, j.ResourceTag(), ctl.ResourceTag())
	if err != nil {
		zapctx.Error(
			ctx,
			"failed to add controller relation between JIMM and controller",
			zap.String("controller", ctl.ResourceTag().Id()),
			zap.Error(err),
		)
	}

	return nil
}

// cloudUsers determines the users that can access a cloud.
func cloudUsers(ctx context.Context, api API, tag names.CloudTag) ([]dbmodel.UserCloudAccess, error) {
	const op = errors.Op("jimm.cloudUsers")
	var ci jujuparams.CloudInfo
	if err := api.CloudInfo(ctx, tag, &ci); err != nil {
		return nil, errors.E(op, err)
	}
	var users []dbmodel.UserCloudAccess
	for _, u := range ci.Users {
		if !strings.Contains(u.UserName, "@") {
			// If the username doesn't contain an "@" the user is local
			// to the controller and we don't want to propagate it.
			continue
		}
		users = append(users, dbmodel.UserCloudAccess{
			Username: u.UserName,
			User: dbmodel.User{
				Username:    u.UserName,
				DisplayName: u.DisplayName,
			},
			Access: u.Access,
		})
	}
	return users, nil
}

// EarliestControllerVersion returns the earliest agent version
// that any of the available public controllers is known to be running.
// If there are no available controllers or none of their versions are
// known, it returns the zero version.
func (j *JIMM) EarliestControllerVersion(ctx context.Context) (version.Number, error) {
	const op = errors.Op("jimm.EarliestControllerVersion")
	var v *version.Number

	err := j.Database.ForEachController(ctx, func(controller *dbmodel.Controller) error {
		if controller.AgentVersion == "" {
			return nil
		}
		versionNumber, err := version.Parse(controller.AgentVersion)
		if err != nil {
			zapctx.Error(
				ctx,
				"failed to parse agent version",
				zap.String("version", controller.AgentVersion),
				zap.String("controller", controller.Name),
			)
			return nil
		}
		if v == nil || versionNumber.Compare(*v) < 0 {
			v = &versionNumber
		}
		return nil
	})
	if err != nil {
		return version.Number{}, errors.E(op, err)
	}
	if v == nil {
		return version.Number{}, nil
	}
	return *v, nil
}

// controllerAccessLevel holds the controller access level for a user.
type controllerAccessLevel string

const (
	// noAccess allows a user no permissions at all.
	noAccess controllerAccessLevel = ""

	// loginAccess allows a user to log-ing into the subject.
	loginAccess controllerAccessLevel = "login"

	// superuserAccess allows user unrestricted permissions in the subject.
	superuserAccess controllerAccessLevel = "superuser"
)

// validate returns error if the current is not a valid access level.
func (a controllerAccessLevel) validate() error {
	switch a {
	case noAccess, loginAccess, superuserAccess:
		return nil
	}
	return errors.E(fmt.Sprintf("invalid access level %q", a))
}

func (a controllerAccessLevel) value() int {
	switch a {
	case noAccess:
		return 0
	case loginAccess:
		return 1
	case superuserAccess:
		return 2
	default:
		return -1
	}
}

// GrantControllerAccess changes the controller access granted to users.
func (j *JIMM) GrantControllerAccess(ctx context.Context, u *dbmodel.User, accessUserTag names.UserTag, accessLevel string) error {
	const op = errors.Op("jimm.GrantControllerAccess")

	if u.ControllerAccess != "superuser" {
		return errors.E(op, errors.CodeUnauthorized, "cannot grant controller access")
	}

	if err := controllerAccessLevel(accessLevel).validate(); err != nil {
		return errors.E(op, err, errors.CodeBadRequest)
	}

	user := dbmodel.User{
		Username: accessUserTag.Id(),
	}
	err := j.Database.GetUser(ctx, &user)
	if err != nil {
		return errors.E(op, err)
	}

	// if user's access level is already greater than what we are trying to set
	// there is nothing to do
	if controllerAccessLevel(user.ControllerAccess).value() >= controllerAccessLevel(accessLevel).value() {
		return nil
	}
	user.ControllerAccess = string(accessLevel)
	err = j.Database.UpdateUser(ctx, &user)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// RevokeControllerAccess revokes the controller access for a users.
func (j *JIMM) RevokeControllerAccess(ctx context.Context, u *dbmodel.User, accessUserTag names.UserTag, accessLevel string) error {
	const op = errors.Op("jimm.RevokeControllerAccess")

	if u.ControllerAccess != "superuser" {
		return errors.E(op, errors.CodeUnauthorized, "cannot revoke controller access")
	}

	if err := controllerAccessLevel(accessLevel).validate(); err != nil {
		return errors.E(op, err, errors.CodeBadRequest)
	}
	userAccess := noAccess
	switch controllerAccessLevel(accessLevel) {
	case loginAccess:
	case superuserAccess:
		userAccess = loginAccess
	}

	user := dbmodel.User{
		Username: accessUserTag.Id(),
	}
	err := j.Database.GetUser(ctx, &user)
	if err != nil {
		return errors.E(op, err)
	}
	user.ControllerAccess = string(userAccess)
	err = j.Database.UpdateUser(ctx, &user)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// GetJimmControllerAccess returns the JIMM controller access level for the
// requested user.
func (j *JIMM) GetJimmControllerAccess(ctx context.Context, user *openfga.User, tag names.UserTag) (string, error) {
	const op = errors.Op("jimm.GetControllerAccess")

	// First we check if the authenticated user is a JIMM administrator.
	isControllerAdmin, err := openfga.IsAdministrator(ctx, user, j.ResourceTag())
	if err != nil {
		zapctx.Error(ctx, "failed to check access rights", zap.Error(err))
		return "", errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	// If the authenticated user is requesting the access level
	// for him/her-self then we return that - either the user
	// is a JIMM admin (aka "superuser"), or they have a "login"
	// access level.
	if user.Username == tag.Id() {
		if isControllerAdmin {
			return "superuser", nil
		}
		return "login", nil
	}

	// Only JIMM administrators are allowed to see the access
	// level of somebody else.
	if !isControllerAdmin {
		return "", errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	var u dbmodel.User
	u.SetTag(tag)
	tagUser := openfga.NewUser(&u, j.OpenFGAClient)

	// Check if the user is jimm administrator.
	isAdmin, err := openfga.IsAdministrator(ctx, tagUser, j.ResourceTag())
	if err != nil {
		zapctx.Error(ctx, "failed to check access rights", zap.Error(err))
		return "", errors.E(op, err)
	}
	if isAdmin {
		return "superuser", nil
	}

	return "login", nil
}

// GetControllerAccess returns the user's level of access to the desired controller.
func (j *JIMM) GetControllerAccess(ctx context.Context, user *openfga.User, controller names.ControllerTag) (string, error) {
	accessLevel := user.GetControllerAccess(ctx, controller)
	return ToControllerAccessString(accessLevel), nil
}

// ImportModel imports model with the specified uuid from the controller.
func (j *JIMM) ImportModel(ctx context.Context, user *openfga.User, controllerName string, modelTag names.ModelTag) error {
	const op = errors.Op("jimm.ImportModel")

	isJIMMAdmin, err := openfga.IsAdministrator(ctx, user, j.ResourceTag())
	if err != nil {
		return errors.E(op, err)
	}
	if !isJIMMAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	controller := dbmodel.Controller{
		Name: controllerName,
	}
	err = j.Database.GetController(ctx, &controller)
	if err != nil {
		return errors.E(op, err)
	}

	api, err := j.dial(ctx, &controller, names.ModelTag{})
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()

	modelInfo := jujuparams.ModelInfo{
		UUID: modelTag.Id(),
	}
	err = api.ModelInfo(ctx, &modelInfo)
	if err != nil {
		return errors.E(op, err)
	}

	model := dbmodel.Model{}
	// fill in data from model info
	err = model.FromJujuModelInfo(modelInfo)
	if err != nil {
		return errors.E(op, err)
	}
	model.ControllerID = controller.ID
	model.Controller = controller

	// fetch the model owner user
	ownerTag, err := names.ParseUserTag(modelInfo.OwnerTag)
	if err != nil {
		return errors.E(op, err)
	}
	owner := dbmodel.User{}
	owner.SetTag(ownerTag)
	err = j.Database.GetUser(ctx, &owner)
	if err != nil {
		return errors.E(op, err)
	}
	model.OwnerUsername = owner.Username
	model.Owner = owner

	ownerUser := openfga.NewUser(&owner, j.OpenFGAClient)
	if err := ownerUser.SetModelAccess(ctx, modelTag, ofganames.AdministratorRelation); err != nil {
		zapctx.Error(
			ctx,
			"failed to set model admin",
			zap.String("owner", owner.Username),
			zap.String("model", modelTag.String()),
			zap.Error(err),
		)
	}

	// fetch cloud credential used by the model
	credentialTag, err := names.ParseCloudCredentialTag(modelInfo.CloudCredentialTag)
	if err != nil {
		return errors.E(op, err)
	}
	cloudCredential := dbmodel.CloudCredential{}
	cloudCredential.SetTag(credentialTag)
	err = j.Database.GetCloudCredential(ctx, &cloudCredential)
	if err != nil {
		return errors.E(op, err)
	}
	model.CloudCredentialID = cloudCredential.ID
	model.CloudCredential = cloudCredential

	// fetch the cloud used by the model
	cloud := dbmodel.Cloud{
		Name: cloudCredential.CloudName,
	}
	err = j.Database.GetCloud(ctx, &cloud)
	if err != nil {
		return errors.E(op, err)
	}

	regionFound := false
	for _, cr := range cloud.Regions {
		if cr.Name == modelInfo.CloudRegion {
			regionFound = true
			model.CloudRegion = cr
			model.CloudRegionID = cr.ID
			break
		}
	}
	if !regionFound {
		return errors.E(op, "cloud region not found")
	}

	for i, userAccess := range model.Users {
		u := userAccess.User
		err = j.Database.GetUser(ctx, &u)
		if err != nil {
			return errors.E(op, err)
		}
		model.Users[i].Username = u.Username
		model.Users[i].User = u

		relation, err := ToModelRelation(userAccess.Access)
		if err != nil {
			return errors.E(op, err)
		}

		modelUser := openfga.NewUser(&u, j.OpenFGAClient)
		if err := modelUser.SetModelAccess(ctx, modelTag, relation); err != nil {
			zapctx.Error(
				ctx,
				"failed to set model access",
				zap.String("user", u.Username),
				zap.String("access", userAccess.Access),
				zap.String("model", modelTag.String()),
				zap.Error(err),
			)
		}
	}

	err = j.Database.AddModel(ctx, &model)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeAlreadyExists {
			return errors.E(op, err, "model already exists")
		}
		return errors.E(op, err)
	}

	modelAPI, err := j.dial(ctx, &controller, modelTag)
	if err != nil {
		return errors.E(op, err)
	}
	defer modelAPI.Close()

	watcherID, err := modelAPI.WatchAll(ctx)
	if err != nil {
		return errors.E(op, err)
	}
	defer modelAPI.ModelWatcherStop(ctx, watcherID)

	deltas, err := modelAPI.ModelWatcherNext(ctx, watcherID)
	if err != nil {
		return errors.E(op, err)
	}

	modelIDf := func(uuid string) *modelState {
		if uuid == model.UUID.String {
			return &modelState{
				id:       model.ID,
				machines: make(map[string]int64),
				units:    make(map[string]bool),
			}
		}
		return nil
	}

	w := &Watcher{
		Database: j.Database,
	}
	for _, d := range deltas {
		if err := w.handleDelta(ctx, modelIDf, d); err != nil {
			return errors.E(op, err)
		}
	}

	return nil
}

// SetControllerConfig changes the value of specified controller configuration
// settings.
func (j *JIMM) SetControllerConfig(ctx context.Context, u *openfga.User, args jujuparams.ControllerConfigSet) error {
	const op = errors.Op("jimm.SetControllerConfig")

	isControllerAdmin, err := openfga.IsAdministrator(ctx, u, j.ResourceTag())
	if err != nil {
		return errors.E(op, err)
	}
	if !isControllerAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	err = j.Database.Transaction(func(tx *db.Database) error {
		config := dbmodel.ControllerConfig{
			Name: "jimm",
		}
		err := tx.GetControllerConfig(ctx, &config)
		if err != nil && errors.ErrorCode(err) != errors.CodeNotFound {
			return err
		}
		if config.Config == nil {
			config.Config = make(map[string]interface{})
		}
		for key, value := range args.Config {
			config.Config[key] = value
		}
		return tx.UpsertControllerConfig(ctx, &config)
	})
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// GetControllerConfig returns jimm's controller config.
func (j *JIMM) GetControllerConfig(ctx context.Context, u *dbmodel.User) (*dbmodel.ControllerConfig, error) {
	const op = errors.Op("jimm.GetControllerConfig")
	config := dbmodel.ControllerConfig{
		Name:   "jimm",
		Config: make(map[string]interface{}),
	}
	err := j.Database.GetControllerConfig(ctx, &config)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return &config, nil
		}
		return nil, errors.E(op, err)
	}
	return &config, nil
}

// UpdateMigratedModel asserts that the model has been migrated to the
// specified controller and updates the internal model representation.
func (j *JIMM) UpdateMigratedModel(ctx context.Context, user *openfga.User, modelTag names.ModelTag, targetControllerName string) error {
	const op = errors.Op("jimm.UpdateMigratedModel")

	isControllerAdmin, err := openfga.IsAdministrator(ctx, user, j.ResourceTag())
	if err != nil {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	if !isControllerAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	model := dbmodel.Model{
		UUID: sql.NullString{
			String: modelTag.Id(),
			Valid:  true,
		},
	}
	err = j.Database.GetModel(ctx, &model)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, "model not found", errors.CodeModelNotFound)
		}
		return errors.E(op, err)
	}

	targetController := dbmodel.Controller{
		Name: targetControllerName,
	}
	err = j.Database.GetController(ctx, &targetController)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, "controller not found", errors.CodeNotFound)
		}
		return errors.E(op, err)
	}

	// check the model is known to the controller
	api, err := j.dial(ctx, &targetController, names.ModelTag{})
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()

	err = api.ModelInfo(ctx, &jujuparams.ModelInfo{
		UUID: modelTag.Id(),
	})
	if err != nil {
		return errors.E(op, err)
	}

	model.Controller = targetController
	model.ControllerID = targetController.ID
	err = j.Database.UpdateModel(ctx, &model)
	if err != nil {
		zapctx.Error(ctx, "failed to update model", zap.String("model", model.UUID.String), zaputil.Error(err))
		return errors.E(op, err)
	}

	return nil
}
