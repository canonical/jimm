package params

import (
	"bytes"
	"fmt"
	"time"

	"github.com/juju/httprequest"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/environschema.v1"
)

// SetControllerPerm holds the parameters for setting the ACL on a
// controller.
type SetControllerPerm struct {
	httprequest.Route `httprequest:"PUT /v2/controller/:User/:Name/perm"`
	EntityPath
	ACL ACL `httprequest:",body"`
}

// GetControllerPerm holds the parameters for getting the ACL of a
// controller.
type GetControllerPerm struct {
	httprequest.Route `httprequest:"GET /v2/controller/:User/:Name/perm"`
	EntityPath
}

// SetModelPerm holds the parameters for setting the ACL on an model.
type SetModelPerm struct {
	httprequest.Route `httprequest:"PUT /v2/model/:User/:Name/perm"`
	EntityPath
	ACL ACL `httprequest:",body"`
}

// GetModelPerm holds the parameters for getting the ACL of an model.
type GetModelPerm struct {
	httprequest.Route `httprequest:"GET /v2/model/:User/:Name/perm"`
	EntityPath
}

// GetControllerLocation holds the parameters for getting the Location
// field of a controller.
type GetControllerLocation struct {
	httprequest.Route `httprequest:"GET /v2/controller/:User/:Name/meta/location"`
	EntityPath
}

// ControllerLocation holds details of a controller's location.
type ControllerLocation struct {
	// Location holds the location attributes.
	Location map[string]string
	// Public holds whether the controller is considered public.
	Public bool
}

// ACL holds an access control list for an entity.
type ACL struct {
	// Read holds users and groups that are allowed to read the
	// entity.
	Read []string `json:"read"`
}

// AddController holds the parameters for adding a new controller.
type AddController struct {
	httprequest.Route `httprequest:"PUT /v2/controller/:User/:Name"`
	EntityPath
	Info ControllerInfo `httprequest:",body"`
}

// DeleteController holds the parameters for removing the controller.
type DeleteController struct {
	httprequest.Route `httprequest:"DELETE /v2/controller/:User/:Name"`
	EntityPath
	// Force forces the delete even if the controller is still alive.
	Force bool `httprequest:"force,form"`
}

type GetAllControllerLocations struct {
	httprequest.Route `httprequest:"GET /v2/location"`

	// Location constrains the controllers that the
	// set of returned values will be returned from
	// to those with matching location attributes.
	// Note that the values in this should be passed in the
	// URL query parameters.
	Location map[string]string
}

type AllControllerLocationsResponse struct {
	Locations []map[string]string
}

type GetControllerLocations struct {
	httprequest.Route `httprequest:"GET /v2/location/:Attr"`
	Attr              string `httprequest:",path"`

	// Location constrains the controllers that the
	// set of returned values will be returned from
	// to those with matching location attributes.
	// Note that the values in this should be passed in the
	// URL query parameters.
	Location map[string]string
}

type ControllerLocationsResponse struct {
	Values []string
}

// ControllerInfo holds information specifying how
// to connect to an existing controller.
type ControllerInfo struct {
	// HostPorts holds host/port pairs (in host:port form)
	// of the controller API endpoints.
	HostPorts []string `json:"host-ports"`

	// CACert holds the CA certificate that will be used
	// to validate the controller's certificate, in PEM format.
	CACert string `json:"ca-cert"`

	// User holds the name of user to use when
	// connecting to the controller.
	User string `json:"user"`

	// Password holds the password for the user.
	Password string `json:"password"`

	// ControllerUUID holds the UUID of the admin model
	// of the controller.
	ControllerUUID string `json:"controller-uuid"`

	// Public specifies whether the controller is considered
	// part of the "pool" of publicly available controllers.
	// Non-public controllers will be ignored when selecting
	// controllers by location.
	//
	// Only privileged users may create public controllers.
	Public bool `json:"public"`
}

// EntityPath holds the path parameters for specifying
// an entity in the API. It can also be used as a value
// in its own right, because it implements TextMarshaler
// and TextUnmarshaler.
type EntityPath struct {
	User User `httprequest:",path"`
	Name Name `httprequest:",path"`
}

func (p EntityPath) String() string {
	return fmt.Sprintf("%s/%s", p.User, p.Name)
}

// IsZero reports whether the receiver is the empty value.
func (p EntityPath) IsZero() bool {
	return p == EntityPath{}
}

var slash = []byte("/")

