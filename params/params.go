package params

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/juju/httprequest"
	jujuparams "github.com/juju/juju/apiserver/params"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/mgo.v2/bson"
)

// SetControllerPerm holds the parameters for setting the ACL on a
// controller.
type SetControllerPerm struct {
	httprequest.Route `httprequest:"PUT /v2/controller/:User/:Name/perm"`
	EntityPath
	ACL ACL `httprequest:",body"`
}

// GetControllerPerm holds the parameters for getting the ACL of a
// controller.
type GetControllerPerm struct {
	httprequest.Route `httprequest:"GET /v2/controller/:User/:Name/perm"`
	EntityPath
}

type SetControllerDeprecated struct {
	httprequest.Route `httprequest:"PUT /v2/controller/:User/:Name/deprecated"`
	EntityPath
	Body DeprecatedBody `httprequest:",body"`
}

type GetControllerDeprecated struct {
	httprequest.Route `httprequest:"GET /v2/controller/:User/:Name/deprecated"`
	EntityPath
}

// DeprecatedBody holds the body of a SetControllerDeprecated request
// or a GetControllerDeprecated response.
type DeprecatedBody struct {
	Deprecated bool `json:"deprecated"`
}

// SetModelPerm holds the parameters for setting the ACL on an model.
type SetModelPerm struct {
	httprequest.Route `httprequest:"PUT /v2/model/:User/:Name/perm"`
	EntityPath
	ACL ACL `httprequest:",body"`
}

// GetModelPerm holds the parameters for getting the ACL of an model.
type GetModelPerm struct {
	httprequest.Route `httprequest:"GET /v2/model/:User/:Name/perm"`
	EntityPath
}

// ACL holds an access control list for an entity.
type ACL struct {
	// Read holds users and groups that are allowed to read the
	// entity.
	Read []string `json:"read"`

	// Write holds users and groups that are allowed to write to the
	// entity.
	Write []string `json:"write,omitempty"`

	// Admin holds users and groups that are allowed to act as
	// administrators on the entity.
	Admin []string `json:"admin,omitempty"`
}

// AddController holds the parameters for adding a new controller.
type AddController struct {
	httprequest.Route `httprequest:"PUT /v2/controller/:User/:Name"`
	EntityPath
	Info ControllerInfo `httprequest:",body"`
}

// DeleteController holds the parameters for removing the controller.
type DeleteController struct {
	httprequest.Route `httprequest:"DELETE /v2/controller/:User/:Name"`
	EntityPath
	// Force forces the delete even if the controller is still alive.
	Force bool `httprequest:"force,form"`
}

// ControllerInfo holds information specifying how
// to connect to an existing controller.
type ControllerInfo struct {
	// HostPorts holds host/port pairs (in host:port form)
	// of the controller API endpoints.
	HostPorts []string `json:"host-ports"`

	// CACert holds the CA certificate that will be used
	// to validate the controller's certificate, in PEM format.
	CACert string `json:"ca-cert"`

	// User holds the name of user to use when
	// connecting to the controller.
	User string `json:"user"`

	// Password holds the password for the user.
	Password string `json:"password"`

	// ControllerUUID holds the UUID of the admin model
	// of the controller.
	ControllerUUID string `json:"controller-uuid"`

	// Public specifies whether the controller is considered
	// part of the "pool" of publicly available controllers.
	// Non-public controllers will be ignored when selecting
	// controllers by location.
	//
	// Only privileged users may create public controllers.
	Public bool `json:"public"`

	// Deprecated holds whether the controller is considered deprecated
	// for adding new models.
	Deprecated bool `json:"deprecated"`
}

// EntityPath holds the path parameters for specifying
// an entity in the API. It can also be used as a value
// in its own right, because it implements TextMarshaler
// and TextUnmarshaler.
type EntityPath struct {
	User User `httprequest:",path"`
	Name Name `httprequest:",path"`
}

func (p EntityPath) String() string {
	return fmt.Sprintf("%s/%s", p.User, p.Name)
}

// IsZero reports whether the receiver is the empty value.
func (p EntityPath) IsZero() bool {
	return p == EntityPath{}
}

var slash = []byte("/")

