// Copyright 2020 Canonical Ltd.

package params

import (
	"time"

	jujuparams "github.com/juju/juju/rpc/params"
)

// An AddCloudToControllerRequest is the request sent when adding a new cloud
// to a specific controller.
type AddCloudToControllerRequest struct {
	jujuparams.AddCloudArgs

	// ControllerName is the name of the controller to which the
	// cloud should be added.
	ControllerName string `json:"controller-name"`
}

// A RemoveCloudFromControllerRequest is the request sent when removing
// cloud from a specific controller.
type RemoveCloudFromControllerRequest struct {
	// CloudTag is the tag of the cloud this controller is running in.
	CloudTag string `json:"cloud-tag"`
	// ControllerName is the name of the controller from which the
	// cloud should be removed.
	ControllerName string `json:"controller-name"`
}

// An AddControllerRequest is the request sent when adding a new controller
// to JIMM.
type AddControllerRequest struct {
	// Name is the name to give to the controller, all controllers must
	// have a unique name.
	Name string `json:"name"`

	// PublicAddress is the public address of the controller. This is
	// normally a DNS name and port which provide the controller endpoints.
	// This address should not change even if the controller units
	// themselves are migrated.
	PublicAddress string `json:"public-address,omitempty"`

	// APIAddresses contains the currently known API addresses for the
	// controller.
	APIAddresses []string `json:"api-addresses,omitempty"`

	// CACertificate contains the CA certificate to use to validate the
	// connection to the controller. This is not needed if certificate is
	// signed by a public CA.
	CACertificate string `json:"ca-certificate,omitempty"`

	// Username contains the username that JIMM should use to connect to
	// the controller.
	Username string `json:"username"`

	// Password contains the password that JIMM should use to connect to
	// the controller.
	Password string `json:"password"`
}

// AuditLogAccessRequest is the request used to modify a user's access
// to the audit log.
type AuditLogAccessRequest struct {
	// UserTag is the user who's audit-log access is being modified.
	UserTag string `json:"user-tag"`

	// Level is the access level being granted or revoked. The only access
	// level is "read".
	Level string `json:"level"`
}

const (
	// AuditActionCreate is the Action value in an audit entry that
	// creates an entity.
	AuditActionCreate = "create"

	// AuditActionDelete is the Action value in an audit entry that
	// deletes an entity.
	AuditActionDelete = "delete"

	// AuditActionGrant is the Action value in an audit entry that
	// grants access to an entity.
	AuditActionGrant = "grant"

	// AuditActionRevoke is the Action value in an audit entry that
	// revokes access from an entity.
	AuditActionRevoke = "revoke"
)

// An AuditEvent is an event in the audit log.
type AuditEvent struct {
	// Time is the time of the audit event.
	Time time.Time `json:"time"`

	// Tag contains the tag of the entity the event is for.
	Tag string `json:"tag"`

	// UserTag contains the user tag of authenticated user that performed
	// the action.
	UserTag string `json:"user-tag"`

	// Action contains the action that occured on the entity.
	Action string `json:"action"`

	// Success indicates whether the action succeeded, or not.
	Success bool `json:"success"`

	// Params contains additional details for the audit entry. The contents
	// will vary depending on the action and the entity.
	Params map[string]string `json:"params"`
}

// An AuditEvents contains events from the audit log.
type AuditEvents struct {
	Events []AuditEvent `json:"events"`
}

// A ControllerInfo describes a controller on a JIMM system.
type ControllerInfo struct {
	// Name is the name of the controller.
	Name string `json:"name"`

	// UUID is the UUID of the controller.
	UUID string `json:"uuid"`

	// PublicAddress is the public address of the controller. This is
	// normally a DNS name and port which provide the controller endpoints.
	// This address should not change even if the controller units
	// themselves are migrated.
	PublicAddress string `json:"public-address,omitempty"`

	// APIAddresses contains the currently known API addresses for the
	// controller.
	APIAddresses []string `json:"api-addresses,omitempty"`

	// CACertificate contains the CA certificate to use to validate the
	// connection to the controller. This is not needed if certificate is
	// signed by a public CA.
	CACertificate string `json:"ca-certificate,omitempty"`

	// CloudTag is the tag of the cloud this controller is running in.
	CloudTag string `json:"cloud-tag,omitempty"`

	// CloudRegion is the region that this controller is running in.
	CloudRegion string `json:"cloud-region,omitempty"`

	// Username contains the username that JIMM uses to connect to the
	// controller.
	Username string `json:"username"`

	// The version of the juju agent running on the controller.
	AgentVersion string `json:"agent-version"`

	// Status contains the current status of the controller. The status
	// will either be "available", "deprecated", or "unavailable".
	Status jujuparams.EntityStatus `json:"status"`
}

