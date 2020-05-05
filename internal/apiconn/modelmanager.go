// Copyright 2020 Canonical Ltd.

package apiconn

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/names/v4"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
)

// CreateModel creates a new model as specified by the given model
// specification. If the model is created successfully then the model
// document passed in will be updated with the model information returned
// from the Create model call. If there is an error returned it will be
// of type *APIError. CreateModel uses the Create model procedure on the
// ModelManager facade version 2.
func (c *Conn) CreateModel(ctx context.Context, model *mongodoc.Model) error {
	if model.Info == nil {
		model.Info = new(mongodoc.ModelInfo)
	}
	if model.Info.Config == nil {
		model.Info.Config = make(map[string]interface{})
	}

	args := jujuparams.ModelCreateArgs{
		Name:        string(model.Path.Name),
		OwnerTag:    conv.ToUserTag(model.Path.User).String(),
		Config:      model.Info.Config,
		CloudRegion: model.CloudRegion,
	}
	if model.Cloud != "" {
		args.CloudTag = conv.ToCloudTag(model.Cloud).String()
	}
	if !model.Credential.IsZero() {
		args.CloudCredentialTag = conv.ToCloudCredentialTag(model.Credential.ToParams()).String()
	}

	var resp jujuparams.ModelInfo
	if err := c.APICall("ModelManager", 2, "", "CreateModel", &args, &resp); err != nil {
		return newAPIError(err)
	}

	model.UUID = resp.UUID
	if ct, err := names.ParseCloudTag(resp.CloudTag); err == nil {
		model.Cloud = conv.FromCloudTag(ct)
	}
	model.CloudRegion = resp.CloudRegion
	model.DefaultSeries = resp.DefaultSeries
	model.Info.Life = string(resp.Life)
	model.Info.Status.Status = string(resp.Status.Status)
	model.Info.Status.Message = resp.Status.Info
	model.Info.Status.Data = resp.Status.Data
	if resp.Status.Since != nil {
		model.Info.Status.Since = *resp.Status.Since
	}
	if resp.AgentVersion != nil {
		model.Info.Config[config.AgentVersionKey] = resp.AgentVersion.String()
	}
	model.Type = resp.Type
	model.ProviderType = resp.ProviderType
	return nil
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
// GrantJIMMModelAdmin uses the Create model procedure on the
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
