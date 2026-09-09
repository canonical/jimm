// Copyright 2025 Canonical.

// Package jujuauth generates JWT tokens to
// authenticate and authorize messages to Juju controllers.
// This package is more specialised than a generic
// JWT token generator as it crafts Juju specific
// permissions that are added as claims to the JWT
// and therefore exists in JIMM's business logic layer.
package jujuauth

import (
	"context"
	"fmt"
	"sync"

	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// GeneratorDatabase specifies the database interface used by the
// JWT generator.
type GeneratorDatabase interface {
	GetController(ctx context.Context, controller *dbmodel.Controller) error
}

// GeneratorAccessChecker specifies the access checker used by the JWT
// generator to obtain user's access rights to various entities.
type GeneratorAccessChecker interface {
	GetUserModelAccess(context.Context, *openfga.User, names.ModelTag) (string, error)
	GetUserApplicationOfferAccess(context.Context, *openfga.User, names.ApplicationOfferTag) (string, error)
	GetUserControllerAccess(context.Context, *openfga.User, names.ControllerTag) (string, error)
	GetUserCloudAccess(context.Context, *openfga.User, names.CloudTag) (string, error)
	CheckPermission(context.Context, *openfga.User, map[string]string, map[string]any) (map[string]string, error)
}

// JWTService specifies the service JWT generator uses to generate JWTs.
type JWTService interface {
	NewJWT(context.Context, jimmjwx.JWTParams) ([]byte, error)
}

// LoginTokenGenerator provides the necessary state and
// methods to authorize a user and generate JWT tokens
// appropriate when logging in for RPC based communication..
type LoginTokenGenerator struct {
	database      GeneratorDatabase
	accessChecker GeneratorAccessChecker
	jwtService    JWTService

	mu             sync.Mutex
	accessMapCache map[string]string
	mt             names.ModelTag
	ct             names.ControllerTag
	user           *openfga.User
	callCount      int
}

// newLoginTokenGenerator returns a new LoginTokenGenerator.
func newLoginTokenGenerator(database GeneratorDatabase, accessChecker GeneratorAccessChecker, jwtService JWTService) LoginTokenGenerator {
	return LoginTokenGenerator{
		database:      database,
		accessChecker: accessChecker,
		jwtService:    jwtService,
	}
}

// SetTags implements TokenGenerator.
// It sets the model and controller we are operating on
// which will be used to set the JWT audience and claims.
func (auth *LoginTokenGenerator) SetTags(mt names.ModelTag, ct names.ControllerTag) {
	auth.mt = mt
	auth.ct = ct
}

// SetTags implements TokenGenerator.
func (auth *LoginTokenGenerator) GetUser() names.UserTag {
	if auth.user != nil {
		return auth.user.ResourceTag()
	}
	return names.UserTag{}
}

// makeSuperuserToken makes a token declaring the user is a controller
// superuser and model admin for the supplied resource tags, without
// actually checking if that's the case.
func (auth *LoginTokenGenerator) makeSuperuserToken(ctx context.Context, user *openfga.User, resourceTags []names.Tag) ([]byte, error) {

	if user == nil {
		return nil, errors.New("user not specified")
	}

	accessMap := make(map[string]string)
	accessMap[auth.ct.String()] = "superuser"
	for _, target := range resourceTags {
		if model, ok := target.(names.ModelTag); ok && model.Id() != "" {
			accessMap[model.String()] = "admin"
		}
	}

	return auth.jwtService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: auth.ct.Id(),
		User:       user.Tag().String(),
		Access:     accessMap,
	})
}

// resolveTargetAccess resolves the caller's access level for a single
// target tag. Unrecognised tag kinds resolve to an empty access level.
func resolveTargetAccess(ctx context.Context, user *openfga.User, target names.Tag, accessChecker GeneratorAccessChecker) (string, error) {
	switch tag := target.(type) {
	case names.ModelTag:
		access, err := accessChecker.GetUserModelAccess(ctx, user, tag)
		if err != nil {
			zapctx.Error(ctx, "model access check failed", zap.Error(err))
			return "", err
		}
		return access, nil
	case names.ApplicationOfferTag:
		access, err := accessChecker.GetUserApplicationOfferAccess(ctx, user, tag)
		if err != nil {
			zapctx.Error(ctx, "application offer access check failed", zap.Error(err))
			return "", err
		}
		return access, nil
	default:
		return "", nil
	}
}

