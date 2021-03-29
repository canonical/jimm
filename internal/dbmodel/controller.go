// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"database/sql"

	"github.com/juju/names/v4"
	"gorm.io/gorm"
)

// A controller represents a juju controller which is hosting models
// within the JAAS system.
type Controller struct {
	gorm.Model

	// Name is the name given to this controller.
	Name string `gorm:"not null;uniqueIndex"`

	// UUID is the UUID of the controller. Note this is not being made a
	// unique value because we occasionally want to add the same
	// controller to the database with different names for testing
	// purposes.
	UUID string `gorm:"not null"`

	// AdminUser is the username that JIMM uses to connect to the
	// controller.
	AdminUser string

	// AdminPassword is the password that JIMM uses to connect to the
	// controller.
	AdminPassword string

	// CACertificate is the CA certificate required to access this
	// controller. This is only set if the controller endpoint's
	// certificate is not signed by a public CA.
	CACertificate string

	// PublicAddress is the public address registered with the controller
	// when it was added. This address will normally be a resolvable DNS
	// name and port.
	PublicAddress string

	// Deprecated records whether this controller is deprecated, and
	// therefore no new models or clouds will be added to the controller.
	Deprecated bool `gorm:"not null;default:FALSE"`

	// AgentVersion holds the string representation of the controller's
	// agent version.
	AgentVersion string

	// Addresses holds the known addresses on which the controller is
	// listening.
	Addresses Strings

	// UnavailableSince records the time that this controller became
	// unavailable, if it has.
	UnavailableSince sql.NullTime

	// CloudRegions is the set of cloud-regions that are available on this
	// controller.
	CloudRegions []CloudRegionControllerPriority

	// Models contains all the models that are running on this controller.
	Models []Model

	// TODO(mhilton) Save controller statistics?
}

// Tag returns a names.Tag for this controller.
func (c Controller) Tag() names.Tag {
	return names.NewControllerTag(c.UUID)
}

// SetTag sets the controller's UUID to the value from the given tag.
func (c *Controller) SetTag(t names.ControllerTag) {
	c.UUID = t.Id()
}

const (
	// CloudRegionControllerPriorityDeployed is the priority given to the
	// controller when deploying to a cloud region to which the controller
	// model is deployed.
	CloudRegionControllerPriorityDeployed = 10

	// CloudRegionControllerPrioritySupported is the priority given to the
	// controller when deploying to a cloud region to which the controller
	// model is not deployed.
	CloudRegionControllerPrioritySupported = 1
)

// A CloudRegionControllerPriority entry specifies the priority with which
// a controller should be chosen when deploying to a particular
// cloud-region.
type CloudRegionControllerPriority struct {
	gorm.Model

	// CloudRegion is the cloud-region this pertains to.
	CloudRegionID uint
	CloudRegion   CloudRegion `gorm:"constraint:OnDelete:CASCADE"`

	// Controller is the controller this pertains to.
	ControllerID uint
	Controller   Controller `gorm:"constraint:OnDelete:CASCADE"`

	// Priority is the priority with which this controller should be
	// chosen when deploying to a cloud-region.
	Priority uint
}
