// Copyright 2025 Canonical.

package mocks

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/juju/api/base"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	jimmcreds "github.com/canonical/jimm/v3/internal/jimm/credentials"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// JujuManager is a default implementation of the jimm.JujuManager interface.
// The mocks in jimm_juju_controller_mock.go and jimm_juju_model_mock.go also belong in here.
type JujuManager struct {
	ControllerService
	ModelManager
	AddAuditLogEntry_                  func(ale *dbmodel.AuditLogEntry)
	AddCloudToController_              func(ctx context.Context, user *openfga.User, controllerName string, tag names.CloudTag, cloud jujuparams.Cloud, force bool) error
	AddHostedCloud_                    func(ctx context.Context, user *openfga.User, tag names.CloudTag, cloud jujuparams.Cloud, force bool) error
	CopyCredential_                    func(ctx context.Context, originalUser *openfga.User, newUser *openfga.User, cred names.CloudCredentialTag) (names.CloudCredentialTag, []jujuparams.UpdateCredentialModelResult, error)
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
	Offer_                             func(ctx context.Context, user *openfga.User, offer juju.AddApplicationOfferParams) error
	PurgeLogs_                         func(ctx context.Context, user *openfga.User, before time.Time) (int64, error)
	RemoveCloud_                       func(ctx context.Context, u *openfga.User, ct names.CloudTag) error
	RemoveCloudFromController_         func(ctx context.Context, u *openfga.User, controllerName string, ct names.CloudTag) error
	RevokeCloudCredential_             func(ctx context.Context, user *dbmodel.Identity, tag names.CloudCredentialTag, force bool) error
	UpdateApplicationOffer_            func(ctx context.Context, controller *dbmodel.Controller, offerUUID string, removed bool) error
	UpdateCloud_                       func(ctx context.Context, u *openfga.User, ct names.CloudTag, cloud jujuparams.Cloud) error
	UpdateCloudCredential_             func(ctx context.Context, u *openfga.User, args juju.UpdateCloudCredentialArgs) ([]jujuparams.UpdateCredentialModelResult, error)
	ListModels_                        func(ctx context.Context, user *openfga.User) ([]base.UserModel, error)
	GrantOfferAccessOnController_      func(ctx context.Context, user *openfga.User, ut names.UserTag, offerURL string, access jujuparams.OfferAccessPermission) error
	RevokeOfferAccessOnController_     func(ctx context.Context, user *openfga.User, ut names.UserTag, offerURL string, access jujuparams.OfferAccessPermission) error

	// These mocks can be removed soon once the jujuManager interface is updated.
	UpdateMetrics_      func(ctx context.Context)
	CleanupDyingModels_ func(ctx context.Context) error
}

func (j *JujuManager) AddAuditLogEntry(ale *dbmodel.AuditLogEntry) {
	if j.AddAuditLogEntry_ == nil {
		panic("not implemented")
	}
	j.AddAuditLogEntry(ale)
}
func (j *JujuManager) AddCloudToController(ctx context.Context, user *openfga.User, controllerName string, tag names.CloudTag, cloud jujuparams.Cloud, force bool) error {
	if j.AddCloudToController_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.AddCloudToController_(ctx, user, controllerName, tag, cloud, force)
}
func (j *JujuManager) AddHostedCloud(ctx context.Context, user *openfga.User, tag names.CloudTag, cloud jujuparams.Cloud, force bool) error {
	if j.AddHostedCloud_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.AddHostedCloud_(ctx, user, tag, cloud, force)
}
func (j *JujuManager) CopyCredential(ctx context.Context, originalUser *openfga.User, newUser *openfga.User, cred names.CloudCredentialTag) (names.CloudCredentialTag, []jujuparams.UpdateCredentialModelResult, error) {
	if j.CopyCredential_ == nil {
		return names.CloudCredentialTag{}, nil, errors.E(errors.CodeNotImplemented)
	}
	return j.CopyCredential_(ctx, originalUser, newUser, cred)
}
func (j *JujuManager) DestroyOffer(ctx context.Context, user *openfga.User, offerURL string, force bool) error {
	if j.DestroyOffer_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.DestroyOffer_(ctx, user, offerURL, force)
}
func (j *JujuManager) FindApplicationOffers(ctx context.Context, user *openfga.User, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error) {
	if j.FindApplicationOffers_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.FindApplicationOffers_(ctx, user, filters...)
}
func (j *JujuManager) FindAuditEvents(ctx context.Context, user *openfga.User, filter db.AuditLogFilter) ([]dbmodel.AuditLogEntry, error) {
	if j.FindAuditEvents_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.FindAuditEvents_(ctx, user, filter)
}
func (j *JujuManager) ForEachCloud(ctx context.Context, user *openfga.User, f func(*dbmodel.Cloud) error) error {
	if j.ForEachCloud_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.ForEachCloud_(ctx, user, f)
}

