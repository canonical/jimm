package params

import "github.com/juju/httprequest"

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

type EnvironmentResponse struct {
	// UUID holds the UUID of the environment.
	UUID string `json:"uuid"`

	// CACert holds the CA certificate that will be used
	// to validate the state server's certificate, in PEM format.
	CACert string `json:"ca-cert"`

	// HostPorts holds host/port pairs (in host:port form)
	// of the state server API endpoints.
	HostPorts []string `json:"host-ports"`
}
