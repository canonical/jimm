// Copyright 2024 Canonical.

package jujuapi

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
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/pubsub"
)

// JIMM defines a comprehensive interface for all sort of operations with our application logic.
type JIMM interface {
	ControllerService
	ModelManager
	AddAuditLogEntry(ale *dbmodel.AuditLogEntry)
	AddCloudToController(ctx context.Context, user *openfga.User, controllerName string, tag names.CloudTag, cloud jujuparams.Cloud, force bool) error
	AddHostedCloud(ctx context.Context, user *openfga.User, tag names.CloudTag, cloud jujuparams.Cloud, force bool) error
	AddServiceAccount(ctx context.Context, u *openfga.User, clientId string) error
	CopyServiceAccountCredential(ctx context.Context, u *openfga.User, svcAcc *openfga.User, cloudCredentialTag names.CloudCredentialTag) (names.CloudCredentialTag, []jujuparams.UpdateCredentialModelResult, error)
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
	RoleManager() jimm.RoleManager
	GroupManager() jimm.GroupManager
	IdentityManager() jimm.IdentityManager
	LoginManager() jimm.LoginManager
	PermissionManager() jimm.PermissionManager

	InitiateInternalMigration(ctx context.Context, user *openfga.User, modelNameOrUUID string, targetController string) (jujuparams.InitiateMigrationResult, error)
	InitiateMigration(ctx context.Context, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error)
	ListApplicationOffers(ctx context.Context, user *openfga.User, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error)
	ListModels(ctx context.Context, user *openfga.User) ([]base.UserModel, error)
	ListResources(ctx context.Context, user *openfga.User, filter pagination.LimitOffsetPagination, namePrefixFilter, typeFilter string) ([]db.Resource, error)
	Offer(ctx context.Context, user *openfga.User, offer jimm.AddApplicationOfferParams) error
	PubSubHub() *pubsub.Hub
	PurgeLogs(ctx context.Context, user *openfga.User, before time.Time) (int64, error)
	RemoveCloud(ctx context.Context, u *openfga.User, ct names.CloudTag) error
	RemoveCloudFromController(ctx context.Context, u *openfga.User, controllerName string, ct names.CloudTag) error
	RemoveController(ctx context.Context, user *openfga.User, controllerName string, force bool) error
	ResourceTag() names.ControllerTag
	RevokeCloudCredential(ctx context.Context, user *dbmodel.Identity, tag names.CloudCredentialTag, force bool) error

	UpdateCloud(ctx context.Context, u *openfga.User, ct names.CloudTag, cloud jujuparams.Cloud) error
	UpdateCloudCredential(ctx context.Context, u *openfga.User, args jimm.UpdateCloudCredentialArgs) ([]jujuparams.UpdateCredentialModelResult, error)
}
