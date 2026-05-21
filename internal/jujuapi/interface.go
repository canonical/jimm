// Copyright 2025 Canonical.

package jujuapi

import (
	"context"
	"net/http"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/juju/api/base"
	jujucloud "github.com/juju/juju/cloud"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/crossmodel"
	coremigration "github.com/juju/juju/core/migration"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/oauth2"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	"github.com/canonical/jimm/v3/internal/jimm/config"
	"github.com/canonical/jimm/v3/internal/jimm/jobs"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jimm/ssh"
	"github.com/canonical/jimm/v3/internal/jimm/sshkeys"
	"github.com/canonical/jimm/v3/internal/jujuclient"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/pubsub"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

// JIMM defines a comprehensive interface for all sort of operations with our application logic.
type JIMM interface {
	RoleManager() RoleManager
	GroupManager() GroupManager
	IdentityManager() IdentityManager
	LoginManager() LoginManager
	PermissionManager() PermissionManager
	AuditLogManager() AuditLogManager
	JujuManager() JujuManager
	ConfigManager() ConfigManager
	BootstrapManager() BootstrapManager
	ControllerProfileManager() ControllerProfileManager
	UpgradeManager() UpgradeManager
	JobManager() JobManager

	ResourceTag() names.ControllerTag
	PubSubHub() *pubsub.Hub
}

// RoleManager provides a means to manage roles within JIMM.
type RoleManager interface {
	// AddRole adds a role to JIMM.
	AddRole(ctx context.Context, user *openfga.User, roleName string) (*dbmodel.RoleEntry, error)
	// GetRoleByUUID returns a role based on the provided UUID.
	GetRoleByUUID(ctx context.Context, user *openfga.User, uuid string) (*dbmodel.RoleEntry, error)
	// GetRoleByName returns a role based on the provided name.
	GetRoleByName(ctx context.Context, user *openfga.User, name string) (*dbmodel.RoleEntry, error)
	// RemoveRole removes the role from JIMM in both the store and authorisation store.
	RemoveRole(ctx context.Context, user *openfga.User, roleName string) error
	// RenameRole renames a role in JIMM's DB.
	RenameRole(ctx context.Context, user *openfga.User, uuid, newName string) error
	// ListRoles returns a list of roles known to JIMM.
	// `match` will filter the list fuzzy matching role's name or uuid.
	ListRoles(ctx context.Context, user *openfga.User, pagination pagination.LimitOffsetPagination, match string) ([]dbmodel.RoleEntry, error)
	// CountRoles returns the number of roles that exist.
	CountRoles(ctx context.Context, user *openfga.User) (int, error)
}

// GroupManager provides a means to manage groups within JIMM.
type GroupManager interface {
	// AddGroup adds a role to JIMM.
	AddGroup(ctx context.Context, user *openfga.User, roleName string) (*dbmodel.GroupEntry, error)
	// GetGroupByUUID returns a role based on the provided UUID.
	GetGroupByUUID(ctx context.Context, user *openfga.User, uuid string) (*dbmodel.GroupEntry, error)
	// GetGroupByName returns a role based on the provided name.
	GetGroupByName(ctx context.Context, user *openfga.User, name string) (*dbmodel.GroupEntry, error)
	// RemoveGroup removes the role from JIMM in both the store and authorisation store.
	RemoveGroup(ctx context.Context, user *openfga.User, roleName string) error
	// RenameGroup renames a role in JIMM's DB.
	RenameGroup(ctx context.Context, user *openfga.User, uuid, newName string) error
	// ListGroups returns a list of roles known to JIMM.
	// `match` will filter the list fuzzy matching role's name or uuid.
	ListGroups(ctx context.Context, user *openfga.User, pagination pagination.LimitOffsetPagination, match string) ([]dbmodel.GroupEntry, error)
	// CountGroups returns the number of roles that exist.
	CountGroups(ctx context.Context, user *openfga.User) (int, error)
}

// IdentityManager provides a means to fetch identities in JIMM.
// Identities cannot be created here, that can only be done via login.
type IdentityManager interface {
	FetchIdentity(ctx context.Context, id string) (*openfga.User, error)
	ListIdentities(ctx context.Context, user *openfga.User, pagination pagination.LimitOffsetPagination, match string) ([]openfga.User, error)
	CountIdentities(ctx context.Context, user *openfga.User) (int, error)
}

// LoginManager provides methods for login/authentication and creates identities (users).
type LoginManager interface {
	// AuthenticateBrowserSession authenticates a browser login.
	AuthenticateBrowserSession(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error)
	// LoginDevice starts the device login flow.
	LoginDevice(ctx context.Context) (*oauth2.DeviceAuthResponse, error)
	// GetDeviceSessionToken returns a session token scoped to the user's identity.
	GetDeviceSessionToken(ctx context.Context, deviceOAuthResponse *oauth2.DeviceAuthResponse) (string, error)
	// LoginClientCredentials logs in a user with client credentials.
	LoginClientCredentials(ctx context.Context, clientID string, clientSecret string) (*openfga.User, error)
	// LoginWithSessionToken logs in a user with a session token.
	LoginWithSessionToken(ctx context.Context, sessionToken string) (*openfga.User, error)
	// LoginWithSessionCookie logs in a user assuming cookie auth was done previously.
	LoginWithSessionCookie(ctx context.Context, identityID string) (*openfga.User, error)
	// UserLogin creates/fetches an identity based on the identity provided and returns an openfga user object.
	UserLogin(ctx context.Context, identity string) (*openfga.User, error)
}

// PermissionManager provides a way to manage permissions within JIMM.
type PermissionManager interface {
	// These methods handle generic permission management through manipulation of OpenFGA tuples.

	// AddRelation creates the provided slice of tuples.
	AddRelation(ctx context.Context, user *openfga.User, tuples []params.RelationshipTuple) error
	// RemoveRelation removes the provided slice of tuples.
	RemoveRelation(ctx context.Context, user *openfga.User, tuples []params.RelationshipTuple) error
	// CheckRelation checks whether the provided tuple provides access.
	CheckRelation(ctx context.Context, user *openfga.User, tuple params.RelationshipTuple, trace bool) (bool, error)
	// CheckRelations checks whether the provided tuples provide access.
	CheckRelations(ctx context.Context, user *openfga.User, tuples []params.RelationshipTuple) ([]openfga.CheckResult, error)
	// ListRelationshipTuples lists a page of tuples based on the provided tuple constraints.
	ListRelationshipTuples(ctx context.Context, user *openfga.User, tuple params.RelationshipTuple, pageSize int32, continuationToken string) ([]openfga.Tuple, string, error)
	// ListObjectRelations lists all the tuples that an object has a direct relation with.
	ListObjectRelations(ctx context.Context, user *openfga.User, object string, pageSize int32, entitlementToken pagination.EntitlementToken) ([]openfga.Tuple, pagination.EntitlementToken, error)
	// ListResources lists all resources known to JIMM.
	ListResources(ctx context.Context, user *openfga.User, filter pagination.LimitOffsetPagination, namePrefixFilter, typeFilter string) ([]db.Resource, error)

	// GetJimmControllerAccess returns the user's level of access to JIMM.
	GetJimmControllerAccess(ctx context.Context, user *openfga.User, tag names.UserTag) (string, error)
	// GetUserCloudAccess returns the user's level of access to a cloud.
	GetUserCloudAccess(ctx context.Context, user *openfga.User, cloud names.CloudTag) (string, error)
	// GetUserModelAccess returns the user's level of access to a model.
	GetUserModelAccess(ctx context.Context, user *openfga.User, model names.ModelTag) (string, error)

	// GrantAuditLogAccess grants a user access to read audit logs.
	GrantAuditLogAccess(ctx context.Context, user *openfga.User, targetUserTag names.UserTag) error
	// GrantCloudAccess grants the user the specified access to a cloud.
	GrantCloudAccess(ctx context.Context, user *openfga.User, ct names.CloudTag, ut names.UserTag, access string) error
	// GrantModelAccess grants the user the specified access to a model.
	GrantModelAccess(ctx context.Context, user *openfga.User, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error
	// GrantOfferAccess grants the user the specified access to an offer.
	GrantOfferAccess(ctx context.Context, u *openfga.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) error

	// RevokeAuditLogAccess revokes a user's access to read audit logs.
	RevokeAuditLogAccess(ctx context.Context, user *openfga.User, targetUserTag names.UserTag) error
	// RevokeCloudAccess revokes the specified access to a cloud.
	RevokeCloudAccess(ctx context.Context, user *openfga.User, ct names.CloudTag, ut names.UserTag, access string) error
	// RevokeModelAccess revokes the specified access to a model.
	RevokeModelAccess(ctx context.Context, user *openfga.User, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error
	// RevokeOfferAccess revokes the specified access to an offer.
	RevokeOfferAccess(ctx context.Context, user *openfga.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) (err error)

	// OpenFGACleanup removes tuples that are no longer valid.
	OpenFGACleanup(ctx context.Context) error
	// ToJAASTag converts a tag used in OpenFGA authorization model to a tag used in JAAS.
	ToJAASTag(ctx context.Context, tag *ofganames.Tag, resolveUUIDs bool) (string, error)
}

// AuditLogManager provides methods to add/find/cleanup audit logs.
type AuditLogManager interface {
	// AddAuditLogEntry saves an audit log entry.
	AddAuditLogEntry(ale *dbmodel.AuditLogEntry)
	// FindAuditEvents queries for audit log entries that match the specified filter(s).
	FindAuditEvents(ctx context.Context, user *openfga.User, filter db.AuditLogFilter) ([]dbmodel.AuditLogEntry, error)
	// PurgeLogs removes logs older than the specified date.
	PurgeLogs(ctx context.Context, user *openfga.User, before time.Time) (int64, error)
	// StartCleanup removes log older than the retention period.
	StartCleanup(ctx context.Context)
}

// SSHKeyManager provides a means to manage SSH keys within JIMM.
type SSHKeyManager interface {
	// AddUserPublicKey saves a user's public key.
	AddUserPublicKey(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, publicKey sshkeys.PublicKey) error
	// ListUserPublicKeys lists a user's public keys.
	ListUserPublicKeys(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter) ([]sshkeys.PublicKey, error)
	// RemoveUserKeyByComment removes a user's public key(s) by the key comment.
	RemoveUserKeyByComment(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, comment string) error
	// RemoveUserKeyByFingerprint removes a user's public key(s) by the key fingerprint.
	RemoveUserKeyByFingerprint(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, fingerprint string) error
	// VerifyPublicKey lists the key for a user and compares the key to find a match.
	VerifyPublicKey(ctx context.Context, claimUser string, publicKey []byte) (bool, error)
}

// SSHManager is the interface to enable the ssh server to operate. Performing public key verification and
// resolving addresses from model uuids.
type SSHManager interface {
	// PublicKeyHandler is the method to verify the public key of the user. It returns a user if successful.
	PublicKeyHandler(ctx context.Context, claimUser string, key []byte) (*openfga.User, error)

	// DialInfo resolves the address of the controller to contact given the model UUID and
	// returns a struct with parameters to connect and authenticate to the controller.
	DialInfo(ctx context.Context, modelUUID string, user *openfga.User) (ssh.DialInfo, error)

	// DialController dials a controller's SSH server using the provided details.
	DialController(ctx context.Context, ctrlInfo ssh.DialInfo, user *openfga.User) (*gossh.Client, error)
}

// ConfigManager provides a means to retrieve the JIMM controller config to expose via facade method.
type ConfigManager interface {
	// GetConfig returns the configuration for the JIMM controller.
	GetConfig() (config.ControllerConfig, error)
}

// OfferAuthorizer provides methods to check if a user is a consumer of an application offer.
type OfferAuthorizer interface {
	// IsUserConsumerForOffer checks if a user is a consumer of an application offer.
	IsUserConsumerForOffer(ctx context.Context, userTag names.UserTag, offerTag names.ApplicationOfferTag) (bool, error)
}

// JujuManager is the interface to manage all Juju related operations.
type JujuManager interface {
	// Controller related methods

	AddController(ctx context.Context, user *openfga.User, ctl *dbmodel.Controller, creds juju.ControllerCreds) error
	ControllerInfo(ctx context.Context, name string) (*dbmodel.Controller, error)
	EarliestControllerVersion(ctx context.Context) (version.Number, error)
	ListControllerBootstraps(ctx context.Context) ([]dbmodel.ControllerBootstrap, error)
	ListControllers(ctx context.Context, user *openfga.User) ([]dbmodel.Controller, error)
	RemoveController(ctx context.Context, user *openfga.User, controllerName string, force bool) error
	SetControllerDeprecated(ctx context.Context, user *openfga.User, controllerName string, deprecated bool) error
	ControllerConfig(ctx context.Context, user *openfga.User, controllerName string) (jujucontroller.Config, error)

	// Model related methods

	AddModel(ctx context.Context, u *openfga.User, args *juju.ModelCreateArgs) (_ base.ModelInfo, err error)
	ChangeModelCredential(ctx context.Context, user *openfga.User, modelTag names.ModelTag, cloudCredentialTag names.CloudCredentialTag) error
	DestroyModel(ctx context.Context, u *openfga.User, mt names.ModelTag, destroyStorage *bool, force *bool, maxWait *time.Duration, timeout *time.Duration) error
	DumpModel(ctx context.Context, u *openfga.User, mt names.ModelTag, simplified bool) (map[string]any, error)
	DumpModelDB(ctx context.Context, u *openfga.User, mt names.ModelTag) (map[string]any, error)
	ForEachModel(ctx context.Context, u *openfga.User, f func(*dbmodel.Model, jujuparams.UserAccessPermission) error) error
	ForEachUserModel(ctx context.Context, u *openfga.User, f func(*dbmodel.Model, string) error) error
	FullModelStatus(ctx context.Context, user *openfga.User, modelTag names.ModelTag, patterns []string) (*jujuparams.FullStatus, error)
	GetModel(ctx context.Context, uuid string) (dbmodel.Model, error)
	ImportModel(ctx context.Context, user *openfga.User, controllerName string, modelTag names.ModelTag, newOwner string) error
	ModelDefaultsForCloud(ctx context.Context, user *dbmodel.Identity, cloudTag names.CloudTag) (jujuparams.ModelDefaultsResult, error)
	ModelInfo(ctx context.Context, u *openfga.User, mt names.ModelTag) (jujuclient.ModelInfo, error)
	ModelControllerInfo(ctx context.Context, user *openfga.User, qualifier juju.ModelControllerInfoQualifier) (*params.ModelControllerInfo, error)
	ListModelSummaries(ctx context.Context, user *openfga.User, maskingControllerUUID string) ([]base.UserModelSummary, error)
	ModelStatus(ctx context.Context, u *openfga.User, mt names.ModelTag) (base.ModelStatus, error)
	QueryModelsJq(ctx context.Context, models []string, jqQuery string) (params.CrossModelQueryResponse, error)
	SetModelDefaults(ctx context.Context, user *dbmodel.Identity, cloudTag names.CloudTag, region string, configs map[string]any) error
	UnsetModelDefaults(ctx context.Context, user *dbmodel.Identity, cloudTag names.CloudTag, region string, keys []string) error
	UpdateMigratedModel(ctx context.Context, user *openfga.User, modelTag names.ModelTag, targetControllerName string) error
	AbortModelUpgrade(ctx context.Context, u *openfga.User, mt names.ModelTag) error
	UpgradeModel(ctx context.Context, u *openfga.User, mt names.ModelTag, targetVersion version.Number, stream string, ignoreAgentVersions bool, dryRun bool) (version.Number, error)
	ValidateModelUpgrade(ctx context.Context, u *openfga.User, mt names.ModelTag, force bool) error
	SupportedVersions(ctx context.Context, contextualVersion *string) (params.SupportedJujuVersionsResponse, error)
	// Migration related methods

	// ControllerDetailsForIncomingModel retrieves details about the
	// target controller for a model that is being migrated.
	ControllerDetailsForIncomingModel(ctx context.Context, modelUUID string) (juju.ControllerConnectionDetails, error)

	// The remaining migration methods below are sorted roughly in the order they are expected to be called.
	// Please MAINTAIN this order as it is helpful to understand the migration flow and which methods
	// can use the IncomingModelMigration table versus which must use the plain Models table.

	PrepareModelMigration(ctx context.Context, user *openfga.User, modelUUID string, targetControllerName string, userMapping map[string]string) (string, error)
	Prechecks(ctx context.Context, user *openfga.User, model juju.MigratingModelInfo) error
	CheckMachines(ctx context.Context, user *openfga.User, modelUUID string) ([]error, error)
	Import(ctx context.Context, user *openfga.User, serialized jujuparams.SerializedModel) error
	Activate(ctx context.Context, user *openfga.User, modelTag names.ModelTag, migrationInfo coremigration.SourceControllerInfo, relatedModels []string) error
	AdoptResources(ctx context.Context, user *openfga.User, modelUUID string, sourceControllerVersion version.Number) error
	LatestLogTime(ctx context.Context, user *openfga.User, modelUUID string) (time.Time, error)
	AbortMigration(ctx context.Context, user *openfga.User, modelUUID string) error
	CleanupPartialModelMigrations(ctx context.Context) error
	ListMigrationTargets(ctx context.Context, user *openfga.User, modelTag names.ModelTag) ([]dbmodel.Controller, error)

	// Other methods
	AddCloudToController(ctx context.Context, user *openfga.User, controllerName string, tag names.CloudTag, cloud jujucloud.Cloud, force bool) error
	AddHostedCloud(ctx context.Context, user *openfga.User, tag names.CloudTag, cloud jujucloud.Cloud, force bool) error
	CopyCredential(ctx context.Context, originalUser *openfga.User, newUser *openfga.User, cred names.CloudCredentialTag) (names.CloudCredentialTag, []jujuparams.UpdateCredentialModelResult, error)
	DestroyOffer(ctx context.Context, user *openfga.User, offerURL string, force bool) error
	FindApplicationOffers(ctx context.Context, user *openfga.User, filters ...crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error)
	ForEachCloud(ctx context.Context, user *openfga.User, f func(*dbmodel.Cloud) error) error
	ForEachUserCloud(ctx context.Context, user *openfga.User, f func(*dbmodel.Cloud) error) error
	ForEachUserCloudCredential(ctx context.Context, u *dbmodel.Identity, ct names.CloudTag, f func(cred *dbmodel.CloudCredential) error) error
	GetApplicationOffer(ctx context.Context, user *openfga.User, offerURL string) (*crossmodel.ApplicationOfferDetails, error)
	GetApplicationOfferConsumeDetails(ctx context.Context, user *openfga.User, details *jujuparams.ConsumeOfferDetails, v bakery.Version) error
	GetCloud(ctx context.Context, u *openfga.User, tag names.CloudTag) (dbmodel.Cloud, error)
	GetCloudCredential(ctx context.Context, user *openfga.User, tag names.CloudCredentialTag) (*dbmodel.CloudCredential, error)
	GetCloudCredentialAttributes(ctx context.Context, u *openfga.User, cred *dbmodel.CloudCredential, hidden bool) (attrs map[string]string, redacted []string, err error)
	ControllerDetailsForModel(ctx context.Context, modelUUID string) (juju.ControllerConnectionDetails, error)
	InitiateInternalMigration(ctx context.Context, user *openfga.User, modelNameOrUUID string, targetController string) (jujuparams.InitiateMigrationResult, error)
	InitiateMigration(ctx context.Context, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error)
	ListApplicationOffers(ctx context.Context, user *openfga.User, filters ...crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error)
	ListModels(ctx context.Context, user *openfga.User) ([]base.UserModel, error)
	Offer(ctx context.Context, user *openfga.User, offer juju.AddApplicationOfferParams) error
	RemoveCloud(ctx context.Context, u *openfga.User, ct names.CloudTag) error
	RemoveCloudFromController(ctx context.Context, u *openfga.User, controllerName string, ct names.CloudTag) error
	RevokeCloudCredential(ctx context.Context, user *dbmodel.Identity, tag names.CloudCredentialTag) error
	UpdateCloud(ctx context.Context, u *openfga.User, ct names.CloudTag, cloud jujucloud.Cloud) error
	UpdateCloudCredential(ctx context.Context, u *openfga.User, args juju.UpdateCloudCredentialArgs) ([]jujuparams.UpdateCredentialModelResult, error)

	// These are methods on the Juju manager that don't need to be mocked and can be removed from this interface later.
	UpdateMetrics(ctx context.Context)
	PollModels(ctx context.Context) error
}

// BootstrapManager provides methods to manage bootstrap jobs.
type BootstrapManager interface {
	// GetJobInfo retrieves the status and logs of a job.
	GetJobInfo(ctx context.Context, user *openfga.User, jobId int64, offset int) (params.GetBootstrapInfoResponse, error)
	// StopJob stops a job.
	StopJob(ctx context.Context, user *openfga.User, jobId int64) error
	// WaitForJobCompletion waits for a job to complete.
	WaitForJobCompletion(ctx context.Context, jobId int64, config bootstrap.WaitConfig) error
	// StartBootstrapJob starts a bootstrap job and returns the job ID.
	StartBootstrapJob(ctx context.Context, user *openfga.User, params bootstrap.BootstrapParams) (int64, error)
	// StartDestroyControllerJob starts a destroy-controller job and returns the job ID.
	StartDestroyControllerJob(ctx context.Context, user *openfga.User, params bootstrap.DestroyControllerParams) (int64, error)
	// BootstrapController bootstraps a new Juju controller.
	BootstrapController(ctx context.Context, p bootstrap.RunBootstrapArgs, cmdFactory bootstrap.CommandFactory, user *openfga.User) error
	// DestroyController destroys a Juju controller.
	DestroyController(ctx context.Context, p bootstrap.RunDestroyControllerArgs, cmdFactory bootstrap.CommandFactory, user *openfga.User) error
}

// ControllerProfileManager provides methods to manage saved controller profiles.
type ControllerProfileManager interface {
	SaveControllerProfile(ctx context.Context, profile *dbmodel.ControllerProfile) error
	GetControllerProfile(ctx context.Context, name string) (*dbmodel.ControllerProfile, error)
	ListControllerProfiles(ctx context.Context, jujuVersion string) ([]dbmodel.ControllerProfile, error)
	RemoveControllerProfile(ctx context.Context, name string) error
}

// UpgradeManager provides methods to manage controller cloning and model automated upgrades.
type UpgradeManager interface {
	UpgradeTo(ctx context.Context, user *openfga.User, modelUUID string, targetController string) (int64, error)
}

// JobManager provides methods to manage long-running jobs such as bootstrapping and upgrading.
type JobManager interface {
	GetJobInfo(ctx context.Context, jobID int64) (jobs.JobInfo, error)
	ListJobs(ctx context.Context, params params.ListJobsRequest) (params.ListJobsResponse, error)
}
