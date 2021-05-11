// Copyright 2020 Canonical Ltd.

// Package jimm contains the business logic used to manage clouds,
// cloudcredentials and models.
package jimm

import (
	"context"
	"database/sql"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	vault "github.com/hashicorp/vault/api"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/pubsub"
)

// A JIMM provides the buisness logic for managing resources in the JAAS
// system. A single JIMM instance is shared by all concurrent API
// connections therefore the JIMM object itself does not contain any per-
// request state.
type JIMM struct {
	// Database is the database used by JIMM, this provides direct access
	// to the data store. Any client accessing the database directly is
	// responsible for ensuring that the authenticated user has access to
	// the data.
	Database db.Database

	// Authenticator is the authenticator JIMM uses to determine the user
	// authenticating with the API. If this is not specified then all
	// authentication requests are considered to have failed.
	Authenticator Authenticator

	// Dialer is the API dialer JIMM uses to contact juju controllers. if
	// this is not configured all connection attempts will fail.
	Dialer Dialer

	// VaultClient is the client for a vault server that is used to store
	// secrets.
	VaultClient *vault.Client

	// VaultPath is the root path in the vault for JIMM's secrets.
	VaultPath string

	// Pubsub is a pub-sub hub used for buffering model summaries.
	Pubsub *pubsub.Hub

	// ReservedCloudNames is the list of names that cannot be used for
	// hosted clouds. If this is empty then DefaultReservedCloudNames
	// is used.
	ReservedCloudNames []string

	// UUID holds the UUID of the JIMM controller.
	UUID string
}

// An Authenticator authenticates login requests.
type Authenticator interface {
	// Authenticate processes the given LoginRequest and returns the user
	// that has authenticated.
	Authenticate(ctx context.Context, req *jujuparams.LoginRequest) (*dbmodel.User, error)
}

// dial dials the controller and model specified by the given Controller
// and ModelTag. If no Dialer has been configured then an error with a
// code of CodeConnectionFailed will be returned.
func (j *JIMM) dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag) (API, error) {
	if j == nil || j.Dialer == nil {
		return nil, errors.E(errors.CodeConnectionFailed, "no dialer configured")
	}
	return j.Dialer.Dial(ctx, ctl, modelTag)
}

// A Dialer provides a connection to a controller.
type Dialer interface {
	// Dial creates an API connection to a controller. If the given
	// model-tag is non-zero the connection will be to that model,
	// otherwise the connection is to the controller. After sucessfully
	// dialing the controller the UUID, AgentVersion and HostPorts fields
	// in the given controller should be updated to the values provided
	// by the controller.
	Dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag) (API, error)
}

// An API is the interface JIMM uses to access the API on a controller.
type API interface {
	// AddCloud adds a new cloud.
	AddCloud(context.Context, names.CloudTag, jujuparams.Cloud) error

	// AllModelWatcherNext returns the next set of deltas from an
	// all-model watcher.
	AllModelWatcherNext(context.Context, string) ([]jujuparams.Delta, error)

	// AllModelWatcherStop stops an all-model watcher.
	AllModelWatcherStop(context.Context, string) error

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
	DestroyModel(context.Context, names.ModelTag, *bool, *bool, *time.Duration) error

	// DumpModel collects a database-agnostic dump of a model.
	DumpModel(context.Context, names.ModelTag, bool) (string, error)

	// DumpModelDB collects a database dump of a model.
	DumpModelDB(context.Context, names.ModelTag) (map[string]interface{}, error)

	// FindApplicationOffers finds application offers that match the
	// filter.
	FindApplicationOffers(context.Context, []jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetails, error)

	// GetApplicationOffer completes the given ApplicationOfferAdminDetails
	// structure.
	GetApplicationOffer(context.Context, *jujuparams.ApplicationOfferAdminDetails) error

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
	ListApplicationOffers(context.Context, []jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetails, error)

	// ModelInfo fetches a model's ModelInfo.
	ModelInfo(context.Context, *jujuparams.ModelInfo) error

	// ModelStatus fetches a model's ModelStatus.
	ModelStatus(context.Context, *jujuparams.ModelStatus) error

	// ModelSummaryWatcherNext returns the next set of model summaries from
	// the watcher.
	ModelSummaryWatcherNext(context.Context, string) ([]jujuparams.ModelAbstract, error)

	// ModelSummaryWatcherStop stops a model summary watcher.
	ModelSummaryWatcherStop(context.Context, string) error

	// ModelWatcherNext receives the next set of results from the model
	// watcher with the given id.
	ModelWatcherNext(ctx context.Context, id string) ([]jujuparams.Delta, error)

	// ModelWatcherStop stops the model watcher with the given id.
	ModelWatcherStop(ctx context.Context, id string) error

	// Offer creates a new application-offer.
	Offer(context.Context, jujuparams.AddApplicationOffer) error

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

	// WatchAll creates a watcher that reports deltas for a specific model.
	WatchAll(context.Context) (string, error)

	// WatchAllModelSummaries creates a ModelSummaryWatcher.
	WatchAllModelSummaries(context.Context) (string, error)

	// WatchAllModels creates a megawatcher.
	WatchAllModels(context.Context) (string, error)
}

