package params

import (
	"github.com/juju/httprequest"
	"gopkg.in/juju/environschema.v1"
)

// AddJES holds the parameters for adding a new state server.
type AddJES struct {
	httprequest.Route `httprequest:"PUT /v1/u/:User/server/:Name"`
	EntityPath
	Info ServerInfo `httprequest:",body"`
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
// an entity in the API.
type EntityPath struct {
	User User `httprequest:",path"`
	Name Name `httprequest:",path"`
}

// GetEnvironment holds parameters for retrieving
// an environment.
type GetEnvironment struct {
	httprequest.Route `httprequest:"GET /v1/u/:User/env/:Name"`
	EntityPath
}

// NewEnvironment holds parameters for creating a new enviromment.
type NewEnvironment struct {
	httprequest.Route `httprequest:"POST /v1/u/:User/env"`

	// User holds the User element from the URL path.
	User User `httprequest:",path"`

	// Info holds the information required to create
	// the environment.
	Info NewEnvironmentInfo `httprequest:",body"`
}

// JESInfo holds information on a given JES.
// Each JES is also associated with an environment
// at /u/:User/env/:Name where User and Name
// are the same as that of the JES's path.
type JESInfo struct {
	// ProviderType holds the kind of provider used
	// by the JES.
	ProviderType string `json:"provider-type"`

	// Template holds the fields required to start
	// a new environment using the JES.
	Template environschema.Fields `json:"template"`
}

// GetJES holds parameters for retrieving information on a JES.
type GetJES struct {
	httprequest.Route `httprequest:"GET /v1/u/:User/server/:Name"`
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
	// to use to start the environment, relative to
	// the API version; for example "jaas/server/ec2-eu-west".
	// TODO use attributes to automatically work out which state server
	// to use.
	StateServer string `json:"state-server"`

	// Config holds the configuration attributes to use to create the new environment.
	Config map[string]interface{} `json:"config"`
}

// EnvironmentResponse holds the response body from
// a NewEnvironment call.
type EnvironmentResponse struct {
	// UUID holds the UUID of the environment.
	UUID string `json:"uuid"`

	// ServerUUID holds the UUID of the state server
	// environment containing this environment.
	ServerUUID  string `json:"server-uuid"`

	// CACert holds the CA certificate that will be used
	// to validate the state server's certificate, in PEM format.
	CACert string `json:"ca-cert"`

	// HostPorts holds host/port pairs (in host:port form)
	// of the state server API endpoints.
	HostPorts []string `json:"host-ports"`
}
