// Copyright 2020 Canonical Ltd.

package apiconn

import (
	"context"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/params"
)

// CreateModel creates a new model as specified by the given model
// specification. If the model is created successfully then the model
// document passed in will be updated with the model information returned
// from the Create model call. If there is an error returned it will be
// of type *APIError. CreateModel uses the Create model procedure on the
// ModelManager facade version 2.
func (c *Conn) CreateModel(ctx context.Context, args *jujuparams.ModelCreateArgs, info *jujuparams.ModelInfo) error {
	return newAPIError(c.APICall("ModelManager", 2, "", "CreateModel", args, info))
}

// ModelInfo retrieves information about a model from the controller. The
// given info structure must specify a UUID, the rest will be filled out
// from the controller response. If an error is returned by the Juju API
// then the resulting error response will be of type *APIError. ModelInfo
// will use the ModelInfo procedure from the ModelManager version 8
// facade if it is available, falling back to version 3.
func (c *Conn) ModelInfo(ctx context.Context, info *jujuparams.ModelInfo) error {
	args := jujuparams.Entities{
		Entities: []jujuparams.Entity{{
			Tag: names.NewModelTag(info.UUID).String(),
		}},
	}

	var resp jujuparams.ModelInfoResults
	var err error
	if c.HasFacadeVersion("ModelManager", 8) {
		err = c.APICall("ModelManager", 8, "", "ModelInfo", &args, &resp)
	} else {
		err = c.APICall("ModelManager", 3, "", "ModelInfo", &args, &resp)
	}
	if err != nil {
		return newAPIError(err)
	}
	if len(resp.Results) != 1 {
		return errgo.Newf("unexpected number of results (expected 1, got %d)", len(resp.Results))
	}
	if resp.Results[0].Result != nil {
		*info = *resp.Results[0].Result
	}
	return newAPIError(resp.Results[0].Error)
}

// GrantJIMMModelAdmin ensures that the JIMM user is an admin level user
// of the given model. This is a specialized wrapper around
// ModifyModelAccess to be used when bootstrapping a model. Any error
// that is returned from the API will be of type *APIError.
// GrantJIMMModelAdmin uses the ModifyModelAccess procedure on the
// ModelManager facade version 2.
func (c *Conn) GrantJIMMModelAdmin(ctx context.Context, uuid string) error {
	args := jujuparams.ModifyModelAccessRequest{
		Changes: []jujuparams.ModifyModelAccess{{
			UserTag:  c.Info.Tag.String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelAdminAccess,
			ModelTag: names.NewModelTag(uuid).String(),
		}},
	}

	var resp jujuparams.ErrorResults
	if err := c.APICall("ModelManager", 2, "", "ModifyModelAccess", &args, &resp); err != nil {
		return newAPIError(err)
	}
	if len(resp.Results) != 1 {
		return errgo.Newf("unexpected number of results (expected 1, got %d)", len(resp.Results))
	}
	return newAPIError(resp.Results[0].Error)
}

// DumpModel dumps debugging details for the given model. DumpModel uses
// the DumpModels method on the ModelManager facade version 2.
func (c *Conn) DumpModel(ctx context.Context, uuid string) (map[string]interface{}, error) {
	args := jujuparams.Entities{
		Entities: []jujuparams.Entity{{
			Tag: names.NewModelTag(uuid).String(),
		}},
	}

	var resp jujuparams.MapResults
	if err := c.APICall("ModelManager", 2, "", "DumpModels", &args, &resp); err != nil {
		return nil, newAPIError(err)
	}

	if len(resp.Results) != 1 {
		return nil, errgo.Newf("unexpected number of results (expected 1, got %d)", len(resp.Results))
	}
	return resp.Results[0].Result, newAPIError(resp.Results[0].Error)
}

// DumpModelV3 dumps debugging details for the given model into the given
// . If the simplied dump is requested then a simplified dump is
// returned. DumpModelV3 uses the DumpModels method on the ModelManager
// facade version 3.
func (c *Conn) DumpModelV3(ctx context.Context, uuid string, simplified bool) (string, error) {
	args := jujuparams.DumpModelRequest{
		Entities: []jujuparams.Entity{{
			Tag: names.NewModelTag(uuid).String(),
		}},
		Simplified: simplified,
	}

	var resp jujuparams.StringResults
	if err := c.APICall("ModelManager", 3, "", "DumpModels", &args, &resp); err != nil {
		return "", newAPIError(err)
	}

	if len(resp.Results) != 1 {
		return "", errgo.Newf("unexpected number of results (expected 1, got %d)", len(resp.Results))
	}
	return resp.Results[0].Result, newAPIError(resp.Results[0].Error)
}

// DumpModelDB dumps the controller database entry given model.
// DumpModelDB uses the DumpModelsDB method on the ModelManager facade
// version 2.
func (c *Conn) DumpModelDB(ctx context.Context, uuid string) (map[string]interface{}, error) {
	args := jujuparams.Entities{
		Entities: []jujuparams.Entity{{
			Tag: names.NewModelTag(uuid).String(),
		}},
	}

	var resp jujuparams.MapResults
	if err := c.APICall("ModelManager", 2, "", "DumpModelsDB", &args, &resp); err != nil {
		return nil, newAPIError(err)
	}

	if len(resp.Results) != 1 {
		return nil, errgo.Newf("unexpected number of results (expected 1, got %d)", len(resp.Results))
	}
	return resp.Results[0].Result, newAPIError(resp.Results[0].Error)
}

