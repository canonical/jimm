// Copyright 2025 Canonical.

package jimmtest

import (
	"github.com/google/uuid"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/pubsub"
)

// JIMM is a default implementation of the jujuapi.JIMM interface. Every method
// has a corresponding funcion field. Whenever the method is called it
// will delegate to the requested funcion or if the funcion is nil return
// a NotImplemented error.
type JIMM struct {
	AuditLogManager_   func() jimm.AuditLogManager
	GroupManager_      func() jimm.GroupManager
	IdentityManager_   func() jimm.IdentityManager
	LoginManager_      func() jimm.LoginManager
	RoleManager_       func() jimm.RoleManager
	PermissionManager_ func() jimm.PermissionManager
	JujuManager_       func() jimm.JujuManager
	PubSubHub_         func() *pubsub.Hub
	ResourceTag_       func() names.ControllerTag
	ConfigManager_     func() jimm.ConfigManager
	OfferAuthorizer_   func() jimm.OfferAuthorizer
	BootstapManager_   func() jimm.BootstrapManager
	UpgradeManager_    func() jimm.UpgradeManager
	JobManager_        func() jimm.JobManager
}

func (j *JIMM) RoleManager() jimm.RoleManager {
	if j.RoleManager_ == nil {
		return nil
	}
	return j.RoleManager_()
}

func (j *JIMM) GroupManager() jimm.GroupManager {
	if j.GroupManager_ == nil {
		return nil
	}
	return j.GroupManager_()
}

func (j *JIMM) IdentityManager() jimm.IdentityManager {
	if j.IdentityManager_ == nil {
		return nil
	}
	return j.IdentityManager_()
}

func (j *JIMM) LoginManager() jimm.LoginManager {
	if j.LoginManager_ == nil {
		return nil
	}
	return j.LoginManager_()
}

func (j *JIMM) PermissionManager() jimm.PermissionManager {
	if j.PermissionManager_ == nil {
		return nil
	}
	return j.PermissionManager_()
}

func (j *JIMM) AuditLogManager() jimm.AuditLogManager {
	if j.AuditLogManager_ == nil {
		return nil
	}
	return j.AuditLogManager_()
}

func (j *JIMM) JujuManager() jimm.JujuManager {
	if j.JujuManager_ == nil {
		return nil
	}
	return j.JujuManager_()
}

func (j *JIMM) ResourceTag() names.ControllerTag {
	if j.ResourceTag_ == nil {
		return names.NewControllerTag(uuid.NewString())
	}
	return j.ResourceTag_()
}

func (j *JIMM) PubSubHub() *pubsub.Hub {
	if j.PubSubHub_ == nil {
		panic("not implemented")
	}
	return j.PubSubHub_()
}

func (j *JIMM) ConfigManager() jimm.ConfigManager {
	if j.ConfigManager_ == nil {
		return nil
	}
	return j.ConfigManager_()
}

func (j *JIMM) OfferAuthorizer() jimm.OfferAuthorizer {
	if j.OfferAuthorizer_ == nil {
		return nil
	}
	return j.OfferAuthorizer_()
}

func (j *JIMM) BootstrapManager() jimm.BootstrapManager {
	if j.BootstapManager_ == nil {
		return nil
	}
	return j.BootstapManager_()
}

func (j *JIMM) UpgradeManager() jimm.UpgradeManager {
	if j.UpgradeManager_ == nil {
		return nil
	}
	return j.UpgradeManager_()
}

func (j *JIMM) JobManager() jimm.JobManager {
	if j.JobManager_ == nil {
		return nil
	}
	return j.JobManager_()
}
