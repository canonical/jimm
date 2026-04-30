// Copyright 2025 Canonical.

package juju

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/credentials"
	"github.com/canonical/jimm/v3/internal/openfga"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

var (
	initiateInternalMigration = func(ctx context.Context, j *JujuManager, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error) {
		internalMigration := true
		return j.initiateMigration(ctx, user, spec, internalMigration)
	}
)

// forEachController runs a given function on multiple controllers
// simultaneously. A connection is established to every controller in the
// given list concurrently and then the given function is called with the
// controller and API connection to use to perform the controller
// operation. ForEachConnection waits until all operations have finished
// before returning, any error returned will be the first error
// encountered when connecting to the controller or returned from the given
// function.
func (j *JujuManager) forEachController(ctx context.Context, controllers []dbmodel.Controller, f func(*dbmodel.Controller, API) error) error {
	eg := new(errgroup.Group)
	for i := range controllers {
		eg.Go(func() error {
			api, err := j.dial(ctx, &controllers[i], names.ModelTag{}, nil)
			if err != nil {
				return err
			}
			defer api.Close()
			return f(&controllers[i], api)
		})
	}
	return eg.Wait()
}

// ControllerInfo returns info about a controller connected to JIMM.
func (j *JujuManager) ControllerInfo(ctx context.Context, name string) (*dbmodel.Controller, error) {

	ctl := dbmodel.Controller{
		Name: name,
	}
	if err := j.Database.GetController(ctx, &ctl); err != nil {
		return nil, err
	}
	return &ctl, nil
}

