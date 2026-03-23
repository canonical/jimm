// Copyright 2026 Canonical.

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

// An AddModelToControllerRequest is the request sent when adding
// a new model to a specific controller.
type AddModelToControllerRequest struct {
	jujuparams.ModelCreateArgs

	// ControllerName is the name of the controller to which the
	// model should be added.
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
	// UUID of the controller.
	UUID string `json:"uuid"`

	// Name is the name to give to the controller, all controllers must
	// have a unique name.
	Name string `json:"name"`

	// PublicAddress is the public address of the controller. This is
	// normally a DNS name and port which provide the controller endpoints.
	// This address should not change even if the controller units
	// themselves are migrated.
	PublicAddress string `json:"public-address,omitempty"`

	// TLSHostname is the hostname used for TLS verification.
	TLSHostname string `json:"tls-hostname,omitempty"`

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
	Time time.Time `json:"time" yaml:"time"`

	// ConversationId contains a unique ID per websocket request.
	ConversationId string `json:"conversation-id" yaml:"conversation-id"`

	// MessageId represents the message ID used to correlate request/responses.
	MessageId uint64 `json:"message-id" yaml:"message-id"`

	// FacadeName contains the request facade name.
	FacadeName string `json:"facade-name,omitempty" yaml:"facade-name,omitempty"`

	// FacadeMethod contains the specific method to be executed on the facade.
	FacadeMethod string `json:"facade-method,omitempty" yaml:"facade-method,omitempty"`

	// FacadeVersion contains the requested version for the facade method.
	FacadeVersion int `json:"facade-version,omitempty" yaml:"facade-version,omitempty"`

	// ObjectId contains the object id to act on, only used by certain facades.
	ObjectId string `json:"object-id,omitempty" yaml:"object-id,omitempty"`

	// UserTag contains the user tag of authenticated user that performed
	// the action.
	UserTag string `json:"user-tag,omitempty" yaml:"user-tag,omitempty"`

	// Model contains the name of the model the event was performed against.
	Model string `json:"model,omitempty" yaml:"model,omitempty"`

	// IsResponse indicates whether the message is a request/response.
	IsResponse bool `json:"is-response" yaml:"is-response"`

	// Params contains client request parameters.
	Params map[string]any `json:"params,omitempty" yaml:"params,omitempty"`

	// Errors contains error info received from the controller.
	Errors map[string]any `json:"errors,omitempty" yaml:"errors,omitempty"`
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

	// UserTag is used to filter the event log to only contain events that
	// were performed by a particular authenticated user.
	UserTag string `json:"user-tag,omitempty"`

	// Model is used to filter the event log to only contain events that
	// were performed against a specific model.
	Model string `json:"model,omitempty"`

	// Method is used to filter the event log to only contain events that
	// called a specific facade method.
	Method string `json:"method,omitempty"`

	// Offset is the number of items to offset the set of returned results.
	Offset int `json:"offset,omitempty"`

	// Limit is the maximum number of audit events to return.
	Limit int `json:"limit,omitempty"`

	// SortTime will sort by most recent (time descending) when true.
	// When false no explicit ordering will be applied.
	SortTime bool `json:"sortTime,omitempty"`
}

