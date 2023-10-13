// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"database/sql"
	"fmt"
	"net"
	"time"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	apiparams "github.com/canonical/jimm/api/params"
)

// A controller represents a juju controller which is hosting models
// within the JAAS system.
type Controller struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

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

	// CloudName is the name of the cloud which is hosting this
	// controller.
	CloudName string

	// CloudRegion is the name of the cloud region which is hosting
	// this controller.
	CloudRegion string

	// Deprecated records whether this controller is deprecated, and
	// therefore no new models or clouds will be added to the controller.
	Deprecated bool `gorm:"not null;default:FALSE"`

	// AgentVersion holds the string representation of the controller's
	// agent version.
	AgentVersion string

	// Addresses holds the known addresses on which the controller is
	// listening.
	Addresses HostPorts

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

// ToAPIControllerInfo converts a controller entry to a JIMM API
// ControllerInfo.
func (c Controller) ToAPIControllerInfo() apiparams.ControllerInfo {
	var ci apiparams.ControllerInfo
	ci.Name = c.Name
	ci.UUID = c.UUID
	ci.PublicAddress = c.PublicAddress
	for _, hps := range c.Addresses {
		for _, hp := range hps {
			ci.APIAddresses = append(ci.APIAddresses, fmt.Sprintf("%s:%d", hp.Value, hp.Port))
		}
	}
	ci.CACertificate = c.CACertificate
	ci.CloudTag = names.NewCloudTag(c.CloudName).String()
	ci.CloudRegion = c.CloudRegion
	ci.Username = c.AdminUser
	ci.AgentVersion = c.AgentVersion
	if c.UnavailableSince.Valid {
		ci.Status = jujuparams.EntityStatus{
			Status: "unavailable",
			Since:  &c.UnavailableSince.Time,
		}
	} else if c.Deprecated {
		ci.Status = jujuparams.EntityStatus{
			Status: "deprecated",
		}
	} else {
		ci.Status = jujuparams.EntityStatus{
			Status: "available",
		}
	}
	return ci
}

// ToJujuRedirectInfoResult converts a controller entry to a juju
// RedirectInfoResult value.
func (c Controller) ToJujuRedirectInfoResult() jujuparams.RedirectInfoResult {
	var servers [][]jujuparams.HostPort
	host, port, err := net.SplitHostPort(c.PublicAddress)
	if err == nil {
		port, err := net.LookupPort("tcp", port)
		if err == nil {
			servers = append(servers, []jujuparams.HostPort{{
				Address: jujuparams.Address{
					Value: host,
					Scope: "public",
					Type:  "hostname",
				},
				Port: port,
			}})
		}
	}
	servers = append(servers, [][]jujuparams.HostPort(c.Addresses)...)
	return jujuparams.RedirectInfoResult{
		Servers: servers,
		CACert:  c.CACertificate,
	}
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
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

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

// ControllerConfig stores controller configuration.
type ControllerConfig struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// Name is the name given to this configuration.
	Name string `gorm:"not null;uniqueIndex"`
	// Config stores the controller configuration
	Config Map
}
