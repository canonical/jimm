package mongodoc

import (
	"encoding/base64"

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

	// Location holds the location attributes associated with the controller.
	Location map[string]string
}

type UserInfo struct {
	Password string
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

// Sanitize returns a version of key that's suitable
// for using as a mongo key.
// TODO base64 encoding is probably overkill - we
// could probably do something that left keys
// more readable.
func Sanitize(key string) string {
	return base64.StdEncoding.EncodeToString([]byte(key))
}
