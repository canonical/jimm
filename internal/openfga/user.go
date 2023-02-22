// Copyright 2023 CanonicalLtd.

package openfga

import (
	"context"

	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	ofganames "github.com/CanonicalLtd/jimm/internal/openfga/names"
)

// NewUser returns a new user structure that can be used to check
// user's access rights to various resources.
func NewUser(u *dbmodel.User, client *OFGAClient) *User {
	return &User{
		User:   u,
		client: client,
	}
}

// User wraps dbmodel.User and implements methods that enable us
// to check user's access rights to various resources.
type User struct {
	*dbmodel.User
	client *OFGAClient
}

// ControllerAdministrator returns true if user has administrator access to the controller.
func (u *User) ControllerAdministrator(ctx context.Context, controller names.ControllerTag) (bool, error) {
	isAdmin, resolution, err := u.client.checkRelation(
		ctx,
		Tuple{
			Object:   ofganames.FromTag(u.Tag().(names.UserTag)),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.FromTag(controller),
		},
		true,
	)
	if err != nil {
		return false, errors.E(err)
	}
	if isAdmin {
		zapctx.Info(
			ctx,
			"user is controller administrator",
			zap.String("user", u.Tag().String()),
			zap.String("controller", controller.String()),
			zap.Any("resolution", resolution),
		)
	}
	return isAdmin, nil
}
