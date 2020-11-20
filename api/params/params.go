// Copyright 2020 Canonical Ltd.

package params

import jujuparams "github.com/juju/juju/apiserver/params"

// An AddControllerRequest is the request sent when adding a new controller
// to JIMM.
type AddControllerRequest struct {
	// Name is the name to give to the controller, all controllers must
	// have a unique name.
	Name string `json:"name"`

	// PublicAddress is the public address of the controller. This is
	// normally a DNS name and port which provide the controller endpoints.
	// This address should not change even if the controller units
	// themselves are migrated.
	PublicAddress string `json:"public-address,omitempty"`

	// APIAddresses contains the currently known API addresses for the
	// controller.
	APIAddresses []string `json:"api-addresses,omitempty"`

	// CACertificate contains the CA certificate to use to validate the
	// connection to the controller. This is not needed if certificate is
	// signed by a public CA.
	CACertificate string `json:"ca-certificate,omitempty"`

	// Username contains the username that JIMM should use to connect to
	// the controller.
	Username string `json:"username"`

	// Password contains the password that JIMM should use to connect to
	// the controller.
	Password string `json:"password"`
}

// A ControllerInfo describes a controller on a JIMM system.
type ControllerInfo struct {
	// Name is the name of the controller.
	Name string `json:"name"`

	// UUID is the UUID of the controller.
	UUID string `json:"uuid"`

	// PublicAddress is the public address of the controller. This is
	// normally a DNS name and port which provide the controller endpoints.
	// This address should not change even if the controller units
	// themselves are migrated.
	PublicAddress string `json:"public-address,omitempty"`

	// APIAddresses contains the currently known API addresses for the
	// controller.
	APIAddresses []string `json:"api-addresses,omitempty"`

	// CACertificate contains the CA certificate to use to validate the
	// connection to the controller. This is not needed if certificate is
	// signed by a public CA.
	CACertificate string `json:"ca-certificate,omitempty"`

	// CloudTag is the tag of the cloud this controller is running in.
	CloudTag string `json:"cloud-tag,omitempty"`

	// CloudRegion is the region that this controller is running in.
	CloudRegion string `json:"cloud-region,omitempty"`

	// Username contains the username that JIMM uses to connect to the
	// controller.
	Username string `json:"username"`

	// The version of the juju agent running on the controller.
	AgentVersion string `json:"agent-version"`

	// Status contains the current status of the controller. The status
	// will either be "available", "deprecated", or "unavailable".
	Status jujuparams.EntityStatus `json:"status"`
}

// ListControllersResponse is the response that is sent in a
// ListControllers method.
type ListControllersResponse struct {
	Controllers []ControllerInfo `json:"controllers"`
}
