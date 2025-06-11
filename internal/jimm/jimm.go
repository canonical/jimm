// Copyright 2025 Canonical.

// Package jimm contains the business logic used to manage clouds,
// cloudcredentials and models.
package jimm

import (
	"context"
	"net/http"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/juju/api/base"
	jujucontroller "github.com/juju/juju/controller"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/version"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/oauth2"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/auditlog"
	"github.com/canonical/jimm/v3/internal/jimm/config"
	"github.com/canonical/jimm/v3/internal/jimm/credentials"
	"github.com/canonical/jimm/v3/internal/jimm/group"
	"github.com/canonical/jimm/v3/internal/jimm/identity"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jimm/jujuauth"
	"github.com/canonical/jimm/v3/internal/jimm/login"
	"github.com/canonical/jimm/v3/internal/jimm/permissions"
	"github.com/canonical/jimm/v3/internal/jimm/role"
	"github.com/canonical/jimm/v3/internal/jimm/ssh"
	"github.com/canonical/jimm/v3/internal/jimm/sshkeys"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/pubsub"
	"github.com/canonical/jimm/v3/pkg/api/params"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

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
	AddRelation(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error
	// RemoveRelation removes the provided slice of tuples.
	RemoveRelation(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error
	// CheckRelation checks whether the provided tuple provides access.
	CheckRelation(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, trace bool) (bool, error)
	// CheckRelations checks whether the provided tuples provide access.
	CheckRelations(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) ([]openfga.CheckResult, error)
	// ListRelationshipTuples lists a page of tuples based on the provided tuple constraints.
	ListRelationshipTuples(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, pageSize int32, continuationToken string) ([]openfga.Tuple, string, error)
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

// JujuManager is the interface to manage all Juju related operations.
type JujuManager interface {
	// Controller related methods

	AddController(ctx context.Context, user *openfga.User, ctl *dbmodel.Controller, creds juju.ControllerCreds) error
	ControllerInfo(ctx context.Context, name string) (*dbmodel.Controller, error)
	EarliestControllerVersion(ctx context.Context) (version.Number, error)
	ListControllers(ctx context.Context, user *openfga.User) ([]dbmodel.Controller, error)
	RemoveController(ctx context.Context, user *openfga.User, controllerName string, force bool) error
	SetControllerDeprecated(ctx context.Context, user *openfga.User, controllerName string, deprecated bool) error
	ControllerConfig(ctx context.Context, controllerName string) (jujucontroller.Config, error)

	// Model related methods

	AddModel(ctx context.Context, u *openfga.User, args *juju.ModelCreateArgs) (_ *jujuparams.ModelInfo, err error)
	ChangeModelCredential(ctx context.Context, user *openfga.User, modelTag names.ModelTag, cloudCredentialTag names.CloudCredentialTag) error
	DestroyModel(ctx context.Context, u *openfga.User, mt names.ModelTag, destroyStorage *bool, force *bool, maxWait *time.Duration, timeout *time.Duration) error
	DumpModel(ctx context.Context, u *openfga.User, mt names.ModelTag, simplified bool) (string, error)
	DumpModelDB(ctx context.Context, u *openfga.User, mt names.ModelTag) (map[string]interface{}, error)
	ForEachModel(ctx context.Context, u *openfga.User, f func(*dbmodel.Model, jujuparams.UserAccessPermission) error) error
	ForEachUserModel(ctx context.Context, u *openfga.User, f func(*dbmodel.Model, jujuparams.UserAccessPermission) error) error
	FullModelStatus(ctx context.Context, user *openfga.User, modelTag names.ModelTag, patterns []string) (*jujuparams.FullStatus, error)
	GetModel(ctx context.Context, uuid string) (dbmodel.Model, error)
	ImportModel(ctx context.Context, user *openfga.User, controllerName string, modelTag names.ModelTag, newOwner string) error
	ModelDefaultsForCloud(ctx context.Context, user *dbmodel.Identity, cloudTag names.CloudTag) (jujuparams.ModelDefaultsResult, error)
	ModelInfo(ctx context.Context, u *openfga.User, mt names.ModelTag) (*jujuparams.ModelInfo, error)
	ListModelSummaries(ctx context.Context, user *openfga.User, maskingControllerUUID string) (jujuparams.ModelSummaryResults, error)
	ModelStatus(ctx context.Context, u *openfga.User, mt names.ModelTag) (*jujuparams.ModelStatus, error)
	QueryModelsJq(ctx context.Context, models []string, jqQuery string) (params.CrossModelQueryResponse, error)
	SetModelDefaults(ctx context.Context, user *dbmodel.Identity, cloudTag names.CloudTag, region string, configs map[string]interface{}) error
	UnsetModelDefaults(ctx context.Context, user *dbmodel.Identity, cloudTag names.CloudTag, region string, keys []string) error
	UpdateMigratedModel(ctx context.Context, user *openfga.User, modelTag names.ModelTag, targetControllerName string) error
	ValidateModelUpgrade(ctx context.Context, u *openfga.User, mt names.ModelTag, force bool) error

	// Other methods

	AddCloudToController(ctx context.Context, user *openfga.User, controllerName string, tag names.CloudTag, cloud jujuparams.Cloud, force bool) error
	AddHostedCloud(ctx context.Context, user *openfga.User, tag names.CloudTag, cloud jujuparams.Cloud, force bool) error
	CopyCredential(ctx context.Context, originalUser *openfga.User, newUser *openfga.User, cred names.CloudCredentialTag) (names.CloudCredentialTag, []jujuparams.UpdateCredentialModelResult, error)
	DestroyOffer(ctx context.Context, user *openfga.User, offerURL string, force bool) error
	FindApplicationOffers(ctx context.Context, user *openfga.User, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error)
	ForEachCloud(ctx context.Context, user *openfga.User, f func(*dbmodel.Cloud) error) error
	ForEachUserCloud(ctx context.Context, user *openfga.User, f func(*dbmodel.Cloud) error) error
	ForEachUserCloudCredential(ctx context.Context, u *dbmodel.Identity, ct names.CloudTag, f func(cred *dbmodel.CloudCredential) error) error
	GetApplicationOffer(ctx context.Context, user *openfga.User, offerURL string) (*jujuparams.ApplicationOfferAdminDetailsV5, error)
	GetApplicationOfferConsumeDetails(ctx context.Context, user *openfga.User, details *jujuparams.ConsumeOfferDetails, v bakery.Version) error
	GetCloud(ctx context.Context, u *openfga.User, tag names.CloudTag) (dbmodel.Cloud, error)
	GetCloudCredential(ctx context.Context, user *openfga.User, tag names.CloudCredentialTag) (*dbmodel.CloudCredential, error)
	GetCloudCredentialAttributes(ctx context.Context, u *openfga.User, cred *dbmodel.CloudCredential, hidden bool) (attrs map[string]string, redacted []string, err error)
	GrantOfferAccessOnController(ctx context.Context, user *openfga.User, ut names.UserTag, offerURL string, access jujuparams.OfferAccessPermission) error
	InitiateInternalMigration(ctx context.Context, user *openfga.User, modelNameOrUUID string, targetController string) (jujuparams.InitiateMigrationResult, error)
	InitiateMigration(ctx context.Context, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error)
	ListApplicationOffers(ctx context.Context, user *openfga.User, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error)
	ListModels(ctx context.Context, user *openfga.User) ([]base.UserModel, error)
	Offer(ctx context.Context, user *openfga.User, offer juju.AddApplicationOfferParams) error
	RemoveCloud(ctx context.Context, u *openfga.User, ct names.CloudTag) error
	RemoveCloudFromController(ctx context.Context, u *openfga.User, controllerName string, ct names.CloudTag) error
	RevokeCloudCredential(ctx context.Context, user *dbmodel.Identity, tag names.CloudCredentialTag) error
	RevokeOfferAccessOnController(ctx context.Context, user *openfga.User, ut names.UserTag, offerURL string, access jujuparams.OfferAccessPermission) error
	UpdateCloud(ctx context.Context, u *openfga.User, ct names.CloudTag, cloud jujuparams.Cloud) error
	UpdateCloudCredential(ctx context.Context, u *openfga.User, args juju.UpdateCloudCredentialArgs) ([]jujuparams.UpdateCredentialModelResult, error)
	PrepareModelMigration(ctx context.Context, user *openfga.User, modelUUID string, targetControllerName string, userMapping map[string]string) error

	// These are methods on the Juju manager that don't need to be mocked and can be removed from this interface later.
	UpdateMetrics(ctx context.Context)
	CleanupNotFoundModels(ctx context.Context) error
}

// Parameters holds the services and static fields passed to the jimm.New() constructor.
// You can provide mock implementations of certain services where necessary for dependency injection.
type Parameters struct {
	// Database is the database used by JIMM, this provides direct access
	// to the data store. Any client accessing the database directly is
	// responsible for ensuring that the authenticated user has access to
	// the data.
	Database *db.Database

	// Dialer is the API dialer JIMM uses to contact juju controllers. if
	// this is not configured all connection attempts will fail.
	Dialer juju.Dialer

	// CredentialStore is a store for the attributes of a
	// cloud credential and controller credentials.
	CredentialStore credentials.CredentialStore

	// Pubsub is a pub-sub hub used for buffering model summaries.
	Pubsub *pubsub.Hub

	// ReservedCloudNames is the list of names that cannot be used for
	// hosted clouds. If this is empty then DefaultReservedCloudNames
	// is used.
	ReservedCloudNames []string

	// UUID holds the UUID of the JIMM controller.
	UUID string

	// OpenFGAClient holds the client used to interact
	// with the OpenFGA ReBAC system.
	OpenFGAClient *openfga.OFGAClient

	// JWTService is responsible for minting JWTs to access controllers.
	JWTService *jimmjwx.JWTService

	// OAuthAuthenticator is responsible for handling authentication
	// via OAuth2.0 AND JWT access tokens to JIMM.
	OAuthAuthenticator login.OAuthAuthenticator

	// AuditLogRetentionDays is the number of days to keep audit logs.
	// The default value of 0 indicates that logs will never be deleted.
	AuditLogRetentionDays int

	// ControllerConfig is the configuration which will be exposed when
	// the ControllerConfig facade is called.
	ControllerConfig config.ControllerConfig

	// CrossModelQueryTimeout is the timeout for cross model queries.
	CrossModelQueryTimeout time.Duration
}

func (p *Parameters) Validate() error {
	if p.Database == nil {
		return errors.E("missing database")
	}

	if p.Dialer == nil {
		return errors.E("missing dialer")
	}

	if p.CredentialStore == nil {
		return errors.E("missing credential store")
	}

	if p.Pubsub == nil {
		return errors.E("missing pubsub hub")
	}

	if p.UUID == "" {
		return errors.E("missing uuid")
	}

	if p.OpenFGAClient == nil {
		return errors.E("missing openfga client")
	}

	if p.JWTService == nil {
		return errors.E("missing jwt service")
	}

	if p.OAuthAuthenticator == nil {
		return errors.E("missing oauth authenticator")
	}

	if p.CrossModelQueryTimeout <= 0 {
		return errors.E("missing cross model query timeout")
	}

	return nil
}

// New returns a new instance of JIMM.
// See [Option] and [Parameters] to better understand how to perform dependency injection.
// Primitives like the dialer or authentication service can be mocked at a low level,
// alternatively top business layer objects like the RoleManager can be mocked instead.
func New(p Parameters) (*JIMM, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	j := &JIMM{
		Parameters: p,
	}
	jimmResourceTag := names.NewControllerTag(j.UUID)

	if err := j.Database.Migrate(context.Background()); err != nil {
		return nil, errors.E(err)
	}

	roleManager, err := role.NewRoleManager(j.Database, j.OpenFGAClient)
	if err != nil {
		return nil, err
	}
	j.roleManager = roleManager

	groupManager, err := group.NewGroupManager(j.Database, j.OpenFGAClient)
	if err != nil {
		return nil, err
	}
	j.groupManager = groupManager

	identityManager, err := identity.NewIdentityManager(j.Database, j.OpenFGAClient)
	if err != nil {
		return nil, err
	}
	j.identityManager = identityManager

	loginManager, err := login.NewLoginManager(j.Database, j.OpenFGAClient, j.OAuthAuthenticator, jimmResourceTag)
	if err != nil {
		return nil, err
	}
	j.loginManager = loginManager

	permissionManager, err := permissions.NewManager(j.Database, j.OpenFGAClient, j.UUID, jimmResourceTag)
	if err != nil {
		return nil, err
	}
	j.permissionManager = permissionManager

	j.jujuAuthFactory = jujuauth.NewFactory(j.Database, j.JWTService, permissionManager)

	jujuManager, err := juju.NewJujuManager(
		j.Database,
		j.OpenFGAClient,
		j.CredentialStore,
		j.permissionManager,
		jimmResourceTag,
		p.ReservedCloudNames,
		j.Dialer,
		p.CrossModelQueryTimeout,
	)
	if err != nil {
		return nil, err
	}
	j.jujuManager = jujuManager

	auditLogManager, err := auditlog.NewAuditLogManager(j.Database, j.OpenFGAClient, jimmResourceTag, p.AuditLogRetentionDays)
	if err != nil {
		return nil, err
	}
	j.auditLogManager = auditLogManager

	sshKeyManager, err := sshkeys.NewSSHKeyManager(j.Database)
	if err != nil {
		return nil, err
	}
	j.sshKeyManager = sshKeyManager

	sshParams := ssh.SSHManagerParams{
		IdentityManager: j.identityManager,
		JujuManager:     j.jujuManager,
		SSHKeyManager:   j.sshKeyManager,
		JWTFactory:      j.jujuAuthFactory,
		Dialer:          &ssh.BasicDialer{},
	}
	sshManager, err := ssh.NewSSHManager(sshParams)
	if err != nil {
		return nil, err
	}
	j.sshManager = sshManager

	configManager, err := config.NewConfigManager(p.ControllerConfig)
	if err != nil {
		return nil, err
	}
	j.configManager = configManager

	return j, nil
}

// A JIMM provides the business logic for managing resources in the JAAS
// system. A single JIMM instance is shared by all concurrent API
// connections therefore the JIMM object itself does not contain any per-
// request state.
type JIMM struct {
	Parameters

	// roleManager provides a means to manage roles within JIMM.
	roleManager RoleManager

	// groupManager provides a means to manage groups within JIMM.
	groupManager GroupManager

	// identityManager provides a means to manage identities within JIMM.
	identityManager IdentityManager

	// loginManager provides a means to authenticate and login/create users/identities within JIMM.
	loginManager LoginManager

	permissionManager PermissionManager

	jujuAuthFactory *jujuauth.Factory

	// auditLogManager provides a means to manage audit logs within JIMM.
	auditLogManager AuditLogManager

	// sshKeyManager provides a means to manage SSH keys within JIMM.
	sshKeyManager SSHKeyManager

	// sshManager provides a means to manage SSH operations withing JIMM.
	sshManager SSHManager

	// configManager provides a means to retrieve the controller config to expose via facade method.
	configManager ConfigManager

	// jujuManager provides a means to manage Juju resources within JIMM.
	jujuManager JujuManager
}

// ResourceTag returns JIMM's controller tag stating its UUID.
func (j *JIMM) ResourceTag() names.ControllerTag {
	return names.NewControllerTag(j.UUID)
}

// PubsubHub returns the pub-sub hub used for buffering model summaries.
func (j *JIMM) PubSubHub() *pubsub.Hub {
	return j.Pubsub
}

// RoleManager returns a manager that enables role management.
func (j *JIMM) RoleManager() RoleManager {
	return j.roleManager
}

// GroupManager returns a manager that enables group management.
func (j *JIMM) GroupManager() GroupManager {
	return j.groupManager
}

// IdentityManager returns a manager that enables identity management.
func (j *JIMM) IdentityManager() IdentityManager {
	return j.identityManager
}

// LoginManager returns a manager that enables login and authentication.
func (j *JIMM) LoginManager() LoginManager {
	return j.loginManager
}

// PermissionManager returns a manager that enables permission checks and
// permissions grants/revocations.
func (j *JIMM) PermissionManager() PermissionManager {
	return j.permissionManager
}

// NewJujuAuthenticator returns a new token generator for authenticating
// requests to a Juju controller.
func (j *JIMM) NewJujuAuthenticator() jujuauth.LoginTokenGenerator {
	return j.jujuAuthFactory.NewLoginGenerator()
}

// AuditLogManager returns a manager that handles audit logging.
func (j *JIMM) AuditLogManager() AuditLogManager {
	return j.auditLogManager
}

// SSHKeyManager returns a manager that enables operations
// related to ssh keys.
func (j *JIMM) SSHKeyManager() SSHKeyManager {
	return j.sshKeyManager
}

// SSHManager returns a manager that enables operations
// related to ssh.
func (j *JIMM) SSHManager() SSHManager {
	return j.sshManager
}

// JujuManager returns a manager that enables operations
// related to Juju resources.
func (j *JIMM) JujuManager() JujuManager {
	return j.jujuManager
}

// ConfigManager returns a manager that exposes the controller config.
// This is used to expose the config via the ControllerConfig facade.
func (j *JIMM) ConfigManager() ConfigManager {
	return j.configManager
}
