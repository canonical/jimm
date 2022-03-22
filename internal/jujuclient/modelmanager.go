// Copyright 2020 Canonical Ltd.

package jujuclient

import (
	"context"
	"time"

	jujuerrors "github.com/juju/errors"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/errors"
)

// CreateModel creates a new model as specified by the given model
// specification. If the model is created successfully then the model
// document passed in will be updated with the model information returned
// from the Create model call. If there is an error returned it will be
// of type *APIError. CreateModel uses the Create model procedure on the
// ModelManager facade version 2.
func (c Connection) CreateModel(ctx context.Context, args *jujuparams.ModelCreateArgs, info *jujuparams.ModelInfo) error {
	const op = errors.Op("jujuclient.CreateModel")
	if err := c.client.Call(ctx, "ModelManager", 2, "", "CreateModel", args, info); err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	return nil
}

// ModelInfo retrieves information about a model from the controller. The
// given info structure must specify a UUID, the rest will be filled out
// from the controller response. If an error is returned by the Juju API
// then the resulting error response will be of type *APIError. ModelInfo
// will use the ModelInfo procedure from the ModelManager version 8
// facade if it is available, falling back to version 3.
func (c Connection) ModelInfo(ctx context.Context, info *jujuparams.ModelInfo) error {
	const op = errors.Op("jujuclient.ModelInfo")
	args := jujuparams.Entities{
		Entities: []jujuparams.Entity{{
			Tag: names.NewModelTag(info.UUID).String(),
		}},
	}

	resp := jujuparams.ModelInfoResults{
		Results: []jujuparams.ModelInfoResult{{
			Result: info,
		}},
	}
	var err error
	if c.hasFacadeVersion("ModelManager", 8) {
		err = c.client.Call(ctx, "ModelManager", 8, "", "ModelInfo", &args, &resp)
	} else {
		err = c.client.Call(ctx, "ModelManager", 3, "", "ModelInfo", &args, &resp)
	}
	if err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return errors.E(op, resp.Results[0].Error)
	}
	return nil
}

// GrantJIMMModelAdmin ensures that the JIMM user is an admin level user
// of the given model. This is a specialized wrapper around
// ModifyModelAccess to be used when bootstrapping a model. Any error
// that is returned from the API will be of type *APIError.
// GrantJIMMModelAdmin uses the ModifyModelAccess procedure on the
// ModelManager facade version 2.
func (c Connection) GrantJIMMModelAdmin(ctx context.Context, tag names.ModelTag) error {
	const op = errors.Op("jujuclient.GrantJIMMModelAdmin")
	args := jujuparams.ModifyModelAccessRequest{
		Changes: []jujuparams.ModifyModelAccess{{
			UserTag:  c.userTag,
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelAdminAccess,
			ModelTag: tag.String(),
		}},
	}

	resp := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, 1),
	}
	if err := c.client.Call(ctx, "ModelManager", 2, "", "ModifyModelAccess", &args, &resp); err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return errors.E(op, resp.Results[0].Error)
	}
	return nil
}

// DumpModel dumps debugging details for the given model. If the simplied
// dump is requested then a simplified dump is returned. DumpModel uses the
// DumpModels method on the ModelManager facade version 3.
func (c Connection) DumpModel(ctx context.Context, tag names.ModelTag, simplified bool) (string, error) {
	const op = errors.Op("jujuclient.DumpModel")
	args := jujuparams.DumpModelRequest{
		Entities: []jujuparams.Entity{{
			Tag: tag.String(),
		}},
		Simplified: simplified,
	}

	resp := jujuparams.StringResults{
		Results: make([]jujuparams.StringResult, 1),
	}
	if err := c.client.Call(ctx, "ModelManager", 3, "", "DumpModels", &args, &resp); err != nil {
		return "", errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return "", errors.E(op, resp.Results[0].Error)
	}
	return resp.Results[0].Result, nil
}

// DumpModelDB dumps the controller database entry given model.
// DumpModelDB uses the DumpModelsDB method on the ModelManager facade
// version 2.
func (c Connection) DumpModelDB(ctx context.Context, tag names.ModelTag) (map[string]interface{}, error) {
	const op = errors.Op("jujuclient.DumpModelDB")
	args := jujuparams.Entities{
		Entities: []jujuparams.Entity{{
			Tag: tag.String(),
		}},
	}

	resp := jujuparams.MapResults{
		Results: make([]jujuparams.MapResult, 1),
	}
	if err := c.client.Call(ctx, "ModelManager", 2, "", "DumpModelsDB", &args, &resp); err != nil {
		return nil, errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return nil, errors.E(op, resp.Results[0].Error)
	}
	return resp.Results[0].Result, nil
}

// GrantModelAccess gives the given user the given access level on the
// given model. GrantModelAccess uses the ModifyModelAccess procedure
// on the ModelManager facade version 2.
func (c Connection) GrantModelAccess(ctx context.Context, modelTag names.ModelTag, userTag names.UserTag, access jujuparams.UserAccessPermission) error {
	const op = errors.Op("jujuclient.GrantModelAccess")
	args := jujuparams.ModifyModelAccessRequest{
		Changes: []jujuparams.ModifyModelAccess{{
			UserTag:  userTag.String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   access,
			ModelTag: modelTag.String(),
		}},
	}

	resp := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, 1),
	}
	err := c.client.Call(ctx, "ModelManager", 2, "", "ModifyModelAccess", &args, &resp)
	if err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return errors.E(op, resp.Results[0].Error)
	}
	return nil
}

