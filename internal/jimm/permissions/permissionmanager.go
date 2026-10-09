// Copyright 2025 Canonical.

// The permissions package provides business logic for handling user permissions.
package permissions

import (
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/offer"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// PermissionManager provides a means to manage roles within JIMM.
type PermissionManager struct {
	store        *db.Database
	authSvc      *openfga.OFGAClient
	groupFetcher offer.IdPGroupFetcher
	jimmUUID     string
	jimmTag      names.ControllerTag
}

// NewManager returns a new permission manager that provides
// permission handling and resolution of JAAS tags.
func NewManager(store *db.Database, authSvc *openfga.OFGAClient, uuid string, tag names.ControllerTag, groupFetcher offer.IdPGroupFetcher) (*PermissionManager, error) {
	if store == nil {
		return nil, errors.New("permission store cannot be nil")
	}
	if authSvc == nil {
		return nil, errors.New("permission authorisation service cannot be nil")
	}
	if groupFetcher == nil {
		return nil, errors.New("permission group fetcher cannot be nil")
	}
	return &PermissionManager{store: store, authSvc: authSvc, groupFetcher: groupFetcher, jimmUUID: uuid, jimmTag: tag}, nil
}
