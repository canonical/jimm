// Copyright 2020 Canonical Ltd.

package params

import (
	"time"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/riverqueue/river/rivertype"
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

// JobAttemptError represents an error that occurred during a job attempt.
type JobAttemptError struct {
	// Error contains the stringified error of an error returned from a job or a
	// panic value in case of a panic.
	Error string `json:"error" yaml:"error"`

	// Trace contains a stack trace from a job that panicked. The trace is
	// produced by invoking `debug.Trace()`.
	Trace string `json:"trace" yaml:"trace"`
}

// Job represents a job to be processed by a worker.
type Job struct {
	// ID of the job.
	ID int64 `json:"id" yaml:"id"`

	// Attempt is the attempt number of the job. Jobs are inserted at 0, the
	// number is incremented to 1 the first time work its worked, and may
	// increment further if it's either snoozed or errors.
	Attempt int `json:"attempt" yaml:"attempt"`

	// AttemptedAt is the time that the job was last worked. Starts out as `nil`
	// on a new insert.
	AttemptedAt *time.Time `json:"attempted_at" yaml:"attempted_at"`

	// CreatedAt is when the job record was created.
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// EncodedArgs is the job's JobArgs encoded as JSON.
	EncodedArgs []byte `json:"encoded_args" yaml:"encoded_args"`

	// Errors is a set of errors that occurred when the job was worked, one for
	// each attempt. Ordered from earliest error to the latest error.
	Errors []JobAttemptError `json:"errors" yaml:"errors"`

	// FinalizedAt is the time at which the job was "finalized", meaning it was
	// either completed successfully or errored for the last time such that
	// it'll no longer be retried.
	FinalizedAt *time.Time `json:"finalized_at,omitempty" yaml:"finalized_at,omitempty"`

	// Kind uniquely identifies the type of job and instructs which worker
	// should work it. It is set at insertion time via `Kind()` on the
	// `JobArgs`.
	Kind string `json:"kind" yaml:"kind"`

	// MaxAttempts is the maximum number of attempts that the job will be tried
	// before it errors for the last time and will no longer be worked.
	MaxAttempts int `json:"max_attempts" yaml:"max_attempts"`

	// State is the state of job like `available` or `completed`. Jobs are
	// `available` when they're first inserted.
	State string `json:"state" yaml:"state"`
}

// Jobs contains the failed, completed, and cancelled jobs.
type Jobs struct {
	// FailedJobs has all the Job Rows info about failed jobs
	FailedJobs []Job `json:"failedJobs"`
	// CompletedJobs has all the Job Rows info about completed jobs
	CompletedJobs []Job `json:"completedJobs"`
	// CancelledJobs has all the Job Rows info about cancelled jobs
	CancelledJobs []Job `json:"cancelledJobs"`
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

type FindJobsRequest struct {
	// IncludeCancelled returns jobs that are in 'JobStateCancelled`
	IncludeCancelled bool `json:"includeCancelled,omitempty"`

	// IncludeCompleted returns jobs that are in 'JobStateCompleted'
	IncludeCompleted bool `json:"includeCompleted,omitempty"`

	// IncludeFailed returns jobs that are in 'JobStateDiscarded'
	IncludeFailed bool `json:"includeFailed,omitempty"`

	// Limit is the maximum number of jobs to return/
	Limit int `json:"limit,omitempty"`

	// SortAsc returns the jobs sorted in ascending order if set to true.
	SortAsc bool `json:"sortAscending,omitempty"`
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
	Results map[string][]any    `json:"results"`
	Errors  map[string][]string `json:"errors"`
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
	// ModelTag is a tag of the form "model-<UIID>"
	ModelTag string `json:"model-tag"`
	// TargetController is the controller name of the form "<name>"
	TargetController string `json:"target-controller"`
}

// MigrateModelRequest allows for multiple migration requests to be made.
type MigrateModelRequest struct {
	Specs []MigrateModelInfo `json:"specs"`
}

func convertAttemptErrors(attemptErrors []rivertype.AttemptError) []JobAttemptError {
	jobAttemptErrors := make([]JobAttemptError, len(attemptErrors))
	for i, err := range attemptErrors {
		jobAttemptErrors[i] = JobAttemptError{
			Error: err.Error,
			Trace: err.Trace,
		}
	}
	return jobAttemptErrors
}

func ConvertJobRowToJob(jobRow *rivertype.JobRow) Job {
	return Job{
		ID:          jobRow.ID,
		Attempt:     jobRow.Attempt,
		AttemptedAt: jobRow.AttemptedAt,
		CreatedAt:   jobRow.CreatedAt,
		EncodedArgs: jobRow.EncodedArgs,
		Errors:      convertAttemptErrors(jobRow.Errors),
		FinalizedAt: jobRow.FinalizedAt,
		Kind:        jobRow.Kind,
		MaxAttempts: jobRow.MaxAttempts,
		State:       string(jobRow.State),
	}
}
