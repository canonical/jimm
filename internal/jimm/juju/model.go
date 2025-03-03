// Copyright 2025 Canonical.

package juju

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/juju/juju/api/base"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/permissions"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
)

// shuffle is used to randomize the order in which possible controllers
// are tried. It is a variable so it can be replaced in tests.
var shuffle func(int, func(int, int)) = rand.Shuffle

func shuffleRegionControllers(controllers []dbmodel.CloudRegionControllerPriority) {
	shuffle(len(controllers), func(i, j int) {
		controllers[i], controllers[j] = controllers[j], controllers[i]
	})
	sort.SliceStable(controllers, func(i, j int) bool {
		return controllers[i].Priority > controllers[j].Priority
	})
}

// ModelCreateArgs contains parameters used to add a new model.
type ModelCreateArgs struct {
	Name            string
	Owner           names.UserTag
	Config          map[string]interface{}
	Cloud           names.CloudTag
	CloudRegion     string
	CloudCredential names.CloudCredentialTag
}

// FromJujuModelCreateArgs converts jujuparams.ModelCreateArgs into AddModelArgs.
func (a *ModelCreateArgs) FromJujuModelCreateArgs(args *jujuparams.ModelCreateArgs) error {
	if args.Name == "" {
		return errors.E("name not specified")
	}
	a.Name = args.Name
	a.Config = args.Config
	a.CloudRegion = args.CloudRegion
	if args.CloudTag != "" {
		ct, err := names.ParseCloudTag(args.CloudTag)
		if err != nil {
			return errors.E(err, errors.CodeBadRequest)
		}
		a.Cloud = ct
	}

	if args.OwnerTag == "" {
		return errors.E("owner tag not specified")
	}
	ot, err := names.ParseUserTag(args.OwnerTag)
	if err != nil {
		return errors.E(err, errors.CodeBadRequest)
	}
	a.Owner = ot

	if args.CloudCredentialTag != "" {
		ct, err := names.ParseCloudCredentialTag(args.CloudCredentialTag)
		if err != nil {
			return errors.E(err, "invalid cloud credential tag")
		}
		if a.Cloud.Id() != "" && ct.Cloud().Id() != a.Cloud.Id() {
			return errors.E("cloud credential cloud mismatch")
		}

		a.CloudCredential = ct
	}
	return nil
}

