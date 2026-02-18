// Copyright 2025 Canonical.

package jujuapi

import (
	"github.com/juju/names/v5"

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

func (j *JIMMAdapter) RoleManager() RoleManager {
	return j.j.RoleManager
}

func (j *JIMMAdapter) GroupManager() GroupManager {
	return j.j.GroupManager
}

func (j *JIMMAdapter) IdentityManager() IdentityManager {
	return j.j.IdentityManager
}

func (j *JIMMAdapter) LoginManager() LoginManager {
	return j.j.LoginManager
}

func (j *JIMMAdapter) PermissionManager() PermissionManager {
	return j.j.PermissionManager
}

func (j *JIMMAdapter) AuditLogManager() AuditLogManager {
	return j.j.AuditLogManager
}

func (j *JIMMAdapter) JujuManager() JujuManager {
	return j.j.JujuManager
}

func (j *JIMMAdapter) ConfigManager() ConfigManager {
	return j.j.ConfigManager
}

func (j *JIMMAdapter) BootstrapManager() BootstrapManager {
	return j.j.BootstrapManager
}

func (j *JIMMAdapter) UpgradeManager() UpgradeManager {
	return j.j.UpgradeManager
}

func (j *JIMMAdapter) JobManager() JobManager {
	return j.j.JobManager
}

func (j *JIMMAdapter) ResourceTag() names.ControllerTag {
	return j.j.ResourceTag()
}

func (j *JIMMAdapter) PubSubHub() *pubsub.Hub {
	return j.j.Pubsub
}
