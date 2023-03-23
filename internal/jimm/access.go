// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"
	"fmt"

	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimmjwx"
	"github.com/CanonicalLtd/jimm/internal/openfga"
	ofganames "github.com/CanonicalLtd/jimm/internal/openfga/names"
	jimmRPC "github.com/CanonicalLtd/jimm/internal/rpc"
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

// JwtGenerator returns an authorizer function that is used when proxying a client connection
// to a controller. The authorizer function will authorize the user and return a JWT token
// with the necessary permissions.
func (j *JIMM) JwtGenerator(ctx context.Context, m *dbmodel.Model) jimmRPC.GetTokenFunc {
	var user *openfga.User
	var accessMapCache map[string]string
	var callCount int
	var authorised bool
	// Take a login request and a map of desired permissions and return a JWT with the desired permissions.
	// Errors if any of the desired permissions cannot be satisfied.
	return func(req *jujuparams.LoginRequest, errMap map[string]interface{}) ([]byte, error) {
		if req != nil {
			// Recreate the accessMapCache to prevent leaking permissions across multiple login requests.
			accessMapCache = make(map[string]string)
			var authErr error
			user, authErr = j.Authenticator.Authenticate(ctx, req)
			if authErr != nil {
				return nil, authErr
			}
			var modelAccess string
			mt := m.ResourceTag()
			modelAccess, authErr = j.GetUserModelAccess(ctx, user, mt)
			if authErr != nil {
				return nil, authErr
			}
			accessMapCache[mt.String()] = modelAccess
			// Get the user's access to the JIMM controller, because all users have login access to controllers controlled by JIMM
			// but only JIMM admins have admin access on other controllers.
			var controllerAccess string
			controllerAccess, authErr = j.GetControllerAccess(ctx, user, user.ResourceTag())
			if authErr != nil {
				return nil, authErr
			}
			accessMapCache[m.Controller.Tag().String()] = controllerAccess
			authorised = true
		}
		if callCount >= 10 {
			return nil, errors.E("Permission check limit exceeded")
		}
		callCount++
		if !authorised {
			return nil, errors.E("Authorization missing.")
		}
		if errMap != nil {
			var err error
			accessMapCache, err = checkPermission(ctx, user, accessMapCache, errMap)
			if err != nil {
				return nil, err
			}
		}
		jwt, err := j.JWTService.NewJWT(ctx, jimmjwx.JWTParams{
			Controller: m.Controller.UUID,
			User:       user.Tag().String(),
			Access:     accessMapCache,
		})
		if err != nil {
			return nil, err
		}
		return jwt, nil
	}
}

func checkPermission(ctx context.Context, user *openfga.User, cachedPerms map[string]string, desiredPerms map[string]interface{}) (map[string]string, error) {
	for key, val := range desiredPerms {
		if _, ok := cachedPerms[key]; !ok {
			stringVal, ok := val.(string)
			if !ok {
				return nil, errors.E("Failed to get permission assertion.")
			}
			check, _, err := openfga.CheckRelation(ctx, user, key, ofganames.Relation(stringVal))
			if err != nil {
				return cachedPerms, err
			}
			if !check {
				err := errors.E(fmt.Sprintf("Missing permission for %s:%s", key, val))
				return cachedPerms, err
			}
			cachedPerms[key] = stringVal
		}
	}
	return cachedPerms, nil
}
