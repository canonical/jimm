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
	Database               *db.Database
	OpenFGAClient          *openfga.OFGAClient
	CredentialStore        credentials.CredentialStore
	permissionManager      PermissionChecker
	resourceTag            names.ControllerTag
	ReservedCloudNames     []string
	Dialer                 Dialer
	CrossModelQueryTimeout time.Duration
}

// NewJujuManager returns a new JIMM struct that manages business logic associated
// with Juju resources.
func NewJujuManager(
	store *db.Database,
	authSvc *openfga.OFGAClient,
	credentialStore credentials.CredentialStore,
	permissionManager PermissionChecker,
	resourceTag names.ControllerTag,
	reservedCloudNames []string,
	dialer Dialer,
	crossModelQueryTimeout time.Duration,
) (*JujuManager, error) {
	if store == nil {
		return nil, errors.E("role store cannot be nil")
	}
	if authSvc == nil {
		return nil, errors.E("role authorisation service cannot be nil")
	}
	if credentialStore == nil {
		return nil, errors.E("credential store cannot be nil")
	}
	if permissionManager == nil {
		return nil, errors.E("permission manager cannot be nil")
	}
	if resourceTag.Id() == "" {
		return nil, errors.E("invalid jimm controller tag")
	}
	if crossModelQueryTimeout <= 0 {
		return nil, errors.E("cross model query timeout must be greater than 0")
	}
	return &JujuManager{
		Database:               store,
		OpenFGAClient:          authSvc,
		CredentialStore:        credentialStore,
		permissionManager:      permissionManager,
		resourceTag:            resourceTag,
		ReservedCloudNames:     reservedCloudNames,
		Dialer:                 dialer,
		CrossModelQueryTimeout: crossModelQueryTimeout,
	}, nil
}

type permission struct {
	resource string
	relation string
}

// dial dials the controller and model specified by the given Controller
// and ModelTag. If no Dialer has been configured then an error with a
// code of CodeConnectionFailed will be returned.
func (j *JujuManager) dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag, user *openfga.User, permissons ...permission) (API, error) {
	if j == nil || j.Dialer == nil {
		return nil, errors.E(errors.CodeConnectionFailed, "no dialer configured")
	}
	var permissionMap map[string]string
	if len(permissons) > 0 {
		permissionMap = make(map[string]string, len(permissons))
		for _, p := range permissons {
			permissionMap[p.resource] = p.relation
		}
	}

	return j.Dialer.Dial(ctx, ctl, modelTag, user, permissionMap)
}

// ResourceTag returns JIMM's controller tag stating its UUID.
func (j *JujuManager) ResourceTag() names.ControllerTag {
	return j.resourceTag
}
