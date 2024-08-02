// Copyright 2023 Canonical Ltd.

package jimmtest

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/google/uuid"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/version"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	jimmcreds "github.com/canonical/jimm/v3/internal/jimm/credentials"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/pubsub"
	"github.com/canonical/jimm/v3/pkg/api/params"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

// JIMM is a default implementation of the jujuapi.JIMM interface. Every method
// has a corresponding funcion field. Whenever the method is called it
// will delegate to the requested funcion or if the funcion is nil return
// a NotImplemented error.
type JIMM struct {
	AddAuditLogEntry_                  func(ale *dbmodel.AuditLogEntry)
	AddCloudToController_              func(ctx context.Context, user *openfga.User, controllerName string, tag names.CloudTag, cloud jujuparams.Cloud, force bool) error
	AddController_                     func(ctx context.Context, u *openfga.User, ctl *dbmodel.Controller) error
	AddGroup_                          func(ctx context.Context, user *openfga.User, name string) (*dbmodel.GroupEntry, error)
	AddHostedCloud_                    func(ctx context.Context, user *openfga.User, tag names.CloudTag, cloud jujuparams.Cloud, force bool) error
	AddModel_                          func(ctx context.Context, u *openfga.User, args *jimm.ModelCreateArgs) (*jujuparams.ModelInfo, error)
	AddServiceAccount_                 func(ctx context.Context, u *openfga.User, clientId string) error
	Authenticate_                      func(ctx context.Context, req *jujuparams.LoginRequest) (*openfga.User, error)
	AuthorizationClient_               func() *openfga.OFGAClient
	ChangeModelCredential_             func(ctx context.Context, user *openfga.User, modelTag names.ModelTag, cloudCredentialTag names.CloudCredentialTag) error
	CheckPermission_                   func(ctx context.Context, user *openfga.User, cachedPerms map[string]string, desiredPerms map[string]interface{}) (map[string]string, error)
	CopyServiceAccountCredential_      func(ctx context.Context, u *openfga.User, svcAcc *openfga.User, cloudCredentialTag names.CloudCredentialTag) (names.CloudCredentialTag, []jujuparams.UpdateCredentialModelResult, error)
	DB_                                func() *db.Database
	DestroyModel_                      func(ctx context.Context, u *openfga.User, mt names.ModelTag, destroyStorage *bool, force *bool, maxWait *time.Duration, timeout *time.Duration) error
	DestroyOffer_                      func(ctx context.Context, user *openfga.User, offerURL string, force bool) error
	DumpModel_                         func(ctx context.Context, u *openfga.User, mt names.ModelTag, simplified bool) (string, error)
	DumpModelDB_                       func(ctx context.Context, u *openfga.User, mt names.ModelTag) (map[string]interface{}, error)
	EarliestControllerVersion_         func(ctx context.Context) (version.Number, error)
	FindApplicationOffers_             func(ctx context.Context, user *openfga.User, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error)
	FindAuditEvents_                   func(ctx context.Context, user *openfga.User, filter db.AuditLogFilter) ([]dbmodel.AuditLogEntry, error)
	ForEachCloud_                      func(ctx context.Context, user *openfga.User, f func(*dbmodel.Cloud) error) error
	ForEachModel_                      func(ctx context.Context, u *openfga.User, f func(*dbmodel.Model, jujuparams.UserAccessPermission) error) error
	ForEachUserCloud_                  func(ctx context.Context, user *openfga.User, f func(*dbmodel.Cloud) error) error
	ForEachUserCloudCredential_        func(ctx context.Context, u *dbmodel.Identity, ct names.CloudTag, f func(cred *dbmodel.CloudCredential) error) error
	ForEachUserModel_                  func(ctx context.Context, u *openfga.User, f func(*dbmodel.Model, jujuparams.UserAccessPermission) error) error
	FullModelStatus_                   func(ctx context.Context, user *openfga.User, modelTag names.ModelTag, patterns []string) (*jujuparams.FullStatus, error)
	GetApplicationOffer_               func(ctx context.Context, user *openfga.User, offerURL string) (*jujuparams.ApplicationOfferAdminDetailsV5, error)
	GetApplicationOfferConsumeDetails_ func(ctx context.Context, user *openfga.User, details *jujuparams.ConsumeOfferDetails, v bakery.Version) error
	GetCloud_                          func(ctx context.Context, u *openfga.User, tag names.CloudTag) (dbmodel.Cloud, error)
	GetCloudCredential_                func(ctx context.Context, user *openfga.User, tag names.CloudCredentialTag) (*dbmodel.CloudCredential, error)
	GetCloudCredentialAttributes_      func(ctx context.Context, u *openfga.User, cred *dbmodel.CloudCredential, hidden bool) (attrs map[string]string, redacted []string, err error)
	GetControllerConfig_               func(ctx context.Context, u *dbmodel.Identity) (*dbmodel.ControllerConfig, error)
	GetCredentialStore_                func() jimmcreds.CredentialStore
	GetJimmControllerAccess_           func(ctx context.Context, user *openfga.User, tag names.UserTag) (string, error)
	GetUser_                           func(ctx context.Context, username string) (*openfga.User, error)
	GetOpenFGAUserAndAuthorise_        func(ctx context.Context, email string) (*openfga.User, error)
	GetUserCloudAccess_                func(ctx context.Context, user *openfga.User, cloud names.CloudTag) (string, error)
	GetUserControllerAccess_           func(ctx context.Context, user *openfga.User, controller names.ControllerTag) (string, error)
	GetUserModelAccess_                func(ctx context.Context, user *openfga.User, model names.ModelTag) (string, error)
	GrantAuditLogAccess_               func(ctx context.Context, user *openfga.User, targetUserTag names.UserTag) error
	GrantCloudAccess_                  func(ctx context.Context, user *openfga.User, ct names.CloudTag, ut names.UserTag, access string) error
	GrantModelAccess_                  func(ctx context.Context, user *openfga.User, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error
	GrantOfferAccess_                  func(ctx context.Context, u *openfga.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) error
	GrantServiceAccountAccess_         func(ctx context.Context, u *openfga.User, svcAccTag jimmnames.ServiceAccountTag, entities []string) error
	ImportModel_                       func(ctx context.Context, user *openfga.User, controllerName string, modelTag names.ModelTag, newOwner string) error
	IdentityModelDefaults_             func(ctx context.Context, user *dbmodel.Identity) (map[string]interface{}, error)
	InitiateMigration_                 func(ctx context.Context, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error)
	InitiateInternalMigration_         func(ctx context.Context, user *openfga.User, modelTag names.ModelTag, targetController string) (jujuparams.InitiateMigrationResult, error)
	ListApplicationOffers_             func(ctx context.Context, user *openfga.User, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error)
	ListControllers_                   func(ctx context.Context, user *openfga.User) ([]dbmodel.Controller, error)
	ListGroups_                        func(ctx context.Context, user *openfga.User) ([]dbmodel.GroupEntry, error)
	ModelDefaultsForCloud_             func(ctx context.Context, user *dbmodel.Identity, cloudTag names.CloudTag) (jujuparams.ModelDefaultsResult, error)
	ModelInfo_                         func(ctx context.Context, u *openfga.User, mt names.ModelTag) (*jujuparams.ModelInfo, error)
	ModelStatus_                       func(ctx context.Context, u *openfga.User, mt names.ModelTag) (*jujuparams.ModelStatus, error)
	Offer_                             func(ctx context.Context, user *openfga.User, offer jimm.AddApplicationOfferParams) error
	OAuthAuthenticationService_        func() jimm.OAuthAuthenticator
	ParseTag_                          func(ctx context.Context, key string) (*ofganames.Tag, error)
	PubSubHub_                         func() *pubsub.Hub
	PurgeLogs_                         func(ctx context.Context, user *openfga.User, before time.Time) (int64, error)
	QueryModelsJq_                     func(ctx context.Context, models []dbmodel.Model, jqQuery string) (params.CrossModelQueryResponse, error)
	RemoveCloud_                       func(ctx context.Context, u *openfga.User, ct names.CloudTag) error
	RemoveCloudFromController_         func(ctx context.Context, u *openfga.User, controllerName string, ct names.CloudTag) error
	RemoveController_                  func(ctx context.Context, user *openfga.User, controllerName string, force bool) error
	RemoveGroup_                       func(ctx context.Context, user *openfga.User, name string) error
	RenameGroup_                       func(ctx context.Context, user *openfga.User, oldName, newName string) error
	ResourceTag_                       func() names.ControllerTag
	RevokeAuditLogAccess_              func(ctx context.Context, user *openfga.User, targetUserTag names.UserTag) error
	RevokeCloudAccess_                 func(ctx context.Context, user *openfga.User, ct names.CloudTag, ut names.UserTag, access string) error
	RevokeCloudCredential_             func(ctx context.Context, user *dbmodel.Identity, tag names.CloudCredentialTag, force bool) error
	RevokeModelAccess_                 func(ctx context.Context, user *openfga.User, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error
	RevokeOfferAccess_                 func(ctx context.Context, user *openfga.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) (err error)
	SetControllerConfig_               func(ctx context.Context, u *openfga.User, args jujuparams.ControllerConfigSet) error
	SetControllerDeprecated_           func(ctx context.Context, user *openfga.User, controllerName string, deprecated bool) error
	SetModelDefaults_                  func(ctx context.Context, user *dbmodel.Identity, cloudTag names.CloudTag, region string, configs map[string]interface{}) error
	SetIdentityModelDefaults_          func(ctx context.Context, user *dbmodel.Identity, configs map[string]interface{}) error
	ToJAASTag_                         func(ctx context.Context, tag *ofganames.Tag, resolveUUIDs bool) (string, error)
	UnsetModelDefaults_                func(ctx context.Context, user *dbmodel.Identity, cloudTag names.CloudTag, region string, keys []string) error
	UpdateApplicationOffer_            func(ctx context.Context, controller *dbmodel.Controller, offerUUID string, removed bool) error
	UpdateCloud_                       func(ctx context.Context, u *openfga.User, ct names.CloudTag, cloud jujuparams.Cloud) error
	UpdateCloudCredential_             func(ctx context.Context, u *openfga.User, args jimm.UpdateCloudCredentialArgs) ([]jujuparams.UpdateCredentialModelResult, error)
	UpdateMigratedModel_               func(ctx context.Context, user *openfga.User, modelTag names.ModelTag, targetControllerName string) error
	UpdateUserLastLogin_               func(ctx context.Context, identifier string) error
	ValidateModelUpgrade_              func(ctx context.Context, u *openfga.User, mt names.ModelTag, force bool) error
	WatchAllModelSummaries_            func(ctx context.Context, controller *dbmodel.Controller) (_ func() error, err error)
}

func (j *JIMM) AddAuditLogEntry(ale *dbmodel.AuditLogEntry) {
	if j.AddAuditLogEntry_ == nil {
		panic("not implemented")
	}
	j.AddAuditLogEntry(ale)
}
func (j *JIMM) AddCloudToController(ctx context.Context, user *openfga.User, controllerName string, tag names.CloudTag, cloud jujuparams.Cloud, force bool) error {
	if j.AddCloudToController_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.AddCloudToController_(ctx, user, controllerName, tag, cloud, force)
}
func (j *JIMM) AddController(ctx context.Context, u *openfga.User, ctl *dbmodel.Controller) error {
	if j.AddController_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.AddController_(ctx, u, ctl)
}
func (j *JIMM) AddGroup(ctx context.Context, u *openfga.User, name string) (*dbmodel.GroupEntry, error) {
	if j.AddGroup_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.AddGroup_(ctx, u, name)
}
func (j *JIMM) AddHostedCloud(ctx context.Context, user *openfga.User, tag names.CloudTag, cloud jujuparams.Cloud, force bool) error {
	if j.AddHostedCloud_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.AddHostedCloud_(ctx, user, tag, cloud, force)
}
func (j *JIMM) AddModel(ctx context.Context, u *openfga.User, args *jimm.ModelCreateArgs) (_ *jujuparams.ModelInfo, err error) {
	if j.AddModel_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.AddModel_(ctx, u, args)
}

func (j *JIMM) AddServiceAccount(ctx context.Context, u *openfga.User, clientId string) error {
	if j.AddServiceAccount_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.AddServiceAccount_(ctx, u, clientId)
}

func (j *JIMM) CopyServiceAccountCredential(ctx context.Context, u *openfga.User, svcAcc *openfga.User, cloudCredentialTag names.CloudCredentialTag) (names.CloudCredentialTag, []jujuparams.UpdateCredentialModelResult, error) {
	if j.CopyServiceAccountCredential_ == nil {
		return names.CloudCredentialTag{}, nil, errors.E(errors.CodeNotImplemented)
	}
	return j.CopyServiceAccountCredential_(ctx, u, svcAcc, cloudCredentialTag)
}

func (j *JIMM) Authenticate(ctx context.Context, req *jujuparams.LoginRequest) (*openfga.User, error) {
	if j.Authenticate_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.Authenticate_(ctx, req)
}
func (j *JIMM) AuthorizationClient() *openfga.OFGAClient {
	if j.AuthorizationClient_ == nil {
		return nil
	}
	return j.AuthorizationClient_()
}
func (j *JIMM) ChangeModelCredential(ctx context.Context, user *openfga.User, modelTag names.ModelTag, cloudCredentialTag names.CloudCredentialTag) error {
	if j.ChangeModelCredential_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.ChangeModelCredential_(ctx, user, modelTag, cloudCredentialTag)
}
func (j *JIMM) CheckPermission(ctx context.Context, user *openfga.User, cachedPerms map[string]string, desiredPerms map[string]interface{}) (map[string]string, error) {
	if j.CheckPermission_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.CheckPermission_(ctx, user, cachedPerms, desiredPerms)
}
func (j *JIMM) DB() *db.Database {
	if j.DB_ == nil {
		panic("not implemented")
	}
	return j.DB_()
}
func (j *JIMM) DestroyModel(ctx context.Context, u *openfga.User, mt names.ModelTag, destroyStorage *bool, force *bool, maxWait *time.Duration, timeout *time.Duration) error {
	if j.DestroyModel_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.DestroyModel_(ctx, u, mt, destroyStorage, force, maxWait, timeout)
}
func (j *JIMM) DestroyOffer(ctx context.Context, user *openfga.User, offerURL string, force bool) error {
	if j.DestroyOffer_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.DestroyOffer_(ctx, user, offerURL, force)
}
func (j *JIMM) DumpModel(ctx context.Context, u *openfga.User, mt names.ModelTag, simplified bool) (string, error) {
	if j.DumpModel_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return j.DumpModel_(ctx, u, mt, simplified)
}
func (j *JIMM) DumpModelDB(ctx context.Context, u *openfga.User, mt names.ModelTag) (map[string]interface{}, error) {
	if j.DumpModelDB_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.DumpModelDB_(ctx, u, mt)
}
func (j *JIMM) EarliestControllerVersion(ctx context.Context) (version.Number, error) {
	if j.EarliestControllerVersion_ == nil {
		return version.Number{}, errors.E(errors.CodeNotImplemented)
	}
	return j.EarliestControllerVersion_(ctx)
}
func (j *JIMM) FindApplicationOffers(ctx context.Context, user *openfga.User, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error) {
	if j.FindApplicationOffers_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.FindApplicationOffers_(ctx, user, filters...)
}
func (j *JIMM) FindAuditEvents(ctx context.Context, user *openfga.User, filter db.AuditLogFilter) ([]dbmodel.AuditLogEntry, error) {
	if j.FindAuditEvents_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.FindAuditEvents_(ctx, user, filter)
}
func (j *JIMM) ForEachCloud(ctx context.Context, user *openfga.User, f func(*dbmodel.Cloud) error) error {
	if j.ForEachCloud_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.ForEachCloud_(ctx, user, f)
}
func (j *JIMM) ForEachModel(ctx context.Context, u *openfga.User, f func(*dbmodel.Model, jujuparams.UserAccessPermission) error) error {
	if j.ForEachModel_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.ForEachModel_(ctx, u, f)
}
func (j *JIMM) ForEachUserCloud(ctx context.Context, user *openfga.User, f func(*dbmodel.Cloud) error) error {
	if j.ForEachUserCloud_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.ForEachUserCloud_(ctx, user, f)
}
func (j *JIMM) ForEachUserCloudCredential(ctx context.Context, u *dbmodel.Identity, ct names.CloudTag, f func(cred *dbmodel.CloudCredential) error) error {
	if j.ForEachUserCloudCredential_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.ForEachUserCloudCredential_(ctx, u, ct, f)
}
func (j *JIMM) ForEachUserModel(ctx context.Context, u *openfga.User, f func(*dbmodel.Model, jujuparams.UserAccessPermission) error) error {
	if j.ForEachUserModel_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.ForEachUserModel_(ctx, u, f)
}
func (j *JIMM) FullModelStatus(ctx context.Context, user *openfga.User, modelTag names.ModelTag, patterns []string) (*jujuparams.FullStatus, error) {
	if j.FullModelStatus_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.FullModelStatus_(ctx, user, modelTag, patterns)
}
func (j *JIMM) GetApplicationOffer(ctx context.Context, user *openfga.User, offerURL string) (*jujuparams.ApplicationOfferAdminDetailsV5, error) {
	if j.GetApplicationOffer_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.GetApplicationOffer_(ctx, user, offerURL)
}
func (j *JIMM) GetApplicationOfferConsumeDetails(ctx context.Context, user *openfga.User, details *jujuparams.ConsumeOfferDetails, v bakery.Version) error {
	if j.GetApplicationOfferConsumeDetails_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.GetApplicationOfferConsumeDetails_(ctx, user, details, v)
}
func (j *JIMM) GetCloud(ctx context.Context, u *openfga.User, tag names.CloudTag) (dbmodel.Cloud, error) {
	if j.GetCloud_ == nil {
		return dbmodel.Cloud{}, errors.E(errors.CodeNotImplemented)
	}
	return j.GetCloud_(ctx, u, tag)
}
func (j *JIMM) GetCloudCredential(ctx context.Context, user *openfga.User, tag names.CloudCredentialTag) (*dbmodel.CloudCredential, error) {
	if j.GetCloudCredential_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.GetCloudCredential_(ctx, user, tag)
}
func (j *JIMM) GetCloudCredentialAttributes(ctx context.Context, u *openfga.User, cred *dbmodel.CloudCredential, hidden bool) (attrs map[string]string, redacted []string, err error) {
	if j.GetCloudCredentialAttributes_ == nil {
		return nil, nil, errors.E(errors.CodeNotImplemented)
	}
	return j.GetCloudCredentialAttributes_(ctx, u, cred, hidden)
}
func (j *JIMM) GetControllerConfig(ctx context.Context, u *dbmodel.Identity) (*dbmodel.ControllerConfig, error) {
	if j.GetControllerConfig_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.GetControllerConfig_(ctx, u)
}
func (j *JIMM) GetCredentialStore() jimmcreds.CredentialStore {
	if j.GetCredentialStore_ == nil {
		return nil
	}
	return j.GetCredentialStore_()
}
func (j *JIMM) GetJimmControllerAccess(ctx context.Context, user *openfga.User, tag names.UserTag) (string, error) {
	if j.GetJimmControllerAccess_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return j.GetJimmControllerAccess_(ctx, user, tag)
}
func (j *JIMM) GetUser(ctx context.Context, username string) (*openfga.User, error) {
	if j.GetUser_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.GetUser(ctx, username)
}
func (j *JIMM) GetOpenFGAUserAndAuthorise(ctx context.Context, email string) (*openfga.User, error) {
	if j.GetOpenFGAUserAndAuthorise_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.GetUser(ctx, email)
}
func (j *JIMM) GetUserCloudAccess(ctx context.Context, user *openfga.User, cloud names.CloudTag) (string, error) {
	if j.GetUserCloudAccess_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return j.GetUserCloudAccess_(ctx, user, cloud)
}
func (j *JIMM) GetUserControllerAccess(ctx context.Context, user *openfga.User, controller names.ControllerTag) (string, error) {
	if j.GetUserControllerAccess_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return j.GetUserControllerAccess_(ctx, user, controller)
}
func (j *JIMM) GetUserModelAccess(ctx context.Context, user *openfga.User, model names.ModelTag) (string, error) {
	if j.GetUserModelAccess_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return j.GetUserModelAccess_(ctx, user, model)
}
func (j *JIMM) GrantAuditLogAccess(ctx context.Context, user *openfga.User, targetUserTag names.UserTag) error {
	if j.GrantAuditLogAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.GrantAuditLogAccess_(ctx, user, targetUserTag)
}
func (j *JIMM) GrantCloudAccess(ctx context.Context, user *openfga.User, ct names.CloudTag, ut names.UserTag, access string) error {
	if j.GrantCloudAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.GrantCloudAccess_(ctx, user, ct, ut, access)
}
func (j *JIMM) GrantModelAccess(ctx context.Context, user *openfga.User, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error {
	if j.GrantModelAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.GrantModelAccess_(ctx, user, mt, ut, access)
}
func (j *JIMM) GrantOfferAccess(ctx context.Context, u *openfga.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) error {
	if j.GrantOfferAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.GrantOfferAccess_(ctx, u, offerURL, ut, access)
}

func (j *JIMM) GrantServiceAccountAccess(ctx context.Context, u *openfga.User, svcAccTag jimmnames.ServiceAccountTag, entities []string) error {
	if j.GrantServiceAccountAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.GrantServiceAccountAccess_(ctx, u, svcAccTag, entities)
}

func (j *JIMM) ImportModel(ctx context.Context, user *openfga.User, controllerName string, modelTag names.ModelTag, newOwner string) error {
	if j.ImportModel_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.ImportModel_(ctx, user, controllerName, modelTag, newOwner)
}
func (j *JIMM) InitiateMigration(ctx context.Context, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error) {
	if j.InitiateMigration_ == nil {
		return jujuparams.InitiateMigrationResult{}, errors.E(errors.CodeNotImplemented)
	}
	return j.InitiateMigration_(ctx, user, spec)
}
func (j *JIMM) InitiateInternalMigration(ctx context.Context, user *openfga.User, modelTag names.ModelTag, targetController string) (jujuparams.InitiateMigrationResult, error) {
	if j.InitiateInternalMigration_ == nil {
		return jujuparams.InitiateMigrationResult{}, errors.E(errors.CodeNotImplemented)
	}
	return j.InitiateInternalMigration_(ctx, user, modelTag, targetController)
}
func (j *JIMM) ListApplicationOffers(ctx context.Context, user *openfga.User, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error) {
	if j.ListApplicationOffers_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ListApplicationOffers_(ctx, user, filters...)
}
func (j *JIMM) ListControllers(ctx context.Context, user *openfga.User) ([]dbmodel.Controller, error) {
	if j.ListControllers_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ListControllers_(ctx, user)
}
func (j *JIMM) ListGroups(ctx context.Context, user *openfga.User) ([]dbmodel.GroupEntry, error) {
	if j.ListGroups_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ListGroups_(ctx, user)
}
func (j *JIMM) ModelDefaultsForCloud(ctx context.Context, user *dbmodel.Identity, cloudTag names.CloudTag) (jujuparams.ModelDefaultsResult, error) {
	if j.ModelDefaultsForCloud_ == nil {
		return jujuparams.ModelDefaultsResult{}, errors.E(errors.CodeNotImplemented)
	}
	return j.ModelDefaultsForCloud_(ctx, user, cloudTag)
}
func (j *JIMM) ModelInfo(ctx context.Context, u *openfga.User, mt names.ModelTag) (*jujuparams.ModelInfo, error) {
	if j.ModelInfo_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ModelInfo_(ctx, u, mt)
}
func (j *JIMM) ModelStatus(ctx context.Context, u *openfga.User, mt names.ModelTag) (*jujuparams.ModelStatus, error) {
	if j.ModelStatus_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ModelStatus_(ctx, u, mt)
}
func (j *JIMM) Offer(ctx context.Context, user *openfga.User, offer jimm.AddApplicationOfferParams) error {
	if j.Offer_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.Offer_(ctx, user, offer)
}
func (j *JIMM) OAuthAuthenticationService() jimm.OAuthAuthenticator {
	if j.OAuthAuthenticationService_ == nil {
		panic("not implemented")
	}
	return j.OAuthAuthenticationService_()
}
func (j *JIMM) ParseTag(ctx context.Context, key string) (*ofganames.Tag, error) {
	if j.ParseTag_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ParseTag_(ctx, key)
}
func (j *JIMM) PubSubHub() *pubsub.Hub {
	if j.PubSubHub_ == nil {
		panic("not implemented")
	}
	return j.PubSubHub_()
}
func (j *JIMM) PurgeLogs(ctx context.Context, user *openfga.User, before time.Time) (int64, error) {
	if j.PurgeLogs_ == nil {
		return 0, errors.E(errors.CodeNotImplemented)
	}
	return j.PurgeLogs_(ctx, user, before)
}
func (j *JIMM) QueryModelsJq(ctx context.Context, models []dbmodel.Model, jqQuery string) (params.CrossModelQueryResponse, error) {
	if j.QueryModelsJq_ == nil {
		return params.CrossModelQueryResponse{}, errors.E(errors.CodeNotImplemented)
	}
	return j.QueryModelsJq_(ctx, models, jqQuery)
}
func (j *JIMM) RemoveCloud(ctx context.Context, u *openfga.User, ct names.CloudTag) error {
	if j.RemoveCloud_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RemoveCloud_(ctx, u, ct)
}
func (j *JIMM) RemoveCloudFromController(ctx context.Context, u *openfga.User, controllerName string, ct names.CloudTag) error {
	if j.RemoveCloudFromController_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RemoveCloudFromController_(ctx, u, controllerName, ct)
}
func (j *JIMM) RemoveController(ctx context.Context, user *openfga.User, controllerName string, force bool) error {
	if j.RemoveController_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RemoveController_(ctx, user, controllerName, force)
}
func (j *JIMM) RemoveGroup(ctx context.Context, user *openfga.User, name string) error {
	if j.RemoveGroup_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RemoveGroup_(ctx, user, name)
}
func (j *JIMM) RenameGroup(ctx context.Context, user *openfga.User, oldName, newName string) error {
	if j.RenameGroup_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RenameGroup_(ctx, user, oldName, newName)
}
func (j *JIMM) ResourceTag() names.ControllerTag {
	if j.ResourceTag_ == nil {
		return names.NewControllerTag(uuid.NewString())
	}
	return j.ResourceTag_()
}
func (j *JIMM) RevokeAuditLogAccess(ctx context.Context, user *openfga.User, targetUserTag names.UserTag) error {
	if j.RevokeAuditLogAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RevokeAuditLogAccess_(ctx, user, targetUserTag)
}
func (j *JIMM) RevokeCloudAccess(ctx context.Context, user *openfga.User, ct names.CloudTag, ut names.UserTag, access string) error {
	if j.RevokeCloudAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RevokeCloudAccess_(ctx, user, ct, ut, access)
}
func (j *JIMM) RevokeCloudCredential(ctx context.Context, user *dbmodel.Identity, tag names.CloudCredentialTag, force bool) error {
	if j.RevokeCloudCredential_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RevokeCloudCredential_(ctx, user, tag, force)
}
func (j *JIMM) RevokeModelAccess(ctx context.Context, user *openfga.User, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error {
	if j.RevokeModelAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RevokeModelAccess_(ctx, user, mt, ut, access)
}
func (j *JIMM) RevokeOfferAccess(ctx context.Context, user *openfga.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) (err error) {
	if j.RevokeOfferAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RevokeOfferAccess_(ctx, user, offerURL, ut, access)
}
func (j *JIMM) SetControllerConfig(ctx context.Context, u *openfga.User, args jujuparams.ControllerConfigSet) error {
	if j.SetControllerConfig_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.SetControllerConfig_(ctx, u, args)
}
func (j *JIMM) SetControllerDeprecated(ctx context.Context, user *openfga.User, controllerName string, deprecated bool) error {
	if j.SetControllerDeprecated_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.SetControllerDeprecated_(ctx, user, controllerName, deprecated)
}
func (j *JIMM) SetModelDefaults(ctx context.Context, user *dbmodel.Identity, cloudTag names.CloudTag, region string, configs map[string]interface{}) error {
	if j.SetModelDefaults_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.SetModelDefaults_(ctx, user, cloudTag, region, configs)
}
func (j *JIMM) SetIdentityModelDefaults(ctx context.Context, user *dbmodel.Identity, configs map[string]interface{}) error {
	if j.SetIdentityModelDefaults_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.SetIdentityModelDefaults_(ctx, user, configs)
}
func (j *JIMM) ToJAASTag(ctx context.Context, tag *ofganames.Tag, resolveUUIDs bool) (string, error) {
	if j.ToJAASTag_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return j.ToJAASTag_(ctx, tag, resolveUUIDs)
}
func (j *JIMM) UnsetModelDefaults(ctx context.Context, user *dbmodel.Identity, cloudTag names.CloudTag, region string, keys []string) error {
	if j.UnsetModelDefaults_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.UnsetModelDefaults_(ctx, user, cloudTag, region, keys)
}
func (j *JIMM) UpdateApplicationOffer(ctx context.Context, controller *dbmodel.Controller, offerUUID string, removed bool) error {
	if j.UpdateApplicationOffer_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.UpdateApplicationOffer_(ctx, controller, offerUUID, removed)
}
func (j *JIMM) UpdateCloud(ctx context.Context, u *openfga.User, ct names.CloudTag, cloud jujuparams.Cloud) error {
	if j.UpdateCloud_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.UpdateCloud_(ctx, u, ct, cloud)
}
func (j *JIMM) UpdateCloudCredential(ctx context.Context, u *openfga.User, args jimm.UpdateCloudCredentialArgs) ([]jujuparams.UpdateCredentialModelResult, error) {
	if j.UpdateCloudCredential_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.UpdateCloudCredential_(ctx, u, args)
}
func (j *JIMM) UpdateMigratedModel(ctx context.Context, user *openfga.User, modelTag names.ModelTag, targetControllerName string) error {
	if j.UpdateMigratedModel_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.UpdateMigratedModel_(ctx, user, modelTag, targetControllerName)
}
func (j *JIMM) UpdateUserLastLogin(ctx context.Context, identifier string) error {
	if j.UpdateUserLastLogin_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.UpdateUserLastLogin(ctx, identifier)
}
func (j *JIMM) IdentityModelDefaults(ctx context.Context, user *dbmodel.Identity) (map[string]interface{}, error) {
	if j.IdentityModelDefaults_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.IdentityModelDefaults_(ctx, user)
}
func (j *JIMM) ValidateModelUpgrade(ctx context.Context, u *openfga.User, mt names.ModelTag, force bool) error {
	if j.ValidateModelUpgrade_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.ValidateModelUpgrade_(ctx, u, mt, force)
}
func (j *JIMM) WatchAllModelSummaries(ctx context.Context, controller *dbmodel.Controller) (_ func() error, err error) {
	if j.WatchAllModelSummaries_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.WatchAllModelSummaries_(ctx, controller)
}
