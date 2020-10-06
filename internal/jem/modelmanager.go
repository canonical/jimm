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
