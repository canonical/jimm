// Copyright 2016 Canonical Ltd.

package mongodoc

import (
	"encoding/base64"
	"net"
	"strconv"
	"time"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/version"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/params"
)

// Controller holds information on a given controller.
// Each controller also has an entry in the models
// collection with the same id.
type Controller struct {
	// Id holds the primary key for a controller.
	// It holds Path.String().
	Id string `bson:"_id"`

	// EntityPath holds the local user and name given to the
	// controller, denormalized from Id for convenience
	// and ease of indexing. Its string value is used as the Id value.
	Path params.EntityPath

	// ACL holds permissions for the controller.
	ACL params.ACL

	// CACert holds the CA certificate of the controller.
	CACert string

	// HostPorts holds all the known HostPorts for the controller.
	HostPorts [][]HostPort

	// Users holds a record for each user that JEM has created
	// in the given controller.
	// Note that keys are sanitized with the Sanitize function.
	Users map[string]UserInfo `bson:",omitempty"`

	// AdminUser and AdminPassword hold the admin
	// credentials presented when the controller was
	// first created.
	AdminUser     string
	AdminPassword string

	// UUID duplicates the controller UUID held in the
	// model associated with the controller, so
	// we can return a model's controller UUID by fetching
	// only the controller document.
	UUID string

	// Public specifies whether the controller is considered
	// part of the "pool" of publicly available controllers.
	// Non-public controllers will be ignored when selecting
	// controllers by location.
	Public bool `bson:",omitempty"`

	// Deprecated specifies whether a public controller is considered
	// deprecated for the purposes of adding new models.
	Deprecated bool `bson:",omitempty"`

	// Cloud holds the details of the cloud provided by this controller.
	Cloud Cloud

	// MonitorLeaseOwner holds the name of the agent
	// currently responsible for monitoring the controller.
	MonitorLeaseOwner string `bson:",omitempty"`

	// MonitorLeaseExpiry holds the time at which the
	// current monitor's lease expires.
	MonitorLeaseExpiry time.Time `bson:",omitempty"`

	// Stats holds runtime information about the controller.
	Stats ControllerStats

	// UnavailableSince is zero when the controller is marked
	// as available; otherwise it holds the time when it became
	// unavailable.
	UnavailableSince time.Time `bson:",omitempty"`

	// UpdateCredentials holds a list of credentials which require
	// updates on this controller.
	UpdateCredentials []params.CredentialPath `bson:",omitempty"`

	// Location holds the location of the controller. The only key
	// values currently supported are "cloud" and "region". All
	// controllers should have an associated cloud, but not clouds
	// have regions.
	Location map[string]string

	// Version holds the latest known agent version
	// of the controller.
	Version *version.Number `bson:",omitempty"`
}

func (s *Controller) Owner() params.User {
	return s.Path.User
}

func (s *Controller) GetACL() params.ACL {
	return s.ACL
}

// ControllerStats holds statistics about a controller.
type ControllerStats struct {
	// UnitCount holds the number of units hosted in the controller.
	UnitCount int

	// ModelCount holds the number of models hosted in the controller.
	ModelCount int

	// ServiceCount holds the number of services hosted in the controller.
	// TODO this should be named ApplicationCount.
	ServiceCount int

	// MachineCount holds the number of machines hosted in the controller.
	// This includes all machines, not just top level instances.
	MachineCount int
}

type UserInfo struct {
	Password string
}

// Cloud holds the details of a cloud provided by a controller.
type Cloud struct {
	// Name holds the name of the cloud.
	Name params.Cloud

	// ProviderType holds the type of cloud.
	ProviderType string

	// Authtypes holds the set of allowed authtypes for use with this
	// cloud.
	AuthTypes []string

	// Endpoint contains the endpoint parameter specified by the
	// controller.
	Endpoint string

	// IdentityEndpoint contains the identity endpoint parameter
	// specified by the controller.
	IdentityEndpoint string

	// StorageEndpoint contains the stroage endpoint parameter
	// specified by the controller.
	StorageEndpoint string

	// Regions contains the details of the set of regions supported
	// by the controller. Note: some providers do not use regions.
	Regions []Region
}

// Region holds details of a cloud region supported by a controller.
type Region struct {
	// Name holds the name of the region.
	Name string

	// Endpoint contains the endpoint parameter specified by the
	// controller.
	Endpoint string

	// IdentityEndpoint contains the identity endpoint parameter
	// specified by the controller.
	IdentityEndpoint string

	// StorageEndpoint contains the storage endpoint parameter
	// specified by the controller.
	StorageEndpoint string
}

