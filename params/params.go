package params

import (
	"bytes"
	"fmt"

	"github.com/juju/httprequest"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/environschema.v1"
)

// SetStateServerPerm holds the parameters for setting the ACL
// on a state server.
type SetStateServerPerm struct {
	httprequest.Route `httprequest:"PUT /v1/server/:User/:Name/perm"`
	EntityPath
	ACL ACL `httprequest:",body"`
}

// GetStateServerPerm holds the parameters for getting the ACL
// of a state server.
type GetStateServerPerm struct {
	httprequest.Route `httprequest:"GET /v1/server/:User/:Name/perm"`
	EntityPath
}

// SetEnvironmentPerm holds the parameters for setting the ACL
// on an environment.
type SetEnvironmentPerm struct {
	httprequest.Route `httprequest:"PUT /v1/env/:User/:Name/perm"`
	EntityPath
	ACL ACL `httprequest:",body"`
}

// GetEnvironmentPerm holds the parameters for getting the ACL
// of an environment.
type GetEnvironmentPerm struct {
	httprequest.Route `httprequest:"GET /v1/env/:User/:Name/perm"`
	EntityPath
}

// SetTemplatePerm holds the parameters for setting the ACL
// on a template.
type SetTemplatePerm struct {
	httprequest.Route `httprequest:"PUT /v1/template/:User/:Name/perm"`
	EntityPath
	ACL ACL `httprequest:",body"`
}

// GetTemplatePerm holds the parameters for getting the ACL
// on a template.
type GetTemplatePerm struct {
	httprequest.Route `httprequest:"GET /v1/template/:User/:Name/perm"`
	EntityPath
}

// ACL holds an access control list for an entity.
type ACL struct {
	// Read holds users and groups that are allowed to read the
	// entity.
	Read []string `json:"read"`
}

// AddJES holds the parameters for adding a new state server.
type AddJES struct {
	httprequest.Route `httprequest:"PUT /v1/server/:User/:Name"`
	EntityPath
	Info ServerInfo `httprequest:",body"`
}

// DeleteJES holds the parameters for removing the JES.
type DeleteJES struct {
	httprequest.Route `httprequest:"DELETE /v1/server/:User/:Name"`
	EntityPath
}

