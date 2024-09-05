// Copyright 2024 Canonical.

package jujuapi

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/rogpeppe/fastuuid"
	"golang.org/x/oauth2"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jimm/credentials"
	"github.com/canonical/jimm/v3/internal/jujuapi/rpc"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/pubsub"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

type JIMM interface {
	GroupService
	RelationService
	ControllerService
	LoginService
	ModelManager
	AddAuditLogEntry(ale *dbmodel.AuditLogEntry)
	AddCloudToController(ctx context.Context, user *openfga.User, controllerName string, tag names.CloudTag, cloud jujuparams.Cloud, force bool) error
	AddHostedCloud(ctx context.Context, user *openfga.User, tag names.CloudTag, cloud jujuparams.Cloud, force bool) error
	AddServiceAccount(ctx context.Context, u *openfga.User, clientId string) error
	CopyServiceAccountCredential(ctx context.Context, u *openfga.User, svcAcc *openfga.User, cloudCredentialTag names.CloudCredentialTag) (names.CloudCredentialTag, []jujuparams.UpdateCredentialModelResult, error)
	CountIdentities(ctx context.Context, user *openfga.User) (int, error)
	DestroyOffer(ctx context.Context, user *openfga.User, offerURL string, force bool) error
	FindApplicationOffers(ctx context.Context, user *openfga.User, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error)
	FindAuditEvents(ctx context.Context, user *openfga.User, filter db.AuditLogFilter) ([]dbmodel.AuditLogEntry, error)
	ForEachCloud(ctx context.Context, user *openfga.User, f func(*dbmodel.Cloud) error) error
	ForEachUserCloud(ctx context.Context, user *openfga.User, f func(*dbmodel.Cloud) error) error
	ForEachUserCloudCredential(ctx context.Context, u *dbmodel.Identity, ct names.CloudTag, f func(cred *dbmodel.CloudCredential) error) error
	GetApplicationOffer(ctx context.Context, user *openfga.User, offerURL string) (*jujuparams.ApplicationOfferAdminDetailsV5, error)
	GetApplicationOfferConsumeDetails(ctx context.Context, user *openfga.User, details *jujuparams.ConsumeOfferDetails, v bakery.Version) error
	GetCloud(ctx context.Context, u *openfga.User, tag names.CloudTag) (dbmodel.Cloud, error)
	GetCloudCredential(ctx context.Context, user *openfga.User, tag names.CloudCredentialTag) (*dbmodel.CloudCredential, error)
	GetCloudCredentialAttributes(ctx context.Context, u *openfga.User, cred *dbmodel.CloudCredential, hidden bool) (attrs map[string]string, redacted []string, err error)
	GetCredentialStore() credentials.CredentialStore
	GetJimmControllerAccess(ctx context.Context, user *openfga.User, tag names.UserTag) (string, error)
	// FetchIdentity finds the user in jimm or returns a not-found error
	FetchIdentity(ctx context.Context, username string) (*openfga.User, error)
	GetUserCloudAccess(ctx context.Context, user *openfga.User, cloud names.CloudTag) (string, error)
	GetUserControllerAccess(ctx context.Context, user *openfga.User, controller names.ControllerTag) (string, error)
	GetUserModelAccess(ctx context.Context, user *openfga.User, model names.ModelTag) (string, error)
	GrantAuditLogAccess(ctx context.Context, user *openfga.User, targetUserTag names.UserTag) error
	GrantCloudAccess(ctx context.Context, user *openfga.User, ct names.CloudTag, ut names.UserTag, access string) error
	GrantModelAccess(ctx context.Context, user *openfga.User, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error
	GrantOfferAccess(ctx context.Context, u *openfga.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) error
	GrantServiceAccountAccess(ctx context.Context, u *openfga.User, svcAccTag jimmnames.ServiceAccountTag, tags []string) error
	InitiateInternalMigration(ctx context.Context, user *openfga.User, modelTag names.ModelTag, targetController string) (jujuparams.InitiateMigrationResult, error)
	InitiateMigration(ctx context.Context, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error)
	ListApplicationOffers(ctx context.Context, user *openfga.User, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error)
	ListIdentities(ctx context.Context, user *openfga.User, filter pagination.LimitOffsetPagination) ([]openfga.User, error)
	ListResources(ctx context.Context, user *openfga.User, filter pagination.LimitOffsetPagination, nameFilter, typeFilter string) ([]db.Resource, error)
	Offer(ctx context.Context, user *openfga.User, offer jimm.AddApplicationOfferParams) error
	PubSubHub() *pubsub.Hub
	PurgeLogs(ctx context.Context, user *openfga.User, before time.Time) (int64, error)
	RemoveCloud(ctx context.Context, u *openfga.User, ct names.CloudTag) error
	RemoveCloudFromController(ctx context.Context, u *openfga.User, controllerName string, ct names.CloudTag) error
	RemoveController(ctx context.Context, user *openfga.User, controllerName string, force bool) error
	ResourceTag() names.ControllerTag
	RevokeAuditLogAccess(ctx context.Context, user *openfga.User, targetUserTag names.UserTag) error
	RevokeCloudAccess(ctx context.Context, user *openfga.User, ct names.CloudTag, ut names.UserTag, access string) error
	RevokeCloudCredential(ctx context.Context, user *dbmodel.Identity, tag names.CloudCredentialTag, force bool) error
	RevokeModelAccess(ctx context.Context, user *openfga.User, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error
	RevokeOfferAccess(ctx context.Context, user *openfga.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) (err error)
	ToJAASTag(ctx context.Context, tag *ofganames.Tag, resolveUUIDs bool) (string, error)
	UpdateApplicationOffer(ctx context.Context, controller *dbmodel.Controller, offerUUID string, removed bool) error
	UpdateCloud(ctx context.Context, u *openfga.User, ct names.CloudTag, cloud jujuparams.Cloud) error
	UpdateCloudCredential(ctx context.Context, u *openfga.User, args jimm.UpdateCloudCredentialArgs) ([]jujuparams.UpdateCredentialModelResult, error)
	UserLogin(ctx context.Context, identityName string) (*openfga.User, error)
}

// controllerRoot is the root for endpoints served on controller connections.
type controllerRoot struct {
	rpc.Root

	params   Params
	jimm     JIMM
	watchers *watcherRegistry
	pingF    func()

	// mu protects the fields below it
	mu                    sync.Mutex
	user                  *openfga.User
	controllerUUIDMasking bool
	generator             *fastuuid.Generator

	// deviceOAuthResponse holds a device code flow response for this request,
	// such that JIMM can retrieve the access and ID tokens via polling the Authentication
	// Service's issuer via the /token endpoint.
	//
	// NOTE: As this is on the controller root struct, and a new controller root
	// is created per WS, it is EXPECTED that the subsequent call to GetDeviceSessionToken
	// happens on the SAME websocket.
	deviceOAuthResponse *oauth2.DeviceAuthResponse

	// identityId is the id of the identity attempting to login via a session cookie.
	identityId string
}

func newControllerRoot(j JIMM, p Params, identityId string) *controllerRoot {
	watcherRegistry := &watcherRegistry{
		watchers: make(map[string]*modelSummaryWatcher),
	}
	r := &controllerRoot{
		params:                p,
		jimm:                  j,
		watchers:              watcherRegistry,
		pingF:                 func() {},
		controllerUUIDMasking: true,
		identityId:            identityId,
	}

	r.AddMethod("Admin", 1, "Login", rpc.Method(unsupportedLogin))
	r.AddMethod("Admin", 2, "Login", rpc.Method(unsupportedLogin))
	r.AddMethod("Admin", 3, "Login", rpc.Method(unsupportedLogin))
	r.AddMethod("Admin", 4, "Login", rpc.Method(unsupportedLogin))
	r.AddMethod("Admin", 4, "LoginDevice", rpc.Method(r.LoginDevice))
	r.AddMethod("Admin", 4, "GetDeviceSessionToken", rpc.Method(r.GetDeviceSessionToken))
	r.AddMethod("Admin", 4, "LoginWithSessionToken", rpc.Method(r.LoginWithSessionToken))
	r.AddMethod("Admin", 4, "LoginWithSessionCookie", rpc.Method(r.LoginWithSessionCookie))
	r.AddMethod("Admin", 4, "LoginWithClientCredentials", rpc.Method(r.LoginWithClientCredentials))
	r.AddMethod("Pinger", 1, "Ping", rpc.Method(r.Ping))
	return r
}

// masquarade allows a controller superuser to perform an action on behalf
// of another user. masquarade checks that the authenticated user is a
// controller user and that the requested is a valid JAAS user. If these
// conditions are met then masquarade returns a replacement user to use in
// JIMM requests.
func (r *controllerRoot) masquerade(ctx context.Context, userTag string) (*openfga.User, error) {
	ut, err := parseUserTag(userTag)
	if err != nil {
		return nil, errors.E(errors.CodeBadRequest, err)
	}
	if r.user.Tag() == ut {
		// allow anyone to masquarade as themselves.
		return r.user, nil
	}
	if !r.user.JimmAdmin {
		return nil, errors.E(errors.CodeUnauthorized, "unauthorized")
	}
	user, err := r.jimm.UserLogin(ctx, ut.Id())
	if err != nil {
		return nil, err
	}
	return user, nil
}

// parseUserTag parses a names.UserTag and validates it is for an
// identity-provider user.
func parseUserTag(tag string) (names.UserTag, error) {
	ut, err := names.ParseUserTag(tag)
	if err != nil {
		return names.UserTag{}, errors.E(errors.CodeBadRequest, err)
	}
	if ut.IsLocal() {
		return names.UserTag{}, errors.E(errors.CodeBadRequest, fmt.Sprintf("unsupported local user; if this is a service account add @%s domain", jimmnames.ServiceAccountDomain))
	}
	return ut, nil
}

// setPingF configures the function to call when an ping is received.
func (r *controllerRoot) setPingF(f func()) {
	r.pingF = f
}

// cleanup releases all resources used by the controllerRoot.
func (r *controllerRoot) cleanup() {
	r.watchers.stop()
}

func (r *controllerRoot) setupUUIDGenerator() error {
	if r.generator != nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var err error
	r.generator, err = fastuuid.NewGenerator()
	if err != nil {
		return errors.E(err)
	}
	return nil
}

func (r *controllerRoot) newAuditLogger() jimm.DbAuditLogger {
	return jimm.NewDbAuditLogger(r.jimm, r.getUser)
}

// getUser implements jujuapi.root interface to return the currently logged in user.
func (r *controllerRoot) getUser() names.UserTag {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.user != nil {
		return r.user.ResourceTag()
	}
	return names.UserTag{}
}
