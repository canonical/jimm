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

// dial dials the controller and model specified by the given Controller
// and ModelTag. If no Dialer has been configured then an error with a
// code of CodeConnectionFailed will be returned.
func (j *JujuManager) dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag, user *openfga.User) (API, error) {
	if j == nil || j.Dialer == nil {
		return nil, errors.Codef(errors.CodeConnectionFailed, "no dialer configured")
	}

	return j.Dialer.Dial(ctx, ctl, modelTag, user)
}

// ResourceTag returns JIMM's controller tag stating its UUID.
func (j *JujuManager) ResourceTag() names.ControllerTag {
	return j.resourceTag
}
