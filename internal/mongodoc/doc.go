// Copyright 2016 Canonical Ltd.

package mongodoc

import (
	"encoding/base64"
	"time"

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

	// Public specifies whether the controller is considered
	// part of the "pool" of publicly available controllers.
	// Non-public controllers will be ignored when selecting
	// controllers by location.
	Public bool `bson:",omitempty"`

	// ProviderType holds the type of the juju provider that the
	// controller is using.
	ProviderType string

	// MonitorLeaseOwner holds the name of the agent
	// currently responsible for monitoring the controller.
	MonitorLeaseOwner string

	// MonitorLeaseExpiry holds the time at which the
	// current monitor's lease expires.
	MonitorLeaseExpiry time.Time

	// Stats holds runtime information about the controller.
	Stats ControllerStats

	// UnavailableSince is zero when the controller is marked
	// as available; otherwise it holds the time when it became
	// unavailable.
	UnavailableSince time.Time `bson:",omitempty"`
}

// ControllerStats holds statistics about a controller.
type ControllerStats struct {
	// UnitCount holds the number of units hosted in the controller.
	UnitCount int

	// ModelCount holds the number of models hosted in the controller.
	ModelCount int

	// ServiceCount holds the number of services hosted in the controller.
	ServiceCount int

	// MachineCount holds the number of machines hosted in the controller.
	// This includes all machines, not just top level instances.
	MachineCount int
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

	// Life holds the current life status of the model ("alive", "dying"
	// or "dead").
	Life string

	// TODO record last time we saw changes on the model?
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

	// Location holds the location attributes associated with the template.
	Location map[string]string `bson:",omitempty"`
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