func (j *JujuManager) ForEachUserCloud(ctx context.Context, user *openfga.User, f func(*dbmodel.Cloud) error) error {
	if j.ForEachUserCloud_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.ForEachUserCloud_(ctx, user, f)
}
func (j *JujuManager) ForEachUserCloudCredential(ctx context.Context, u *dbmodel.Identity, ct names.CloudTag, f func(cred *dbmodel.CloudCredential) error) error {
	if j.ForEachUserCloudCredential_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.ForEachUserCloudCredential_(ctx, u, ct, f)
}

func (j *JujuManager) GetApplicationOffer(ctx context.Context, user *openfga.User, offerURL string) (*jujuparams.ApplicationOfferAdminDetailsV5, error) {
	if j.GetApplicationOffer_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.GetApplicationOffer_(ctx, user, offerURL)
}
func (j *JujuManager) GetApplicationOfferConsumeDetails(ctx context.Context, user *openfga.User, details *jujuparams.ConsumeOfferDetails, v bakery.Version) error {
	if j.GetApplicationOfferConsumeDetails_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.GetApplicationOfferConsumeDetails_(ctx, user, details, v)
}
func (j *JujuManager) GetCloud(ctx context.Context, u *openfga.User, tag names.CloudTag) (dbmodel.Cloud, error) {
	if j.GetCloud_ == nil {
		return dbmodel.Cloud{}, errors.E(errors.CodeNotImplemented)
	}
	return j.GetCloud_(ctx, u, tag)
}
func (j *JujuManager) GetCloudCredential(ctx context.Context, user *openfga.User, tag names.CloudCredentialTag) (*dbmodel.CloudCredential, error) {
	if j.GetCloudCredential_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.GetCloudCredential_(ctx, user, tag)
}
func (j *JujuManager) GetCloudCredentialAttributes(ctx context.Context, u *openfga.User, cred *dbmodel.CloudCredential, hidden bool) (attrs map[string]string, redacted []string, err error) {
	if j.GetCloudCredentialAttributes_ == nil {
		return nil, nil, errors.E(errors.CodeNotImplemented)
	}
	return j.GetCloudCredentialAttributes_(ctx, u, cred, hidden)
}

func (j *JujuManager) GetCredentialStore() jimmcreds.CredentialStore {
	if j.GetCredentialStore_ == nil {
		return nil
	}
	return j.GetCredentialStore_()
}

