// Copyright 2025 Canonical.

package jujuapi

import (
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/pubsub"
)

// JIMM defines a comprehensive interface for all sort of operations with our application logic.
type JIMM interface {
	RoleManager() jimm.RoleManager
	GroupManager() jimm.GroupManager
	IdentityManager() jimm.IdentityManager
	LoginManager() jimm.LoginManager
	PermissionManager() jimm.PermissionManager
	AuditLogManager() jimm.AuditLogManager
	ServiceAccountManager() jimm.ServiceAccountManager
	JujuManager() jimm.JujuManager

	ResourceTag() names.ControllerTag
	PubSubHub() *pubsub.Hub
}
