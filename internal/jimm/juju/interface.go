// Copyright 2026 Canonical.

package juju

import (
	"context"
	"time"

	"github.com/juju/juju/api/base"
	jujucloud "github.com/juju/juju/cloud"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/semversion"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v6"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jujuclient"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// A Dialer provides a connection to a controller.
type Dialer interface {
	// Dial creates an API connection to a controller. If the given
	// model-tag is non-zero the connection will be to that model,
	// otherwise the connection is to the controller. After successfully
	// dialing the controller the UUID, AgentVersion and HostPorts fields
	// in the given controller should be updated to the values provided
	// by the controller.
	Dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag, user *openfga.User) (API, error)
}

// An API is the interface JIMM uses to access the API on a controller.
type API interface {
	// API implements the base.APICallCloser so that we can
	// use the juju api clients to interact with juju controllers.
	base.APICallCloser

	// Abort aborts a model migration.
	Abort(context.Context, string) error

	// Activate activates a model on the controller.
	// It is used to activate a model that has been migrated from another controller.
	Activate(ctx context.Context, modelUUID string, sourceInfo migration.SourceControllerInfo, relatedModels []string) error

	// AddCloud adds a new cloud.
	AddCloud(context.Context, names.CloudTag, jujucloud.Cloud, bool) error

	// AdoptResources adopts resources from a model with the given UUID
	// and controller version. This is used to adopt resources from a
	// model that is being migrated.
	AdoptResources(context.Context, string, semversion.Number) error

	// ChangeModelCredential replaces cloud credential for a given model with the provided one.
	ChangeModelCredential(context.Context, names.ModelTag, names.CloudCredentialTag) error

	// CheckCredentialModels checks that an updated credential can be used
	// with the associated models.
	CheckCredentialModels(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error)

	// CheckMachines compares the machines in state with the ones
	// reported by the provider and reports any discrepancies.
	CheckMachines(context.Context, string) ([]error, error)

	// Import imports a model from a serialized format.
	Import(context.Context, []byte) error

	// Close closes the API connection.
	Close() error

	// Cloud fetches the cloud data for the given cloud.
	Cloud(context.Context, names.CloudTag) (jujucloud.Cloud, error)

	// Clouds returns the set of clouds supported by the controller.
	Clouds(context.Context) (map[names.CloudTag]jujucloud.Cloud, error)

	// ControllerModelSummary fetches the model summary of the model on the
	// controller that hosts the controller machines.
	ControllerModelSummary(context.Context) (base.UserModelSummary, error)

	// ControllerConfig fetches the controller configuration.
	ControllerConfig(context.Context) (jujucontroller.Config, error)

	// CreateModel creates a new model.
	CreateModel(context.Context, *jujuclient.CreateModelArgs) (base.ModelInfo, error)

	// DestroyApplicationOffer destroys an application offer.
	DestroyApplicationOffer(ctx context.Context, offerURL string, force bool) error

	// DestroyModel destroys a model.
	DestroyModel(ctx context.Context, tag names.ModelTag, destroyStorage *bool, force *bool, maxWait, timeout *time.Duration) error

	// DumpModel collects a database-agnostic dump of a model.
	DumpModel(ctx context.Context, tag names.ModelTag) (map[string]any, error)

	// DumpModelDB collects a database dump of a model.
	DumpModelDB(context.Context, names.ModelTag) (map[string]any, error)

	// FindApplicationOffers finds application offers that match the filter.
	FindApplicationOffers(context.Context, []crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error)

	// GetApplicationOffer completes the given ApplicationOfferAdminDetails structure.
	GetApplicationOffer(ctx context.Context, urlStr string) (*crossmodel.ApplicationOfferDetails, error)

	// GetApplicationOfferConsumeDetails gets the details required to
	// consume an application offer
	GetApplicationOfferConsumeDetails(ctx context.Context, url string) (jujuparams.ConsumeOfferDetails, error)

	// GrantJIMMModelAdmin makes the JIMM user an admin on a model.
	GrantJIMMModelAdmin(context.Context, names.ModelTag) error

	// IsBroken returns true if the API connection has failed.
	IsBroken() bool

	// LatestLogTime returns the time of the latest log record
	// seen by the controller for the given model.
	LatestLogTime(context.Context, string) (time.Time, error)

	// ListApplicationOffers lists application offers that match the filter.
	ListApplicationOffers(context.Context, []crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error)

	// ListModelSummaries lists models summaries
	ListModelSummaries(context.Context, jujuparams.ModelSummariesRequest) ([]base.UserModelSummary, error)

	// ModelConfigSchema fetches the model config schema for the given
	// provider type.
	ModelConfigSchema(context.Context, string) (map[string]jujuparams.ModelConfigSchemaField, error)

	// ModelInfo fetches a model's ModelInfo.
	ModelInfo(context.Context, names.ModelTag) (jujuclient.ModelInfo, error)

	// ModelStatus fetches a model's ModelStatus.
	ModelStatus(context.Context, names.ModelTag) (base.ModelStatus, error)

	// Offer creates a new application-offer.
	Offer(context.Context, jujuclient.OfferParams) error

	// PreChecks runs pre-checks for a model migration.
	Prechecks(ctx context.Context, model jujuparams.MigrationModelInfo) error

	// RemoveCloud removes a cloud.
	RemoveCloud(context.Context, names.CloudTag) error

	// RevokeCredential revokes a credential.
	RevokeCredential(context.Context, names.CloudCredentialTag) error

	// SupportsModelSummaryWatcher returns true if the connection supports
	// a ModelSummaryWatcher.
	SupportsModelSummaryWatcher() bool

	// Status returns the status of the juju model.
	Status(ctx context.Context, patterns []string) (*jujuparams.FullStatus, error)

	// UpdateCloud updates a cloud definition.
	UpdateCloud(context.Context, names.CloudTag, jujucloud.Cloud) error

	// UpdateCredential updates a credential.
	UpdateCloudsCredentialForce(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error)

	// ValidateModelUpgrade validates that a model can be upgraded.
	ValidateModelUpgrade(ctx context.Context, model names.ModelTag, force bool) error

	// WatchAllModelSummaries creates a ModelSummaryWatcher.
	WatchAllModelSummaries(context.Context) (jujuclient.SummaryWatcher, error)

	// ListFilesystems lists filesystems for desired machines.
	// If no machines provided, a list of all filesystems is returned.
	ListFilesystems(ctx context.Context, machines []string) ([]jujuparams.FilesystemDetailsListResult, error)

	// ListVolumes lists volumes for desired machines.
	// If no machines provided, a list of all volumes is returned.
	ListVolumes(ctx context.Context, machines []string) ([]jujuparams.VolumeDetailsListResult, error)

	// ListStorageDetails lists all storage.
	ListStorageDetails(context.Context) ([]jujuparams.StorageDetails, error)

	// ListModels returns all UserModel's on the controller.
	ListModels(context.Context) ([]base.UserModel, error)

	// CredentialContents returns contents of the credential values for the specified
	// cloud and credential name. Secrets will be included if requested.
	CredentialContents(ctx context.Context, cloud string, credential string, withSecrets bool) ([]jujuparams.CredentialContentResult, error)

	// AbortModelUpgrade aborts and archives any in-progress model upgrade.
	AbortModelUpgrade(ctx context.Context, modelUUID string) error

	// UpgradeModel upgrades the model to the provided agent version.
	// The provided target version could be semversion.Zero, in which case the
	// best version is selected by the controller and returned as ChosenVersion
	// in the result.
	UpgradeModel(
		ctx context.Context,
		modelUUID string,
		targetVersion semversion.Number,
		stream string,
		ignoreAgentVersions bool,
		dryRun bool,
	) (semversion.Number, error)

	// ControllerModelUUID returns the UUID of the controller model on the
	// connected controller. It reads the model configuration of the model
	// the connection is scoped to (the controller model) and returns its
	// UUID.
	ControllerModelUUID(context.Context) (string, error)
}

// PermissionManager provides a way to manage permissions within JIMM.
type PermissionManager interface {
	// GetUserCloudAccess returns the user's level of access to a cloud.
	GetUserCloudAccess(ctx context.Context, user *openfga.User, cloud names.CloudTag) (string, error)
	// GetUserModelAccess returns the user's level of access to a model.
	GetUserModelAccess(ctx context.Context, user *openfga.User, model names.ModelTag) (string, error)
}

// MigrationTokenGenerator is an interface for generating migration tokens
// that are used to authenticate Juju controllers with JIMM during model migrations.
type MigrationTokenGenerator interface {
	// NewToken generates a new migration token with the specified user, groups, and model tag.
	// The token allows a client to authenticate with JIMM as the specified user.
	// Groups are included in the token so that the controller can verify group membership.
	NewMigrationToken(ctx context.Context, username string, groups []string) (string, error)
}
