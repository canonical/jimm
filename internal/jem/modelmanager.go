// Copyright 2020 Canonical Ltd.

package jem

import (
	"context"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/params"
)

// ValidateModelUpgrade validates if a model is allowed to perform an upgrade.
func (j *JEM) ValidateModelUpgrade(ctx context.Context, id identchecker.ACLIdentity, modelUUID string, force bool) error {
	model := mongodoc.Model{UUID: modelUUID}
	if err := j.GetModel(ctx, id, jujuparams.ModelAdminAccess, &model); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}

	conn, err := j.OpenAPI(ctx, model.Controller)
	if err != nil {
		return errgo.Notef(err, "cannot connect to controller")
	}
	defer conn.Close()

	return errgo.Mask(conn.ValidateModelUpgrade(ctx, names.NewModelTag(model.UUID), force), apiconn.IsAPIError)
}

// DestroyModel destroys the specified model. The model will have its
// Life set to dying, but won't be removed until it is removed from the
// controller.
func (j *JEM) DestroyModel(ctx context.Context, id identchecker.ACLIdentity, model *mongodoc.Model, destroyStorage *bool, force *bool, maxWait *time.Duration) error {
	if err := j.GetModel(ctx, id, jujuparams.ModelAdminAccess, model); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	conn, err := j.OpenAPI(ctx, model.Controller)
	if err != nil {
		return errgo.Mask(err)
	}
	if err := conn.DestroyModel(ctx, model.UUID, destroyStorage, force, maxWait); err != nil {
		return errgo.Mask(err, apiconn.IsAPIError)
	}
	if err := j.DB.SetModelLife(ctx, model.Controller, model.UUID, "dying"); err != nil {
		// If this update fails then don't worry as the watcher
		// will detect the state change and update as appropriate.
		zapctx.Warn(ctx, "error updating model life", zap.Error(err), zap.String("model", model.UUID))
	}
	j.DB.AppendAudit(ctx, &params.AuditModelDestroyed{
		ID:   model.Id,
		UUID: model.UUID,
	})
	return nil
}

// SetModelDefaults writes new default model setting values for the specified cloud/region.
func (j *JEM) SetModelDefaults(ctx context.Context, id identchecker.ACLIdentity, cloud, region string, configs map[string]interface{}) error {
	return errgo.Mask(j.DB.SetModelDefaults(ctx, mongodoc.CloudRegionDefaults{
		User:     id.Id(),
		Cloud:    cloud,
		Region:   region,
		Defaults: configs,
	}))
}

// UnsetModelDefaults resets  default model setting values for the specified cloud/region.
func (j *JEM) UnsetModelDefaults(ctx context.Context, id identchecker.ACLIdentity, cloud, region string, keys []string) error {
	return errgo.Mask(j.DB.UnsetModelDefaults(
		ctx,
		id.Id(),
		cloud,
		region,
		keys,
	))
}

// ModelDefaultsForCloud returns the default config values for the specified cloud.
func (j *JEM) ModelDefaultsForCloud(ctx context.Context, id identchecker.ACLIdentity, cloud params.Cloud) (jujuparams.ModelDefaultsResult, error) {
	result := jujuparams.ModelDefaultsResult{
		Config: make(map[string]jujuparams.ModelDefaults),
	}

	values, err := j.DB.ModelDefaults(ctx, id.Id(), string(cloud))
	if err != nil {
		return result, errgo.Mask(err)
	}
	for _, configPerRegion := range values {
		for attr, value := range configPerRegion.Defaults {
			modelDefaults := result.Config[attr]
			modelDefaults.Regions = append(modelDefaults.Regions, jujuparams.RegionDefaults{
				RegionName: configPerRegion.Region,
				Value:      value,
			})
			result.Config[attr] = modelDefaults
		}
	}

	return result, nil
}
