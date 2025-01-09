// Copyright 2025 Canonical.

package jimmtest

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/google/uuid"
	"github.com/juju/juju/api/base"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	jimmcreds "github.com/canonical/jimm/v3/internal/jimm/credentials"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/pubsub"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
)

// JIMM is a default implementation of the jujuapi.JIMM interface. Every method
// has a corresponding funcion field. Whenever the method is called it
// will delegate to the requested funcion or if the funcion is nil return
// a NotImplemented error.
type JIMM struct {
	mocks.ControllerService
	mocks.ModelManager

	GroupManager_      func() jimm.GroupManager
	IdentityManager_   func() jimm.IdentityManager
	LoginManager_      func() jimm.LoginManager
	RoleManager_       func() jimm.RoleManager
	PermissionManager_ func() jimm.PermissionManager

	AddAuditLogEntry_                  func(ale *dbmodel.AuditLogEntry)
	AddCloudToController_              func(ctx context.Context, user *openfga.User, controllerName string, tag names.CloudTag, cloud jujuparams.Cloud, force bool) error
	AddHostedCloud_                    func(ctx context.Context, user *openfga.User, tag names.CloudTag, cloud jujuparams.Cloud, force bool) error
	AddServiceAccount_                 func(ctx context.Context, u *openfga.User, clientId string) error
	CopyServiceAccountCredential_      func(ctx context.Context, u *openfga.User, svcAcc *openfga.User, cloudCredentialTag names.CloudCredentialTag) (names.CloudCredentialTag, []jujuparams.UpdateCredentialModelResult, error)
	DestroyOffer_                      func(ctx context.Context, user *openfga.User, offerURL string, force bool) error
	FindApplicationOffers_             func(ctx context.Context, user *openfga.User, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error)
	FindAuditEvents_                   func(ctx context.Context, user *openfga.User, filter db.AuditLogFilter) ([]dbmodel.AuditLogEntry, error)
	ForEachCloud_                      func(ctx context.Context, user *openfga.User, f func(*dbmodel.Cloud) error) error
	ForEachUserCloud_                  func(ctx context.Context, user *openfga.User, f func(*dbmodel.Cloud) error) error
	ForEachUserCloudCredential_        func(ctx context.Context, u *dbmodel.Identity, ct names.CloudTag, f func(cred *dbmodel.CloudCredential) error) error
	GetApplicationOffer_               func(ctx context.Context, user *openfga.User, offerURL string) (*jujuparams.ApplicationOfferAdminDetailsV5, error)
	GetApplicationOfferConsumeDetails_ func(ctx context.Context, user *openfga.User, details *jujuparams.ConsumeOfferDetails, v bakery.Version) error
	GetCloud_                          func(ctx context.Context, u *openfga.User, tag names.CloudTag) (dbmodel.Cloud, error)
	GetCloudCredential_                func(ctx context.Context, user *openfga.User, tag names.CloudCredentialTag) (*dbmodel.CloudCredential, error)
	GetCloudCredentialAttributes_      func(ctx context.Context, u *openfga.User, cred *dbmodel.CloudCredential, hidden bool) (attrs map[string]string, redacted []string, err error)
	GetCredentialStore_                func() jimmcreds.CredentialStore
	InitiateInternalMigration_         func(ctx context.Context, user *openfga.User, modelNameOrUUID string, targetController string) (jujuparams.InitiateMigrationResult, error)
	InitiateMigration_                 func(ctx context.Context, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error)
	ListApplicationOffers_             func(ctx context.Context, user *openfga.User, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error)
	ListResources_                     func(ctx context.Context, user *openfga.User, filter pagination.LimitOffsetPagination, namePrefixFilter, typeFilter string) ([]db.Resource, error)
	Offer_                             func(ctx context.Context, user *openfga.User, offer jimm.AddApplicationOfferParams) error
	PubSubHub_                         func() *pubsub.Hub
	PurgeLogs_                         func(ctx context.Context, user *openfga.User, before time.Time) (int64, error)
	RemoveCloud_                       func(ctx context.Context, u *openfga.User, ct names.CloudTag) error
	RemoveCloudFromController_         func(ctx context.Context, u *openfga.User, controllerName string, ct names.CloudTag) error
	ResourceTag_                       func() names.ControllerTag
	RevokeCloudCredential_             func(ctx context.Context, user *dbmodel.Identity, tag names.CloudCredentialTag, force bool) error
	UpdateApplicationOffer_            func(ctx context.Context, controller *dbmodel.Controller, offerUUID string, removed bool) error
	UpdateCloud_                       func(ctx context.Context, u *openfga.User, ct names.CloudTag, cloud jujuparams.Cloud) error
	UpdateCloudCredential_             func(ctx context.Context, u *openfga.User, args jimm.UpdateCloudCredentialArgs) ([]jujuparams.UpdateCredentialModelResult, error)
	ListModels_                        func(ctx context.Context, user *openfga.User) ([]base.UserModel, error)
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
func (j *JIMM) AddHostedCloud(ctx context.Context, user *openfga.User, tag names.CloudTag, cloud jujuparams.Cloud, force bool) error {
	if j.AddHostedCloud_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.AddHostedCloud_(ctx, user, tag, cloud, force)
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

func (j *JIMM) DestroyOffer(ctx context.Context, user *openfga.User, offerURL string, force bool) error {
	if j.DestroyOffer_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.DestroyOffer_(ctx, user, offerURL, force)
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

func (j *JIMM) GetCredentialStore() jimmcreds.CredentialStore {
	if j.GetCredentialStore_ == nil {
		return nil
	}
	return j.GetCredentialStore_()
}

func (j *JIMM) RoleManager() jimm.RoleManager {
	if j.RoleManager_ == nil {
		return nil
	}
	return j.RoleManager_()
}

func (j *JIMM) GroupManager() jimm.GroupManager {
	if j.GroupManager_ == nil {
		return nil
	}
	return j.GroupManager_()
}

func (j *JIMM) IdentityManager() jimm.IdentityManager {
	if j.IdentityManager_ == nil {
		return nil
	}
	return j.IdentityManager_()
}

func (j *JIMM) LoginManager() jimm.LoginManager {
	if j.LoginManager_ == nil {
		return nil
	}
	return j.LoginManager_()
}

func (j *JIMM) PermissionManager() jimm.PermissionManager {
	if j.PermissionManager_ == nil {
		return nil
	}
	return j.PermissionManager_()
}

func (j *JIMM) InitiateMigration(ctx context.Context, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error) {
	if j.InitiateMigration_ == nil {
		return jujuparams.InitiateMigrationResult{}, errors.E(errors.CodeNotImplemented)
	}
	return j.InitiateMigration_(ctx, user, spec)
}
func (j *JIMM) InitiateInternalMigration(ctx context.Context, user *openfga.User, modelNameOrUUID string, targetController string) (jujuparams.InitiateMigrationResult, error) {
	if j.InitiateInternalMigration_ == nil {
		return jujuparams.InitiateMigrationResult{}, errors.E(errors.CodeNotImplemented)
	}
	return j.InitiateInternalMigration_(ctx, user, modelNameOrUUID, targetController)
}
func (j *JIMM) ListApplicationOffers(ctx context.Context, user *openfga.User, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error) {
	if j.ListApplicationOffers_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ListApplicationOffers_(ctx, user, filters...)
}
func (j *JIMM) ListResources(ctx context.Context, user *openfga.User, filter pagination.LimitOffsetPagination, namePrefixFilter, typeFilter string) ([]db.Resource, error) {
	if j.ListResources_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ListResources_(ctx, user, filter, namePrefixFilter, typeFilter)
}
func (j *JIMM) Offer(ctx context.Context, user *openfga.User, offer jimm.AddApplicationOfferParams) error {
	if j.Offer_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.Offer_(ctx, user, offer)
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
func (j *JIMM) ResourceTag() names.ControllerTag {
	if j.ResourceTag_ == nil {
		return names.NewControllerTag(uuid.NewString())
	}
	return j.ResourceTag_()
}
func (j *JIMM) RevokeCloudCredential(ctx context.Context, user *dbmodel.Identity, tag names.CloudCredentialTag, force bool) error {
	if j.RevokeCloudCredential_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RevokeCloudCredential_(ctx, user, tag, force)
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
func (j *JIMM) ListModels(ctx context.Context, user *openfga.User) ([]base.UserModel, error) {
	if j.ListModels_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ListModels_(ctx, user)
}