// ServerInfo holds information specifying how
// to connect to an existing state server.
type ServerInfo struct {
	// HostPorts holds host/port pairs (in host:port form)
	// of the state server API endpoints.
	HostPorts []string `json:"host-ports"`

	// CACert holds the CA certificate that will be used
	// to validate the state server's certificate, in PEM format.
	CACert string `json:"ca-cert"`

	// User holds the name of user to use when
	// connecting to the server.
	User string `json:"user"`

	// Password holds the password for the user.
	Password string `json:"password"`

	// EnvironUUID holds the environ UUID of the environment we are
	// trying to connect to.
	EnvironUUID string `json:"environ-uuid"`
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

var slash = []byte("/")

func (p *EntityPath) UnmarshalText(data []byte) error {
	parts := bytes.Split(data, slash)
	if len(parts) != 2 {
		return errgo.New("wrong number of parts in entity path")
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

// GetEnvironment holds parameters for retrieving
// an environment.
type GetEnvironment struct {
	httprequest.Route `httprequest:"GET /v1/env/:User/:Name"`
	EntityPath
}

// DeleteEnvironment holds parameters for deletion of
// an environment.
type DeleteEnvironment struct {
	httprequest.Route `httprequest:"DELETE /v1/env/:User/:Name"`
	EntityPath
}

// NewEnvironment holds parameters for creating a new enviromment.
type NewEnvironment struct {
	httprequest.Route `httprequest:"POST /v1/env/:User"`

	// User holds the User element from the URL path.
	User User `httprequest:",path"`

	// Info holds the information required to create
	// the environment.
	Info NewEnvironmentInfo `httprequest:",body"`
}

// ListEnvironments holds parameters for listing
// current environments.
type ListEnvironments struct {
	httprequest.Route `httprequest:"GET /v1/env"`

	// TODO add parameters for restricting results.
}

// ListEnvironmentsResponse holds a list of state servers as returned
// by ListEnvironments.
type ListEnvironmentsResponse struct {
	Environments []EnvironmentResponse `json:"environments"`
}

// ListJES holds parameters for listing all current state servers.
type ListJES struct {
	httprequest.Route `httprequest:"GET /v1/server"`

	// TODO add parameters for restricting results.
}

// ListJESResponse holds a list of state servers as returned
// by ListJES.
type ListJESResponse struct {
	// TODO factor out common items in the templates
	// into a separate field.
	StateServers []JESResponse `json:"state-servers"`
}

// ListTemplate holds parameters for listing all current templates.
type ListTemplates struct {
	httprequest.Route `httprequest:"GET /v1/template"`

	// TODO add parameters for restricting results.
}

// ListTemplatesResponse holds a list of templates as returned
// by ListTemplates.
type ListTemplatesResponse struct {
	Templates []TemplateResponse `json:"templates"`
}

// TemplateResponse holds information on a template
type TemplateResponse struct {
	// Path holds the path of the template.
	Path EntityPath `json:"path"`

	// Schema holds the state server schema that was used
	// to create the template.
	Schema environschema.Fields `json:"schema"`

	// Config holds the template's attributes, with all secret attributes
	// replaced with their zero value.
	Config map[string]interface{} `json:"config"`
}

// GetTemplate holds parameters for retrieving information on a template.
type GetTemplate struct {
	httprequest.Route `httprequest:"GET /v1/template/:User/:Name"`
	EntityPath
}

// DeleteTemplate holds parameters for deletion of a template.
type DeleteTemplate struct {
	httprequest.Route `httprequest:"DELETE /v1/template/:User/:Name"`
	EntityPath
}

// JESResponse holds information on a given JES.
// Each JES is also associated with an environment
// at /v1/env/:User/:Name where User and Name
// are the same as that of the JES's path.
type JESResponse struct {
	// Path holds the path of the state server.
	Path EntityPath `json:"path"`

	// ProviderType holds the kind of provider used
	// by the JES.
	ProviderType string `json:"provider-type,omitempty"`

	// Schema holds the fields required to start
	// a new environment using the JES.
	Schema environschema.Fields `json:"schema,omitempty"`
}

// GetJES holds parameters for retrieving information on a JES.
type GetJES struct {
	httprequest.Route `httprequest:"GET /v1/server/:User/:Name"`
	EntityPath
}

// NewEnvironmentInfo holds the JSON body parameters
// for a NewEnvironment request.
type NewEnvironmentInfo struct {
	// Name holds the name to give to the new environment
	// within its user name space.
	Name Name `json:"name"`

	// Password holds the password to associate with the
	// creating user if their account does not already
	// exist on the state server. If the user has already
	// been created, this is ignored.
	// TODO when juju-core supports macaroon authorization,
	// this can be removed.
	Password string `json:"password"`

	// StateServer holds the path to the state server entity
	// to use to start the environment.
	// TODO use attributes to automatically work out which state server to use.
	StateServer EntityPath `json:"state-server"`

	// TemplatePaths optionally holds a sequence of templates to use
	// to create the base configuration entry on top of which Config is applied.
	// Each path must refer to an entry in /template, and overrides attributes in the one before it,
	// followed finally by Config itself. The resulting configuration
	// is checked for compatibility with the schema of the above state server.
	TemplatePaths []EntityPath `json:"templates,omitempty"`

	// Config holds the configuration attributes to use to create the new environment.
	// It is applied on top of the above templates.
	Config map[string]interface{} `json:"config"`
}

// EnvironmentResponse holds the response body from
// a NewEnvironment call.
type EnvironmentResponse struct {
	// Path holds the path of the environment.
	Path EntityPath `json:"path"`

	// User holds the admin user name associated
	// with the environment.
	User string `json:"user"`

	// Password holds the admin password associated with the
	// environment. If it is empty, macaroon authentication should
	// be used to connect to the environment (only possible when
	// macaroon authentication is implemented by Juju).
	Password string `json:"password"`

	// UUID holds the UUID of the environment.
	UUID string `json:"uuid"`

	// ServerUUID holds the UUID of the state server
	// environment containing this environment.
	ServerUUID string `json:"server-uuid"`

	// CACert holds the CA certificate that will be used
	// to validate the state server's certificate, in PEM format.
	CACert string `json:"ca-cert"`

	// HostPorts holds host/port pairs (in host:port form)
	// of the state server API endpoints.
	HostPorts []string `json:"host-ports"`
}

// AddTemplate holds parameters for adding a template.
type AddTemplate struct {
	httprequest.Route `httprequest:"PUT /v1/template/:User/:Name"`
	EntityPath

	Info AddTemplateInfo `httprequest:",body"`
}

// AddTemplateInfo holds information on a template to
// be added.
type AddTemplateInfo struct {
	// StateServer holds the name of a state server to use
	// as the base schema for the template. The Config attributes
	// below will be checked against the schema of this state
	// server.
	StateServer EntityPath `json:"state-server"`

	// Config holds the template's configuration attributes.
	Config map[string]interface{} `json:"config"`
}
