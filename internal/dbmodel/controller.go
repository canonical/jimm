// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
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

	// HostPorts_ holds the known addresses on which the controller is
	// listening.
	HostPorts_ string `gorm:"column:host_ports"`

	// UnavailableSince records the time that this controller became
	// unavailable, if it has.
	UnavailableSince time.Time

	// CloudRegions is the set of cloud-regions that are available on this
	// controller.
	CloudRegions []CloudRegionControllerPriority

	// TODO(mhilton) Record details of the controller model.
	// TODO(mhilton) Monitor Lease management.
	// TODO(mhilton) Save controller statistics?
}

// Tag returns a names.Tag for this controller.
func (c Controller) Tag() names.Tag {
	return names.NewControllerTag(c.UUID)
}

// HostPorts returns all the known addresses that can be used to access the
// controller. See SetHostPorts for details of the internal representation.
// When parsing the internal HostPort representation any missing fields are
// taken to have their zero value, any additional fields are ignored.
func (c Controller) HostPorts() [][]jujuparams.HostPort {
	var hpss [][]jujuparams.HostPort

	groups := strings.Split(c.HostPorts_, "\x1d")
	for _, g := range groups {
		var hps []jujuparams.HostPort
		records := strings.Split(g, "\x1e")
		for _, r := range records {
			var hp jujuparams.HostPort
			parts := strings.Split(r, "\x1f")
			if len(parts) > 0 {
				port, _ := strconv.Atoi(parts[0])
				// Ignore the error here. failing to parse will set the
				// port to be 0 which will cause the whole record to be
				// rejected later.
				hp.Port = port
			}
			if len(parts) > 1 {
				hp.Value = parts[1]
			}
			if len(parts) > 2 {
				hp.Type = parts[2]
			}
			if len(parts) > 3 {
				hp.Scope = parts[3]
			}
			if len(parts) > 4 {
				hp.SpaceName = parts[4]
			}
			if len(parts) > 5 {
				hp.ProviderSpaceID = parts[5]
			}

			if hp.Port != 0 && hp.Value != "" {
				hps = append(hps, hp)
			}
		}
		if len(hps) > 0 {
			hpss = append(hpss, hps)
		}
	}

	return hpss
}

// SetHostPorts sets the HostPorts for the controller to be the given
// values. The given HostPorts will be flattened to an internal string
// representation. Each HostPort is encoded with the individual fields
// sepatated with \x1f (unit separator) i.e:
//
//     <port>\x1f<value>\x1f<type>\x1f<scope>\x1f<spacename>\x1f<providerspaceid>
//
// These string representations are then flattened by joining the members
// of the inner slices with \x1e (record separator) and the outer slice
// with \x1d (group separator).
func (c *Controller) SetHostPorts(hpss [][]jujuparams.HostPort) {
	var groups []string
	for _, hps := range hpss {
		var records []string
		for _, hp := range hps {
			records = append(records, fmt.Sprintf("%d\x1f%s\x1f%s\x1f%s\x1f%s\x1f%s",
				hp.Port, hp.Value, hp.Type, hp.Scope, hp.SpaceName, hp.ProviderSpaceID))
		}
		groups = append(groups, strings.Join(records, "\x1e"))
	}

	c.HostPorts_ = strings.Join(groups, "\x1d")
}

const (
	CloudRegionControllerPriorityDeployed  = 10
	CloudRegionControllerPrioritySupported = 1
)

// A CloudRegionControllerPriority entry specifies the priority with which
// a controller should be chosen when deploying to a particular
// cloud-region.
type CloudRegionControllerPriority struct {
	gorm.Model

	// CloudRegion is the cloud-region this pertains to.
	CloudRegionID uint
	CloudRegion   CloudRegion

	// Controller is the controller this pertains to.
	ControllerID uint
	Controller   Controller

	// Priority is the priority with which this controller should be
	// chosen when deploying to a cloud-region.
	Priority uint
}
