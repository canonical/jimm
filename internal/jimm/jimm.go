// Copyright 2025 Canonical.

// Package jimm contains the business logic used to manage clouds,
// cloudcredentials and models.
package jimm

import (
	"context"
	"time"

	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/auditlog"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	"github.com/canonical/jimm/v3/internal/jimm/config"
	"github.com/canonical/jimm/v3/internal/jimm/controllerprofile"
	"github.com/canonical/jimm/v3/internal/jimm/credentials"
	"github.com/canonical/jimm/v3/internal/jimm/group"
	"github.com/canonical/jimm/v3/internal/jimm/identity"
	"github.com/canonical/jimm/v3/internal/jimm/jobs"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jimm/jujuauth"
	"github.com/canonical/jimm/v3/internal/jimm/login"
	"github.com/canonical/jimm/v3/internal/jimm/offer"
	"github.com/canonical/jimm/v3/internal/jimm/permissions"
	"github.com/canonical/jimm/v3/internal/jimm/role"
	"github.com/canonical/jimm/v3/internal/jimm/ssh"
	"github.com/canonical/jimm/v3/internal/jimm/sshkeys"
	"github.com/canonical/jimm/v3/internal/jimm/upgrade"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
	"github.com/canonical/jimm/v3/internal/jujuclient"
	"github.com/canonical/jimm/v3/internal/jujuclistore"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/pubsub"
	"github.com/canonical/jimm/v3/internal/river"
)

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

	// MigrationTokenGenerator is used to generate migration tokens for
	// authentication between Juju and JIMM during model migration.
	MigrationTokenGenerator juju.MigrationTokenGenerator

	// RiverClient is the client used to enqueue long-running jobs.
	RiverClient *river.Client

	// AuditLogRetentionDays is the number of days to keep audit logs.
	// The default value of 0 indicates that logs will never be deleted.
	AuditLogRetentionDays int

	// ControllerConfig is the configuration which will be exposed when
	// the ControllerConfig facade is called.
	ControllerConfig config.ControllerConfig

	// CrossModelQueryTimeout is the timeout for cross model queries.
	CrossModelQueryTimeout time.Duration

	// BootstrapLoginTokenRefreshURL is the URL when bootstrapping a controller via JIMM.
	// It should look something like:
	// <scheme><ip/dns>[<port>]/.well-known/jwks.json"
	BootstrapLoginTokenRefreshURL string
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

	if p.MigrationTokenGenerator == nil {
		return errors.E("missing migration token generator")
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
	j.RoleManager = roleManager

	groupManager, err := group.NewGroupManager(j.Database, j.OpenFGAClient)
	if err != nil {
		return nil, err
	}
	j.GroupManager = groupManager

	identityManager, err := identity.NewIdentityManager(j.Database, j.OpenFGAClient)
	if err != nil {
		return nil, err
	}
	j.IdentityManager = identityManager

	loginManager, err := login.NewLoginManager(j.Database, j.OpenFGAClient, j.OAuthAuthenticator, jimmResourceTag)
	if err != nil {
		return nil, err
	}
	j.LoginManager = loginManager

	permissionManager, err := permissions.NewManager(j.Database, j.OpenFGAClient, j.UUID, jimmResourceTag)
	if err != nil {
		return nil, err
	}
	j.PermissionManager = permissionManager

	j.JujuAuthFactory = jujuauth.NewFactory(j.Database, j.JWTService, permissionManager)
	jujuManager, err := juju.NewJujuManager(
		j.Database,
		j.OpenFGAClient,
		j.CredentialStore,
		j.PermissionManager,
		jimmResourceTag,
		p.ReservedCloudNames,
		j.Dialer,
		p.CrossModelQueryTimeout,
		j.MigrationTokenGenerator,
	)
	if err != nil {
		return nil, err
	}
	j.JujuManager = jujuManager

	auditLogManager, err := auditlog.NewAuditLogManager(j.Database, j.OpenFGAClient, jimmResourceTag, p.AuditLogRetentionDays)
	if err != nil {
		return nil, err
	}
	j.AuditLogManager = auditLogManager

	sshKeyManager, err := sshkeys.NewSSHKeyManager(j.Database)
	if err != nil {
		return nil, err
	}
	j.SSHKeyManager = sshKeyManager

	sshParams := ssh.SSHManagerParams{
		IdentityManager: j.IdentityManager,
		JujuManager:     j.JujuManager,
		SSHKeyManager:   j.SSHKeyManager,
		JWTFactory:      j.JujuAuthFactory,
		Dialer:          &ssh.BasicDialer{},
	}
	sshManager, err := ssh.NewSSHManager(sshParams)
	if err != nil {
		return nil, err
	}
	j.SSHManager = sshManager

	configManager, err := config.NewConfigManager(p.ControllerConfig)
	if err != nil {
		return nil, err
	}
	j.ConfigManager = configManager

	offerAuthorizer, err := offer.NewOfferAuthorizer(j.Database, j.OpenFGAClient)
	if err != nil {
		return nil, err
	}
	j.OfferAuthorizer = offerAuthorizer

	binaryStore, err := jujuclistore.NewJujuCLIStore(jujuclistore.Config{})
	if err != nil {
		return nil, err
	}

	bootstrapManager, err := bootstrap.NewBootstrapManager(
		j.Database,
		p.RiverClient,
		j.JujuManager,
		binaryStore,
		p.BootstrapLoginTokenRefreshURL,
		j.CredentialStore,
	)
	if err != nil {
		return nil, err
	}

	j.BootstrapManager = bootstrapManager

	controllerProfileManager, err := controllerprofile.NewControllerProfileManager(j.Database)
	if err != nil {
		return nil, err
	}

	j.ControllerProfileManager = controllerProfileManager

	upgradeManager, err := upgrade.NewUpgradeManager(j.JujuManager, j.Database, j.Dialer, j.RiverClient)
	if err != nil {
		return nil, err
	}

	j.UpgradeManager = upgradeManager

	jobManager, err := jobs.NewJobManager(j.RiverClient)
	if err != nil {
		return nil, err
	}

	j.JobManager = jobManager

	return j, nil
}

