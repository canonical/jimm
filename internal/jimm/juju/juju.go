// Copyright 2025 Canonical.

package juju

import (
	"context"
	"time"

	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/credentials"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// JujuManager handles all business logic with Juju resources.
type JujuManager struct {
	Database                *db.Database
	OpenFGAClient           *openfga.OFGAClient
	CredentialStore         credentials.CredentialStore
	permissionManager       PermissionManager
	resourceTag             names.ControllerTag
	ReservedCloudNames      []string
	Dialer                  Dialer
	crossModelQueryTimeout  time.Duration
	migrationTokenGenerator MigrationTokenGenerator
	GitHubClient            GitHubClient
	releaseCache            cacheGithubResponse
}

// NewJujuManager returns a new JIMM struct that manages business logic associated
// with Juju resources.
func NewJujuManager(
	store *db.Database,
	authSvc *openfga.OFGAClient,
	credentialStore credentials.CredentialStore,
	permissionManager PermissionManager,
	resourceTag names.ControllerTag,
	reservedCloudNames []string,
	dialer Dialer,
	crossModelQueryTimeout time.Duration,
	migrationTokenGenerator MigrationTokenGenerator,
) (*JujuManager, error) {
	if store == nil {
		return nil, errors.New("role store cannot be nil")
	}
	if authSvc == nil {
		return nil, errors.New("role authorisation service cannot be nil")
	}
	if credentialStore == nil {
		return nil, errors.New("credential store cannot be nil")
	}
	if permissionManager == nil {
		return nil, errors.New("permission manager cannot be nil")
	}
	if resourceTag.Id() == "" {
		return nil, errors.New("invalid jimm controller tag")
	}
	if crossModelQueryTimeout <= 0 {
		return nil, errors.New("cross model query timeout must be greater than 0")
	}
	if migrationTokenGenerator == nil {
		return nil, errors.New("migration token generator cannot be nil")
	}
	return &JujuManager{
		Database:                store,
		OpenFGAClient:           authSvc,
		CredentialStore:         credentialStore,
		permissionManager:       permissionManager,
		resourceTag:             resourceTag,
		ReservedCloudNames:      reservedCloudNames,
		Dialer:                  dialer,
		crossModelQueryTimeout:  crossModelQueryTimeout,
		migrationTokenGenerator: migrationTokenGenerator,
	}, nil
}

// dialControllerAsUser dials the controller on behalf of the given user
// with the caller's real permissions.
func (j *JujuManager) dialControllerAsUser(ctx context.Context, user *openfga.User, ctl *dbmodel.Controller, resourceTags ...names.Tag) (API, error) {
	if j == nil || j.Dialer == nil {
		return nil, errors.Codef(errors.CodeConnectionFailed, "no dialer configured")
	}

	return j.Dialer.DialControllerAsUser(ctx, user, ctl, resourceTags...)
}

// dialControllerAsSuperuser dials the controller on behalf of the given
// user with forced superuser permissions.
func (j *JujuManager) dialControllerAsSuperuser(ctx context.Context, user *openfga.User, ctl *dbmodel.Controller, resourceTags ...names.Tag) (API, error) {
	if j == nil || j.Dialer == nil {
		return nil, errors.Codef(errors.CodeConnectionFailed, "no dialer configured")
	}

	return j.Dialer.DialControllerAsSuperuser(ctx, user, ctl, resourceTags...)
}

// dialModelAsService dials the model using JIMM's own service identity.
// Use this for internal housekeeping that has no associated user.
func (j *JujuManager) dialModelAsService(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag) (API, error) {
	if j == nil || j.Dialer == nil {
		return nil, errors.Codef(errors.CodeConnectionFailed, "no dialer configured")
	}

	return j.Dialer.DialModelAsService(ctx, ctl, modelTag)
}

// dialControllerAsService dials the controller using JIMM's own service
// identity. Use this for internal housekeeping that has no associated
// user.
func (j *JujuManager) dialControllerAsService(ctx context.Context, ctl *dbmodel.Controller) (API, error) {
	if j == nil || j.Dialer == nil {
		return nil, errors.Codef(errors.CodeConnectionFailed, "no dialer configured")
	}

	return j.Dialer.DialControllerAsService(ctx, ctl)
}

// ResourceTag returns JIMM's controller tag stating its UUID.
func (j *JujuManager) ResourceTag() names.ControllerTag {
	return j.resourceTag
}
