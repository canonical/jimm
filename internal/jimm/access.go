// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"
	"fmt"
	"sync"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimmjwx"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
)

// ToOfferAccessString maps relation to an application offer access string.
func ToOfferAccessString(relation openfga.Relation) string {
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
func ToCloudAccessString(relation openfga.Relation) string {
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
func ToModelAccessString(relation openfga.Relation) string {
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
func ToControllerAccessString(relation openfga.Relation) string {
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
func ToCloudRelation(accessLevel string) (openfga.Relation, error) {
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
func ToModelRelation(accessLevel string) (openfga.Relation, error) {
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
func ToOfferRelation(accessLevel string) (openfga.Relation, error) {
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

// JWTGeneratorDatabase specifies the database interface used by the
// JWT generator.
type JWTGeneratorDatabase interface {
	GetController(ctx context.Context, controller *dbmodel.Controller) error
}

// JWTGeneratorAccessChecker specifies the access checker used by the JWT
// generator to obtain user's access rights to various entities.
type JWTGeneratorAccessChecker interface {
	GetUserModelAccess(context.Context, *openfga.User, names.ModelTag) (string, error)
	GetUserControllerAccess(context.Context, *openfga.User, names.ControllerTag) (string, error)
	GetUserCloudAccess(context.Context, *openfga.User, names.CloudTag) (string, error)
	CheckPermission(context.Context, *openfga.User, map[string]string, map[string]interface{}) (map[string]string, error)
}

// JWTService specifies the service JWT generator uses to generate JWTs.
type JWTService interface {
	NewJWT(context.Context, jimmjwx.JWTParams) ([]byte, error)
}

// JWTGenerator provides the necessary state and methods to authorize a user and generate JWT tokens.
type JWTGenerator struct {
	authenticator Authenticator
	database      JWTGeneratorDatabase
	accessChecker JWTGeneratorAccessChecker
	jwtService    JWTService

	mu             sync.Mutex
	accessMapCache map[string]string
	mt             names.ModelTag
	ct             names.ControllerTag
	user           *openfga.User
	callCount      int
}

// NewJWTGenerator returns a new JwtAuthorizer struct
func NewJWTGenerator(authenticator Authenticator, database JWTGeneratorDatabase, accessChecker JWTGeneratorAccessChecker, jwtService JWTService) JWTGenerator {
	return JWTGenerator{
		authenticator: authenticator,
		database:      database,
		accessChecker: accessChecker,
		jwtService:    jwtService,
	}
}

// SetTags implements TokenGenerator
func (auth *JWTGenerator) SetTags(mt names.ModelTag, ct names.ControllerTag) {
	auth.mt = mt
	auth.ct = ct
}

// SetTags implements TokenGenerator
func (auth *JWTGenerator) GetUser() names.UserTag {
	if auth.user != nil {
		return auth.user.ResourceTag()
	}
	return names.UserTag{}
}

// MakeLoginToken authorizes the user based on the provided login requests and returns
// a JWT containing claims about user's access to the controller, model (if applicable)
// and all clouds that the controller knows about.
func (auth *JWTGenerator) MakeLoginToken(ctx context.Context, req *jujuparams.LoginRequest) ([]byte, error) {
	const op = errors.Op("jimm.MakeLoginToken")

	auth.mu.Lock()
	defer auth.mu.Unlock()

	if req == nil {
		return nil, errors.E(op, "missing login request.")
	}
	// Recreate the accessMapCache to prevent leaking permissions across multiple login requests.
	auth.accessMapCache = make(map[string]string)
	var authErr error
	auth.user, authErr = auth.authenticator.Authenticate(ctx, req)
	if authErr != nil {
		zapctx.Error(ctx, "authentication failed", zap.Error(authErr))
		return nil, authErr
	}
	var modelAccess string
	if auth.mt.Id() == "" {
		return nil, errors.E(op, "model not set")
	}
	modelAccess, authErr = auth.accessChecker.GetUserModelAccess(ctx, auth.user, auth.mt)
	if authErr != nil {
		zapctx.Error(ctx, "model access check failed", zap.Error(authErr))
		return nil, authErr
	}
	auth.accessMapCache[auth.mt.String()] = modelAccess

	if auth.ct.Id() == "" {
		return nil, errors.E(op, "controller not set")
	}
	var controllerAccess string
	controllerAccess, authErr = auth.accessChecker.GetUserControllerAccess(ctx, auth.user, auth.ct)
	if authErr != nil {
		return nil, authErr
	}
	auth.accessMapCache[auth.ct.String()] = controllerAccess

	var ctl dbmodel.Controller
	ctl.SetTag(auth.ct)
	err := auth.database.GetController(ctx, &ctl)
	if err != nil {
		zapctx.Error(ctx, "failed to fetch controller", zap.Error(err))
		return nil, errors.E(op, "failed to fetch controller", err)
	}
	clouds := make(map[names.CloudTag]bool)
	for _, cloudRegion := range ctl.CloudRegions {
		clouds[cloudRegion.CloudRegion.Cloud.ResourceTag()] = true
	}
	for cloudTag, _ := range clouds {
		accessLevel, err := auth.accessChecker.GetUserCloudAccess(ctx, auth.user, cloudTag)
		if err != nil {
			zapctx.Error(ctx, "cloud access check failed", zap.Error(err))
			return nil, errors.E(op, "failed to check user's cloud access", err)
		}
		auth.accessMapCache[cloudTag.String()] = accessLevel
	}

	jwt, err := auth.jwtService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: auth.ct.Id(),
		User:       auth.user.Tag().String(),
		Access:     auth.accessMapCache,
	})
	if err != nil {
		return nil, err
	}
	return jwt, nil
}

// MakeToken assumes MakeLoginToken has already been called and checks the permissions
// specified in the permissionMap. If the logged in user has all those permissions
// a JWT will be returned with assertions confirming all those permissions.
func (auth *JWTGenerator) MakeToken(ctx context.Context, permissionMap map[string]interface{}) ([]byte, error) {
	const op = errors.Op("jimm.MakeToken")

	auth.mu.Lock()
	defer auth.mu.Unlock()

	if auth.callCount >= 10 {
		return nil, errors.E(op, "Permission check limit exceeded")
	}
	auth.callCount++
	if auth.user == nil {
		return nil, errors.E(op, "User authorization missing.")
	}
	if permissionMap != nil {
		var err error
		auth.accessMapCache, err = auth.accessChecker.CheckPermission(ctx, auth.user, auth.accessMapCache, permissionMap)
		if err != nil {
			return nil, err
		}
	}
	jwt, err := auth.jwtService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: auth.ct.Id(),
		User:       auth.user.Tag().String(),
		Access:     auth.accessMapCache,
	})
	if err != nil {
		return nil, err
	}
	return jwt, nil
}

// CheckPermission loops over the desired permissions in desiredPerms and adds these permissions
// to cachedPerms if they exist. If the user does not have any of the desired permissions then an
// error is returned.
// Note that cachedPerms map is modified and returned.
func (j *JIMM) CheckPermission(ctx context.Context, user *openfga.User, cachedPerms map[string]string, desiredPerms map[string]interface{}) (map[string]string, error) {
	const op = errors.Op("jimm.CheckPermission")
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
			check, err := openfga.CheckRelation(ctx, user, tag, relation)
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