func (j *JujuManager) InitiateMigration(ctx context.Context, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error) {
	if j.InitiateMigration_ == nil {
		return jujuparams.InitiateMigrationResult{}, errors.E(errors.CodeNotImplemented)
	}
	return j.InitiateMigration_(ctx, user, spec)
}
func (j *JujuManager) InitiateInternalMigration(ctx context.Context, user *openfga.User, modelNameOrUUID string, targetController string) (jujuparams.InitiateMigrationResult, error) {
	if j.InitiateInternalMigration_ == nil {
		return jujuparams.InitiateMigrationResult{}, errors.E(errors.CodeNotImplemented)
	}
	return j.InitiateInternalMigration_(ctx, user, modelNameOrUUID, targetController)
}
func (j *JujuManager) ListApplicationOffers(ctx context.Context, user *openfga.User, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error) {
	if j.ListApplicationOffers_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ListApplicationOffers_(ctx, user, filters...)
}
func (j *JujuManager) ListResources(ctx context.Context, user *openfga.User, filter pagination.LimitOffsetPagination, namePrefixFilter, typeFilter string) ([]db.Resource, error) {
	if j.ListResources_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ListResources_(ctx, user, filter, namePrefixFilter, typeFilter)
}
func (j *JujuManager) Offer(ctx context.Context, user *openfga.User, offer juju.AddApplicationOfferParams) error {
	if j.Offer_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.Offer_(ctx, user, offer)
}
func (j *JujuManager) PurgeLogs(ctx context.Context, user *openfga.User, before time.Time) (int64, error) {
	if j.PurgeLogs_ == nil {
		return 0, errors.E(errors.CodeNotImplemented)
	}
	return j.PurgeLogs_(ctx, user, before)
}
func (j *JujuManager) RemoveCloud(ctx context.Context, u *openfga.User, ct names.CloudTag) error {
	if j.RemoveCloud_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RemoveCloud_(ctx, u, ct)
}
func (j *JujuManager) RemoveCloudFromController(ctx context.Context, u *openfga.User, controllerName string, ct names.CloudTag) error {
	if j.RemoveCloudFromController_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RemoveCloudFromController_(ctx, u, controllerName, ct)
}
func (j *JujuManager) RevokeCloudCredential(ctx context.Context, user *dbmodel.Identity, tag names.CloudCredentialTag, force bool) error {
	if j.RevokeCloudCredential_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RevokeCloudCredential_(ctx, user, tag, force)
}
func (j *JujuManager) UpdateApplicationOffer(ctx context.Context, controller *dbmodel.Controller, offerUUID string, removed bool) error {
	if j.UpdateApplicationOffer_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.UpdateApplicationOffer_(ctx, controller, offerUUID, removed)
}
func (j *JujuManager) UpdateCloud(ctx context.Context, u *openfga.User, ct names.CloudTag, cloud jujuparams.Cloud) error {
	if j.UpdateCloud_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.UpdateCloud_(ctx, u, ct, cloud)
}
func (j *JujuManager) UpdateCloudCredential(ctx context.Context, u *openfga.User, args juju.UpdateCloudCredentialArgs) ([]jujuparams.UpdateCredentialModelResult, error) {
	if j.UpdateCloudCredential_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.UpdateCloudCredential_(ctx, u, args)
}
func (j *JujuManager) ListModels(ctx context.Context, user *openfga.User) ([]base.UserModel, error) {
	if j.ListModels_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ListModels_(ctx, user)
}
func (j *JujuManager) UpdateMetrics(ctx context.Context) {
	if j.UpdateMetrics_ == nil {
		return
	}
	j.UpdateMetrics_(ctx)
}
func (j *JujuManager) CleanupDyingModels(ctx context.Context) error {
	if j.CleanupDyingModels_ == nil {
		return nil
	}
	return j.CleanupDyingModels_(ctx)
}

func (j *JujuManager) GrantOfferAccessOnController(ctx context.Context, user *openfga.User, ut names.UserTag, offerURL string, access jujuparams.OfferAccessPermission) error {
	if j.GrantOfferAccessOnController_ == nil {
		return nil
	}
	return j.GrantOfferAccessOnController_(ctx, user, ut, offerURL, access)
}

func (j *JujuManager) RevokeOfferAccessOnController(ctx context.Context, user *openfga.User, ut names.UserTag, offerURL string, access jujuparams.OfferAccessPermission) error {
	if j.RevokeOfferAccessOnController_ == nil {
		return nil
	}
	return j.RevokeOfferAccessOnController_(ctx, user, ut, offerURL, access)
}
