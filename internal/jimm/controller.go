// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/controller"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/version"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"gopkg.in/macaroon.v2"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
)

// ControllerClient defines an interface of the juju controller api client
// used by JIMM to interact with the Controller facade of Juju controllers.
type ControllerClient interface {
	// InitiateMigration attempts to begin the migration of one or
	// more models to other controllers.
	InitiateMigration(controller.MigrationSpec) (string, error)
	// Close closes the connection to the API server.
	Close() error
}

var (
	newControllerClient = func(api base.APICallCloser) ControllerClient {
		return controller.NewClient(api)
	}
)

// AddController adds the specified controller to JIMM. Only
// controller-admin level users may add new controllers. If the user adding
// the controller is not authorized then an error with a code of
// CodeUnauthorized will be returned. If there already exists a controller
// with the same name as the controller being added then an error with a
// code of CodeAlreadyExists will be returned. If the controller cannot be
// contacted then an error with a code of CodeConnectionFailed will be
// returned.
func (j *JIMM) AddController(ctx context.Context, user *openfga.User, ctl *dbmodel.Controller) error {
	const op = errors.Op("jimm.AddController")

	if !user.JimmAdmin {
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
		var cloud dbmodel.Cloud
		cloud.FromJujuCloud(cld)
		cloud.Name = tag.Id()
		dbClouds = append(dbClouds, cloud)
	}

	credentialsStored := false
	if j.CredentialStore != nil {
		err := j.CredentialStore.PutControllerCredentials(ctx, ctl.Name, ctl.AdminIdentityName, ctl.AdminPassword)
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
			ctl.AdminIdentityName = ""
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
		// If this cloud is the one used by the controller model then
		// it is available to all users. Other clouds require `juju grant-cloud` to add permissions.
		if cloud.ResourceTag().String() == ms.CloudTag {
			everyoneTag := names.NewUserTag(ofganames.EveryoneUser)
			everyoneIdentity, err := dbmodel.NewIdentity(everyoneTag.Id())
			if err != nil {
				zapctx.Error(ctx, "failed to create identity model", zap.Error(err))
				return errors.E(op, err)
			}
			everyone := openfga.NewUser(
				everyoneIdentity,
				j.OpenFGAClient,
			)
			if err := everyone.SetCloudAccess(ctx, cloud.ResourceTag(), ofganames.CanAddModelRelation); err != nil {
				zapctx.Error(ctx, "failed to grant everyone add-model access", zap.Error(err))
			}
		}

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

// GetJimmControllerAccess returns the JIMM controller access level for the
// requested user.
func (j *JIMM) GetJimmControllerAccess(ctx context.Context, user *openfga.User, tag names.UserTag) (string, error) {
	const op = errors.Op("jimm.GetJIMMControllerAccess")

	// If the authenticated user is requesting the access level
	// for him/her-self then we return that - either the user
	// is a JIMM admin (aka "superuser"), or they have a "login"
	// access level.
	if user.Name == tag.Id() {
		if user.JimmAdmin {
			return "superuser", nil
		}
		return "login", nil
	}

	// Only JIMM administrators are allowed to see the access
	// level of somebody else.
	if !user.JimmAdmin {
		return "", errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	var targetUser dbmodel.Identity
	targetUser.SetTag(tag)
	targetUserTag := openfga.NewUser(&targetUser, j.OpenFGAClient)

	// Check if the user is jimm administrator.
	isAdmin, err := openfga.IsAdministrator(ctx, targetUserTag, j.ResourceTag())
	if err != nil {
		zapctx.Error(ctx, "failed to check access rights", zap.Error(err))
		return "", errors.E(op, err)
	}
	if isAdmin {
		return "superuser", nil
	}

	return "login", nil
}

// GetUserControllerAccess returns the user's level of access to the desired controller.
func (j *JIMM) GetUserControllerAccess(ctx context.Context, user *openfga.User, controller names.ControllerTag) (string, error) {
	accessLevel := user.GetControllerAccess(ctx, controller)
	return ToControllerAccessString(accessLevel), nil
}

// ImportModel imports model with the specified uuid from the controller.
func (j *JIMM) ImportModel(ctx context.Context, user *openfga.User, controllerName string, modelTag names.ModelTag, newOwner string) error {
	const op = errors.Op("jimm.ImportModel")

	if !user.JimmAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	controller := dbmodel.Controller{
		Name: controllerName,
	}
	err := j.Database.GetController(ctx, &controller)
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

	var ownerTag names.UserTag
	if newOwner != "" {
		// Switch the model to be owned by the specified user.
		if !names.IsValidUser(newOwner) {
			return errors.E(op, errors.CodeBadRequest, "invalid new username for new model owner")
		}
		ownerTag = names.NewUserTag(newOwner)
	} else {
		// Use the model owner user
		ownerTag, err = names.ParseUserTag(modelInfo.OwnerTag)
		if err != nil {
			return errors.E(op, fmt.Sprintf("invalid username %s from original model owner", modelInfo.OwnerTag))
		}
	}
	if ownerTag.IsLocal() {
		return errors.E(op, "cannot import model from local user, try --owner to switch the model owner")
	}
	ownerUser := dbmodel.Identity{}
	ownerUser.SetTag(ownerTag)
	err = j.Database.GetIdentity(ctx, &ownerUser)
	if err != nil {
		return errors.E(op, err)
	}
	model.SwitchOwner(&ownerUser)

	// Note that only the new owner is given access. All previous users that had access according to Juju
	// are discarded as access must now be governed by JIMM and OpenFGA.
	ofgaUser := openfga.NewUser(&ownerUser, j.OpenFGAClient)
	if err := ofgaUser.SetModelAccess(ctx, modelTag, ofganames.AdministratorRelation); err != nil {
		zapctx.Error(
			ctx,
			"failed to set model admin",
			zap.String("owner", ownerUser.Name),
			zap.String("model", modelTag.String()),
			zap.Error(err),
		)
	}

	// TODO(CSS-5458): Remove the below section on cloud credentials once we no longer persist the relation between
	// cloud credentials and models

	// fetch cloud credential used by the model
	cloudTag, err := names.ParseCloudTag(modelInfo.CloudTag)
	if err != nil {
		errors.E(op, err)
	}
	// Note that the model already has a cloud credential configured which it will use when deploying new
	// applications. JIMM needs some cloud credential reference to be able to import the model so use any
	// credential against the cloud the model is deployed against. Even using the correct cloud for the
	// credential is not strictly necessary, but will help prevent the user thinking they can create new
	// models on the incoming cloud.
	allCredentials, err := j.Database.GetIdentityCloudCredentials(ctx, &ownerUser, cloudTag.Id())
	if err != nil {
		return errors.E(op, err)
	}
	if len(allCredentials) == 0 {
		return errors.E(op, errors.CodeNotFound, fmt.Sprintf("Failed to find cloud credential for user %s on cloud %s", ownerUser.Name, cloudTag.Id()))
	}
	cloudCredential := allCredentials[0]

	model.CloudCredentialID = cloudCredential.ID
	model.CloudCredential = cloudCredential

	// fetch the cloud used by the model
	cloud := dbmodel.Cloud{
		Name: cloudCredential.CloudName,
	}
	err = j.Database.GetCloud(ctx, &cloud)
	if err != nil {
		zapctx.Error(ctx, "failed to get cloud", zap.String("cloud", cloud.Name))
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
func (j *JIMM) SetControllerConfig(ctx context.Context, user *openfga.User, args jujuparams.ControllerConfigSet) error {
	const op = errors.Op("jimm.SetControllerConfig")

	if !user.JimmAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	err := j.Database.Transaction(func(tx *db.Database) error {
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
func (j *JIMM) GetControllerConfig(ctx context.Context, u *dbmodel.Identity) (*dbmodel.ControllerConfig, error) {
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

	if !user.JimmAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	model := dbmodel.Model{
		UUID: sql.NullString{
			String: modelTag.Id(),
			Valid:  true,
		},
	}
	err := j.Database.GetModel(ctx, &model)
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

// InitiateMigration triggers the migration of the specified model to a target controller.
// externalMigration indicates whether this model is moving to a controller managed by
// JIMM or not.
func (j *JIMM) InitiateMigration(ctx context.Context, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error) {
	const op = errors.Op("jimm.InitiateMigration")

	result := jujuparams.InitiateMigrationResult{
		ModelTag: spec.ModelTag,
	}
	mt, err := names.ParseModelTag(spec.ModelTag)
	if err != nil {
		return result, errors.E(op, err, errors.CodeBadRequest)
	}
	isAdministrator, err := openfga.IsAdministrator(ctx, user, mt)
	if err != nil {
		return result, errors.E(op, err, errors.CodeOpenFGARequestFailed)
	}
	if !isAdministrator {
		return result, errors.E(op, errors.CodeUnauthorized)
	}

	targetControllerTag, err := names.ParseControllerTag(spec.TargetInfo.ControllerTag)
	if err != nil {
		return result, errors.E(op, err, errors.CodeBadRequest)
	}

	targetUserTag, err := names.ParseUserTag(spec.TargetInfo.AuthTag)
	if err != nil {
		return result, errors.E(op, err, errors.CodeBadRequest)
	}

	var targetMacaroons []macaroon.Slice
	if spec.TargetInfo.Macaroons != "" {
		err = json.Unmarshal([]byte(spec.TargetInfo.Macaroons), &targetMacaroons)
		if err != nil {
			return result, errors.E(op, err, "failed to unmarshal macaroons", errors.CodeBadRequest)
		}
	}

	model := dbmodel.Model{}
	model.SetTag(mt)
	err = j.Database.GetModel(ctx, &model)
	if err != nil {
		return result, errors.E(op, "failed to retrieve the model from the database", err)
	}

	api, err := j.dial(ctx, &model.Controller, names.ModelTag{})
	if err != nil {
		return result, errors.E(op, "failed to dial the controller", err)
	}

	client := newControllerClient(api)
	defer client.Close()

	result.MigrationId, err = client.InitiateMigration(controller.MigrationSpec{
		ModelUUID:             mt.Id(),
		TargetControllerUUID:  targetControllerTag.Id(),
		TargetControllerAlias: spec.TargetInfo.ControllerAlias,
		TargetAddrs:           spec.TargetInfo.Addrs,
		TargetCACert:          spec.TargetInfo.CACert,
		TargetUser:            targetUserTag.Id(),
		TargetPassword:        spec.TargetInfo.Password,
		TargetMacaroons:       targetMacaroons,
	})
	if err != nil {
		return result, errors.E(op, err)
	}
	return result, nil
}
