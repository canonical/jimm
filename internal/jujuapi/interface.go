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
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/pubsub"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

// JIMM defines a comprehensive interface for all sort of operations with our application logic.
type JIMM interface {
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
	RoleManager() jimm.RoleManager
	GroupManager() jimm.GroupManager
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
	InitiateInternalMigration(ctx context.Context, user *openfga.User, modelNameOrUUID string, targetController string) (jujuparams.InitiateMigrationResult, error)
	InitiateMigration(ctx context.Context, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error)
	ListApplicationOffers(ctx context.Context, user *openfga.User, filters ...jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error)
	ListIdentities(ctx context.Context, user *openfga.User, pagination pagination.LimitOffsetPagination, match string) ([]openfga.User, error)
	ListModels(ctx context.Context, user *openfga.User) ([]base.UserModel, error)
	ListResources(ctx context.Context, user *openfga.User, filter pagination.LimitOffsetPagination, namePrefixFilter, typeFilter string) ([]db.Resource, error)
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
	UpdateCloud(ctx context.Context, u *openfga.User, ct names.CloudTag, cloud jujuparams.Cloud) error
	UpdateCloudCredential(ctx context.Context, u *openfga.User, args jimm.UpdateCloudCredentialArgs) ([]jujuparams.UpdateCredentialModelResult, error)
	UserLogin(ctx context.Context, identityName string) (*openfga.User, error)
}
