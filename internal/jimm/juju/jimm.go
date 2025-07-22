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
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/credentials"
	"github.com/canonical/jimm/v3/internal/openfga"
)

var (
	initiateMigration = func(ctx context.Context, j *JujuManager, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error) {
		return j.InitiateMigration(ctx, user, spec)
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
		i := i
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
	const op = errors.Op("jimm.ListControllers")
	ctl := dbmodel.Controller{
		Name: name,
	}
	if err := j.Database.GetController(ctx, &ctl); err != nil {
		return nil, errors.E(op, err)
	}
	return &ctl, nil
}

// ListControllers returns a list of controllers the user has access to.
func (j *JujuManager) ListControllers(ctx context.Context, user *openfga.User) ([]dbmodel.Controller, error) {
	const op = errors.Op("jimm.ListControllers")

	if !user.JimmAdmin {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	var controllers []dbmodel.Controller
	err := j.Database.ForEachController(ctx, func(c *dbmodel.Controller) error {
		controllers = append(controllers, *c)
		return nil
	})
	if err != nil {
		return nil, errors.E(op, err)
	}

	return controllers, nil
}

// SetControllerDeprecated records if the controller is to be deprecated.
// No new models or clouds can be added to a deprecated controller.
func (j *JujuManager) SetControllerDeprecated(ctx context.Context, user *openfga.User, controllerName string, deprecated bool) error {
	const op = errors.Op("jimm.SetControllerDeprecated")

	if !user.JimmAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
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
		return errors.E(op, err)
	}

	return nil
}

// RemoveController removes a controller.
func (j *JujuManager) RemoveController(ctx context.Context, user *openfga.User, controllerName string, force bool) error {
	const op = errors.Op("jimm.RemoveController")

	if !user.JimmAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
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

		// if c.UnavailableSince is valid, then we can delete is
		// if c.UnavailableSince is no valid, then we can't delete is
		// if force is true, we can always delete is
		if !(force || c.UnavailableSince.Valid) {
			return errors.E(errors.CodeStillAlive, "controller is still alive")
		}

		models, err := db.GetModelsByController(ctx, c)
		if err != nil {
			return err
		}
		// Delete its models first.
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
		return errors.E(op, err)
	}

	return nil
}

// FullModelStatus returns the full status of the juju model.
func (j *JujuManager) FullModelStatus(ctx context.Context, user *openfga.User, modelTag names.ModelTag, patterns []string) (*jujuparams.FullStatus, error) {
	const op = errors.Op("jimm.RemoveController")

	if !user.JimmAdmin {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	model := dbmodel.Model{
		UUID: sql.NullString{
			String: modelTag.Id(),
			Valid:  true,
		},
	}
	err := j.Database.GetModel(ctx, &model)
	if err != nil {
		return nil, errors.E(op, err)
	}

	api, err := j.dial(ctx, &model.Controller, modelTag, nil)
	if err != nil {
		return nil, errors.E(op, err)
	}

	status, err := api.Status(ctx, patterns)
	if err != nil {
		return nil, errors.E(op, err)
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
			return jujuparams.MigrationTargetInfo{}, 0, err
		}
		return jujuparams.MigrationTargetInfo{}, 0, errors.E(err, fmt.Errorf("failed to get controller with name %q", controllerName))
	}
	adminUser, adminPass, err := credStore.GetControllerCredentials(ctx, controllerName)
	if err != nil {
		return jujuparams.MigrationTargetInfo{}, 0, err
	}
	if adminUser == "" || adminPass == "" {
		return jujuparams.MigrationTargetInfo{}, 0, errors.E("missing target controller credentials")
	}
	// Should we verify controller can access the cloud where the model is currently hosted?
	apiControllerInfo := dbController.ToAPIControllerInfo()
	targetInfo := jujuparams.MigrationTargetInfo{
		ControllerTag: dbController.ResourceTag().String(),
		Addrs:         apiControllerInfo.APIAddresses,
		CACert:        dbController.CACertificate,
		// The target user must be the admin user as external users don't have username/password credentials.
		AuthTag:  names.NewUserTag(adminUser).String(),
		Password: adminPass,
	}
	return targetInfo, dbController.ID, nil
}

// InitiateInternalMigration initiates a model migration between two controllers within JIMM.
func (j *JujuManager) InitiateInternalMigration(ctx context.Context, user *openfga.User, modelNameOrUUID string, targetController string) (jujuparams.InitiateMigrationResult, error) {
	const op = errors.Op("jimm.InitiateInternalMigration")

	migrationTarget, _, err := fillMigrationTarget(j.Database, j.CredentialStore, targetController)
	if err != nil {
		return jujuparams.InitiateMigrationResult{}, errors.E(op, err)
	}

	model := dbmodel.Model{}
	// Check if the user is providing a model UUID or name
	_, err = uuid.Parse(modelNameOrUUID)
	if err != nil {
		s := strings.Split(modelNameOrUUID, "/")
		if len(s) != 2 {
			return jujuparams.InitiateMigrationResult{}, errors.E(op, "invalid model target")
		}

		owner, name := s[0], s[1]
		if !names.IsValidUser(owner) {
			return jujuparams.InitiateMigrationResult{}, errors.E(op, "invalid user name")
		}
		if !names.IsValidModelName(name) {
			return jujuparams.InitiateMigrationResult{}, errors.E(op, "invalid model name")
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
		return jujuparams.InitiateMigrationResult{}, errors.E(op, err)
	}
	spec := jujuparams.MigrationSpec{ModelTag: model.ResourceTag().String(), TargetInfo: migrationTarget}
	result, err := initiateMigration(ctx, j, user, spec)
	if err != nil {
		return result, errors.E(op, err)
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
	const op = errors.Op("jujumanager.PrepareModelMigration")

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
			return errors.E(op, "model migration for the specified model is already in progress/completed")
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
		zapctx.Error(ctx, "failed to add incoming model migration details", zap.Error(err))
		return "", errors.E(op, err)
	}

	migrationToken, err := j.migrationTokenGenerator.NewMigrationToken(ctx, user.Name)
	if err != nil {
		zapctx.Error(ctx, "failed to generate migration token", zap.Error(err))
		return "", errors.E(op, err)
	}

	return migrationToken, nil
}