// RevokeModelAccess removes the given access level from the given user on
// the given model. Revoke ModelAccess uses the ModifyModelAccess procedure
// on the ModelManager facade version 2.
func (c Connection) RevokeModelAccess(ctx context.Context, modelTag names.ModelTag, userTag names.UserTag, access jujuparams.UserAccessPermission) error {
	const op = errors.Op("jujuclient.RevokeModelAccess")
	args := jujuparams.ModifyModelAccessRequest{
		Changes: []jujuparams.ModifyModelAccess{{
			UserTag:  userTag.String(),
			Action:   jujuparams.RevokeModelAccess,
			Access:   access,
			ModelTag: modelTag.String(),
		}},
	}

	resp := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, 1),
	}
	err := c.client.Call(ctx, "ModelManager", 2, "", "ModifyModelAccess", &args, &resp)
	if err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return errors.E(op, resp.Results[0].Error)
	}
	return nil
}

// ControllerModelSummary retrieves the ModelSummary for the controller
// model. ControllerModelSummary uses the ListModelSummaries procedure on
// the ModelManager facade version 4.
func (c Connection) ControllerModelSummary(ctx context.Context, ms *jujuparams.ModelSummary) error {
	const op = errors.Op("jujuclient.ControllerModelSummary")
	args := jujuparams.ModelSummariesRequest{
		UserTag: c.userTag,
		All:     true,
	}
	var resp jujuparams.ModelSummaryResults
	err := c.client.Call(ctx, "ModelManager", 4, "", "ListModelSummaries", &args, &resp)
	if err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	for _, r := range resp.Results {
		if r.Result != nil && r.Result.IsController {
			*ms = *r.Result
			return nil
		}
	}
	return errors.E(op, "controller model not found", errors.CodeNotFound)
}

// ValidateModelUpgrade validates if a model is allowed to perform an upgrade. It
// uses ValidateModelUpgrades on the ModelManager facade version 9.
func (c Connection) ValidateModelUpgrade(ctx context.Context, model names.ModelTag, force bool) error {
	const op = errors.Op("jujuclient.ValidateModelUpgrade")
	args := jujuparams.ValidateModelUpgradeParams{
		Models: []jujuparams.ValidateModelUpgradeParam{{
			ModelTag: model.String(),
		}},
		Force: force,
	}
	resp := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, 1),
	}
	err := c.client.Call(ctx, "ModelManager", 9, "", "ValidateModelUpgrades", &args, &resp)
	if err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return errors.E(op, resp.Results[0].Error)
	}
	return nil
}

// DestroyModel starts the destruction of the given model. This method uses
// the highest available method from:
//
//  - ModelManager(7).DestroyModels
//  - ModelManager(4).DestroyModels
//  - ModelManager(2).DestroyModels
func (c Connection) DestroyModel(ctx context.Context, tag names.ModelTag, destroyStorage *bool, force *bool, maxWait, timeout *time.Duration) error {
	const op = errors.Op("jujuclient.DestroyModel")
	args := jujuparams.DestroyModelsParams{
		Models: []jujuparams.DestroyModelParams{{
			ModelTag:       tag.String(),
			DestroyStorage: destroyStorage,
			Force:          force,
			MaxWait:        maxWait,
			Timeout:        timeout,
		}},
	}

	resp := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, 1),
	}
	var err error
	switch {
	case c.hasFacadeVersion("ModelManager", 7):
		err = c.client.Call(ctx, "ModelManager", 7, "", "DestroyModels", &args, &resp)
	case c.hasFacadeVersion("ModelManager", 4):
		err = c.client.Call(ctx, "ModelManager", 4, "", "DestroyModels", &args, &resp)
	default:
		err = c.client.Call(ctx, "ModelManager", 2, "", "DestroyModels",
			&jujuparams.Entities{Entities: []jujuparams.Entity{{Tag: tag.String()}}},
			&resp,
		)
	}

	if err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return errors.E(op, resp.Results[0].Error)
	}
	return nil
}

// ModelStatus retrieves the status of a model from the controller. The
// given status structure must specify a ModelTag, the rest will be filled
// out from the controller response. If an error is returned by the Juju
// API then the resulting error response will be of type *APIError.
// ModelStatus will use the ModelStatus procedure from the ModelManager
// version 4 facade if it is available, falling back to version 2.
func (c Connection) ModelStatus(ctx context.Context, status *jujuparams.ModelStatus) error {
	const op = errors.Op("jujuclient.ModelStatus")
	args := jujuparams.Entities{
		Entities: []jujuparams.Entity{{
			Tag: status.ModelTag,
		}},
	}

	resp := jujuparams.ModelStatusResults{
		Results: make([]jujuparams.ModelStatus, 1),
	}
	var err error
	if c.hasFacadeVersion("ModelManager", 4) {
		err = c.client.Call(ctx, "ModelManager", 4, "", "ModelStatus", &args, &resp)
	} else {
		err = c.client.Call(ctx, "ModelManager", 2, "", "ModelStatus", &args, &resp)
	}
	if err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return errors.E(op, resp.Results[0].Error)
	}
	*status = resp.Results[0]
	return nil
}

// ChangeModelCredential replaces cloud credential for a given model with the provided one.
func (c Connection) ChangeModelCredential(ctx context.Context, model names.ModelTag, credential names.CloudCredentialTag) error {
	const op = errors.Op("jujuclient.ChangeModelCredential")

	var out jujuparams.ErrorResults
	args := jujuparams.ChangeModelCredentialsParams{
		Models: []jujuparams.ChangeModelCredentialParams{{
			ModelTag:           model.String(),
			CloudCredentialTag: credential.String(),
		}},
	}

	err := c.client.Call(ctx, "ModelManager", 5, "", "ChangeModelCredential", &args, &out)
	if err != nil {
		return errors.E(op, err)
	}
	return out.OneError()
}