// ListControllers returns a list of controllers the user has can_addmodel to.
// JIMM admins get all controllers.
func (j *JujuManager) ListControllers(ctx context.Context, user *openfga.User) ([]dbmodel.Controller, error) {
	var controllers []dbmodel.Controller
	err := j.Database.ForEachController(ctx, func(c *dbmodel.Controller) error {
		canAddModel, err := user.IsAllowedAddModelToController(ctx, c.ResourceTag())
		if err != nil {
			zapctx.Error(ctx, "error checking user permissions for controller", zap.String("controller", c.Name), zap.Error(err))
			return nil
		}
		if canAddModel {
			controllers = append(controllers, *c)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return controllers, nil
}

// SetControllerDeprecated records if the controller is to be deprecated.
// No new models or clouds can be added to a deprecated controller.
func (j *JujuManager) SetControllerDeprecated(ctx context.Context, user *openfga.User, controllerName string, deprecated bool) error {

	if !user.JimmAdmin {
		return errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}

	// Update the local database with the updated cloud definition. We
	// do this in a transaction so that the local view cannot finish in
	// an inconsistent state.
	err := j.Database.Transaction(func(db *db.Database) error {
		c := dbmodel.Controller{
			Name: controllerName,
		}
		if err := db.GetController(ctx, &c); err != nil {
			return err
		}
		c.Deprecated = deprecated
		return db.UpdateController(ctx, &c)
	})
	if err != nil {
		return err
	}

	return nil
}

// RemoveController removes a controller.
func (j *JujuManager) RemoveController(ctx context.Context, user *openfga.User, controllerName string, force bool) error {
	// Update the local database with the updated cloud definition. We
	// do this in a transaction so that the local view cannot finish in
	// an inconsistent state.
	err := j.Database.Transaction(func(db *db.Database) error {
		c := dbmodel.Controller{
			Name: controllerName,
		}
		if err := db.GetController(ctx, &c); err != nil {
			return err
		}

		models, err := db.GetModelsByController(ctx, c)
		if err != nil {
			return err
		}
		if len(models) > 0 && !force {
			return errors.Codef(errors.CodeStillAlive, "controller still has models")
		}

		// Remove all models associated with the controller. If force is false,
		// we can only reach here with an empty list of models.
		for _, model := range models {
			err := db.DeleteModel(ctx, &model)
			if err != nil {
				return err
			}
		}

		// Then delete the controller
		return db.DeleteController(ctx, &c)
	})
	if err != nil {
		return err
	}

	return nil
}

// FullModelStatus returns the full status of the juju model.
func (j *JujuManager) FullModelStatus(ctx context.Context, user *openfga.User, modelTag names.ModelTag, patterns []string) (*jujuparams.FullStatus, error) {
	if !user.JimmAdmin {
		return nil, errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}

	model := dbmodel.Model{
		UUID: sql.NullString{
			String: modelTag.Id(),
			Valid:  true,
		},
	}
	err := j.Database.GetModel(ctx, &model)
	if err != nil {
		return nil, err
	}

	api, err := j.dial(ctx, &model.Controller, modelTag, nil)
	if err != nil {
		return nil, err
	}

	status, err := api.Status(ctx, patterns)
	if err != nil {
		return nil, err
	}

	return status, nil
}

type migrationControllerID = uint

func fillMigrationTarget(db *db.Database, credStore credentials.CredentialStore, controllerName string) (jujuparams.MigrationTargetInfo, migrationControllerID, error) {
	dbController := dbmodel.Controller{
		Name: controllerName,
	}
	ctx := context.Background()
	err := db.GetController(ctx, &dbController)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return jujuparams.MigrationTargetInfo{}, 0, errors.Codef(errors.CodeNotFound, "controller not found")
		}
		return jujuparams.MigrationTargetInfo{}, 0, fmt.Errorf("failed to get controller with name %q: %w", controllerName, err)
	}
	adminUser, adminPass, err := credStore.GetControllerCredentials(ctx, controllerName)
	if err != nil {
		return jujuparams.MigrationTargetInfo{}, 0, err
	}
	if adminUser == "" || adminPass == "" {
		return jujuparams.MigrationTargetInfo{}, 0, errors.New("missing target controller credentials")
	}
	// Should we verify controller can access the cloud where the model is currently hosted?
	apiControllerInfo := dbController.ToAPIControllerInfo()
	targetInfo := jujuparams.MigrationTargetInfo{
		ControllerAlias: dbController.Name, // This value will be returned to us on successful migration.
		ControllerTag:   dbController.ResourceTag().String(),
		Addrs:           apiControllerInfo.APIAddresses,
		CACert:          dbController.CACertificate,
		// The target user must be the admin user as external users don't have username/password credentials.
		AuthTag:  names.NewUserTag(adminUser).String(),
		Password: adminPass,
	}
	return targetInfo, dbController.ID, nil
}

// InitiateInternalMigration initiates a model migration between two controllers within JIMM.
func (j *JujuManager) InitiateInternalMigration(ctx context.Context, user *openfga.User, modelNameOrUUID string, targetController string) (jujuparams.InitiateMigrationResult, error) {

	migrationTarget, _, err := fillMigrationTarget(j.Database, j.CredentialStore, targetController)
	if err != nil {
		return jujuparams.InitiateMigrationResult{}, err
	}

	model := dbmodel.Model{}
	// Check if the user is providing a model UUID or name
	_, err = uuid.Parse(modelNameOrUUID)
	if err != nil {
		s := strings.Split(modelNameOrUUID, "/")
		if len(s) != 2 {
			return jujuparams.InitiateMigrationResult{}, errors.New("invalid model target")
		}

		owner, name := s[0], s[1]
		if !names.IsValidUser(owner) {
			return jujuparams.InitiateMigrationResult{}, errors.New("invalid user name")
		}
		if !names.IsValidModelName(name) {
			return jujuparams.InitiateMigrationResult{}, errors.New("invalid model name")
		}

		model.Name = name
		model.OwnerIdentityName = owner
	} else {
		model.UUID = sql.NullString{
			String: modelNameOrUUID,
			Valid:  true,
		}
	}

	err = j.Database.GetModel(ctx, &model)
	if err != nil {
		return jujuparams.InitiateMigrationResult{}, err
	}
	spec := jujuparams.MigrationSpec{ModelTag: model.ResourceTag().String(), TargetInfo: migrationTarget}
	result, err := initiateInternalMigration(ctx, j, user, spec)
	if err != nil {
		return result, err
	}
	return result, nil
}

// PrepareModelMigration takes the model ID from the migrating controller and stores a record
// in the IncomingModelMigation table to prepare it for migration against the target controller's name.
func (j *JujuManager) PrepareModelMigration(
	ctx context.Context,
	user *openfga.User,
	modelUUID string,
	targetControllerName string,
	userMapping map[string]string,
) (string, error) {

	err := j.Database.Transaction(func(d *db.Database) error {
		ctl := dbmodel.Controller{Name: targetControllerName}
		if err := d.GetController(ctx, &ctl); err != nil {
			return err
		}

		// Verify the model doesn't exist - if it does that means a migration is
		// in progress or completed or it could also mean the model failed to be removed
		// during migration ABORT but that problem should be dealt with separately.
		model := &dbmodel.Model{
			UUID: sql.NullString{String: modelUUID, Valid: true},
		}
		err := d.GetModel(ctx, model)
		if err == nil {
			return errors.New("model migration for the specified model is already in progress/completed")
		} else if errors.ErrorCode(err) != errors.CodeNotFound {
			return err
		}

		if err := d.AddOrUpdateIncomingModelMigration(ctx, &dbmodel.IncomingModelMigration{
			ModelUUID:          sql.NullString{String: modelUUID, Valid: true},
			TargetControllerID: ctl.ID,
			UserMapping:        dbmodel.StringMap(userMapping),
		}); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to add incoming model migration details: %w", err)
	}

	migrationToken, err := j.migrationTokenGenerator.NewMigrationToken(ctx, user.Name)
	if err != nil {
		return "", fmt.Errorf("failed to generate migration token: %w", err)
	}

	return migrationToken, nil
}

// ListMigrationTargets returns the list of juju controllers that the given internal
// model could be migrated to. This includes controllers that support the model's
// cloud region and version, but excludes the controller the model is already on.
func (j *JujuManager) ListMigrationTargets(ctx context.Context, user *openfga.User, modelTag names.ModelTag) ([]dbmodel.Controller, error) {
	var model dbmodel.Model
	model.SetTag(modelTag)
	if err := j.Database.GetModel(ctx, &model); err != nil {
		return nil, err
	}

	currentVersion, err := version.Parse(model.Controller.AgentVersion)
	if err != nil {
		return nil, err
	}

	cloudRegion, err := j.Database.FindRegionByCloudName(ctx, model.CloudRegion.CloudName, model.CloudRegion.Name)
	if err != nil {
		return nil, err
	}

	var controllers []dbmodel.Controller
	for _, ctl := range cloudRegion.Controllers {
		candidateVersion, err := version.Parse(ctl.Controller.AgentVersion)
		if err != nil {
			return nil, err
		}

		if model.Controller.ID != ctl.Controller.ID &&
			currentVersion.Compare(candidateVersion) <= 0 {
			controllers = append(controllers, ctl.Controller)
		}
	}

	return controllers, nil
}

// ModelControllerInfoQualifier specifies a qualifier for ModelControllerInfo.
type ModelControllerInfoQualifier func(*dbmodel.Model)

// WithModelUUID specifies the model by its UUID.
func WithModelUUID(uuid string) ModelControllerInfoQualifier {
	return func(o *dbmodel.Model) {
		o.UUID = sql.NullString{
			String: uuid,
			Valid:  true,
		}
	}
}

// WithOwnerAndModelName specifies the model by owner and model name.
func WithOwnerAndModelName(ownerName, modelName string) ModelControllerInfoQualifier {
	return func(o *dbmodel.Model) {
		o.OwnerIdentityName = ownerName
		o.Name = modelName
	}
}

// ModelControllerInfo returns information about a model.
// The model can be specified using functional options:
// - WithModelUUID(uuid) to specify by model UUID
// - WithOwnerAndModelName(owner, name) to specify by owner and model name
func (j *JujuManager) ModelControllerInfo(ctx context.Context, user *openfga.User, qualifier ModelControllerInfoQualifier) (*apiparams.ModelControllerInfo, error) {
	if !user.JimmAdmin {
		return nil, errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}

	var model dbmodel.Model
	qualifier(&model)

	if !model.UUID.Valid && (model.OwnerIdentityName == "" || model.Name == "") {
		return nil, errors.Codef(errors.CodeBadRequest, "either model uuid or both model name and owner must be provided")
	}

	err := j.Database.GetModel(ctx, &model)
	if err != nil {
		return nil, err
	}

	return &apiparams.ModelControllerInfo{
		ModelName:      model.Name,
		ModelUUID:      model.UUID.String,
		ControllerName: model.Controller.Name,
		ControllerUUID: model.Controller.UUID,
	}, nil
}
