// Copyright 2016 Canonical Ltd.

package mongodoc

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/version/v2"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/params"
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
	UpdateCredentials []CredentialPath `bson:",omitempty"`

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

	// ApplicationOfferCount hols the numer of application offers
	// hoster in the controller.
	ApplicationOfferCount int
}

type UserInfo struct {
	Password string
}

// CloudRegion holds the details of a cloud or a region within a cloud.
// For information about a whole cloud, Region will be empty, and the
// document should not contain any secondary controllers, as all
// controllers located in that cloud are considered equally primary.
//
// If there are any region entries in the collection, there should be a
// cloud entry that represents the cloud for those regions.
//
// For a cloud region, secondary controllers are those that can be used
// to manage a model within a region but are not located within that
// region (primary controllers should be used in preference if
// available).
type CloudRegion struct {
	// Id holds the primary key for a CloudRegion.
	// It is in the format <cloud>/<region>
	Id string `bson:"_id"`

	// Cloud holds the cloud name.
	Cloud params.Cloud

	// Region holds the name of the region.
	Region string

	// ProviderType holds the type of cloud.
	ProviderType string

	// Authtypes holds the set of allowed authtypes for use with this
	// cloud.
	AuthTypes []string

	// Endpoint contains the region or cloud endpoint parameter specified by the
	// controller.
	Endpoint string

	// IdentityEndpoint contains the region or cloud identity endpoint parameter
	// specified by the controller.
	IdentityEndpoint string

	// IdentityEndpoint contains the region or cloud storage endpoint parameter
	// specified by the controller.
	StorageEndpoint string

	// CACertificates contains any CA root certificates for the cloud
	// instances.
	CACertificates []string

	// PrimaryControllers holds the local user and name given to the
	// controllers that can use that cloud and which are hosted on this region.
	PrimaryControllers []params.EntityPath

	// SecondaryControllers holds the local user and name given to the
	// controllers that can use that cloud and which are hosted outside this region.
	SecondaryControllers []params.EntityPath

	// ACL holds permissions for the cloud.
	ACL params.ACL
}

func (c CloudRegion) GetId() string {
	if c.Id != "" {
		return c.Id
	}
	return fmt.Sprintf("%s/%s", c.Cloud, c.Region)
}

func (c *CloudRegion) GetACL() params.ACL {
	return c.ACL
}

func (c *CloudRegion) Owner() params.User {
	return ""
}

// Model holds information on a given model.
type Model struct {
	// Id holds the primary key for an model.
	// It holds Path.String().
	Id string `bson:"_id"`

	// Controller holds the path of the model's
	// controller.
	Controller params.EntityPath

	// ControllerUUID holds the UUID of the model's controller.
	ControllerUUID string `bson:",omitempty"`

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
	Credential CredentialPath

	// DefaultSeries holds the default series for the model.
	DefaultSeries string

	// Status holds the current status of the model
	Info *ModelInfo `bson:",omitempty"`

	// Type holds the type of the model (IAAS or CAAS)
	Type string

	// ProviderType holds the provider type of the model.
	ProviderType string
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
	// Id holds the combination of the controller path, model UUID and machine id.
	Id         string `bson:"_id"`
	Controller string
	Cloud      params.Cloud
	Region     string
	Info       *jujuparams.MachineInfo
}

// Application holds information on an application in a model, as discovered by the
// controller monitor.
type Application struct {
	// Id holds the combination of the controller path, model UUID and application id.
	Id         string `bson:"_id"`
	Controller string
	Cloud      params.Cloud
	Region     string
	Info       *ApplicationInfo
}

// ApplicationInfo holds a subset of information about an application that is tracked
// by multiwatcherStore.
type ApplicationInfo struct {
	ModelUUID       string
	Name            string
	Exposed         bool
	CharmURL        string
	OwnerTag        string
	Life            life.Value
	Subordinate     bool
	Status          jujuparams.StatusInfo
	WorkloadVersion string
}

type ModelUserInfo struct {
	// Granted holds whether we granted the given user
	// access (if false, we revoked it).
	Granted bool
}

// Owner returns the model owner.
func (m *Model) Owner() params.User {
	return m.Path.User
}

// GetACL returns the model's ACL.
func (m *Model) GetACL() params.ACL {
	return m.ACL
}