// forEachController runs a given function on multiple controllers
// simultaneously. A connection is established to every controller in the
// given list concurrently and then the given function is called with the
// controller and API connection to use to perform the controller
// operation. ForEachConnection waits until all operations have finished
// before returning, any error returned will be ther first error
// encountered when connecting to the controller or returned from the given
// function.
func (j *JIMM) forEachController(ctx context.Context, controllers []dbmodel.Controller, f func(*dbmodel.Controller, API) error) error {
	eg := new(errgroup.Group)
	for i := range controllers {
		i := i
		eg.Go(func() error {
			api, err := j.dial(ctx, &controllers[i], names.ModelTag{})
			if err != nil {
				return err
			}
			defer api.Close()
			return f(&controllers[i], api)
		})
	}
	return eg.Wait()
}

// addAuditLogEntry causes an entry to be added the the audit log.
func (j *JIMM) addAuditLogEntry(ale *dbmodel.AuditLogEntry) {
	ctx := context.Background()
	if err := j.Database.AddAuditLogEntry(ctx, ale); err != nil {
		zapctx.Error(ctx, "cannot store audit log entry", zap.Error(err), zap.Any("entry", *ale))
	}
}

// FindAuditEvents returns audit events matching the given filter.
func (j *JIMM) FindAuditEvents(ctx context.Context, user *dbmodel.User, filter db.AuditLogFilter) ([]dbmodel.AuditLogEntry, error) {
	const op = errors.Op("jimm.FindAuditEvents")

	if user.ControllerAccess != "superuser" {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	var entries []dbmodel.AuditLogEntry
	err := j.Database.ForEachAuditLogEntry(ctx, filter, func(entry *dbmodel.AuditLogEntry) error {
		entries = append(entries, *entry)
		return nil
	})
	if err != nil {
		return nil, errors.E(op, err)
	}

	return entries, nil
}

// ListControllers returns a list of controllers the user has access to.
func (j *JIMM) ListControllers(ctx context.Context, user *dbmodel.User) ([]dbmodel.Controller, error) {
	const op = errors.Op("jimm.ListControllers")

	if user.ControllerAccess != "superuser" {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	var controllers []dbmodel.Controller
	err := j.Database.ForEachController(ctx, func(c *dbmodel.Controller) error {
		controllers = append(controllers, *c)
		return nil
	})
	if err != nil {
		return nil, errors.E(op, err)
	}

	return controllers, nil
}

// SetControllerDeprecated records if the controller is to be deprecated.
// No new models or clouds can be added to a deprecated controller.
func (j *JIMM) SetControllerDeprecated(ctx context.Context, user *dbmodel.User, controllerName string, deprecated bool) error {
	const op = errors.Op("jimm.SetControllerDeprecated")

	if user.ControllerAccess != "superuser" {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	// Update the local database with the updated cloud definition. We
	// do this in a transaction so that the local view cannot finish in
	// an inconsistent state.
	err := j.Database.Transaction(func(db *db.Database) error {
		c := dbmodel.Controller{
			Name: controllerName,
		}
		if err := db.GetController(ctx, &c); err != nil {
			return err
		}
		c.Deprecated = deprecated
		return db.UpdateController(ctx, &c)
	})
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

// RemoveController removes a controller.
func (j *JIMM) RemoveController(ctx context.Context, user *dbmodel.User, controllerName string, force bool) error {
	const op = errors.Op("jimm.RemoveController")

	if user.ControllerAccess != "superuser" {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	// Update the local database with the updated cloud definition. We
	// do this in a transaction so that the local view cannot finish in
	// an inconsistent state.
	err := j.Database.Transaction(func(db *db.Database) error {
		c := dbmodel.Controller{
			Name: controllerName,
		}
		if err := db.GetController(ctx, &c); err != nil {
			return err
		}

		// if c.UnavailableSince is valid, then we can delete is
		// if c.UnavailableSince is no valid, then we can't delete is
		// if force is true, we can always delete is
		if !(force || c.UnavailableSince.Valid) {
			return errors.E(errors.CodeStillAlive, "controller is still alive")
		}

		// Delete its models first.
		for _, model := range c.Models {
			err := db.DeleteModel(ctx, &model)
			if err != nil {
				return err
			}
		}

		// Then delete the controller
		return db.DeleteController(ctx, &c)
	})
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

// FullModelStatus returns the full status of the juju model.
func (j *JIMM) FullModelStatus(ctx context.Context, user *dbmodel.User, modelTag names.ModelTag, patterns []string) (*jujuparams.FullStatus, error) {
	const op = errors.Op("jimm.RemoveController")

	if user.ControllerAccess != "superuser" {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	model := dbmodel.Model{
		UUID: sql.NullString{
			String: modelTag.Id(),
			Valid:  true,
		},
	}
	err := j.Database.GetModel(ctx, &model)
	if err != nil {
		return nil, errors.E(op, err)
	}

	api, err := j.dial(ctx, &model.Controller, modelTag)
	if err != nil {
		return nil, errors.E(op, err)
	}

	status, err := api.Status(ctx, patterns)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return status, nil
}