// buildAccessMap resolves the caller's OpenFGA permissions for the given
// controller and resource tags, returning a JWT access-claim map.
func buildAccessMap(
	ctx context.Context,
	user *openfga.User,
	resourceTags []names.Tag,
	ct names.ControllerTag,
	ctl dbmodel.Controller,
	accessChecker GeneratorAccessChecker,
) (map[string]string, error) {
	accessMap := make(map[string]string)

	targetAccess := make(map[names.Tag]string, len(resourceTags))
	hasRealAccess := false
	for _, target := range resourceTags {
		access, err := resolveTargetAccess(ctx, user, target, accessChecker)
		if err != nil {
			return nil, err
		}
		targetAccess[target] = access
		if access != "" {
			hasRealAccess = true
		}
	}

	// Juju rejects logins with empty access claims. Drop empty claims
	// when the user holds real access to at least one tag. Otherwise,
	// keep one so Juju itself denies the login.
	for target, access := range targetAccess {
		if access == "" && hasRealAccess {
			continue
		}
		accessMap[target.String()] = access
	}

	controllerAccess, err := accessChecker.GetUserControllerAccess(ctx, user, ct)
	if err != nil {
		return nil, err
	}
	accessMap[ct.String()] = controllerAccess

	clouds := make(map[names.CloudTag]bool)
	for _, cloudRegion := range ctl.CloudRegions {
		clouds[cloudRegion.CloudRegion.Cloud.ResourceTag()] = true
	}
	for cloudTag := range clouds {
		accessLevel, err := accessChecker.GetUserCloudAccess(ctx, user, cloudTag)
		if err != nil {
			return nil, fmt.Errorf("failed to check user's cloud access: %w", err)
		}
		if accessLevel == "" {
			continue
		}
		accessMap[cloudTag.String()] = accessLevel
	}

	return accessMap, nil
}

// MakeLoginToken authorizes the user and returns a JWT containing claims about user's access
// to the controller, model and all clouds that the controller knows about.
func (auth *LoginTokenGenerator) MakeLoginToken(ctx context.Context, user *openfga.User) ([]byte, error) {

	auth.mu.Lock()
	defer auth.mu.Unlock()

	if user == nil {
		return nil, errors.New("user not specified")
	}
	auth.user = user

	// Recreate the accessMapCache to prevent leaking permissions across multiple login requests.
	auth.accessMapCache = make(map[string]string)
	var authErr error

	var modelAccess string
	if auth.mt.Id() == "" {
		return nil, errors.New("model not set")
	}
	modelAccess, authErr = auth.accessChecker.GetUserModelAccess(ctx, auth.user, auth.mt)
	if authErr != nil {
		zapctx.Error(ctx, "model access check failed", zap.Error(authErr))
		return nil, authErr
	}
	auth.accessMapCache[auth.mt.String()] = modelAccess

	if auth.ct.Id() == "" {
		return nil, errors.New("controller not set")
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
		return nil, fmt.Errorf("failed to fetch controller: %w", err)
	}
	clouds := make(map[names.CloudTag]bool)
	for _, cloudRegion := range ctl.CloudRegions {
		clouds[cloudRegion.CloudRegion.Cloud.ResourceTag()] = true
	}
	for cloudTag := range clouds {
		accessLevel, err := auth.accessChecker.GetUserCloudAccess(ctx, auth.user, cloudTag)
		if err != nil {
			return nil, fmt.Errorf("failed to check user's cloud access: %w", err)
		}
		auth.accessMapCache[cloudTag.String()] = accessLevel
	}

	return auth.jwtService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: auth.ct.Id(),
		User:       auth.user.Tag().String(),
		Access:     auth.accessMapCache,
	})
}

// MakeToken assumes MakeLoginToken has already been called and checks the permissions
// specified in the permissionMap. If the logged in user has all those permissions
// a JWT will be returned with assertions confirming all those permissions.
func (auth *LoginTokenGenerator) MakeToken(ctx context.Context, permissionMap map[string]any) ([]byte, error) {

	auth.mu.Lock()
	defer auth.mu.Unlock()

	if auth.callCount >= 10 {
		return nil, errors.New("Permission check limit exceeded")
	}
	auth.callCount++
	if auth.user == nil {
		return nil, errors.New("User authorization missing.")
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