// EntityPath holds the user and name part of the credential path. It is in place
// to enable backwards due to legacy credential paths already stored in the
// database.
type EntityPath struct {
	User string
	Name string
}

// CredentialPath implements the params.CredentialPath interface.
type CredentialPath struct {
	Cloud string
	EntityPath
}

// IsZero reports whether the receiver is the empty value.
func (c CredentialPath) IsZero() bool {
	return c.Cloud == "" && c.User == "" && c.Name == ""
}

// String returns a string representation of the CredentialPath.
func (c CredentialPath) String() string {
	p := params.CredentialPath{
		Cloud: params.Cloud(c.Cloud),
		User:  params.User(c.User),
		Name:  params.CredentialName(c.Name),
	}
	return p.String()
}

// ToParams returns a params package representation of the CredentialPath.
func (c CredentialPath) ToParams() params.CredentialPath {
	return params.CredentialPath{
		Cloud: params.Cloud(c.Cloud),
		User:  params.User(c.User),
		Name:  params.CredentialName(c.Name),
	}
}

// CredentialPathFromParams returns a mongodoc package representation
// of the CredentialPath.
func CredentialPathFromParams(p params.CredentialPath) CredentialPath {
	return CredentialPath{
		Cloud: string(p.Cloud),
		EntityPath: EntityPath{
			User: string(p.User),
			Name: string(p.Name),
		},
	}
}

// Credential holds the credentials.
type Credential struct {
	// Id holds the primary key for a credential.
	// It holds "<User>/<Cloud>/<Name>"
	Id string `bson:"_id"`

	// Path holds the local cloud, user and name given to the
	// credential, denormalized from Id for convenience and ease of
	// indexing. Its string value is used as the Id value.
	Path CredentialPath

	// ACL holds permissions for the credential.
	ACL params.ACL

	// Type holds the type of credential.
	Type string

	// Label holds an optional label for the credentials.
	Label string

	// Attributes holds the credential attributes.
	Attributes map[string]string `bson:",omitempty"`

	// Controllers holds the controllers to which this credential has
	// been uploaded.
	Controllers []params.EntityPath

	// Revoked records that the credential has been revoked.
	Revoked bool

	// AttributesInVault records that the actual credential attributes are
	// stored in a seperate vault.
	AttributesInVault bool

	// ProviderType holds the provider type of the cloud that this
	// credential is for.
	ProviderType string
}

// Owner returns the owner of the credentials.
func (c *Credential) Owner() params.User {
	return params.User(c.Path.User)
}

// GetACL returns the credential's ACL.
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
	hp.Host = hp1.Host()
	hp.Port = hp1.Port()
	hp.Scope = string(hp1.AddressScope())
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
	nhps, err := network.ParseProviderHostPorts(addresses...)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	hps := make([]HostPort, len(nhps))
	for i, nhp := range nhps {
		hps[i].SetJujuHostPort(nhp)
	}
	return hps, nil
}

// ApplicationOffer represents a cross model application offer.
type ApplicationOffer struct {
	ModelUUID string `bson:"model-uuid"`
	ModelName string `bson:"model-name"`
	// ControllerPath contains the path of the controller that owns the
	// application offer.
	ControllerPath params.EntityPath `bson:"controller-path"`

	OfferUUID string `bson:"_id"`
	// OfferURL is the URL of the offer. The OfferURL is normalised such
	// that it includes the owner ID, but it does not include the
	// controller name.
	OfferURL               string                    `bson:"offer-url"`
	OfferName              string                    `bson:"offer-name"`
	OwnerName              string                    `bson:"owner-name"`
	ApplicationName        string                    `bson:"application-name"`
	ApplicationDescription string                    `bson:"application-description"`
	CharmURL               string                    `bson:"charm-url"`
	Endpoints              []RemoteEndpoint          `bson:"endpoints"`
	Spaces                 []RemoteSpace             `bson:"spaces"`
	Bindings               map[string]string         `bson:"bindings"`
	Users                  ApplicationOfferAccessMap `bson:"users,omitempty"`
	Connections            []OfferConnection         `bson:"connections"`
}

