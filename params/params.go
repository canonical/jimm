package params

import "github.com/juju/httprequest"

// AddJES holds the parameters for adding a new state server.
type AddJES struct {
	httprequest.Route `httprequest:"PUT /v1/u/:User/server/:Name"`
	User              User       `httprequest:",path"`
	Name              Name       `httprequest:",path"`
	Info              ServerInfo `httprequest:",body"`
}

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

	// EnvironUUID holds the environ tag for the environment we are
	// trying to connect to.
	EnvironUUID string `json:"environ-uuid"`
}
