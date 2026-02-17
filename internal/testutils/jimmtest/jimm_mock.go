// Copyright 2025 Canonical.

package jimmtest

import (
	"github.com/google/uuid"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/pubsub"
)

// JIMM is a default implementation of the jujuapi.JIMM interface. Every method
// has a corresponding funcion field. Whenever the method is called it
// will delegate to the requested funcion or if the funcion is nil return
// a NotImplemented error.
type JIMM struct {
	AuditLogManager_   func() jujuapi.AuditLogManager
	GroupManager_      func() jujuapi.GroupManager
	IdentityManager_   func() jujuapi.IdentityManager
	LoginManager_      func() jujuapi.LoginManager
	RoleManager_       func() jujuapi.RoleManager
	PermissionManager_ func() jujuapi.PermissionManager
	JujuManager_       func() jujuapi.JujuManager
	PubSubHub_         func() *pubsub.Hub
	ResourceTag_       func() names.ControllerTag
	ConfigManager_     func() jujuapi.ConfigManager
	OfferAuthorizer_   func() jujuapi.OfferAuthorizer
	BootstapManager_   func() jujuapi.BootstrapManager
	UpgradeManager_    func() jujuapi.UpgradeManager
	JobManager_        func() jujuapi.JobManager
}

func (j *JIMM) RoleManager() jujuapi.RoleManager {
	if j.RoleManager_ == nil {
		return nil
	}
	return j.RoleManager_()
}

func (j *JIMM) GroupManager() jujuapi.GroupManager {
	if j.GroupManager_ == nil {
		return nil
	}
	return j.GroupManager_()
}

func (j *JIMM) IdentityManager() jujuapi.IdentityManager {
	if j.IdentityManager_ == nil {
		return nil
	}
	return j.IdentityManager_()
}

func (j *JIMM) LoginManager() jujuapi.LoginManager {
	if j.LoginManager_ == nil {
		return nil
	}
	return j.LoginManager_()
}

func (j *JIMM) PermissionManager() jujuapi.PermissionManager {
	if j.PermissionManager_ == nil {
		return nil
	}
	return j.PermissionManager_()
}

func (j *JIMM) AuditLogManager() jujuapi.AuditLogManager {
	if j.AuditLogManager_ == nil {
		return nil
	}
	return j.AuditLogManager_()
}

func (j *JIMM) JujuManager() jujuapi.JujuManager {
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

func (j *JIMM) ConfigManager() jujuapi.ConfigManager {
	if j.ConfigManager_ == nil {
		return nil
	}
	return j.ConfigManager_()
}

func (j *JIMM) OfferAuthorizer() jujuapi.OfferAuthorizer {
	if j.OfferAuthorizer_ == nil {
		return nil
	}
	return j.OfferAuthorizer_()
}

func (j *JIMM) BootstrapManager() jujuapi.BootstrapManager {
	if j.BootstapManager_ == nil {
		return nil
	}
	return j.BootstapManager_()
}

func (j *JIMM) UpgradeManager() jujuapi.UpgradeManager {
	if j.UpgradeManager_ == nil {
		return nil
	}
	return j.UpgradeManager_()
}

func (j *JIMM) JobManager() jujuapi.JobManager {
	if j.JobManager_ == nil {
		return nil
	}
	return j.JobManager_()
}
