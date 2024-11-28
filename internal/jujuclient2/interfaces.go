// Copyright 2024 Canonical.
package jujuclient2

import (
	"context"
	"net/url"
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/client/cloud"
	"github.com/juju/juju/api/controller/controller"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/crossmodel"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
)

type ModelManagerClient interface {
	// ChangeModelCredential replaces cloud credential for a given model with the provided one
	ChangeModelCredential(model names.ModelTag, credential names.CloudCredentialTag) error

	// CreateModel creates a new model using the model config, cloud region and credential specified in the args.
	CreateModel(name string, owner string, cloud string, cloudRegion string, cloudCredential names.CloudCredentialTag, config map[string]interface{}) (base.ModelInfo, error)

	// 	DestroyModel puts the specified model into a "dying" state, which will cause the model's resources to be cleaned up, after which the model will be removed.
	DestroyModel(tag names.ModelTag, destroyStorage *bool, force *bool, maxWait *time.Duration, timeout *time.Duration) error

	// ??
	ModelInfo(tags []names.ModelTag) ([]jujuparams.ModelInfoResult, error)

	// ModelStatus returns a status summary for each model tag passed in.
	ModelStatus(tags ...names.ModelTag) ([]base.ModelStatus, error)

	//	DumpModel returns the serialized database agnostic model representation.
	DumpModel(model names.ModelTag, simplified bool) (map[string]interface{}, error)

	// DumpModelDB returns all relevant mongo documents for the model.
	DumpModelDB(model names.ModelTag) (map[string]interface{}, error)

	// GrantModel grants a user access to the specified models.
	GrantModel(user string, access string, modelUUIDs ...string) error

	// RevokeModel revokes a user's access to the specified models.
	RevokeModel(user string, access string, modelUUIDs ...string) error

	// ValidateModelUpgrade checks to see if it's possible to upgrade a model, before actually attempting to do the real environ-upgrade.
	ValidateModelUpgrade(model names.ModelTag, force bool) error

	// Close closes the connection.
	Close() error
}

type CloudClient interface {
	AddCloud(cloud jujucloud.Cloud, force bool) error

	// Cloud returns the details of the cloud with the given tag.
	Cloud(tag names.CloudTag) (jujucloud.Cloud, error)

	// CloudInfo returns details and user access for the cloud with the given tag.
	CloudInfo(tags []names.CloudTag) ([]cloud.CloudInfo, error)

	// RemoveCloud removes a cloud from the current controller.
	RemoveCloud(cloud string) error

	// Clouds returns the details of all clouds supported by the controller.
	Clouds() (map[names.CloudTag]jujucloud.Cloud, error)

	// GrantCloud grants a user access to a cloud.
	GrantCloud(user string, access string, clouds ...string) error

	// RevokeCloud revokes a user's access to a cloud.
	RevokeCloud(user string, access string, clouds ...string) error

	// RevokeCredential revokes/deletes a cloud credential.
	RevokeCredential(tag names.CloudCredentialTag, force bool) error

	// UpdateCloud updates an existing cloud on a current controller.
	UpdateCloud(cloud jujucloud.Cloud) error

	// Close closes the connection.
	Close() error
}

type ApplicationOffersClient interface {
	// FindApplicationOffers returns all application offers matching the supplied filter.
	FindApplicationOffers(filters ...crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error)

	// GetConsumeDetails returns details necessary to consume an offer at a given URL.
	GetConsumeDetails(urlStr string) (jujuparams.ConsumeOfferDetails, error)

	// ListOffers gets all remote applications that have been offered from this Juju model. Each returned application satisfies at least one of the the specified filters.
	ListOffers(filters ...crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error)

	// Offer prepares application's endpoints for consumption.
	Offer(modelUUID string, application string, endpoints []string, owner string, offerName string, desc string) ([]jujuparams.ErrorResult, error)

	// DestroyOffers removes the specified application offers.
	DestroyOffers(force bool, offerURLs ...string) error

	// GrantOffer grants a user access to the specified offers.
	GrantOffer(user string, access string, offerURLs ...string) error

	// RevokeOffer revokes a user's access to the specified offers.
	RevokeOffer(user string, access string, offerURLs ...string) error

	// Close closes the connection.
	Close() error
}

type ClientClient interface {
	// Status returns the status of the juju model.
	Status(args *client.StatusArgs) (*jujuparams.FullStatus, error)

	// Close closes the connection.
	Close() error
}

type ControllerClient interface {
	// WatchAllModelSummaries returns a SummaryWatcher, from which you can request the Next set of ModelAbstracts. This method is only valid for controller superusers and returns abstracts for all models in the controller.
	WatchAllModelSummaries() (*controller.SummaryWatcher, error)

	// WatchModelSummaries returns a SummaryWatcher, from which you can request the Next set of ModelAbstracts for all models the user can see.
	WatchModelSummaries() (*controller.SummaryWatcher, error)

	// WatchAllModels returns an AllWatcher, from which you can request the Next collection of Deltas (for all models).
	WatchAllModels() (*api.AllWatcher, error)

	// Close closes the connection.
	Close() error
}

type StorageClient interface {
	// ListFilesystems lists filesystems for desired machines. If no machines provided, a list of all filesystems is returned.
	ListFilesystems(machines []string) ([]jujuparams.FilesystemDetailsListResult, error)

	// ListVolumes lists volumes for desired machines. If no machines provided, a list of all volumes is returned.
	ListVolumes(machines []string) ([]jujuparams.VolumeDetailsListResult, error)

	// ListStorageDetails lists all storage.
	ListStorageDetails() ([]jujuparams.StorageDetails, error)

	// Close closes the connection.
	Close() error
}

// MiscClient holds methods that may not have been implemented in the api/client package yet.
type MiscClient interface {
	// UpdateCredential updates a credential.
	UpdateCredential(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error)

	// WatchAll creates a watcher that reports deltas for a specific model.
	WatchAll(context.Context) (string, error)

	// CheckCredentialModels checks that an updated credential can be used
	// with the associated models.
	CheckCredentialModels(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error)

	// ControllerModelSummary fetches the model summary of the model on the
	// controller that hosts the controller machines.
	ControllerModelSummary(context.Context, *jujuparams.ModelSummary) error

	// ConnectStream creates a new connection to a streaming endpoint.
	ConnectStream(string, url.Values) (base.Stream, error)

	// GetApplicationOffer completes the given ApplicationOfferAdminDetails
	// structure.
	GetApplicationOffer(context.Context, *jujuparams.ApplicationOfferAdminDetailsV5) error

	// GrantJIMMModelAdmin makes the JIMM user an admin on a model.
	GrantJIMMModelAdmin(context.Context, names.ModelTag) error

	// Close closes the connection.
	Close() error
}