func (p *EntityPath) UnmarshalText(data []byte) error {
	parts := bytes.Split(data, slash)
	if len(parts) != 2 {
		return errgo.New("need <user>/<name>")
	}
	if err := p.User.UnmarshalText(parts[0]); err != nil {
		return errgo.Mask(err)
	}
	if err := p.Name.UnmarshalText(parts[1]); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func (p EntityPath) MarshalText() ([]byte, error) {
	data := make([]byte, 0, len(p.User)+1+len(p.Name))
	data = append(data, p.User...)
	data = append(data, '/')
	data = append(data, p.Name...)
	return data, nil
}

// CredentialPath holds the path parameters for specifying a credential
// in the API. It can also be used as a value in its own right, because
// it implements TextMarshaler and TextUnmarshaler.
type CredentialPath struct {
	Cloud Cloud `httprequest:",path"`
	EntityPath
}

func (p CredentialPath) String() string {
	return fmt.Sprintf("%s/%s", p.Cloud, p.EntityPath)
}

// IsZero reports whether the receiver is the empty value.
func (p CredentialPath) IsZero() bool {
	return p.Cloud == "" && p.EntityPath.IsZero()
}

func (p *CredentialPath) UnmarshalText(data []byte) error {
	parts := bytes.Split(data, slash)
	if len(parts) != 3 {
		return errgo.New("need <cloud>/<user>/<name>")
	}
	if err := p.Cloud.UnmarshalText(parts[0]); err != nil {
		return errgo.Mask(err)
	}
	if err := p.EntityPath.User.UnmarshalText(parts[1]); err != nil {
		return errgo.Mask(err)
	}
	if err := p.EntityPath.Name.UnmarshalText(parts[2]); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func (p CredentialPath) MarshalText() ([]byte, error) {
	data := make([]byte, 0, len(p.Cloud)+1+len(p.User)+1+len(p.Name))
	data = append(data, p.Cloud...)
	data = append(data, '/')
	data = append(data, p.User...)
	data = append(data, '/')
	data = append(data, p.Name...)
	return data, nil
}

// GetModel holds parameters for retrieving a model.
type GetModel struct {
	httprequest.Route `httprequest:"GET /v2/model/:User/:Name"`
	EntityPath
}

// DeleteModel holds parameters for deletion of a model.
type DeleteModel struct {
	httprequest.Route `httprequest:"DELETE /v2/model/:User/:Name"`
	EntityPath
}

// NewModel holds parameters for creating a new model.
type NewModel struct {
	httprequest.Route `httprequest:"POST /v2/model/:User"`

	// User holds the User element from the URL path.
	User User `httprequest:",path"`

	// Info holds the information required to create
	// the model.
	Info NewModelInfo `httprequest:",body"`
}

// ListModels holds parameters for listing
// current models.
type ListModels struct {
	httprequest.Route `httprequest:"GET /v2/model"`

	// TODO add parameters for restricting results.
}

// ListModelsResponse holds a list of controllers as returned
// by ListModels.
type ListModelsResponse struct {
	Models []ModelResponse `json:"models"`
}

// ListController holds parameters for listing all current controllers.
type ListController struct {
	httprequest.Route `httprequest:"GET /v2/controller"`

	// TODO add parameters for restricting results.
}

// ListControllerResponse holds a list of controllers as returned
// by ListController.
type ListControllerResponse struct {
	Controllers []ControllerResponse `json:"controllers"`
}

// ControllerResponse holds information on a given Controller.
// Each Controller is also associated with an model
// at /v2/model/:User/:Name where User and Name
// are the same as that of the Controller's path.
type ControllerResponse struct {
	// Path holds the path of the controller.
	Path EntityPath `json:"path"`

	// TODO perhaps these two fields should be put
	// into the same type as returned from GetSchema.

	// ProviderType holds the kind of provider used
	// by the Controller.
	ProviderType string `json:"provider-type,omitempty"`

	// Schema holds the fields required to start
	// a new model using the Controller.
	Schema environschema.Fields `json:"schema,omitempty"`

	// Location holds location attributes associated with the controller.
	Location map[string]string `json:"location,omitempty"`

	// Public holds whether the controller is part of the public
	// pool of controllers.
	Public bool

	// UnavailableSince holds the time that the JEM server
	// noticed that the model's controller could not be
	// contacted. It is empty when the model is available.
	UnavailableSince *time.Time `json:"unavailable-since,omitempty"`
}

// GetController holds parameters for retrieving information on a Controller.
type GetController struct {
	httprequest.Route `httprequest:"GET /v2/controller/:User/:Name"`
	EntityPath
}

// GetSchema holds parameters for getting a schema.
type GetSchema struct {
	httprequest.Route `httprequest:"GET /v2/schema"`

	// Location constrains the controllers that will be used to
	// fetch the schema.
	//
	// Note that the values in this should be passed in the
	// URL query parameters.
	Location map[string]string
}

// SchemaResponse holds the information returned by a GetSchema request.
type SchemaResponse struct {
	// ProviderType holds the kind of provider associated with the schema.
	ProviderType string `json:"provider-type,omitempty"`

	// Schema holds the fields required to start a new model.
	Schema environschema.Fields `json:"schema,omitempty"`
}

// NewModelInfo holds the JSON body parameters
// for a NewModel request.
type NewModelInfo struct {
	// Name holds the name to give to the new model
	// within its user name space.
	Name Name `json:"name"`

	// Controller holds the path to the controller entity
	// to use to start the model.
	// This is optional and may not be available to all user.
	Controller *EntityPath `json:"controller,omitempty"`

	// Location holds location attributes that narrow down the range
	// of possible controllers to be used for the model.
	Location map[string]string `json:"location,omitempty"`

	// Credential holds the name of the provider credential that will
	// be used to provision machines in the new model.
	Credential CredentialPath `json:"credential"`

	// Config holds the configuration attributes to use to create the new model.
	Config map[string]interface{} `json:"config"`
}

// ModelResponse holds the response body from a NewModel call.
type ModelResponse struct {
	// Path holds the path of the model.
	Path EntityPath `json:"path"`

	// UUID holds the UUID of the model.
	UUID string `json:"uuid"`

	// ControllerPath holds the path of the controller holding this model.
	ControllerPath EntityPath `json:"controller-path"`

	// ControllerUUID holds the UUID of the controller's admin UUID.
	ControllerUUID string `json:"controller-uuid"`

	// CACert holds the CA certificate that will be used
	// to validate the controller's certificate, in PEM format.
	CACert string `json:"ca-cert"`

	// HostPorts holds host/port pairs (in host:port form)
	// of the controller API endpoints.
	HostPorts []string `json:"host-ports"`

	// Life holds the last reported lifecycle status of the model.
	// It is omitted when we have no information on the model's
	// life yet. Possible values are "alive", "dying" and "dead".
	Life string `json:"life,omitempty"`

	// UnavailableSince holds the time that the JEM server
	// noticed that the model's controller could not be
	// contacted. It is empty when the model is available.
	UnavailableSince *time.Time `json:"unavailable-since,omitempty"`

	// Counts holds information about the number of various kinds
	// of entities in the model.
	Counts map[EntityCount]Count

	// Creator holds the name of the user that created the model.
	Creator string
}

// WhoAmI holds parameters for requesting the current user name.
type WhoAmI struct {
	httprequest.Route `httprequest:"GET /v2/whoami"`
}

// WhoAmIResponse holds information on the currently
// authenticated user.
type WhoAmIResponse struct {
	User string `json:"user"`
}

// UpdateCredential holds parameters for adding or updating a credential.
type UpdateCredential struct {
	httprequest.Route `httprequest:"PUT /v2/credential/:User/:Cloud/:Name"`
	CredentialPath
	Credential Credential `httprequest:",body"`
}

// Credential holds the details of a credential.
type Credential struct {
	// AuthType holds the authentiction type of the credential. Valid
	// AuthTypes are listed in github.com/juju/juju/cloud/clouds.go.
	AuthType string `json:"auth-type"`

	// Attributes holds the map of attributes that form the
	// credential.
	Attributes map[string]string `json:"attrs,omitempty"`
}

// GetUserStats returns statistics related to the requested user.
type GetUserStats struct {
	httprequest.Route `httprequest:"GET /v2/user/:User/stats"`
	User              string `httprequest:",path"`
}

// UserStats holds per-user statistics as requested by GetUserStats.
type UserStatsResponse struct {
	// Counts holds counts for model entities in models created by
	// the user.
	Counts map[EntityCount]Count
}

// EntityCount represents some kind of entity we
// want count over time.
type EntityCount string

const (
	UnitCount        EntityCount = "units"
	ApplicationCount EntityCount = "applications"
	MachineCount     EntityCount = "machines"
)

// Count records information about a changing count of
// of entities over time.
type Count struct {
	// Time holds the time when the count record was recorded.
	Time time.Time `json:"time"`

	// Current holds the most recent count value,
	// recorded at the above time.
	Current int `json:"current"`

	// MaxCount holds the maximum count recorded.
	Max int `json:"max"`

	// Total holds the total number created over time.
	// This may be approximate if creation events are missed.
	Total int64 `json:"total"`

	// TotalTime holds the total time in milliseconds that any
	// entities have existed for. That is, if two entities have
	// existed for two seconds, this metric will record four
	// seconds.
	TotalTime int64 `json:"total-time"`
}
