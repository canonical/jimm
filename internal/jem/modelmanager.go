// Copyright 2020 Canonical Ltd.

package jem

import (
	"context"

	"github.com/juju/names/v4"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/params"
)

// ValidateModelUpgrade validates if a model is allowed to perform an upgrade.
func (j *JEM) ValidateModelUpgrade(ctx context.Context, id identchecker.ACLIdentity, modelUUID string, force bool) error {
	model, err := j.DB.ModelFromUUID(ctx, modelUUID)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}

	// The model owner is currently always an admin.
	if err = auth.CheckIsUser(ctx, id, model.Path.User); err != nil {
		if err = auth.CheckACL(ctx, id, model.ACL.Admin); err != nil {
			return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
		}
	}

	conn, err := j.OpenAPI(ctx, model.Controller)
	if err != nil {
		return errgo.Notef(err, "cannot connect to controller")
	}
	defer conn.Close()

	return errgo.Mask(conn.ValidateModelUpgrade(ctx, names.NewModelTag(model.UUID), force), apiconn.IsAPIError)
}