// Model holds information on a given model.
type Model struct {
	// Id holds the primary key for an model.
	// It holds Path.String().
	Id string `bson:"_id"`

	// Controller holds the path of the model's
	// controller.
	Controller params.EntityPath

	// EntityPath holds the local user and name given to the
	// model, denormalized from Id for convenience
	// and ease of indexing. Its string value is used as the Id value.
	Path params.EntityPath

	// ACL holds permissions for the model.
	ACL params.ACL

	// UUID holds the UUID of the model.
	UUID string

	// AdminUser holds the user name to use
	// when connecting to the controller.
	AdminUser string

	// Users holds a map holding information about all
	// the users we have managed on the model.
	// Note that keys are sanitized with the Sanitize function.
	Users map[string]ModelUserInfo `bson:",omitempty"`

	// Life holds the current life status of the model ("alive", "dying"
	// or "dead").
	Life_ string `bson:"life"`

	// Counts holds information about the number of various kinds
	// of entities in the model.
	Counts map[params.EntityCount]params.Count `bson:",omitempty"`

	// TODO record last time we saw changes on the model?

	// Templates holds the paths of the templates that the model was created with.
	Templates []string `bson:",omitempty"`

	// ExplicitConfigFields holds those configuration options that
	// were explictly specified when creating the model. These
	// override all values in the template, so when any of the
	// templates change, none of these fields will be changed.
	ExplicitConfigFields []string `bson:",omitempty"`

	// TemplateVersions holds a map from template path (we can't use
	// params.EntityPath as a BSON map key) to the version number of
	// the template that has been most recently set in the model
	// configuration.
	TemplateVersions map[string]int `bson:",omitempty"`

	// CreationTime holds the time the model was created.
	CreationTime time.Time

	// Creator holds the name of the user that issued the model creation
	// request.
	Creator string

	// Cloud holds the name of the cloud that the model was created in.
	Cloud params.Cloud

	// CloudRegion holds the region of the cloud that the model was created in.
	CloudRegion string `bson:",omitempty"`

	// Credential holds a reference to the credential used to create the model.
	Credential params.CredentialPath

	// DefaultSeries holds the default series for the model.
	DefaultSeries string

	// UsageSenderCredentials prove that we are authorized to send usage
	// information for this model.
	UsageSenderCredentials []byte

	// Status holds the current status of the model
	Info *ModelInfo `bson:",omitempty"`
}

type ModelInfo struct {
	// Life holds the life of the model
	Life string

	// Config holds the model configuration
	Config map[string]interface{} `bson:",omitempty"`

	// Status holds the current status of the model.
	Status ModelStatus
}

type ModelStatus struct {
	// Status holds the actual status value.
	Status string

	// Message holds a message associated with the status.
	Message string

	// Since contains the time this status has been valid since.
	Since time.Time `bson:",omitempty"`

	// Data contains data associated with the status.
	Data map[string]interface{} `bson:",omitempty"`
}

// Life determines the current life of a model object.
func (m *Model) Life() string {
	if m.Info != nil {
		return m.Info.Life
	}
	return m.Life_
}

// Machine holds information on a machine in a model, as discovered by the
// controller monitor.
type Machine struct {
	// Id holds the combination of the model UUID and machine id.
	Id   string `bson:"_id"`
	Info *multiwatcher.MachineInfo
}

type ModelUserInfo struct {
	// Granted holds whether we granted the given user
	// access (if false, we revoked it).
	Granted bool
}

func (e *Model) Owner() params.User {
	return e.Path.User
}

func (e *Model) GetACL() params.ACL {
	return e.ACL
}

type Credential struct {
	// Id holds the primary key for a credential.
	// It holds "<User>/<Cloud>/<Name>"
	Id string `bson:"_id"`

	// Path holds the local cloud, user and name given to the
	// credential, denormalized from Id for convenience and ease of
	// indexing. Its string value is used as the Id value.
	Path params.CredentialPath

	// ACL holds permissions for the credential.
	ACL params.ACL

	// Type holds the type of credential.
	Type string

	// Label holds an optional label for the credentials.
	Label string

	// Attributes holds the credential attributes.
	Attributes map[string]string

	// Controllers holds the controllers to which this credential has
	// been uploaded.
	Controllers []params.EntityPath

	// Revoked records that the credential has been revoked.
	Revoked bool
}

func (c *Credential) Owner() params.User {
	return c.Path.User
}

func (c *Credential) GetACL() params.ACL {
	return c.ACL
}

type HostPort struct {
	// Host holds the name or address of the host.
	Host string

	// Port holds the port number.
	Port int

	// Scope holds the scope of the host
	Scope string
}

func (hp HostPort) Address() string {
	return net.JoinHostPort(hp.Host, strconv.Itoa(hp.Port))
}

func (hp *HostPort) SetJujuHostPort(hp1 network.HostPort) {
	hp.Host = hp1.Value
	hp.Port = hp1.Port
	hp.Scope = string(hp1.Scope)
}

// Addresses collapses a slice of slices of HostPorts to a single list of
// unique addresses.
func Addresses(hpss [][]HostPort) []string {
	var addrs []string
	seen := make(map[string]bool)
	for _, hps := range hpss {
		for _, hp := range hps {
			addr := hp.Address()
			if seen[addr] {
				continue
			}
			seen[addr] = true
			addrs = append(addrs, addr)
		}
	}
	return addrs
}

// ParseAddresses parses the given addresses into a HostPort slice.
func ParseAddresses(addresses []string) ([]HostPort, error) {
	nhps, err := network.ParseHostPorts(addresses...)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	hps := make([]HostPort, len(nhps))
	for i, nhp := range nhps {
		hps[i].SetJujuHostPort(nhp)
	}
	return hps, nil
}

// Sanitize returns a version of key that's suitable
// for using as a mongo key.
// TODO base64 encoding is probably overkill - we
// could probably do something that left keys
// more readable.
func Sanitize(key string) string {
	return base64.StdEncoding.EncodeToString([]byte(key))
}
