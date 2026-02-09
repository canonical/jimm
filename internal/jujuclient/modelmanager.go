// Copyright 2025 Canonical.

package jujuclient

import (
	"time"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/modelmanager"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
)

// CreateModelArgs holds the arguments for creating a model.
type CreateModelArgs struct {
	Name               string
	Owner              string
	Cloud              string
	CloudRegion        string
	CloudCredentialTag names.CloudCredentialTag
	Config             map[string]interface{}
}

// CreateModel creates a new model as specified by the given model
// specification returning the model details created. CreateModel
// uses the Create model procedure on the ModelManager facade.
func (c Connection) CreateModel(args *CreateModelArgs) (base.ModelInfo, error) {
	return modelmanager.NewClient(&c).CreateModel(
		args.Name,
		args.Owner,
		args.Cloud,
		args.CloudRegion,
		args.CloudCredentialTag,
		args.Config)
}

type ModelMigrationStatus struct {
	Status string
	Start  *time.Time
	End    *time.Time
}

type ModelInfo struct {
	base.ModelInfo
	MigrationStatus *ModelMigrationStatus
}

// ModelInfo retrieves information about a model from the controller.
func (c Connection) ModelInfo(model names.ModelTag) (ModelInfo, error) {
	res, err := modelmanager.NewClient(&c).ModelInfo([]names.ModelTag{model})
	if err != nil {
		return ModelInfo{}, err
	}
	if res[0].Error != nil {
		return ModelInfo{}, errors.E(res[0].Error)
	}
	return convertParamsModelInfo(*res[0].Result)
}

// DumpModel dumps debugging details for the given model. If the simplied
// dump is requested then a simplified dump is returned. DumpModel uses the
// DumpModels method on the ModelManager facade.
func (c Connection) DumpModel(tag names.ModelTag, simplified bool) (map[string]interface{}, error) {
	return modelmanager.NewClient(&c).DumpModel(tag, simplified)
}

// DumpModelDB dumps the controller database entry given model.
// DumpModelDB uses the DumpModelsDB method on the ModelManager facade..
func (c Connection) DumpModelDB(tag names.ModelTag) (map[string]interface{}, error) {
	return modelmanager.NewClient(&c).DumpModelDB(tag)
}

// ControllerModelSummary retrieves the ModelSummary for the controller
// model. ControllerModelSummary uses the ListModelSummaries procedure on
// the ModelManager facade.
func (c Connection) ControllerModelSummary() (base.UserModelSummary, error) {
	modelSummaires, err := modelmanager.NewClient(&c).ListModelSummaries(c.user.ResourceTag().String(), true)
	if err != nil {
		return base.UserModelSummary{}, err
	}
	for _, r := range modelSummaires {
		if r.IsController {
			return r, nil
		}
	}
	return base.UserModelSummary{}, errors.E("controller model not found", errors.CodeNotFound)
}

// ListModelSummaries retrieves the list of model summaries from the controler
func (c Connection) ListModelSummaries(ms jujuparams.ModelSummariesRequest) ([]base.UserModelSummary, error) {
	return modelmanager.NewClient(&c).ListModelSummaries(c.user.ResourceTag().String(), ms.All)
}

// ValidateModelUpgrade validates if a model is allowed to perform an upgrade. It
// uses ValidateModelUpgrades on the ModelManager facade.
func (c Connection) ValidateModelUpgrade(model names.ModelTag, force bool) error {
	return modelmanager.NewClient(&c).ValidateModelUpgrade(model, force)
}

// DestroyModel starts the destruction of the given model.
func (c Connection) DestroyModel(tag names.ModelTag, destroyStorage *bool, force *bool, maxWait, timeout *time.Duration) error {
	return modelmanager.NewClient(&c).DestroyModel(tag, destroyStorage, force, maxWait, timeout)
}

// ModelStatus retrieves the status of a model from the controller.
func (c Connection) ModelStatus(modelTag names.ModelTag) (base.ModelStatus, error) {
	statuses, err := modelmanager.NewClient(&c).ModelStatus(modelTag)
	if err != nil {
		return base.ModelStatus{}, err
	}
	return statuses[0], nil
}

// ChangeModelCredential replaces cloud credential for a given model with the provided one.
func (c Connection) ChangeModelCredential(model names.ModelTag, credential names.CloudCredentialTag) error {
	return modelmanager.NewClient(&c).ChangeModelCredential(model, credential)
}

// ListModels returns UserModel's for the user that is logged in. If the user logged
// in is "admin" they may specify another user's models.
//
// In our wrapper, we ask as the controller admin. So expect ALL models from
// the controller.
func (c Connection) ListModels() ([]base.UserModel, error) {
	return modelmanager.NewClient(&c).ListModels("admin")
}
