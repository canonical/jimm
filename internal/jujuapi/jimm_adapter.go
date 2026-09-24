// Copyright 2025 Canonical.

package jujuapi

import (
	"github.com/juju/names/v6"

	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/pubsub"
)

// JIMMAdapter adapts a concrete *jimm.JIMM to the jujuapi.JIMM interface.
type JIMMAdapter struct {
	j *jimm.JIMM
}

// NewJIMMAdapter returns a jujuapi.JIMM adapter for a concrete *jimm.JIMM.
func NewJIMMAdapter(j *jimm.JIMM) JIMM {
	return &JIMMAdapter{j: j}
}

// RoleManager returns the role manager from the underlying JIMM instance.
func (j *JIMMAdapter) RoleManager() RoleManager {
	return j.j.RoleManager
}

// GroupManager returns the group manager from the underlying JIMM instance.
func (j *JIMMAdapter) GroupManager() GroupManager {
	return j.j.GroupManager
}

// IdentityManager returns the identity manager from the underlying JIMM instance.
func (j *JIMMAdapter) IdentityManager() IdentityManager {
	return j.j.IdentityManager
}

// LoginManager returns the login manager from the underlying JIMM instance.
func (j *JIMMAdapter) LoginManager() LoginManager {
	return j.j.LoginManager
}

// PermissionManager returns the permission manager from the underlying JIMM instance.
func (j *JIMMAdapter) PermissionManager() PermissionManager {
	return j.j.PermissionManager
}

// AuditLogManager returns the audit log manager from the underlying JIMM instance.
func (j *JIMMAdapter) AuditLogManager() AuditLogManager {
	return j.j.AuditLogManager
}

// JujuManager returns the Juju manager from the underlying JIMM instance.
func (j *JIMMAdapter) JujuManager() JujuManager {
	return j.j.JujuManager
}

// ConfigManager returns the config manager from the underlying JIMM instance.
func (j *JIMMAdapter) ConfigManager() ConfigManager {
	return j.j.ConfigManager
}

// BootstrapManager returns the bootstrap manager from the underlying JIMM instance.
func (j *JIMMAdapter) BootstrapManager() BootstrapManager {
	return j.j.BootstrapManager
}

// ControllerProfileManager returns the controller profile manager from the underlying JIMM instance.
func (j *JIMMAdapter) ControllerProfileManager() ControllerProfileManager {
	return j.j.ControllerProfileManager
}

// UpgradeManager returns the upgrade manager from the underlying JIMM instance.
func (j *JIMMAdapter) UpgradeManager() UpgradeManager {
	return j.j.UpgradeManager
}

// JobManager returns the job manager from the underlying JIMM instance.
func (j *JIMMAdapter) JobManager() JobManager {
	return j.j.JobManager
}

// ResourceTag returns JIMM's controller tag.
func (j *JIMMAdapter) ResourceTag() names.ControllerTag {
	return j.j.ResourceTag()
}

// PubSubHub returns the pub/sub hub from the underlying JIMM instance.
func (j *JIMMAdapter) PubSubHub() *pubsub.Hub {
	return j.j.Pubsub
}
