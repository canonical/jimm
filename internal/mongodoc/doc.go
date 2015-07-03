package mongodoc

import (
	"github.com/CanonicalLtd/jem/params"
)

// StateServer holds information on a given state server.
// Each state server also has an entry in the environments
// collection with the same id.
type StateServer struct {
	// Id holds the primary key for a state server.
	// It holds Path.String().
	Id string `bson:"_id"`

	// EntityPath holds the local user and name given to the
	// state server, denormalized from Id for convenience
	// and ease of indexing. Its string value is used as the Id value.
	Path params.EntityPath

	// CACert holds the CA certificate of the server.
	CACert string

	// HostPorts holds the most recently known set
	// of host-port addresses for the API servers,
	// with the most-recently-dialed address at the start.
	HostPorts []string
}

type Environment struct {
	// Id holds the primary key for an environment.
	// It holds Path.String().
	Id string `bson:"_id"`

	// EntityPath holds the local user and name given to the
	// environment, denormalized from Id for convenience
	// and ease of indexing. Its string value is used as the Id value.
	Path params.EntityPath

	// UUID holds the UUID of the environment.
	UUID string

	// AdminUser holds the user name to use
	// when connecting to the state server.
	AdminUser string

	// AdminPassword holds the password for the admin user.
	AdminPassword string

	// StateServer holds the path of the environment's
	// state server.
	StateServer params.EntityPath
}
