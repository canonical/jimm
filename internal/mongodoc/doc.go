package mongodoc

import (
	"gopkg.in/juju/environschema.v1"

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

	// HostPorts holds the most recently known set
	// of host-port addresses for the API servers,
	// with the most-recently-dialed address at the start.
	HostPorts []string
}

func (s *Controller) Owner() params.User {
	return s.Path.User
}

func (s *Controller) GetACL() params.ACL {
	return s.ACL
}

type Model struct {
	// Id holds the primary key for an model.
	// It holds Path.String().
	Id string `bson:"_id"`

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

	// AdminPassword holds the password for the admin user.
	AdminPassword string

	// Controller holds the path of the model's
	// controller.
	Controller params.EntityPath
}

func (e *Model) Owner() params.User {
	return e.Path.User
}

func (e *Model) GetACL() params.ACL {
	return e.ACL
}

type Template struct {
	// Id holds the primary key for a template.
	// It holds Path.String().
	Id string `bson:"_id"`

	// EntityPath holds the local user and name given to the
	// template, denormalized from Id for convenience
	// and ease of indexing. Its string value is used as the Id value.
	Path params.EntityPath

	// ACL holds permissions for the model.
	ACL params.ACL

	// Schema holds the schema used to create the template.
	Schema environschema.Fields

	// Config holds the configuration attributes associated with template.
	Config map[string]interface{}
}

func (t *Template) Owner() params.User {
	return t.Path.User
}

func (t *Template) GetACL() params.ACL {
	return t.ACL
}
