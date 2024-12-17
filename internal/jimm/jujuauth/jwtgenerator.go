// Copyright 2024 Canonical.

// Package jujuauth generates JWT tokens to
// authenticate and authorize messages to Juju controllers.
// This package is more specialised than a generic
// JWT token generator as it crafts Juju specific
// permissions that are added as claims to the JWT
// and therefore exists in JIMM's business logic layer.
package jujuauth

import (
	"context"
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
	GetUserControllerAccess(context.Context, *openfga.User, names.ControllerTag) (string, error)
	GetUserCloudAccess(context.Context, *openfga.User, names.CloudTag) (string, error)
	CheckPermission(context.Context, *openfga.User, map[string]string, map[string]interface{}) (map[string]string, error)
}

// JWTService specifies the service JWT generator uses to generate JWTs.
type JWTService interface {
	NewJWT(context.Context, jimmjwx.JWTParams) ([]byte, error)
}

// TokenGenerator provides the necessary state and methods to authorize a user and generate JWT tokens.
type TokenGenerator struct {
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

// New returns a new JWTGenerator.
func New(database GeneratorDatabase, accessChecker GeneratorAccessChecker, jwtService JWTService) TokenGenerator {
	return TokenGenerator{
		database:      database,
		accessChecker: accessChecker,
		jwtService:    jwtService,
	}
}

// SetTags implements TokenGenerator.
func (auth *TokenGenerator) SetTags(mt names.ModelTag, ct names.ControllerTag) {
	auth.mt = mt
	auth.ct = ct
}

// SetTags implements TokenGenerator.
func (auth *TokenGenerator) GetUser() names.UserTag {
	if auth.user != nil {
		return auth.user.ResourceTag()
	}
	return names.UserTag{}
}

// MakeLoginToken authorizes the user based on the provided login requests and returns
// a JWT containing claims about user's access to the controller, model (if applicable)
// and all clouds that the controller knows about.
func (auth *TokenGenerator) MakeLoginToken(ctx context.Context, user *openfga.User) ([]byte, error) {
	const op = errors.Op("jimm.MakeLoginToken")

	auth.mu.Lock()
	defer auth.mu.Unlock()

	if user == nil {
		return nil, errors.E(op, "user not specified")
	}
	auth.user = user

	// Recreate the accessMapCache to prevent leaking permissions across multiple login requests.
	auth.accessMapCache = make(map[string]string)
	var authErr error

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
	for cloudTag := range clouds {
		accessLevel, err := auth.accessChecker.GetUserCloudAccess(ctx, auth.user, cloudTag)
		if err != nil {
			zapctx.Error(ctx, "cloud access check failed", zap.Error(err))
			return nil, errors.E(op, "failed to check user's cloud access", err)
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
func (auth *TokenGenerator) MakeToken(ctx context.Context, permissionMap map[string]interface{}) ([]byte, error) {
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
