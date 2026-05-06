// Copyright 2026 Canonical.

package jujuauth

import (
	"context"

	"github.com/juju/juju/core/permission"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
	"github.com/canonical/jimm/v3/internal/openfga"
)

func controllerSuperuserAccess(controllerUUID string, modelTag names.ModelTag) map[string]string {
	access := map[string]string{
		names.NewControllerTag(controllerUUID).String(): string(permission.SuperuserAccess),
	}
	if modelTag.Id() != "" {
		access[modelTag.String()] = string(jujuparams.ModelAdminAccess)
	}
	return access
}

// NewControllerSuperuserToken generates a JWT for controller HTTP and websocket
// requests that must run with controller-superuser access. When a model tag is
// provided, the token also carries model-admin access for that model.
func (f *Factory) NewControllerSuperuserToken(ctx context.Context, controllerUUID string, modelTag names.ModelTag, user *openfga.User) ([]byte, error) {
	if f == nil {
		return nil, errors.New("auth factory not specified")
	}
	if f.jwtService == nil {
		return nil, errors.New("jwt service not specified")
	}
	if controllerUUID == "" {
		return nil, errors.New("missing controller uuid")
	}
	if user == nil {
		return nil, errors.New("user not specified")
	}

	return f.jwtService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: controllerUUID,
		User:       user.ResourceTag().String(),
		Access:     controllerSuperuserAccess(controllerUUID, modelTag),
	})
}