// A JIMM provides the business logic for managing resources in the JAAS
// system. A single JIMM instance is shared by all concurrent API
// connections therefore the JIMM object itself does not contain any per-
// request state.
type JIMM struct {
	Parameters

	// RoleManager provides a means to manage roles within JIMM.
	RoleManager *role.RoleManager

	// GroupManager provides a means to manage groups within JIMM.
	GroupManager *group.GroupManager

	// IdentityManager provides a means to manage identities within JIMM.
	IdentityManager *identity.IdentityManager

	// LoginManager provides a means to authenticate and login/create users/identities within JIMM.
	LoginManager *login.LoginManager

	PermissionManager *permissions.PermissionManager

	JujuAuthFactory *jujuauth.Factory

	// AuditLogManager provides a means to manage audit logs within JIMM.
	AuditLogManager *auditlog.AuditLogManager

	// SSHKeyManager provides a means to manage SSH keys within JIMM.
	SSHKeyManager *sshkeys.SSHKeyManager

	// SSHManager provides a means to manage SSH operations withing JIMM.
	SSHManager *ssh.SSHManager

	// ConfigManager provides a means to retrieve the controller config to expose via facade method.
	ConfigManager *config.ConfigManager

	// JujuManager provides a means to manage Juju resources within JIMM.
	JujuManager *juju.JujuManager

	// OfferAuthorizer provides a means to check if a user is a consumer of an application offer.
	OfferAuthorizer *offer.OfferAuthorizer

	// BootstrapManager provides a means to manage bootstrap jobs.
	BootstrapManager *bootstrap.BootstrapManager

	// ControllerProfileManager provides a means to manage saved controller profiles.
	ControllerProfileManager *controllerprofile.ControllerProfileManager

	// UpgradeManager provides a means to manage controller cloning and model automated upgrades.
	UpgradeManager *upgrade.UpgradeManager

	// JobManager provides a means to manage long-running jobs.
	JobManager *jobs.JobManager
}

// ResourceTag returns JIMM's controller tag stating its UUID.
func (j *JIMM) ResourceTag() names.ControllerTag {
	return names.NewControllerTag(j.UUID)
}

// NewJujuAuthenticator returns a new token generator for authenticating
// requests to a Juju controller.
func (j *JIMM) NewJujuAuthenticator() jujuauth.LoginTokenGenerator {
	return j.JujuAuthFactory.NewLoginGenerator()
}

// DialerAdapter is an adapter that implements the juju.Dialer interface
// using the jujuclient.Dialer. This is useful for integrating with JIMM's
// existing dialer logic while allowing the jujuclient package to return
// a concrete type.
type DialerAdapter struct {
	dialer *jujuclient.Dialer
}

// NewDialerAdapter creates a new DialerAdapter instance.
func NewDialerAdapter(dialer *jujuclient.Dialer) *DialerAdapter {
	return &DialerAdapter{
		dialer: dialer,
	}
}

// Dial implements the juju.Dialer interface for the DialerAdapter.
// It uses the underlying jujuclient.Dialer to establish a connection
// to the Juju controller and returns a juju.API connection.
func (d *DialerAdapter) Dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag, user *openfga.User, withPermissions map[string]string) (juju.API, error) {
	return d.dialer.Dial(ctx, ctl, modelTag, user, withPermissions)
}
