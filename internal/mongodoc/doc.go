package mongodoc

import (
	"github.com/CanonicalLtd/jem/params"
)

// StateServer holds information on a given state server.
// Each state server also has an entry in the environments
// collection with the same id.
type StateServer struct {
	// Id holds the primary key for a state server.
	// It's actually in the form user/name.
	Id   string `bson:"_id"` // Actually user/name.

	// User holds the user name containing the state server.
	User params.User

	// Name holds the local name given to the state
	// server. It is unique within a given user.
	Name params.Name

	// CACert holds the CA certificate of the server.
	CACert string

	// HostPorts holds the most recently known set
	// of host-port addresses for the API servers,
	// with the most-recently-dialed address at the start.
	HostPorts []string
}

type Environment struct {
	Id string `bson:"_id"` // Actually user/name.

	// User holds the user name containing the environment.
	User params.User

	// Name holds the local name given to the environment.
	// It is unique within a given user.
	Name params.Name

	// UUID holds the UUID of the environment.
	UUID string

	// AdminUser holds the user name to use
	// when connecting to the state server.
	AdminUser string

	// AdminPassword holds the password for the admin user.
	AdminPassword string

	// StateServer holds the id of the environment's
	// state server.
	StateServer string
}
