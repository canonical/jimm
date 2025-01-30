// Copyright 2025 Canonical.

package juju

import (
	"context"
	"net/url"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/crossmodel"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
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
	Dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag, requiredPermissions map[string]string) (API, error)
}

// An API is the interface JIMM uses to access the API on a controller.
type API interface {
	// API implements the base.APICallCloser so that we can
	// use the juju api clients to interact with juju controllers.
	base.APICallCloser

	// AddCloud adds a new cloud.
	AddCloud(context.Context, names.CloudTag, jujuparams.Cloud, bool) error

	// ChangeModelCredential replaces cloud credential for a given model with the provided one.
	ChangeModelCredential(context.Context, names.ModelTag, names.CloudCredentialTag) error

	// CheckCredentialModels checks that an updated credential can be used
	// with the associated models.
	CheckCredentialModels(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error)

	// Close closes the API connection.
	Close() error

	// Cloud fetches the cloud data for the given cloud.
	Cloud(context.Context, names.CloudTag, *jujuparams.Cloud) error

	// CloudInfo fetches the cloud information for the cloud with the given
	// tag.
	CloudInfo(context.Context, names.CloudTag, *jujuparams.CloudInfo) error

	// Clouds returns the set of clouds supported by the controller.
	Clouds(context.Context) (map[names.CloudTag]jujuparams.Cloud, error)

	// ControllerModelSummary fetches the model summary of the model on the
	// controller that hosts the controller machines.
	ControllerModelSummary(context.Context, *jujuparams.ModelSummary) error

	// CreateModel creates a new model.
	CreateModel(context.Context, *jujuparams.ModelCreateArgs, *jujuparams.ModelInfo) error

	// DestroyApplicationOffer destroys an application offer.
	DestroyApplicationOffer(context.Context, string, bool) error

	// DestroyModel destroys a model.
	DestroyModel(context.Context, names.ModelTag, *bool, *bool, *time.Duration, *time.Duration) error

	// ConnectStream creates a new connection to a streaming endpoint.
	ConnectStream(string, url.Values) (base.Stream, error)

	// DumpModel collects a database-agnostic dump of a model.
	DumpModel(context.Context, names.ModelTag, bool) (string, error)

	// DumpModelDB collects a database dump of a model.
	DumpModelDB(context.Context, names.ModelTag) (map[string]interface{}, error)

	// FindApplicationOffers finds application offers that match the
	// filter.
	FindApplicationOffers(context.Context, []jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error)

	// GetApplicationOffer completes the given ApplicationOfferAdminDetails
	// structure.
	GetApplicationOffer(context.Context, *jujuparams.ApplicationOfferAdminDetailsV5) error

	// GetApplicationOfferConsumeDetails gets the details required to
	// consume an application offer
	GetApplicationOfferConsumeDetails(context.Context, names.UserTag, *jujuparams.ConsumeOfferDetails, bakery.Version) error

	// GrantApplicationOfferAccess grants access to an application offer to
	// a user.
	GrantApplicationOfferAccess(context.Context, string, names.UserTag, jujuparams.OfferAccessPermission) error

	// GrantCloudAccess grants cloud access to a user.
	GrantCloudAccess(context.Context, names.CloudTag, names.UserTag, string) error

	// GrantJIMMModelAdmin makes the JIMM user an admin on a model.
	GrantJIMMModelAdmin(context.Context, names.ModelTag) error

	// GrantModelAccess grants model access to a user.
	GrantModelAccess(context.Context, names.ModelTag, names.UserTag, jujuparams.UserAccessPermission) error

	// IsBroken returns true if the API connection has failed.
	IsBroken() bool

	// ListApplicationOffers lists application offers that match the
	// filter.
	ListApplicationOffers(context.Context, []jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error)

	// ListModelSummaries lists models summaries
	ListModelSummaries(context.Context, jujuparams.ModelSummariesRequest) (jujuparams.ModelSummaryResults, error)

	// ModelInfo fetches a model's ModelInfo.
	ModelInfo(context.Context, *jujuparams.ModelInfo) error

	// ModelStatus fetches a model's ModelStatus.
	ModelStatus(context.Context, *jujuparams.ModelStatus) error

	// ModelSummaryWatcherNext returns the next set of model summaries from
	// the watcher.
	ModelSummaryWatcherNext(context.Context, string) ([]jujuparams.ModelAbstract, error)

	// ModelSummaryWatcherStop stops a model summary watcher.
	ModelSummaryWatcherStop(context.Context, string) error

	// Offer creates a new application-offer.
	Offer(context.Context, crossmodel.OfferURL, jujuparams.AddApplicationOffer) error

	// Ping tests the connection is working.
	Ping(context.Context) error

	// RemoveCloud removes a cloud.
	RemoveCloud(context.Context, names.CloudTag) error

	// RevokeApplicationOfferAccess revokes access to an application offer
	// from a user.
	RevokeApplicationOfferAccess(context.Context, string, names.UserTag, jujuparams.OfferAccessPermission) error

	// RevokeCloudAccess revokes cloud access from a user.
	RevokeCloudAccess(context.Context, names.CloudTag, names.UserTag, string) error

	// RevokeCredential revokes a credential.
	RevokeCredential(context.Context, names.CloudCredentialTag) error

	// RevokeModelAccess revokes model access from a user.
	RevokeModelAccess(context.Context, names.ModelTag, names.UserTag, jujuparams.UserAccessPermission) error

	// SupportsCheckCredentialModels returns true if the
	// CheckCredentialModels method can be used.
	SupportsCheckCredentialModels() bool

	// SupportsModelSummaryWatcher returns true if the connection supports
	// a ModelSummaryWatcher.
	SupportsModelSummaryWatcher() bool

	// Status returns the status of the juju model.
	Status(ctx context.Context, patterns []string) (*jujuparams.FullStatus, error)

	// UpdateCloud updates a cloud definition.
	UpdateCloud(context.Context, names.CloudTag, jujuparams.Cloud) error

	// UpdateCredential updates a credential.
	UpdateCredential(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error)

	// ValidateModelUpgrade validates that a model can be upgraded.
	ValidateModelUpgrade(context.Context, names.ModelTag, bool) error

	// WatchAllModelSummaries creates a ModelSummaryWatcher.
	WatchAllModelSummaries(context.Context) (string, error)

	// ListFilesystems lists filesystems for desired machines.
	// If no machines provided, a list of all filesystems is returned.
	ListFilesystems(ctx context.Context, machines []string) ([]jujuparams.FilesystemDetailsListResult, error)

	// ListVolumes lists volumes for desired machines.
	// If no machines provided, a list of all volumes is returned.
	ListVolumes(ctx context.Context, machines []string) ([]jujuparams.VolumeDetailsListResult, error)

	// ListStorageDetails lists all storage.
	ListStorageDetails(ctx context.Context) ([]jujuparams.StorageDetails, error)

	// ListModels returns all UserModel's on the controller.
	ListModels(ctx context.Context) ([]base.UserModel, error)
}

// PermissionManager provides a way to manage permissions within JIMM.
type PermissionChecker interface {
	// GetUserCloudAccess returns the user's level of access to a cloud.
	GetUserCloudAccess(ctx context.Context, user *openfga.User, cloud names.CloudTag) (string, error)
	// GetUserModelAccess returns the user's level of access to a model.
	GetUserModelAccess(ctx context.Context, user *openfga.User, model names.ModelTag) (string, error)
}