// GrantModelAccess gives the given user the given access level on the
// given model. GrantModelAccess uses the ModifyModelAccess procedure
// on the ModelManager facade version 2.
func (c *Conn) GrantModelAccess(ctx context.Context, uuid string, user params.User, access jujuparams.UserAccessPermission) error {
	args := jujuparams.ModifyModelAccessRequest{
		Changes: []jujuparams.ModifyModelAccess{{
			UserTag:  conv.ToUserTag(user).String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   access,
			ModelTag: names.NewModelTag(uuid).String(),
		}},
	}

	var resp jujuparams.ErrorResults
	err := c.APICall("ModelManager", 2, "", "ModifyModelAccess", &args, &resp)
	if err != nil {
		return newAPIError(err)
	}
	if len(resp.Results) != 1 {
		return errgo.Newf("unexpected number of results (expected 1, got %d)", len(resp.Results))
	}
	return newAPIError(resp.Results[0].Error)
}

// RevokeModelAccess removes the given access level from the given user on
// the given model. Revoke ModelAccess uses the ModifyModelAccess procedure
// on the ModelManager facade version 2.
func (c *Conn) RevokeModelAccess(ctx context.Context, uuid string, user params.User, access jujuparams.UserAccessPermission) error {
	args := jujuparams.ModifyModelAccessRequest{
		Changes: []jujuparams.ModifyModelAccess{{
			UserTag:  conv.ToUserTag(user).String(),
			Action:   jujuparams.RevokeModelAccess,
			Access:   access,
			ModelTag: names.NewModelTag(uuid).String(),
		}},
	}

	var resp jujuparams.ErrorResults
	err := c.APICall("ModelManager", 2, "", "ModifyModelAccess", &args, &resp)
	if err != nil {
		return newAPIError(err)
	}
	if len(resp.Results) != 1 {
		return errgo.Newf("unexpected number of results (expected 1, got %d)", len(resp.Results))
	}
	return newAPIError(resp.Results[0].Error)
}

// ControllerModelSummary retrieves the ModelSummary for the controller
// model. ControllerModelSummary uses the ListModelSummaries procedure on
// the ModelManager facade version 4.
func (c *Conn) ControllerModelSummary(ctx context.Context, ms *jujuparams.ModelSummary) error {
	args := jujuparams.ModelSummariesRequest{
		UserTag: c.Info.Tag.String(),
		All:     true,
	}
	var resp jujuparams.ModelSummaryResults
	err := c.APICall("ModelManager", 4, "", "ListModelSummaries", &args, &resp)
	if err != nil {
		return newAPIError(err)
	}
	for _, r := range resp.Results {
		if r.Result != nil && r.Result.IsController {
			*ms = *r.Result
			return nil
		}
	}
	return errgo.WithCausef(nil, params.ErrNotFound, "controller model not found")
}

// DestroyModel starts the destruction of the given model. This method uses
// the highest available method from:
//
//  - ModelManager(7).DestroyModels
//  - ModelManager(4).DestroyModels
//  - ModelManager(2).DestroyModels
func (c *Conn) DestroyModel(ctx context.Context, uuid string, destroyStorage *bool, force *bool, maxWait *time.Duration) error {
	mt := names.NewModelTag(uuid)
	args := jujuparams.DestroyModelsParams{
		Models: []jujuparams.DestroyModelParams{{
			ModelTag:       mt.String(),
			DestroyStorage: destroyStorage,
			Force:          force,
			MaxWait:        maxWait,
		}},
	}

	resp := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, 1),
	}
	var err error
	switch {
	case c.HasFacadeVersion("ModelManager", 7):
		err = c.APICall("ModelManager", 7, "", "DestroyModels", &args, &resp)
	case c.HasFacadeVersion("ModelManager", 4):
		err = c.APICall("ModelManager", 4, "", "DestroyModels", &args, &resp)
	default:
		err = c.APICall("ModelManager", 2, "", "DestroyModels",
			&jujuparams.Entities{Entities: []jujuparams.Entity{{Tag: mt.String()}}},
			&resp,
		)
	}

	if err != nil {
		return newAPIError(err)
	}
	return newAPIError(resp.Results[0].Error)
}

// ModelStatus retrieves the status of a model from the controller. The
// given status structure must specify a ModelTag, the rest will be filled
// out from the controller response. If an error is returned by the Juju
// API then the resulting error response will be of type *APIError.
// ModelStatus will use the ModelStatus procedure from the ModelManager
// version 4 facade if it is available, falling back to version 2.
func (c *Conn) ModelStatus(ctx context.Context, status *jujuparams.ModelStatus) error {
	args := jujuparams.Entities{
		Entities: []jujuparams.Entity{{
			Tag: status.ModelTag,
		}},
	}

	resp := jujuparams.ModelStatusResults{
		Results: make([]jujuparams.ModelStatus, 1),
	}
	var err error
	if c.HasFacadeVersion("ModelManager", 4) {
		err = c.APICall("ModelManager", 4, "", "ModelStatus", &args, &resp)
	} else {
		err = c.APICall("ModelManager", 2, "", "ModelStatus", &args, &resp)
	}
	if err != nil {
		return newAPIError(err)
	}
	if resp.Results[0].Error != nil {
		return newAPIError(resp.Results[0].Error)
	}
	*status = resp.Results[0]
	return nil
}
