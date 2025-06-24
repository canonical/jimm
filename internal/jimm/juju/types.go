// Copyright 2025 Canonical.

package juju

import (
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/juju/juju/core/network"
	jujuparams "github.com/juju/juju/rpc/params"
)

// ControllerCreds represent the admin username and password
// used to authenticate with a Juju controller via basic auth.
type ControllerCreds struct {
	AdminIdentityName string
	AdminPassword     string
}

// ControllerConnectionDetails contains details for connecting
// to a Juju controller and the credentials used to access it.
type ControllerConnectionDetails struct {
	// CACertificate is the CA certificate required to access this
	// controller. This is only set if the controller endpoint's
	// certificate is not signed by a public CA.
	CACertificate string
	// PublicAddress is the public address registered with the controller
	// when it was added. This address will normally be a resolvable DNS
	// name and port.
	PublicAddress string
	// TLSHostname provides a hostname that should be used for TLS verfication.
	// Useful for local dev to avoid TLS issues.
	TLSHostname string `gorm:"column:tls_hostname"`
	// Addresses holds the known addresses on which the controller is
	// listening.
	Addresses []network.MachineHostPorts
	// Credentials holds admin credentials for the controller.
	Credentials ControllerCreds
}

func toControllerConnectionDetails(controller dbmodel.Controller, username, password string) ControllerConnectionDetails {
	addresses := jujuparams.ToMachineHostsPorts(controller.Addresses)
	return ControllerConnectionDetails{
		CACertificate: controller.CACertificate,
		PublicAddress: controller.PublicAddress,
		TLSHostname:   controller.TLSHostname,
		Addresses:     addresses,
		Credentials: ControllerCreds{
			AdminIdentityName: username,
			AdminPassword:     password,
		},
	}
}
