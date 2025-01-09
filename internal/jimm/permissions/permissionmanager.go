// Copyright 2025 Canonical.

// The permissions package provides business logic for handling user permissions.
package permissions

import (
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// permissionManager provides a means to manage roles within JIMM.
type permissionManager struct {
	store    *db.Database
	authSvc  *openfga.OFGAClient
	jimmUUID string
	jimmTag  names.ControllerTag
}

// NewManager returns a new permission manager that provides
// permission handling and resolution of JAAS tags.
func NewManager(store *db.Database, authSvc *openfga.OFGAClient, uuid string, tag names.ControllerTag) (*permissionManager, error) {
	if store == nil {
		return nil, errors.E("permission store cannot be nil")
	}
	if authSvc == nil {
		return nil, errors.E("permission authorisation service cannot be nil")
	}
	return &permissionManager{store, authSvc, uuid, tag}, nil
}