// A ListControllersResponse is the response that is sent in a
// ListControllers method.
type ListControllersResponse struct {
	Controllers []ControllerInfo `json:"controllers" yaml:"controllers"`
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

// ControllerProfile stores reusable, non-secret controller bootstrap settings.
type ControllerProfile struct {
	// Name is the profile's name and must be unique across all profiles.
	Name string `json:"name" yaml:"name"`
	// Description is an optional human-readable summary of the profile.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// JujuVersion is the Juju version(s) this profile is intended for e.g. 3, 3.6, 3.6.1.
	JujuVersion string `json:"juju-version" yaml:"juju-version"`
	// Version must be provided when updating an existing profile.
	Version uint `json:"version" yaml:"version"`
	// CreatedAt is the time the profile was first created.
	CreatedAt string `json:"created-at,omitempty" yaml:"created-at,omitempty"`
	// UpdatedAt is the time the profile was last updated.
	UpdatedAt string `json:"updated-at,omitempty" yaml:"updated-at,omitempty"`
	// Cloud stores the cloud definition for the profile.
	Cloud ControllerProfileCloud `json:"cloud" yaml:"cloud"`
	// BootstrapOptions holds the reusable bootstrap settings saved in the profile.
	BootstrapOptions BootstrapOptions `json:"bootstrap-options" yaml:"bootstrap-options"`
}

// ControllerProfileSummary contains the fields returned when listing controller profiles.
type ControllerProfileSummary struct {
	// Name is the profile's name and must be unique across all profiles.
	Name string `json:"name" yaml:"name"`
	// Description is an optional human-readable summary of the profile.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// CreatedAt is the time the profile was first created.
	CreatedAt string `json:"created-at,omitempty" yaml:"created-at,omitempty"`
	// UpdatedAt is the time the profile was last updated.
	UpdatedAt string `json:"updated-at,omitempty" yaml:"updated-at,omitempty"`
}

// ControllerProfileCloud stores the cloud definition persisted in a controller profile.
type ControllerProfileCloud struct {
	// Name is the cloud's name, e.g. "aws", "azure", "gcp", "localhost", etc.
	Name string `json:"name" yaml:"name"`
	// Type is the cloud's type, e.g. "ec2", "azure", "openstack", etc.
	Type string `json:"type,omitempty" yaml:"type,omitempty"`
	// AuthTypes contains the supported cloud auth types.
	AuthTypes []string `json:"auth-types,omitempty" yaml:"auth-types,omitempty"`
	// CACertificates contains the cloud CA certificates.
	CACertificates []string `json:"ca-certificates,omitempty" yaml:"ca-certificates,omitempty"`
	// Config contains cloud-specific configuration.
	Config map[string]any `json:"config,omitempty" yaml:"config,omitempty"`
	// Endpoint contains the cloud API endpoint, if needed.
	Endpoint string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	// HostCloudRegion contains the host cloud region for the cloud, if any.
	HostCloudRegion string `json:"host-cloud-region,omitempty" yaml:"host-cloud-region,omitempty"`
	// Region contains the cloud region definition for the profile's bootstrap region.
	Region ControllerProfileCloudRegion `json:"region" yaml:"region"`
}

// ControllerProfileCloudRegion stores the single bootstrap region definition for a profile.
type ControllerProfileCloudRegion struct {
	// Name is the region's name, e.g. "us-east-1".
	Name string `json:"name" yaml:"name"`
	// Endpoint contains the region-specific cloud API endpoint, if needed.
	Endpoint string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	// IdentityEndpoint contains the region-specific cloud identity API endpoint, if needed.
	IdentityEndpoint string `json:"identity-endpoint,omitempty" yaml:"identity-endpoint,omitempty"`
	// StorageEndpoint contains the region-specific cloud storage API endpoint, if needed.
	StorageEndpoint string `json:"storage-endpoint,omitempty" yaml:"storage-endpoint,omitempty"`
}

// BootstrapOptions stores the supported bootstrap settings shared by
// controller profiles and bootstrap requests.
type BootstrapOptions struct {
	// BootstrapBase specifies the base of the bootstrap machine, e.g. "ubuntu@24.04".
	BootstrapBase string `json:"bootstrap-base,omitempty" yaml:"bootstrap-base,omitempty"`
	// BootstrapConstraints specifies bootstrap machine constraints.
	BootstrapConstraints map[string]string `json:"bootstrap-constraints,omitempty" yaml:"bootstrap-constraints,omitempty"`
	// ModelConstraints sets the default constraints for workload machines in the controller model.
	ModelConstraints map[string]string `json:"model-constraints,omitempty" yaml:"model-constraints,omitempty"`
	// ModelDefault specifies default model configuration values.
	ModelDefault map[string]string `json:"model-default,omitempty" yaml:"model-default,omitempty"`
	// StoragePool holds the options for an initial storage pool created in the controller model.
	StoragePool *BootstrapStoragePool `json:"storage-pool,omitempty" yaml:"storage-pool,omitempty"`
	// BootstrapConfig holds bootstrap configuration values.
	BootstrapConfig map[string]string `json:"bootstrap-config,omitempty" yaml:"bootstrap-config,omitempty"`
	// ControllerConfig holds controller configuration.
	ControllerConfig map[string]string `json:"controller-config,omitempty" yaml:"controller-config,omitempty"`
	// ControllerModelConfig holds model configuration values that apply only to the controller model.
	ControllerModelConfig map[string]string `json:"controller-model-config,omitempty" yaml:"controller-model-config,omitempty"`
}

// BootstrapStoragePool stores the optional storage pool configuration used by
// bootstrap settings.
type BootstrapStoragePool struct {
	// Name is the storage pool name and is required.
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// Type is the storage pool type and is required.
	Type string `json:"type,omitempty" yaml:"type,omitempty"`
	// Attributes holds additional storage pool attributes.
	Attributes map[string]string `json:"attributes,omitempty" yaml:"attributes,omitempty"`
}

// SaveControllerProfileRequest saves or replaces a named controller profile.
type SaveControllerProfileRequest struct {
	ControllerProfile
}

// SaveControllerProfileResponse contains the saved controller profile.
type SaveControllerProfileResponse struct {
	ControllerProfile
}

// GetControllerProfileRequest retrieves a controller profile by name.
type GetControllerProfileRequest struct {
	Name string `json:"name" yaml:"name"`
}

// GetControllerProfileResponse contains a single controller profile.
type GetControllerProfileResponse struct {
	ControllerProfile
}

// ListControllerProfilesRequest lists saved controller profiles and can filter
// them by Juju version.
type ListControllerProfilesRequest struct {
	// JujuVersion allows clients to filter profiles to those appropriate for the
	// Juju version(s) they are intending to use. The filter matches profiles that
	// specify a Juju version that is a prefix of the provided version. For example,
	// a filter value of "3.6.7" will show profiles with juju-version set to "3",
	// "3.6", and "3.6.7" but not "3.6.5", or "3.7", or "4".
	JujuVersion string `json:"juju-version,omitempty" yaml:"juju-version,omitempty"`
}

// ListControllerProfilesResponse contains the summary fields for all saved controller profiles.
type ListControllerProfilesResponse struct {
	Profiles []ControllerProfileSummary `json:"profiles" yaml:"profiles"`
}

// RemoveControllerProfileRequest removes a controller profile by name.
type RemoveControllerProfileRequest struct {
	Name string `json:"name" yaml:"name"`
}

// UpgradeToRequest holds the parameters for phase 1 for automated upgrades.
type UpgradeToRequest struct {
	// ModelTag is the tag of the model to upgrade.
	ModelTag string `json:"model-tag"`
	// TargetControllerName is the target controller's name to upgrade to.
	TargetControllerName string `json:"target-controller-name"`
}

// UpgradeToResponse holds the response for phase 1 of an automated upgrade.
type UpgradeToResponse struct {
	Success bool `json:"success"`
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

	// Owner specifies the new owner of the model after import.
	// Can be empty to skip switching the owner.
	Owner string `json:"owner"`
}

// Authorisation request parameters / responses:

// AddGroupRequest holds a request to add a group.
type AddGroupRequest struct {
	// Name holds the name of the group.
	Name string `json:"name"`
}

// AddGroupResponse holds the details of the added group.
type AddGroupResponse struct {
	Group
}

// GetGroupRequest holds a request to get a group by UUID or name.
type GetGroupRequest struct {
	// UUID holds the UUID of the group to be retrieved.
	UUID string `json:"uuid"`
	// Name holds the name of the group to be retrieved.
	Name string `json:"name"`
}

// GetGroupResponse holds the details of the group.
type GetGroupResponse struct {
	Group
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

type ListGroupsRequest struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// Group holds the details of a group currently residing in JIMM.
type Group struct {
	UUID      string `json:"uuid" yaml:"uuid"`
	Name      string `json:"name" yaml:"name"`
	CreatedAt string `json:"created_at" yaml:"created_at"`
	UpdatedAt string `json:"updated_at" yaml:"updated_at"`
}

// ListGroupResponse returns the group tuples currently residing within OpenFGA.
type ListGroupResponse struct {
	Groups []Group `json:"name" yaml:"name"`
}

// RelationshipTuple represents a OpenFGA Tuple.
type RelationshipTuple struct {
	// Object represents an OFGA object that we wish to apply a relational tuple to.
	Object string `yaml:"object" json:"object"`
	// Relation is exactly that, the kind of relation this request modifies.
	Relation string `yaml:"relation" json:"relation"`
	// TargetObject is the kind of object we wish to create/remove a tuple for/with
	// the provided relation.
	TargetObject string `yaml:"target_object" json:"target_object"`
}

// AddRelationRequest holds the tuples to be added to OpenFGA in an AddRelation request.
type AddRelationRequest struct {
	Tuples []RelationshipTuple `yaml:"tuples" json:"tuples"`
}

// RemoveRelationRequest holds the request information to remove tuples.
type RemoveRelationRequest struct {
	Tuples []RelationshipTuple `json:"tuples"`
}

// CheckRelationRequest holds a tuple containing the object, target object and relation that we wish
// verify authorisation with.
type CheckRelationRequest struct {
	Tuple RelationshipTuple `json:"tuple"`
}

// CheckRelationResponse simple responds with an object containing a boolean of 'allowed' or not
// when a check for access is requested.
type CheckRelationResponse struct {
	Allowed bool   `json:"allowed" yaml:"allowed"`
	Error   string `json:"error,omitempty" yaml:"error,omitempty"`
}

// CheckRelationsRequest holds the tuples containing the object, target object and relation that we wish
// verify authorisation with.
type CheckRelationsRequest struct {
	Tuples []RelationshipTuple `json:"tuples"`
}

// CheckRelationResponse simple responds with an object containing a boolean of 'allowed' or not
// when a check for access is requested.
type CheckRelationsResponse struct {
	Results []CheckRelationResponse `json:"results" yaml:"results"`
}

// ListRelationshipTuplesRequests holds the request information to list tuples.
type ListRelationshipTuplesRequest struct {
	Tuple             RelationshipTuple `json:"tuple,omitempty"`
	PageSize          int32             `json:"page_size,omitempty"`
	ContinuationToken string            `json:"continuation_token,omitempty"`
	ResolveUUIDs      bool              `json:"resolve_uuids,omitempty"`
}

// ListRelationshipTuplesResponse holds the response of the ListRelationshipTuples method.
type ListRelationshipTuplesResponse struct {
	Tuples            []RelationshipTuple `json:"tuples,omitempty" yaml:"tuples,omitempty"`
	Errors            []string            `json:"errors,omitempty" yaml:"errors,omitempty"`
	ContinuationToken string              `json:"continuation_token,omitempty" yaml:"continuation_token,omitempty"`
}

// Role request parameters / responses:

// AddRoleRequest holds a request to add a role.
type AddRoleRequest struct {
	// Name holds the name of the role.
	Name string `json:"name"`
}

// AddRoleResponse holds the details of the added role.
type AddRoleResponse struct {
	Role
}

// GetRoleRequest holds a request to get a role by UUID or name.
type GetRoleRequest struct {
	// UUID holds the UUID of the role to be retrieved.
	UUID string `json:"uuid"`
	// Name holds the name of the role to be retrieved.
	Name string `json:"name"`
}

// GetRoleResponse holds the details of the role.
type GetRoleResponse struct {
	Role
}

// RenameRoleRequest holds a request to rename a role.
type RenameRoleRequest struct {
	// Name holds the name of the role.
	Name string `json:"name"`
	// NewName holds the new name of the role.
	NewName string `json:"new-name"`
}

// RemoveRoleRequest holds a request to remove a role.
type RemoveRoleRequest struct {
	// Name holds the name of the role.
	Name string `json:"name"`
}

// ListRolesRequest holds a request to list roles.
type ListRolesRequest struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// Role holds the details of a role currently.
type Role struct {
	UUID      string `json:"uuid" yaml:"uuid"`
	Name      string `json:"name" yaml:"name"`
	CreatedAt string `json:"created_at" yaml:"created_at"`
	UpdatedAt string `json:"updated_at" yaml:"updated_at"`
}

// ListRoleResponse contains a list of roles
type ListRoleResponse struct {
	Roles []Role `json:"name" yaml:"name"`
}

// CrossModelQueryRequest holds the parameters to perform a cross model query against
// JSON model statuses for every model this user has access to.
type CrossModelQueryRequest struct {
	Type  string `json:"type"`
	Query string `json:"query"`
}

// CrossModelJqQueryResponse holds results for a cross-model query that has been filtered utilising JQ.
// It has two fields:
//   - Results - A map of each iterated JQ output result. The key for this map is the model UUID.
//   - Errors - A map of each iterated JQ *or* Status call error. The key for this map is the model UUID.
type CrossModelQueryResponse struct {
	Results map[string][]any    `json:"results" yaml:"results"`
	Errors  map[string][]string `json:"errors" yaml:"errors"`
}

// PurgeLogsRequest is the request used to purge logs.
type PurgeLogsRequest struct {
	// Date is the date before which logs should be purged.
	Date time.Time `json:"date"`
}

// PurgeLogsResponse is the response returned by the PurgeLogs method.
// It has one field:
// - DeletedCount - the number of logs that were deleted.
type PurgeLogsResponse struct {
	DeletedCount int64 `json:"deleted-count" yaml:"deleted-count"`
}

// MigrateModelInfo represents a single migration where a source model
// target controller must be specified with both the source model and
// target controller residing within JIMM.
type MigrateModelInfo struct {
	// TargetModelNameOrUUID can be either the model name or model UUID.
	TargetModelNameOrUUID string `json:"model-tag"`
	// TargetController is the controller name of the form "<name>"
	TargetController string `json:"target-controller"`
}

// MigrateModelRequest allows for multiple migration requests to be made.
type MigrateModelRequest struct {
	Specs []MigrateModelInfo `json:"specs"`
}

// LoginDeviceResponse holds the details to complete a LoginDevice flow.
type LoginDeviceResponse struct {
	// VerificationURI holds the URI that the user must navigate to
	// when entering their "user-code" to consent to this authorisation
	// request.
	VerificationURI string `json:"verification-uri" yaml:"verification-uri"`
	// UserCode holds the one-time use user consent code.
	UserCode string `json:"user-code" yaml:"user-code"`
}

// GetDeviceSessionTokenResponse returns a session token to be used against
// LoginWithSessionToken for authentication. The session token will be base64
// encoded.
type GetDeviceSessionTokenResponse struct {
	// SessionToken is a base64 encoded JWT capable of authenticating
	// a user. The JWT contains the users email address in the subject,
	// and this is used to identify this user.
	SessionToken string `json:"session-token" yaml:"session-token"`
}

// LoginWithSessionTokenRequest accepts a session token minted by JIMM and logs
// the user in.
//
// The login response for this login request type is that of jujuparams.LoginResult,
// such that the behaviour of previous macroon based authentication is unchanged.
// However, on unauthenticated requests, the error is different and is not a macaroon
// discharge request.
type LoginWithSessionTokenRequest struct {
	// SessionToken is a base64 encoded JWT capable of authenticating
	// a user. The JWT contains the users email address in the subject,
	// and this is used to identify this user.
	SessionToken string `json:"session-token"`
}

// Service Account related request parameters

// LoginWithClientCredentialsRequest holds the client id and secret used
// to authenticate with JIMM.
type LoginWithClientCredentialsRequest struct {
	ClientID     string `json:"client-id"`
	ClientSecret string `json:"client-secret"`
}

// WhoamiResponse holds the response for a /auth/whoami call.
type WhoamiResponse struct {
	DisplayName string `json:"display-name" yaml:"display-name"`
	Email       string `json:"email" yaml:"email"`
}

// VersionResponse holds the response for a version call.
type VersionResponse struct {
	Version string `json:"version" yaml:"version"`
	Commit  string `json:"commit" yaml:"commit"`
}

// PrepareModelMigrationRequest holds the details to prepare JIMM
// for a model migration.
type PrepareModelMigrationRequest struct {
	ModelTag              string            `json:"model-tag" yaml:"model-tag"`
	BackingControllerName string            `json:"backing-controller-name" yaml:"backing-controller-name"`
	UserMapping           map[string]string `json:"user-mapping" yaml:"user-mapping"`
}

// PrepareModelMigrationResponse holds the response for a model migration.
type PrepareModelMigrationResponse struct {
	// Token is the token that should be used to initiate the migration
	// as it allows the source controller to authenticate with JIMM.
	Token string `json:"token" yaml:"token"`
}

// ListMigrationTargetsRequest holds the model to query for controllers
// that are valid targets for an internal model migration.
type ListMigrationTargetsRequest struct {
	// ModelTag holds the tag of the model.
	ModelTag string `json:"model-tag"`
}

// JobStatus represents the status of a job.
type JobStatus string

const (
	StatusRunning    JobStatus = "running"
	StatusSuccessful JobStatus = "successful"
	StatusPending    JobStatus = "pending"
	StatusFailed     JobStatus = "failed"
	StatusUnknown    JobStatus = "unknown"
)

// GetBootstrapInfoRequest holds the request to get the status
// of a bootstrap operation.
type GetBootstrapInfoRequest struct {
	// JobID is the ID of the job to get the status for.
	JobID string `json:"job-id"`
	// Watermark is the line number to start reading logs from.
	Watermark int `json:"watermark"`
}

// GetBootstrapInfoResponse holds the status of a bootstrap job.
type GetBootstrapInfoResponse struct {
	// Status is the status of the job.
	Status JobStatus `json:"status"`
	// Logs are the logs for the job.
	Logs []string `json:"logs"`
	// Watermark is the line number to use for the next request.
	Watermark int `json:"watermark"`
	// Error is the error message if the job failed.
	Error string `json:"error,omitempty"`
}

// StopBootstrapRequest holds the request to stop a bootstrap operation.
type StopBootstrapRequest struct {
	// JobID is the ID of the job to stop.
	JobID string `json:"job-id"`
}

// StartBootstrapResponse holds the response for starting
// a bootstrap job.
type StartBootstrapResponse struct {
	// JobID is the ID of the job that was started.
	JobID string `json:"job-id"`
}

// BootstrapParams holds parameters for starting
// a controller bootstrap job.
type BootstrapParams struct {
	// CloudName specifies the target cloud for the controller.
	CloudName string `json:"cloud-name"`
	// RegionName specifies the target region for the controller.
	RegionName string `json:"region-name"`
	// Cloud holds the cloud definition that will be used to bootstrap the controller.
	Cloud jujuparams.Cloud `json:"cloud,omitempty"`
	// Credential contains the cloud credential and its tag, this credential will be used against the
	// the cloud provided to bootstrap the controller.
	Credential jujuparams.CloudCredential `json:"credential"`

	// ControllerName specifies the name of the controller as recorded in JIMM.
	ControllerName string `json:"controller-name"`
	// BootstrapOptions holds the supported bootstrap settings for the job.
	BootstrapOptions BootstrapOptions `json:"bootstrap-options"`

	// ControllerVersion is the version of the controller to be bootstrapped.
	ControllerVersion string `json:"controller-version"`
}

// DestroyControllerRequest holds the name of
// the controller to be destroyed.
type DestroyControllerRequest struct {
	// ControllerName of the controller to destroy
	ControllerName string `json:"controller-name"`
}

// ListUserCloudsRequest holds the request parameters
// for listing clouds available to the specified user.
type ListUserCloudsRequest struct {
	// UserTag is the tag of the user for which we are listing clouds.
	UserTag string `json:"user"`
}

// ModelControllerInfoRequest is the request for ModelControllerInfo.
// The Model field can be:
//   - Model UUID (e.g., "2cb433a6-04eb-4ec4-9567-90426d20a004")
//   - Owner and model name (e.g., "alice@canonical.com/my-model")
type ModelControllerInfoRequest struct {
	// ModelQualifier is the model qualifier string.
	ModelQualifier string `json:"model"`
}

// ModelControllerInfo holds information about a model.
type ModelControllerInfo struct {
	// ModelName is the name of the model.
	ModelName string `json:"model-name" yaml:"model-name"`
	// ModelUUID is the UUID of the model.
	ModelUUID string `json:"model-uuid" yaml:"model-uuid"`
	// ControllerName is the name of the controller hosting the model.
	ControllerName string `json:"controller-name" yaml:"controller-name"`
	// ControllerUUID is the UUID of the controller hosting the model.
	ControllerUUID string `json:"controller-uuid" yaml:"controller-uuid"`
}

// JobInfoRequest holds the request to get information about a job.
type JobInfoRequest struct {
	// JobID is the ID of the job to get information about.
	JobID string `json:"job-id"`
}

// JobError represents an error that occurred during a job.
type JobError struct {
	At      time.Time `json:"at" yaml:"at"`
	Attempt int       `json:"attempt" yaml:"attempt"`
	Error   string    `json:"error" yaml:"error"`
}

// JobInfoResponse holds information about a job.
type JobInfoResponse struct {
	ID             int64      `json:"id" yaml:"id"`
	Status         JobStatus  `json:"status" yaml:"status"`
	Kind           string     `json:"kind" yaml:"kind"`
	CurrentAttempt int        `json:"current_attempt" yaml:"current_attempt"`
	MaxAttempts    int        `json:"max_attempts" yaml:"max_attempts"`
	FinishedAt     *time.Time `json:"finished_at,omitempty" yaml:"finished_at,omitempty"`
	Errors         []JobError `json:"errors,omitempty" yaml:"errors,omitempty"`
}

// ListJobsRequest holds the parameters to list jobs.
type ListJobsRequest struct {
	// Kinds is used to filter the jobs by their types. If empty, returns all kinds.
	Kinds []string `json:"kinds,omitempty"`
	// Statuses is used to filter the jobs by their statuses. If empty, returns all statuses.
	Statuses []JobStatus `json:"statuses,omitempty"`
	// Count is the maximum number of jobs to return. If not set, defaults to 100.
	Count int `json:"count,omitempty"`
	// Cursor is the pagination cursor to continue from a previous query.
	Cursor string `json:"cursor,omitempty"`
}

// ListJobInfo holds summary information about a job.
type ListJobInfo struct {
	// ID is the unique identifier for the job.
	ID int64 `json:"id" yaml:"id"`
	// Status is the current status of the job.
	Status JobStatus `json:"status" yaml:"status"`
	// Kind is the type of job.
	Kind string `json:"kind" yaml:"kind"`
	// MaxAttempts is the maximum number of attempts for this job.
	MaxAttempts int `json:"max_attempts" yaml:"max_attempts"`
	// Attempt is the current attempt number for this job.
	Attempt int `json:"attempt" yaml:"attempt"`
}

// ListJobsResponse holds the response for listing jobs.
// It contains a list of jobs that match the request parameters.
type ListJobsResponse struct {
	Jobs []ListJobInfo `json:"jobs" yaml:"jobs"`
	// NextCursor is the cursor to use for the next page of results.
	// If empty, there are no more results.
	NextCursor string `json:"next_cursor,omitempty" yaml:"next_cursor,omitempty"`
}