// AddModel adds the specified model to JIMM.
func (j *JujuManager) AddModel(ctx context.Context, user *openfga.User, args *ModelCreateArgs) (_ *jujuparams.ModelInfo, err error) {
	const op = errors.Op("jimm.AddModel")
	zapctx.Info(ctx, string(op))

	owner, err := dbmodel.NewIdentity(args.Owner.Id())
	if err != nil {
		return nil, errors.E(op, err)
	}

	err = j.Database.GetIdentity(ctx, owner)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// Only JIMM admins are able to add models on behalf of other users.
	if owner.Name != user.Name && !user.JimmAdmin {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	builder := newModelBuilder(ctx, j)
	builder = builder.WithOwner(owner)
	builder = builder.WithName(args.Name)
	if err := builder.Error(); err != nil {
		return nil, errors.E(op, err)
	}

	builder = builder.WithCloud(user, args.Cloud)
	if err := builder.Error(); err != nil {
		return nil, errors.E(op, err)
	}

	builder = builder.WithCloudRegion(args.CloudRegion)
	if err := builder.Error(); err != nil {
		return nil, errors.E(op, err)
	}
	// fetch cloud defaults
	cloudDefaults := dbmodel.CloudDefaults{
		IdentityName: user.Name,
		Cloud:        *builder.cloud,
	}
	err = j.Database.CloudDefaults(ctx, &cloudDefaults)
	if err != nil && errors.ErrorCode(err) != errors.CodeNotFound {
		return nil, errors.E(op, "failed to fetch cloud defaults")
	}
	builder = builder.WithConfig(cloudDefaults.Defaults)

	// fetch cloud region defaults
	cloudRegionDefaults := dbmodel.CloudDefaults{
		IdentityName: user.Name,
		Cloud:        *builder.cloud,
		Region:       builder.cloudRegion,
	}
	err = j.Database.CloudDefaults(ctx, &cloudRegionDefaults)
	if err != nil && errors.ErrorCode(err) != errors.CodeNotFound {
		return nil, errors.E(op, "failed to fetch cloud defaults")
	}
	builder = builder.WithConfig(cloudRegionDefaults.Defaults)

	// at this point we know which cloud will host the model and
	// we must check the user has add-model permission on the cloud
	canAddModel, err := openfga.NewUser(owner, j.OpenFGAClient).IsAllowedAddModel(ctx, builder.cloud.ResourceTag())
	if err != nil {
		return nil, errors.E(op, "permission check failed")
	}
	if !canAddModel {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	// last but not least, use the provided config values
	// overriding all defaults
	builder = builder.WithConfig(args.Config)

	if args.CloudCredential != (names.CloudCredentialTag{}) {
		builder = builder.WithCloudCredential(args.CloudCredential)
		if err := builder.Error(); err != nil {
			return nil, errors.E(op, err)
		}
	}
	builder = builder.CreateDatabaseModel()
	if err := builder.Error(); err != nil {
		return nil, errors.E(op, err)
	}
	defer builder.Cleanup()

	builder = builder.CreateControllerModel()
	if err := builder.Error(); err != nil {
		return nil, errors.E(op, err)
	}

	builder = builder.UpdateDatabaseModel()
	if err := builder.Error(); err != nil {
		return nil, errors.E(op, err)
	}

	mi := builder.JujuModelInfo()

	ownerUser := openfga.NewUser(owner, j.OpenFGAClient)
	modelTag := names.NewModelTag(mi.UUID)
	controllerTag := builder.controller.ResourceTag()

	if err := j.addModelPermissions(ctx, ownerUser, modelTag, controllerTag); err != nil {
		return nil, errors.E(op, err)
	}
	return mi, nil
}

// GetModel retrieves a model object by the model UUID.
func (j *JujuManager) GetModel(ctx context.Context, uuid string) (dbmodel.Model, error) {
	model := dbmodel.Model{
		UUID: sql.NullString{
			String: uuid,
			Valid:  uuid != "",
		},
	}
	if err := j.Database.GetModel(context.Background(), &model); err != nil {
		zapctx.Error(ctx, "failed to find model", zap.String("uuid", uuid), zap.Error(err))
		return dbmodel.Model{}, fmt.Errorf("failed to get model: %s", err.Error())
	}
	return model, nil
}

// addModelPermissions grants a user access to a model and sets the relation between the controller and model.
// Call this when adding/importing a model to set the necessary permissions.
func (j *JujuManager) addModelPermissions(ctx context.Context, owner *openfga.User, mt names.ModelTag, ct names.ControllerTag) error {
	if err := j.OpenFGAClient.AddControllerModel(ctx, ct, mt); err != nil {
		zapctx.Error(
			ctx,
			"failed to add controller->model relation",
			zap.String("controller", ct.Id()),
			zap.String("model", mt.Id()),
		)
		return err
	}
	if err := owner.SetModelAccess(ctx, mt, ofganames.AdministratorRelation); err != nil {
		zapctx.Error(
			ctx,
			"failed to add user->model administrator relation",
			zap.String("user", owner.Tag().Id()),
			zap.String("model", mt.Id()),
		)
		return err
	}
	return nil
}

// ModelInfo returns the model info for the model with the given ModelTag.
// The returned ModelInfo will be appropriate for the given user's
// access-level on the model. If the model does not exist then the returned
// error will have the code CodeNotFound. If the given user does not have
// access to the model then the returned error will have the code
// CodeUnauthorized.
func (j *JujuManager) ModelInfo(ctx context.Context, user *openfga.User, mt names.ModelTag) (*jujuparams.ModelInfo, error) {
	const op = errors.Op("jimm.ModelInfo")
	zapctx.Info(ctx, string(op))

	var m dbmodel.Model
	m.SetTag(mt)
	if err := j.Database.GetModel(ctx, &m); err != nil {
		return nil, errors.E(op, err)
	}

	if ok, err := user.IsModelReader(ctx, mt); !ok || err != nil {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	api, err := j.dial(ctx, &m.Controller, names.ModelTag{})
	if err != nil {
		return nil, errors.E(op, err)
	}
	defer api.Close()

	mi := &jujuparams.ModelInfo{
		UUID: mt.Id(),
	}
	if err := api.ModelInfo(ctx, mi); err != nil {
		return nil, errors.E(op, err)
	}

	return j.mergeModelInfo(ctx, user, mi, m)
}

// modelSummariesMap is a safe map to add records concurrently because the access is guarded by a Mutex.
// The read operations are not guarded because only inserts are done concurrently.
type modelSummariesMap struct {
	mu             sync.Mutex
	modelSummaries map[string]jujuparams.ModelSummaryResult
}

func (m *modelSummariesMap) addModelSummary(summary jujuparams.ModelSummaryResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.modelSummaries == nil {
		m.modelSummaries = make(map[string]jujuparams.ModelSummaryResult)
	}
	m.modelSummaries[summary.Result.UUID] = summary
}

// ListModelSummaries returns the list of modelsummary the user has access to.
// It queries the controllers and then merge the info from the JIMM db.
func (j *JujuManager) ListModelSummaries(ctx context.Context, user *openfga.User, maskingControllerUUID string) (jujuparams.ModelSummaryResults, error) {
	const op = errors.Op("jimm.ListModelSummaries")

	modelSummariesSafeMap := modelSummariesMap{}
	modelSummaryResults := []jujuparams.ModelSummaryResult{}

	var models []struct {
		model      *dbmodel.Model
		userAccess jujuparams.UserAccessPermission
	}
	// we collect models belonging to the user and we extract the unique controllers.
	var uniqueControllers []dbmodel.Controller
	uniqueControllerMap := make(map[string]struct{}, 0)
	err := j.ForEachUserModel(ctx, user, func(m *dbmodel.Model, uap jujuparams.UserAccessPermission) error {
		models = append(models, struct {
			model      *dbmodel.Model
			userAccess jujuparams.UserAccessPermission
		}{model: m, userAccess: uap})

		if _, ok := uniqueControllerMap[m.Controller.UUID]; !ok {
			uniqueControllers = append(uniqueControllers, m.Controller)
			uniqueControllerMap[m.Controller.UUID] = struct{}{}
		}

		return nil
	})
	if err != nil {
		return jujuparams.ModelSummaryResults{}, errors.E(op, err)
	}

	// we query the model summaries for each controller
	err = j.forEachController(ctx, uniqueControllers, func(c *dbmodel.Controller, a API) error {
		results, err := a.ListModelSummaries(ctx, jujuparams.ModelSummariesRequest{All: true})
		if err != nil {
			return err
		}
		for _, res := range results.Results {
			modelSummariesSafeMap.addModelSummary(res)
		}
		return nil
	})
	if err != nil {
		// we log the error and continue, because even if one controller is not reachable we are still able to fill the response.
		zapctx.Error(ctx, "Error querying the controllers for model summaries", zap.Error(err))
	}

	// we map models to modelsummaries
	for _, m := range models {
		modelSummaryFromController, ok := modelSummariesSafeMap.modelSummaries[m.model.UUID.String]
		modelSummaryResult := m.model.MergeModelSummaryFromController(modelSummaryFromController.Result, maskingControllerUUID, m.userAccess)
		if modelSummaryFromController.Error != nil {
			modelSummaryResults = append(modelSummaryResults, jujuparams.ModelSummaryResult{
				Result: &modelSummaryResult,
				Error:  modelSummaryFromController.Error,
			})
			continue
		}
		if !ok {
			// if model was not found in any controller we mark it as anavailable
			modelSummaryResult.Status.Status = "unavailable"
		}
		modelSummaryResults = append(modelSummaryResults, jujuparams.ModelSummaryResult{
			Result: &modelSummaryResult,
		})
	}
	return jujuparams.ModelSummaryResults{
		Results: modelSummaryResults,
	}, nil
}

// mergeModelInfo replaces fields on the juju model info object with
// information from JIMM where JIMM specific information should be used.
func (j *JujuManager) mergeModelInfo(ctx context.Context, user *openfga.User, modelInfo *jujuparams.ModelInfo, jimmModel dbmodel.Model) (*jujuparams.ModelInfo, error) {
	const op = errors.Op("jimm.mergeModelInfo")
	zapctx.Info(ctx, string(op))

	modelInfo.CloudCredentialTag = jimmModel.CloudCredential.Tag().String()
	modelInfo.ControllerUUID = jimmModel.Controller.UUID
	modelInfo.OwnerTag = jimmModel.Owner.Tag().String()

	userAccess := make(map[string]string)

	for _, relation := range []openfga.Relation{
		// Here we list possible relation in decreasing level
		// of access privilege.
		ofganames.AdministratorRelation,
		ofganames.WriterRelation,
		ofganames.ReaderRelation,
	} {
		usersWithSpecifiedRelation, err := openfga.ListUsersWithAccess(ctx, j.OpenFGAClient, jimmModel.ResourceTag(), relation)
		if err != nil {
			return nil, errors.E(op, err)
		}
		for _, u := range usersWithSpecifiedRelation {
			// Since we are checking user relations in decreasing level of
			// access privilege, we want to make sure the user has not
			// already been recorded with a higher access level.
			if _, ok := userAccess[u.Name]; !ok {
				userAccess[u.Name] = permissions.ToModelAccessString(relation)
			}
		}
	}

	modelAccess, err := j.permissionManager.GetUserModelAccess(ctx, user, jimmModel.ResourceTag())
	if err != nil {
		return nil, errors.E(op, err)
	}

	users := make([]jujuparams.ModelUserInfo, 0, len(userAccess))
	for username, access := range userAccess {
		// If the user does not contain an "@" sign (no domain), it means
		// this is a local user of this controller and JIMM does not
		// care or know about local users.
		if !strings.Contains(username, "@") {
			continue
		}
		if modelAccess == "admin" || username == user.Name || username == ofganames.EveryoneUser {
			users = append(users, jujuparams.ModelUserInfo{
				UserName: username,
				Access:   jujuparams.UserAccessPermission(access),
			})
		}
	}
	modelInfo.Users = users

	if modelAccess != "admin" && modelAccess != "write" {
		// Users need "write" level access (or above) to see machine
		// information.
		modelInfo.Machines = nil
	}

	return modelInfo, nil
}

// ModelStatus returns a jujuparams.ModelStatus for the given model. If
// the model doesn't exist then the returned error will have the code
// CodeNotFound, If the given user does not have admin access to the model
// then the returned error will have the code CodeUnauthorized.
func (j *JujuManager) ModelStatus(ctx context.Context, user *openfga.User, mt names.ModelTag) (*jujuparams.ModelStatus, error) {
	const op = errors.Op("jimm.ModelStatus")
	zapctx.Info(ctx, string(op))

	var ms jujuparams.ModelStatus
	err := j.doModelAdmin(ctx, user, mt, func(m *dbmodel.Model, api API) error {
		ms.OwnerTag = m.Owner.Tag().String()
		ms.ModelTag = mt.String()
		return api.ModelStatus(ctx, &ms)
	})
	if err != nil {
		return nil, errors.E(op, err)
	}
	return &ms, nil
}

// ForEachUserModel calls the given function once for each model that the
// given user has been granted explicit access to. The UserModelAccess
// object passed to f will always include the Model_, Access, and
// LastConnection fields populated. ForEachUserModel ignores a user's
// controller access when determining the set of models to return, for
// superusers the ForEachModel method should be used to get every model in
// the system. If the given function returns an error the error will be
// returned unmodified and iteration will stop immediately. The given
// function should not update the database.
func (j *JujuManager) ForEachUserModel(ctx context.Context, user *openfga.User, f func(*dbmodel.Model, jujuparams.UserAccessPermission) error) error {
	const op = errors.Op("jimm.ForEachUserModel")
	zapctx.Info(ctx, string(op))

	errStop := errors.E("stop")
	var iterErr error
	err := j.Database.ForEachModel(ctx, func(m *dbmodel.Model) error {
		model := *m

		access, err := j.permissionManager.GetUserModelAccess(ctx, user, model.ResourceTag())
		if err != nil {
			return errors.E(op, err)
		}
		if access == "read" || access == "write" || access == "admin" {
			if err := f(&model, jujuparams.UserAccessPermission(access)); err != nil {
				iterErr = err
				return errStop
			}
			return nil
		}
		return nil
	})
	switch err {
	case nil:
		return nil
	case errStop:
		return iterErr
	default:
		return errors.E(op, err)
	}
}

// ForEachModel calls the given function once for each model in the system.
// The UserModelAccess object passed to f will always specify that the
// user's Access is "admin" and will not include the LastConnection time.
// ForEachModel will return an error with the code CodeUnauthorized when
// the user is not a controller admin. If the given function returns an
// error the error will be returned unmodified and iteration will stop
// immediately. The given function should not update the database.
func (j *JujuManager) ForEachModel(ctx context.Context, user *openfga.User, f func(*dbmodel.Model, jujuparams.UserAccessPermission) error) error {
	const op = errors.Op("jimm.ForEachModel")
	zapctx.Info(ctx, string(op))

	if !user.JimmAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	errStop := errors.E("stop")
	var iterErr error
	err := j.Database.ForEachModel(ctx, func(m *dbmodel.Model) error {
		if err := f(m, jujuparams.UserAccessPermission("admin")); err != nil {
			iterErr = err
			return errStop
		}
		return nil
	})
	switch err {
	case nil:
		return nil
	case errStop:
		return iterErr
	default:
		return errors.E(op, err)
	}
}

// DestroyModel starts the process of destroying the given model. If the
// given user is not a controller superuser or a model admin an error
// with a code of CodeUnauthorized is returned. Any error returned from
// the juju API will not have it's code masked.
func (j *JujuManager) DestroyModel(ctx context.Context, user *openfga.User, mt names.ModelTag, destroyStorage, force *bool, maxWait, timeout *time.Duration) error {
	const op = errors.Op("jimm.DestroyModel")
	zapctx.Info(ctx, string(op))

	err := j.doModelAdmin(ctx, user, mt, func(m *dbmodel.Model, api API) error {
		m.Life = state.Dying.String()
		if err := j.Database.UpdateModel(ctx, m); err != nil {
			zapctx.Error(ctx, "failed to store model change", zaputil.Error(err))
			return err
		}
		if err := api.DestroyModel(ctx, mt, destroyStorage, force, maxWait, timeout); err != nil {
			zapctx.Error(ctx, "failed to call DestroyModel juju api", zaputil.Error(err))
			// this is a manual way of restoring the life state to alive if the JUJU api fails.
			m.Life = state.Alive.String()
			if uerr := j.Database.UpdateModel(ctx, m); uerr != nil {
				zapctx.Error(ctx, "failed to store model change", zaputil.Error(uerr))
			}
			return err
		}

		return nil
	})
	if err != nil {
		return errors.E(op, err)
	}

	// NOTE (alesstimec) If we remove OpenFGA relation now, the user
	// will no longer be authorised to check for model status (which
	// will show the model as dying for a bit, until the Juju controller
	// completes the model destuction).

	return nil
}

// DumpModel retrieves a database-agnostic dump of the given model from its
// juju controller. If simplified is true a simpllified dump is requested.
// If the given user is not a controller superuser or a model admin an
// error with the code CodeUnauthorized is returned.
func (j *JujuManager) DumpModel(ctx context.Context, user *openfga.User, mt names.ModelTag, simplified bool) (string, error) {
	const op = errors.Op("jimm.DumpModel")
	zapctx.Info(ctx, string(op))

	var dump string
	err := j.doModelAdmin(ctx, user, mt, func(m *dbmodel.Model, api API) error {
		var err error
		dump, err = api.DumpModel(ctx, mt, simplified)
		return err
	})
	if err != nil {
		return "", errors.E(op, err)
	}
	return dump, nil
}

// DumpModelDB retrieves a database dump of the given model from its juju
// controller. If the given user is not a controller superuser or a model
// admin an error with the code CodeUnauthorized is returned.
func (j *JujuManager) DumpModelDB(ctx context.Context, user *openfga.User, mt names.ModelTag) (map[string]interface{}, error) {
	const op = errors.Op("jimm.DumpModelDB")
	zapctx.Info(ctx, string(op))

	var dump map[string]interface{}
	err := j.doModelAdmin(ctx, user, mt, func(m *dbmodel.Model, api API) error {
		var err error
		dump, err = api.DumpModelDB(ctx, mt)
		return err
	})
	if err != nil {
		return nil, errors.E(op, err)
	}
	return dump, nil
}

// ValidateModelUpgrade validates that a model is in a state that can be
// upgraded. If the given user is not a controller superuser or a model
// admin then an error with the code CodeUnauthorized is returned. Any
// error returned from the API will have the code maintained therefore if
// the controller doesn't support the ValidateModelUpgrades command the
// CodeNotImplemented error code will be propagated back to the client.
func (j *JujuManager) ValidateModelUpgrade(ctx context.Context, user *openfga.User, mt names.ModelTag, force bool) error {
	const op = errors.Op("jimm.ValidateModelUpgrade")
	zapctx.Info(ctx, string(op))

	err := j.doModelAdmin(ctx, user, mt, func(_ *dbmodel.Model, api API) error {
		return api.ValidateModelUpgrade(ctx, mt, force)
	})
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// doModelAdmin is a simple wrapper that provides the common parts of model
// administration commands. doModelAdmin finds the model with the given tag
// and validates that the given user has admin access to the model.
// doModelAdmin then connects to the controller hosting the model and calls
// the given function with the model and API connection to perform the
// operation specific commands. If the model cannot be found then an error
// with the code CodeNotFound is returned. If the given user does not have
// admin access to the model then an error with the code CodeUnauthorized
// is returned. If there is an error connecting to the controller hosting
// the model then the returned error will have the same code as the error
// returned from the dial operation. If the given function returns an error
// that error will be returned with the code unmasked.
func (j *JujuManager) doModelAdmin(ctx context.Context, user *openfga.User, mt names.ModelTag, f func(*dbmodel.Model, API) error) error {
	return j.doModel(ctx, user, mt, ofganames.AdministratorRelation, f)
}

func (j *JujuManager) doModel(ctx context.Context, user *openfga.User, mt names.ModelTag, requireRelation openfga.Relation, f func(*dbmodel.Model, API) error) error {
	const op = errors.Op("jimm.doModel")
	zapctx.Info(ctx, string(op))

	var m dbmodel.Model
	m.SetTag(mt)

	if err := j.Database.GetModel(ctx, &m); err != nil {
		return errors.E(op, err)
	}

	hasAccess, err := user.HasModelRelation(ctx, mt, requireRelation)
	if err != nil {
		return errors.E(op, err)
	}

	if !hasAccess {
		// If the user doesn't have correct access on the model return
		// an unauthorized error.
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	api, err := j.dial(ctx, &m.Controller, names.ModelTag{})
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()
	if err := f(&m, api); err != nil {
		return errors.E(op, err)
	}
	return nil
}

// ChangeModelCredential changes the credential used with a model on both
// the controller and the local database.
func (j *JujuManager) ChangeModelCredential(ctx context.Context, user *openfga.User, modelTag names.ModelTag, cloudCredentialTag names.CloudCredentialTag) error {
	const op = errors.Op("jimm.ChangeModelCredential")
	zapctx.Info(ctx, string(op))

	if !user.JimmAdmin && user.Tag() != cloudCredentialTag.Owner() {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	credential := dbmodel.CloudCredential{}
	credential.SetTag(cloudCredentialTag)

	err := j.Database.GetCloudCredential(ctx, &credential)
	if err != nil {
		return errors.E(op, err)
	}

	var m *dbmodel.Model
	err = j.doModelAdmin(ctx, user, modelTag, func(model *dbmodel.Model, api API) error {
		_, err = j.updateControllerCloudCredential(ctx, &credential, api.UpdateCredential)
		if err != nil {
			return errors.E(op, err)
		}

		err = api.ChangeModelCredential(ctx, modelTag, cloudCredentialTag)
		if err != nil {
			return errors.E(op, err)
		}
		m = model
		return nil
	})
	if err != nil {
		return errors.E(op, err)
	}

	m.CloudCredential = credential
	m.CloudCredentialID = credential.ID
	err = j.Database.UpdateModel(ctx, m)
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

// ListModels list the models that the user has access to. It intentionally excludes the
// controller model as this call is used within the context of login and register commands.
func (j *JujuManager) ListModels(ctx context.Context, user *openfga.User) ([]base.UserModel, error) {
	const op = errors.Op("jimm.ListModels")
	zapctx.Info(ctx, string(op))

	// Get uuids of models the user has access to
	uuids, err := user.ListModels(ctx, ofganames.ReaderRelation)
	if err != nil {
		return nil, errors.E(op, fmt.Sprintf("failed to list user models: %v", err))
	}

	// Get the models from the database
	models, err := j.Database.GetModelsByUUID(ctx, uuids)
	if err != nil {
		return nil, errors.E(op, fmt.Sprintf("failed to get models by uuid: %v", err))
	}

	// Create map for lookup later
	modelsMap := make(map[string]dbmodel.Model)
	// Find the controllers these models reside on and remove duplicates
	var controllers []dbmodel.Controller
	seen := make(map[uint]bool)
	for _, model := range models {
		modelsMap[model.UUID.String] = model // Set map for lookup
		if seen[model.ControllerID] {
			continue
		}
		seen[model.ControllerID] = true
		controllers = append(controllers, model.Controller)
	}

	// Call controllers for their models. We always call as admin, and we're
	// filtering ourselves. We do this rather than send the user to be 100%
	// certain that the models do belong to user according to OpenFGA. We could
	// in theory rely on Juju correctly returning the models (by owner), but this
	// is more reliable.
	var userModels []base.UserModel
	var mutex sync.Mutex
	err = j.forEachController(ctx, controllers, func(_ *dbmodel.Controller, api API) error {
		ums, err := api.ListModels(ctx)
		if err != nil {
			return err
		}
		mutex.Lock()
		defer mutex.Unlock()

		// Filter the models returned according to the uuids
		// returned from OpenFGA for read access.
		//
		// NOTE: Controller models are not included because we never relate
		// controller models to users, and as such, they will not appear in the
		// authorised uuid map.
		for _, um := range ums {
			mapModel, ok := modelsMap[um.UUID]
			if !ok {
				continue
			}
			um.Owner = mapModel.OwnerIdentityName
			userModels = append(userModels, um)
		}
		return nil
	})
	if err != nil {
		return nil, errors.E(op, fmt.Sprintf("failed to list models: %v", err))
	}

	return userModels, nil
}