// A FindAuditEventsRequest finds audit events that match the specified
// query.
type FindAuditEventsRequest struct {
	// After is used to filter the event log to only contain events that
	// happened after a certain time. If this is specified it must contain
	// an RFC3339 encoded time value.
	After string `json:"after,omitempty"`

	// Before is used to filter the event log to only contain events that
	// happened before a certain time. If this is specified it must contain
	// an RFC3339 encoded time value.
	Before string `json:"before,omitempty"`

	// Tag is used to filter the event log to only contain events that
	// occured to a particular entity.
	Tag string `json:"tag,omitempty"`

	// UserTag is used to filter the event log to only contain events that
	// were performed by a particular authenticated user.
	UserTag string `json:"user-tag,omitempty"`

	// Action is used to filter the event log to only contain events that
	// perform a particular action.
	Action string `json:"action,omitempty"`

	// Limit is the maximum number of audit events to return.
	Limit int64 `json:"limit,omitempty"`
}

// A ListControllersResponse is the response that is sent in a
// ListControllers method.
type ListControllersResponse struct {
	Controllers []ControllerInfo `json:"controllers"`
}

// A RemoveControllerRequest is the request that is sent in a
// RemoveController method.
type RemoveControllerRequest struct {
	Name  string `json:"name"`
	Force bool   `json:"force"`
}

// A SetControllerDeprecatedRequest is the request this is sent in a
// SetControllerDeprecated method.
type SetControllerDeprecatedRequest struct {
	// Name is the name of the controller to set deprecated.
	Name string `json:"name"`

	// Deprecated specifies whether the controller should be set to
	// deprecated or not.
	Deprecated bool `json:"deprecated"`
}

// FullModelStatusRequest is the request that is sent in a FullModelStatus method.
type FullModelStatusRequest struct {
	ModelTag string
	Patterns []string
}

// UpdateMigratedModelRequest holds a request to check
// if the specified model has been migrated to the specified controller
// and update the model accordingly.
type UpdateMigratedModelRequest struct {
	// ModelTag holds the tag of the model that has been migrated.
	ModelTag string `json:"model-tag"`
	// TargetController holds the name of the controller the
	// model has been migrated to.
	TargetController string `json:"target-controller"`
}

// An ImportModelRequest holds a request to import a model running on the
// specified controller such that the model is known to JIMM.
type ImportModelRequest struct {
	// Controller holds that name of the controller that is running the
	// model.
	Controller string `json:"controller"`

	// ModelTag is the tag of the model that is to be imported.
	ModelTag string `json:"model-tag"`
}

// Authorisation request parameters / responses:

// AddGroupRequest holds a request to add a group.
type AddGroupRequest struct {
	// Name holds the name of the group.
	Name string `json:"name"`
}

// RenameGroupRequest holds a request to rename a group.
type RenameGroupRequest struct {
	// Name holds the name of the group.
	Name string `json:"name"`

	// NewName holds the new name of the group.
	NewName string `json:"new-name"`
}

// RemoveGroupRequest holds a request to remove a group.
type RemoveGroupRequest struct {
	// Name holds the name of the group.
	Name string `json:"name"`
}

// Group holds the details of a group currently residing in JIMM.
type Group struct {
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ListGroupResponse returns the group tuples currently residing within OpenFGA.
type ListGroupResponse struct {
	Groups []Group `json:"name"`
}

// RelationshipTuple represents a OpenFGA Tuple.
type RelationshipTuple struct {
	// Object represents an OFGA object that we wish to apply a relational tuple to.
	Object string `json:"object" yaml:"object"`
	// Relation is exactly that, the kind of relation this request modifies.
	Relation string `json:"relation" yaml:"relation"`
	// TargetObject is the kind of object we wish to create/remove a tuple for/with
	// the provided relation.
	TargetObject string `json:"target_object" yaml:"target_object"`
}

// AddRelationRequest holds the tuples to be added to OpenFGA in an AddRelation request.
type AddRelationRequest struct {
	Tuples []RelationshipTuple `json:"tuples"`
}

// RemoveRelationRequest holds the request information to remove tuples.
type RemoveRelationRequest struct {
	Tuples []RelationshipTuple `json:"tuples"`
}

// CheckRelationRequest holds a tuple containg the object, target object and relation that we wish
// verify authorisation with.
type CheckRelationRequest struct {
	Tuple RelationshipTuple `json:"tuple"`
}

// CheckRelationResponse simple responds with an object containing a boolean of 'allowed' or not
// when a check for access is requested.
type CheckRelationResponse struct {
	Allowed bool `json:"allowed"`
}

// ListRelationshipTuplesRequests holds the request information to list tuples.
type ListRelationshipTuplesRequest struct {
	Tuple             RelationshipTuple `json:"tuple,omitempty"`
	PageSize          int32             `json:"page_size,omitempty"`
	ContinuationToken string            `json:"continuation_token,omitempty"`
}

// ListRelationshipTuplesResponse holds the response of the ListRelationshipTuples method.
type ListRelationshipTuplesResponse struct {
	Tuples            []RelationshipTuple `json:"tuples,omitempty"`
	ContinuationToken string              `json:"continuation_token,omitempty"`
}