func (p *EntityPath) UnmarshalText(data []byte) error {
	parts := bytes.Split(data, slash)
	if len(parts) != 2 {
		return errgo.New("need <user>/<name>")
	}
	if err := p.User.UnmarshalText(parts[0]); err != nil {
		return errgo.Mask(err)
	}
	if err := p.Name.UnmarshalText(parts[1]); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func (p EntityPath) MarshalText() ([]byte, error) {
	data := make([]byte, 0, len(p.User)+1+len(p.Name))
	data = append(data, p.User...)
	data = append(data, '/')
	data = append(data, p.Name...)
	return data, nil
}

// CredentialPath holds the path parameters for specifying a credential
// in the API. It can also be used as a value in its own right, because
// it implements TextMarshaler and TextUnmarshaler.
type CredentialPath struct {
	Cloud Cloud `httprequest:",path"`
	EntityPath
}

func (p CredentialPath) String() string {
	return fmt.Sprintf("%s/%s", p.Cloud, p.EntityPath)
}

// IsZero reports whether the receiver is the empty value.
func (p CredentialPath) IsZero() bool {
	return p.Cloud == "" && p.EntityPath.IsZero()
}

func (p *CredentialPath) UnmarshalText(data []byte) error {
	parts := bytes.Split(data, slash)
	if len(parts) != 3 {
		return errgo.New("need <cloud>/<user>/<name>")
	}
	if err := p.Cloud.UnmarshalText(parts[0]); err != nil {
		return errgo.Mask(err)
	}
	if err := p.EntityPath.User.UnmarshalText(parts[1]); err != nil {
		return errgo.Mask(err)
	}
	if err := p.EntityPath.Name.UnmarshalText(parts[2]); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func (p CredentialPath) MarshalText() ([]byte, error) {
	data := make([]byte, 0, len(p.Cloud)+1+len(p.User)+1+len(p.Name))
	data = append(data, p.Cloud...)
	data = append(data, '/')
	data = append(data, p.User...)
	data = append(data, '/')
	data = append(data, p.Name...)
	return data, nil
}

// GetModel holds parameters for retrieving a model.
type GetModel struct {
	httprequest.Route `httprequest:"GET /v2/model/:User/:Name"`
	EntityPath
}

// DeleteModel holds parameters for deletion of a model.
type DeleteModel struct {
	httprequest.Route `httprequest:"DELETE /v2/model/:User/:Name"`
	EntityPath
}

// NewModel holds parameters for creating a new model.
type NewModel struct {
	httprequest.Route `httprequest:"POST /v2/model/:User"`

	// User holds the User element from the URL path.
	User User `httprequest:",path"`

	// Info holds the information required to create
	// the model.
	Info NewModelInfo `httprequest:",body"`
}

// ListModels holds parameters for listing
// current models.
type ListModels struct {
	httprequest.Route `httprequest:"GET /v2/model"`

	// All requests the list of models for all users. This is only
	// available to admin users.
	All bool `httprequest:"all,form"`

	// TODO add parameters for restricting results.
}

// ListModelsResponse holds a list of controllers as returned
// by ListModels.
type ListModelsResponse struct {
	Models []ModelResponse `json:"models"`
}

// ListController holds parameters for listing all current controllers.
type ListController struct {
	httprequest.Route `httprequest:"GET /v2/controller"`

	// TODO add parameters for restricting results.
}

// ListControllerResponse holds a list of controllers as returned
// by ListController.
type ListControllerResponse struct {
	Controllers []ControllerResponse `json:"controllers"`
}

// ControllerResponse holds information on a given Controller.
// Each Controller is also associated with an model
// at /v2/model/:User/:Name where User and Name
// are the same as that of the Controller's path.
type ControllerResponse struct {
	// Path holds the path of the controller.
	Path EntityPath `json:"path"`

	// TODO perhaps these two fields should be put
	// into the same type as returned from GetSchema.

	// ProviderType holds the kind of provider used
	// by the Controller.
	ProviderType string `json:"provider-type,omitempty"`

	// Schema holds the fields required to start
	// a new model using the Controller.
	Schema environschema.Fields `json:"schema,omitempty"`

	// Location holds location attributes associated with the controller.
	Location map[string]string `json:"location,omitempty"`

	// Public holds whether the controller is part of the public
	// pool of controllers.
	Public bool

	// UnavailableSince holds the time that the JEM server
	// noticed that the model's controller could not be
	// contacted. It is empty when the model is available.
	UnavailableSince *time.Time `json:"unavailable-since,omitempty"`
}

// GetController holds parameters for retrieving information on a Controller.
type GetController struct {
	httprequest.Route `httprequest:"GET /v2/controller/:User/:Name"`
	EntityPath
}

// GetSchema holds parameters for getting a schema.
type GetSchema struct {
	httprequest.Route `httprequest:"GET /v2/schema"`

	// Location constrains the controllers that will be used to
	// fetch the schema.
	//
	// Note that the values in this should be passed in the
	// URL query parameters.
	Location map[string]string
}

// SchemaResponse holds the information returned by a GetSchema request.
type SchemaResponse struct {
	// ProviderType holds the kind of provider associated with the schema.
	ProviderType string `json:"provider-type,omitempty"`

	// Schema holds the fields required to start a new model.
	Schema environschema.Fields `json:"schema,omitempty"`
}

// NewModelInfo holds the JSON body parameters
// for a NewModel request.
type NewModelInfo struct {
	// Name holds the name to give to the new model
	// within its user name space.
	Name Name `json:"name"`

	// Controller holds the path to the controller entity
	// to use to start the model.
	// This is optional and may not be available to all user.
	Controller *EntityPath `json:"controller,omitempty"`

	// Location holds location attributes that narrow down the range
	// of possible controllers to be used for the model.
	Location map[string]string `json:"location,omitempty"`

	// Credential holds the name of the provider credential that will
	// be used to provision machines in the new model.
	Credential CredentialPath `json:"credential"`

	// Config holds the configuration attributes to use to create the new model.
	Config map[string]interface{} `json:"config"`
}

// ModelResponse holds the response body from a NewModel call.
type ModelResponse struct {
	// Path holds the path of the model.
	Path EntityPath `json:"path"`

	// UUID holds the UUID of the model.
	UUID string `json:"uuid"`

	// ControllerPath holds the path of the controller holding this model.
	ControllerPath EntityPath `json:"controller-path"`

	// ControllerUUID holds the UUID of the controller's admin UUID.
	ControllerUUID string `json:"controller-uuid"`

	// CACert holds the CA certificate that will be used
	// to validate the controller's certificate, in PEM format.
	CACert string `json:"ca-cert"`

	// HostPorts holds host/port pairs (in host:port form)
	// of the controller API endpoints.
	HostPorts []string `json:"host-ports"`

	// Life holds the last reported lifecycle status of the model.
	// It is omitted when we have no information on the model's
	// life yet. Possible values are "alive", "dying" and "dead".
	Life string `json:"life,omitempty"`

	// UnavailableSince holds the time that the JEM server
	// noticed that the model's controller could not be
	// contacted. It is empty when the model is available.
	UnavailableSince *time.Time `json:"unavailable-since,omitempty"`

	// Counts holds information about the number of various kinds
	// of entities in the model.
	Counts map[EntityCount]Count

	// Creator holds the name of the user that created the model.
	Creator string
}

// WhoAmI holds parameters for requesting the current user name.
type WhoAmI struct {
	httprequest.Route `httprequest:"GET /v2/whoami"`
}

// WhoAmIResponse holds information on the currently
// authenticated user.
type WhoAmIResponse struct {
	User string `json:"user"`
}

// UpdateCredential holds parameters for adding or updating a credential.
type UpdateCredential struct {
	httprequest.Route `httprequest:"PUT /v2/credential/:User/:Cloud/:Name"`
	CredentialPath
	Credential Credential `httprequest:",body"`
}

// Credential holds the details of a credential.
type Credential struct {
	// AuthType holds the authentiction type of the credential. Valid
	// AuthTypes are listed in github.com/juju/juju/cloud/clouds.go.
	AuthType string `json:"auth-type"`

	// Attributes holds the map of attributes that form the
	// credential.
	Attributes map[string]string `json:"attrs,omitempty"`
}

// GetUserStats returns statistics related to the requested user.
type GetUserStats struct {
	httprequest.Route `httprequest:"GET /v2/user/:User/stats"`
	User              string `httprequest:",path"`
}

// UserStats holds per-user statistics as requested by GetUserStats.
type UserStatsResponse struct {
	// Counts holds counts for model entities in models created by
	// the user.
	Counts map[EntityCount]Count
}

// EntityCount represents some kind of entity we
// want count over time.
type EntityCount string

const (
	UnitCount        EntityCount = "units"
	ApplicationCount EntityCount = "applications"
	MachineCount     EntityCount = "machines"
)

// Count records information about a changing count of
// of entities over time.
type Count struct {
	// Time holds the time when the count record was recorded.
	Time time.Time `json:"time"`

	// Current holds the most recent count value,
	// recorded at the above time.
	Current int `json:"current"`

	// MaxCount holds the maximum count recorded.
	Max int `json:"max"`

	// Total holds the total number created over time.
	// This may be approximate if creation events are missed.
	Total int64 `json:"total"`

	// TotalTime holds the total time in milliseconds that any
	// entities have existed for. That is, if two entities have
	// existed for two seconds, this metric will record four
	// seconds.
	TotalTime int64 `json:"total-time"`
}

// JujuStatus requests the status of the specified model.
type JujuStatus struct {
	httprequest.Route `httprequest:"GET /v2/model/:User/:Name/status"`
	EntityPath
}

// JujuStatusResponse contains the full status of a juju model.
type JujuStatusResponse struct {
	Status jujuparams.FullStatus `json:"status"`
}

// Migrate holds a request to migrate a model to a different controller.
type Migrate struct {
	httprequest.Route `httprequest:"POST /v2/model/:User/:Name/migrate"`
	EntityPath
	// Controller holds the name of the controller.
	Controller EntityPath `httprequest:"controller,form"`
}

// LogLevel holds a request to get the current logging level of the
// server.
type LogLevel struct {
	httprequest.Route `httprequest:"GET /v2/log-level"`
}

// SetLogLevel holds a request to set the logging level of the server.
type SetLogLevel struct {
	httprequest.Route `httprequest:"PUT /v2/log-level"`
	Level             Level `httprequest:",body"`
}

// Level holds a logging level name.
type Level struct {
	Level string `json:"level"`
}

// ModelNameRequest holds a request to get the model UUID based on the
// UUID.
type ModelNameRequest struct {
	httprequest.Route `httprequest:"GET /v2/model-uuid/:UUID/name"`
	UUID              string `httprequest:",path"`
}

// ModelNameResponse holds the model name.
type ModelNameResponse struct {
	Name string `json:"name"`
}

// AuditLogRequest represents the http request for audit logs.
type AuditLogRequest struct {
	httprequest.Route `httprequest:"GET /v2/audit"`
	Start             QueryTime `httprequest:"start,form"`
	End               QueryTime `httprequest:"end,form"`
	Limit             int64     `httprequest:"limit,form"`
	Type              string    `httprequest:"type,form"`
}

// AuditLogEntry represents a single line in the audit log and contains
// information on the specific entry as content.
type AuditLogEntry struct {
	Content AuditEntry
}

// AuditEntry represents an entry in the audit log.
type AuditEntry interface {
	// Type returns the type information of the concrete type.
	Type() string
	// Created returns the creation time of the entry.
	Created() time.Time
	// isAuditEntry to specify that this this an audit entry log.
	isAuditEntry()

	// Common returns the AutryEntryCommon part of that entry.
	Common() *AuditEntryCommon
}

// AuditLogEntries represents a list of audit log entry.
type AuditLogEntries []AuditLogEntry

// AuditEntryCommon holds the type information so SetBSON can work for an AuditEntry and the Created field.
type AuditEntryCommon struct {
	// Originator holds the user who initiates the request.
	Originator string `bson:"originator" json:"id"`

	// Type is used for GetBSON and SetBSON to store the concrete type.
	Type_ string `bson:"type" json:"type"`

	// Created represents the ceation time of the entry.
	Created_ time.Time `bson:"created" json:"created"`
}

func (a *AuditEntryCommon) Common() *AuditEntryCommon {
	return a
}

// AuditModelCreated represents an audit log when a model is created.
type AuditModelCreated struct {
	// ID holds the id for a model.
	ID string `bson:"modelid" json:"id"`
	// UUID holds the UUID of the model.
	UUID string `bson:"uuid" json:"uuid"`
	// ControllerPath holds the controller path normalized as string.
	ControllerPath string `bson:"controller-path" json:"controller_path"`
	// Owner holds the name of the user who owns the model.
	Owner string `bson:"owner" json:"owner"`
	// Creator holds the name of the user that issued the model creation request.
	Creator string `bson:"creator" json:"creator"`
	// Cloud holds the name of the cloud of the model.
	Cloud string `bson:"cloud" json:"cloud"`
	// Region holds the name of the cloud region of the model.
	Region string `bson:"region" json:"region"`
	// holds the common part for any entry.
	AuditEntryCommon `bson:",inline"`
}

// isAuditEntry implements AuditEntry.isAuditEntry.
func (AuditModelCreated) isAuditEntry() {}

// AuditModelDestroyed represents an audit log when a model is destroyed.
type AuditModelDestroyed struct {
	// ID holds the id for a model.
	ID string `bson:"modelid" json:"id"`
	// UUID holds the UUID of the model.
	UUID string `bson:"uuid" json:"uuid"`
	// holds the common part for any entry.
	AuditEntryCommon `bson:",inline"`
}

// isAuditEntry implements AuditEntry.isAuditEntry.
func (AuditModelDestroyed) isAuditEntry() {}

// AuditCloudCreated represents an audit log when a cloud is created.
type AuditCloudCreated struct {
	// ID holds the id for a cloud.
	ID string `bson:"modelid" json:"id"`
	// Cloud holds the name of the cloud.
	Cloud string `bson:"cloud" json:"cloud"`
	// Region holds the name of the cloud region.
	Region string `bson:"region" json:"region"`
	// holds the common part for any entry.
	AuditEntryCommon `bson:",inline"`
}

// isAuditEntry implements AuditEntry.isAuditEntry.
func (AuditCloudCreated) isAuditEntry() {}

// AuditCloudRemoved represents an audit log when a cloud is removed.
type AuditCloudRemoved struct {
	// ID holds the id for a cloud.
	ID string `bson:"modelid" json:"id"`
	// Cloud holds the name of the cloud.
	Cloud string `bson:"cloud" json:"cloud"`
	// Region holds the name of the cloud region.
	Region string `bson:"region" json:"region"`
	// holds the common part for any entry.
	AuditEntryCommon `bson:",inline"`
}

// isAuditEntry implements AuditEntry.isAuditEntry.
func (AuditCloudRemoved) isAuditEntry() {}

// Type implements AuditEntry.Type.
func (e AuditEntryCommon) Type() string {
	return e.Type_
}

// Created implements AuditEntry.Created.
func (e AuditEntryCommon) Created() time.Time {
	return e.Created_
}

// auditLogTypes holds a list of the possible audit logs type.
var auditLogTypes = []AuditEntry{
	&AuditCloudCreated{},
	&AuditCloudRemoved{},
	&AuditModelCreated{},
	&AuditModelDestroyed{},
}

var validAuditLogTypes = func() map[string]AuditEntry {
	m := make(map[string]AuditEntry)
	for _, v := range auditLogTypes {
		m[AuditLogType(v)] = v
	}
	return m
}()

// AuditLogType gives the type name of an audit log.
func AuditLogType(v interface{}) string {
	t := reflect.TypeOf(v).Elem()
	return strings.TrimPrefix(t.Name(), "Audit")
}

// SetBSON implements the bson.Setter interface.
func (e *AuditLogEntry) SetBSON(raw bson.Raw) error {
	var t struct {
		Type string `bson:"type"`
	}
	if err := raw.Unmarshal(&t); err != nil {
		return errgo.Mask(err)
	}
	v, ok := validAuditLogTypes[t.Type]
	if !ok {
		return errgo.Notef(nil, "cannot unmarshal unknown type %q", t.Type)
	}
	content := reflect.New(reflect.TypeOf(v).Elem())
	if err := raw.Unmarshal(content.Interface()); err != nil {
		return errgo.Mask(err)
	}
	e.Content = content.Interface().(AuditEntry)
	return nil
}

// GetBSON implements the bson.Getter interface.
func (e *AuditLogEntry) GetBSON() (interface{}, error) {
	return e.Content, nil
}

func (e *AuditLogEntry) MarshalJSON() ([]byte, error) {
	out, err := json.Marshal(e.Content)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return out, err
}

func (e *AuditLogEntry) UnmarshalJSON(b []byte) error {
	var t struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(b, &t); err != nil {
		return errgo.Mask(err)
	}
	v, ok := validAuditLogTypes[t.Type]
	if !ok {
		return errgo.Notef(nil, "cannot unmarshal unknown type %q", t.Type)
	}
	content := reflect.New(reflect.TypeOf(v).Elem())
	if err := json.Unmarshal(b, content.Interface()); err != nil {
		return errgo.Mask(err)
	}
	e.Content = content.Interface().(AuditEntry)
	return nil
}

// ModelStatusRequest represents the http request for model statuses.
type ModelStatusesRequest struct {
	httprequest.Route `httprequest:"GET /v2/modelstatus"`
}

// ModelStatus represent the status of a model with its description.
type ModelStatus struct {
	// ID holds the id for a model.
	ID string `json:"id"`
	// UUID holds the UUID of the model.
	UUID string `json:"uuid"`
	// Cloud holds the name of the cloud of the model.
	Cloud string `json:"cloud"`
	// Region holds the name of the cloud region of the model.
	Region string `json:"region"`
	// Created represents the creation time of the model.
	Created time.Time `json:"created"`
	// Status holds the status of the model.
	Status string `json:"status"`
	// Controller holds the controller name of the model.
	Controller string `json:"controller"`
}

// ModelStatuses holds the list of model statuses.
type ModelStatuses []ModelStatus

// MissingModelsRequest holds a request to the missing-models endpoint.
type MissingModelsRequest struct {
	httprequest.Route `httprequest:"GET /v2/controller/:User/:Name/missing-models"`
	EntityPath
}

// MissingModels is a response from the missing-models endpoint
type MissingModels struct {
	Models []ModelStatus
}
