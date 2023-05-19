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

// ToModelAccessString maps relation to a controller access string.
func ToControllerAccessString(relation ofganames.Relation) string {
	switch relation {
	case ofganames.AdministratorRelation:
		return "superuser"
	default:
		return "login"
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
		return ofganames.NoRelation, errors.E("unknown model access")
	}
}

// ToOfferRelation returns a valid relation for the application offer.
func ToOfferRelation(accessLevel string) (ofganames.Relation, error) {
	switch accessLevel {
	case "":
		return ofganames.NoRelation, nil
	case string(jujuparams.OfferAdminAccess):
		return ofganames.AdministratorRelation, nil
	case string(jujuparams.OfferConsumeAccess):
		return ofganames.ConsumerRelation, nil
	case string(jujuparams.OfferReadAccess):
		return ofganames.ReaderRelation, nil
	default:
		return ofganames.NoRelation, errors.E("unknown application offer access")
	}
}

// JwtGenerator provides the necessary state and methods to authorize a user and generate JWT tokens.
type JwtGenerator struct {
	jimm           *JIMM
	accessMapCache map[string]string
	mt             names.ModelTag
	ct             names.ControllerTag
	user           *openfga.User
	callCount      int
}

// NewJwtGenerator returns a new JwtAuthorizer struct
func NewJwtGenerator(jimm *JIMM) JwtGenerator {
	return JwtGenerator{jimm: jimm}
}

// SetTags implements TokenGenerator
func (auth *JwtGenerator) SetTags(mt names.ModelTag, ct names.ControllerTag) {
	auth.mt = mt
	auth.ct = ct
}

// SetTags implements TokenGenerator
func (auth *JwtGenerator) GetUser() names.UserTag {
	if auth.user != nil {
		return auth.user.ResourceTag()
	}
	return names.UserTag{}
}

// MakeToken takes a login request and a map of needed permissions and returns a JWT token if the user satisfies
// all the needed permissions. A loginRequest object should be provided on the first invocation of this function
// after which point subsequent calls can provide a nil object.
// Note that this function is not thread-safe and should only be called by a single Go routine at a time.
func (auth *JwtGenerator) MakeToken(ctx context.Context, initialLogin bool, req *jujuparams.LoginRequest, permissionMap map[string]interface{}) ([]byte, error) {
	const op = errors.Op("jimm.MakeToken")

	if initialLogin {
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
		if auth.mt.Id() == "" {
			return nil, errors.E(op, "Desired Model not set")
		}
		modelAccess, authErr = auth.jimm.GetUserModelAccess(ctx, auth.user, auth.mt)
		if authErr != nil {
			return nil, authErr
		}
		auth.accessMapCache[auth.mt.String()] = modelAccess

		if auth.ct.Id() == "" {
			return nil, errors.E(op, "Desired Controller not set")
		}
		var controllerAccess string
		controllerAccess, authErr = auth.jimm.GetControllerAccess(ctx, auth.user, auth.ct)
		if authErr != nil {
			return nil, authErr
		}
		auth.accessMapCache[auth.ct.String()] = controllerAccess
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
		Controller: auth.ct.Id(),
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
				return nil, errors.E(op, fmt.Sprintf("failed to get permission assertion: expected %T, got %T", stringVal, val))
			}
			tag, err := names.ParseTag(key)
			if err != nil {
				return cachedPerms, errors.E(op, fmt.Sprintf("failed to parse tag %s", key))
			}
			relation, err := ofganames.ConvertJujuRelation(stringVal)
			if err != nil {
				return cachedPerms, errors.E(op, fmt.Sprintf("failed to parse relation %s", stringVal), err)
			}
			check, _, err := openfga.CheckRelation(ctx, user, tag, relation)
			if err != nil {
				return cachedPerms, errors.E(op, err)
			}
			if !check {
				return cachedPerms, errors.E(op, fmt.Sprintf("Missing permission for %s:%s", key, val))
			}
			cachedPerms[key] = stringVal
		}
	}
	return cachedPerms, nil
}

// GrantAuditLogAccess grants audit log access for the target user.
func (j *JIMM) GrantAuditLogAccess(ctx context.Context, user *openfga.User, targetUserTag names.UserTag) error {
	const op = errors.Op("jimm.GrantAuditLogAccess")

	access := user.GetControllerAccess(ctx, j.ResourceTag())
	if access != ofganames.AdministratorRelation {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	targetUser := &dbmodel.User{}
	targetUser.SetTag(targetUserTag)
	err := j.Database.GetUser(ctx, targetUser)
	if err != nil {
		return errors.E(op, err)
	}

	err = openfga.NewUser(targetUser, j.OpenFGAClient).SetControllerAccess(ctx, j.ResourceTag(), ofganames.AuditLogViewerRelation)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// RevokeAuditLogAccess revokes audit log access for the target user.
func (j *JIMM) RevokeAuditLogAccess(ctx context.Context, user *openfga.User, targetUserTag names.UserTag) error {
	const op = errors.Op("jimm.RevokeAuditLogAccess")

	access := user.GetControllerAccess(ctx, j.ResourceTag())
	if access != ofganames.AdministratorRelation {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	targetUser := &dbmodel.User{}
	targetUser.SetTag(targetUserTag)
	err := j.Database.GetUser(ctx, targetUser)
	if err != nil {
		return errors.E(op, err)
	}

	err = openfga.NewUser(targetUser, j.OpenFGAClient).UnsetAuditLogViewerAccess(ctx, j.ResourceTag())
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}
