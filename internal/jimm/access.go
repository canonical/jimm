// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"
	"fmt"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimmjwx"
	"github.com/CanonicalLtd/jimm/internal/openfga"
	ofganames "github.com/CanonicalLtd/jimm/internal/openfga/names"
)

// ToOfferAccessString maps relation to an application offer access string.
func ToOfferAccessString(relation ofganames.Relation) string {
	switch relation {
	case ofganames.AdministratorRelation:
		return string(jujuparams.OfferAdminAccess)
	case ofganames.ConsumerRelation:
		return string(jujuparams.OfferConsumeAccess)
	case ofganames.ReaderRelation:
		return string(jujuparams.OfferReadAccess)
	default:
		return ""
	}
}

// ToCloudAccessString maps relation to a cloud access string.
func ToCloudAccessString(relation ofganames.Relation) string {
	switch relation {
	case ofganames.AdministratorRelation:
		return "admin"
	case ofganames.CanAddModelRelation:
		return "add-model"
	default:
		return ""
	}
}

// ToModelAccessString maps relation to a model access string.
func ToModelAccessString(relation ofganames.Relation) string {
	switch relation {
	case ofganames.AdministratorRelation:
		return "admin"
	case ofganames.WriterRelation:
		return "write"
	case ofganames.ReaderRelation:
		return "read"
	default:
		return ""
	}
}

// ToCloudRelation returns a valid relation for the cloud. Access level
// string can be either "admin", in which case the administrator relation
// is returned, or "add-model", in which case the can_addmodel relation is
// returned.
func ToCloudRelation(accessLevel string) (ofganames.Relation, error) {
	switch accessLevel {
	case "admin":
		return ofganames.AdministratorRelation, nil
	case "add-model":
		return ofganames.CanAddModelRelation, nil
	default:
		return ofganames.NoRelation, errors.E("unknown cloud access")
	}
}

// ToModelRelation returns a valid relation for the model.
func ToModelRelation(accessLevel string) (ofganames.Relation, error) {
	switch accessLevel {
	case "admin":
		return ofganames.AdministratorRelation, nil
	case "write":
		return ofganames.WriterRelation, nil
	case "read":
		return ofganames.ReaderRelation, nil
	default:
		return ofganames.NoRelation, errors.E("unknown cloud access")
	}
}

// JwtGenerator provides the necessary state and methods to authorize a user and generate JWT tokens.
type JwtGenerator struct {
	jimm           *JIMM
	accessMapCache map[string]string
	model          *dbmodel.Model
	user           *openfga.User
	callCount      int
}

// NewJwtAuthorizer returns a new JwtAuthorizer struct
func NewJwtAuthorizer(jimm *JIMM, model *dbmodel.Model) JwtGenerator {
	return JwtGenerator{jimm: jimm, model: model}
}

// MakeToken takes a login request and a map of needed permissions and returns a JWT token if the user satisfies
// all the needed permissions. A loginRequest object should be provided on the first invocation of this function
// after which point subsequent calls can provide a nil object.
// Note that this function is not thread-safe and should only be called by a single Go routine at a time.
func (auth *JwtGenerator) MakeToken(ctx context.Context, req *jujuparams.LoginRequest, permissionMap map[string]interface{}) ([]byte, error) {
	const op = errors.Op("jimm.MakeToken")

	if auth.user == nil || req != nil {
		if req == nil {
			return nil, errors.E(op, "Missing login request.")
		}
		// Recreate the accessMapCache to prevent leaking permissions across multiple login requests.
		auth.accessMapCache = make(map[string]string)
		var authErr error
		auth.user, authErr = auth.jimm.Authenticator.Authenticate(ctx, req)
		if authErr != nil {
			return nil, authErr
		}
		var modelAccess string
		mt := auth.model.ResourceTag()
		modelAccess, authErr = auth.jimm.GetUserModelAccess(ctx, auth.user, mt)
		if authErr != nil {
			return nil, authErr
		}
		auth.accessMapCache[names.ModelTagKind] = modelAccess
		// Get the user's access to the JIMM controller, because all users have login access to controllers controlled by JIMM
		// but only JIMM admins have admin access on other controllers.
		var controllerAccess string
		controllerAccess, authErr = auth.jimm.GetControllerAccess(ctx, auth.user, auth.user.ResourceTag())
		if authErr != nil {
			return nil, authErr
		}
		auth.accessMapCache[names.ControllerTagKind] = controllerAccess
	}
	if auth.callCount >= 10 {
		return nil, errors.E(op, "Permission check limit exceeded")
	}
	auth.callCount++
	if auth.user == nil {
		return nil, errors.E(op, "User authorization missing.")
	}
	if permissionMap != nil {
		var err error
		auth.accessMapCache, err = checkPermission(ctx, auth.user, auth.accessMapCache, permissionMap)
		if err != nil {
			return nil, err
		}
	}
	jwt, err := auth.jimm.JWTService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: auth.model.Controller.UUID,
		User:       auth.user.Tag().String(),
		Access:     auth.accessMapCache,
	})
	if err != nil {
		return nil, err
	}
	return jwt, nil
}

// checkPermission loops over the desired permissions in desiredPerms and adds these permissions
// to cachedPerms if they exist. If the user does not have any of the desired permissions then an
// error is returned.
// Note that cachedPerms map is modified and returned.
func checkPermission(ctx context.Context, user *openfga.User, cachedPerms map[string]string, desiredPerms map[string]interface{}) (map[string]string, error) {
	const op = errors.Op("jimm.checkPermission")
	for key, val := range desiredPerms {
		if _, ok := cachedPerms[key]; !ok {
			stringVal, ok := val.(string)
			if !ok {
				return nil, errors.E(op, "Failed to get permission assertion.")
			}
			tag, err := names.ParseTag(key)
			if err != nil {
				err := errors.E(op, fmt.Sprintf("Failed to parse tag %s", key))
				return cachedPerms, err
			}
			check, _, err := openfga.CheckRelation(ctx, user, tag, ofganames.Relation(stringVal))
			if err != nil {
				return cachedPerms, err
			}
			if !check {
				err := errors.E(op, fmt.Sprintf("Missing permission for %s:%s", key, val))
				return cachedPerms, err
			}
			cachedPerms[key] = stringVal
		}
	}
	return cachedPerms, nil
}