// OfferConnection holds details about a connection to an offer.
type OfferConnection struct {
	SourceModelTag string                `bson:"source-model-tag"`
	RelationId     int                   `bson:"relation-id"`
	Username       string                `bson:"username"`
	Endpoint       string                `bson:"endpoint"`
	IngressSubnets []string              `bson:"ingress-subnets"`
	Status         OfferConnectionStatus `bson:"status"`
}

// OfferConnectionStatus holds the status of an application
// offer connection.
type OfferConnectionStatus struct {
	Status string
	Info   string
	Data   map[string]interface{}
	Since  *time.Time
}

// RemoteSpace represents a space in some remote model.
type RemoteSpace struct {
	CloudType          string                 `bson:"cloud-type"`
	Name               string                 `bson:"name"`
	ProviderId         string                 `bson:"provider-id"`
	ProviderAttributes map[string]interface{} `bson:"provider-attributes"`
}

// RemoteEndpoint represents a remote application endpoint.
type RemoteEndpoint struct {
	Name      string `bson:"name"`
	Role      string `bson:"role"`
	Interface string `bson:"interface"`
	Limit     int    `bson:"limit"`
}

// ApplicationOfferAccessPermission holds the access permission level.
type ApplicationOfferAccessPermission int

// String implements fmt.Stringer.
func (p ApplicationOfferAccessPermission) String() string {
	switch p {
	case ApplicationOfferReadAccess:
		return string(jujuparams.OfferReadAccess)
	case ApplicationOfferConsumeAccess:
		return string(jujuparams.OfferConsumeAccess)
	case ApplicationOfferAdminAccess:
		return string(jujuparams.OfferAdminAccess)
	default:
		return ""
	}
}

const (
	ApplicationOfferNoAccess ApplicationOfferAccessPermission = iota
	ApplicationOfferReadAccess
	ApplicationOfferConsumeAccess
	ApplicationOfferAdminAccess
)

type CloudRegionDefaults struct {
	User     string                 `bson:"user"`
	Cloud    string                 `bson:"cloud"`
	Region   string                 `bson:"region"`
	Defaults map[string]interface{} `bson:"defaults"`
}

// An ApplicationOfferAccessMap is a map from user to AccessPermission.
type ApplicationOfferAccessMap map[User]ApplicationOfferAccessPermission

// GetBSON implements bson.Getter.
func (m ApplicationOfferAccessMap) GetBSON() (interface{}, error) {
	if m == nil {
		return nil, nil
	}
	b := make(map[string]ApplicationOfferAccessPermission, len(m))
	for k, v := range m {
		b[tildeEscape.Replace(string(k))] = v
	}
	return b, nil
}

// SetBSON implements bson.Setter.
func (m *ApplicationOfferAccessMap) SetBSON(raw bson.Raw) error {
	b := make(map[string]ApplicationOfferAccessPermission)
	if err := raw.Unmarshal(&b); err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	if *m == nil {
		*m = make(ApplicationOfferAccessMap, len(b))
	}
	for k, v := range b {
		(*m)[User(tildeUnescape.Replace(k))] = v
	}
	return nil
}

// A User represents a user in the database. A User automatically escapes
// any chararacters in the username that are not valid field names making
// a user suitable for use in map keys.
type User string

// GetBSON implements bson.Getter.
func (u User) GetBSON() (interface{}, error) {
	return tildeEscape.Replace(string(u)), nil
}

// SetBSON implements bson.Setter.
func (u *User) SetBSON(r bson.Raw) error {
	var s string
	if err := r.Unmarshal(&s); err != nil {
		return err
	}
	*u = User(tildeUnescape.Replace(s))
	return nil
}

// FieldName returns the field name for this user. Any givne prefix strings
// are the fields names of the embedded documents that are hold the field.
func (u User) FieldName(prefix ...string) string {
	prefix = append(prefix, tildeEscape.Replace(string(u)))
	return strings.Join(prefix, ".")
}

// tildeEscape escapes strings with the following translation:
//
//     ~ -> ~0
//     $ -> ~1
//     . -> ~2
var tildeEscape = strings.NewReplacer("~", "~0", "$", "~1", ".", "~2")

// tildeUnescape unescapes strings with the following translation:
//
//     ~0 -> ~
//     ~1 -> $
//     ~2 -> .
var tildeUnescape = strings.NewReplacer("~0", "~", "~1", "$", "~2", ".")