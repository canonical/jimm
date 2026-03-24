// Copyright 2026 Canonical.

package jimmtest

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/juju/juju/api/base"
	jujucloud "github.com/juju/juju/cloud"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/crossmodel"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/environs/cloudspec"
	jujuparams "github.com/juju/juju/rpc/params"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuclient"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// DefaultControllerUUID is the controller UUID returned by Dialer if
// the is no configured controller UUID.
const DefaultControllerUUID = "982b16d9-a945-4762-b684-fd4fd885aa10"

// A Dialer is a juju.Dialer that either returns an error if Err is
// non-zero, or returns the value of API. The number of open API
// connections is tracked.
type Dialer struct {
	// API contains the API implementation to return, if Err is nil.
	API juju.API

	// Err contains the error to return when a Dial is attempted.
	Err error

	// UUID is the UUID of the connected controller, if this is not set
	// then DefaultControllerUUID will be used.
	UUID string

	// AgentVersion contains the juju-agent version to the report to the
	// controller connection. If this is empty the version of the linked
	// juju is used.
	AgentVersion string

	// Addresses contains the addresses to set on the controller.
	Addresses [][]jujuparams.HostPort

	open int64
}

// Dialer implements juju.Dialer.
func (d *Dialer) Dial(_ context.Context, ctl *dbmodel.Controller, _ names.ModelTag, _ *openfga.User) (juju.API, error) {
	if d.Err != nil {
		return nil, d.Err
	}
	atomic.AddInt64(&d.open, 1)
	if ctl.UUID == "" {
		if d.UUID == "" {
			ctl.UUID = DefaultControllerUUID
		} else {
			ctl.UUID = d.UUID
		}
	}
	ctl.AgentVersion = d.AgentVersion
	if ctl.AgentVersion == "" {
		ctl.AgentVersion = jujuversion.Current.String()
	}
	ctl.Addresses = dbmodel.HostPorts(d.Addresses)
	return apiWrapper{
		API:  d.API,
		open: &d.open,
	}, nil
}

// IsClosed returns true if all opened connections have been closed.
func (d *Dialer) IsClosed() bool {
	return atomic.LoadInt64(&d.open) == 0
}

// apiWrapper is the API implementation used by Dialer to track usage.
type apiWrapper struct {
	juju.API
	open *int64
}

// Close closes the API and decrements the open count.
func (w apiWrapper) Close() error {
	atomic.AddInt64(w.open, -1)
	return w.API.Close()
}

// ModelDialerMap enables the dialing of many models on the same controller,
// it is designed such that should you need to query multiple models, you can.
type ModelDialerMap map[string]juju.Dialer

// Dial implements juju.Dialer.
func (m ModelDialerMap) Dial(ctx context.Context, ctl *dbmodel.Controller, mt names.ModelTag, u *openfga.User) (juju.API, error) {
	if d, ok := m[mt.Id()]; ok {
		return d.Dial(ctx, ctl, mt, u)
	}
	return nil, fmt.Errorf("dialer not configured for controller %s", ctl.Name)
}

// A DialerMap implements a juju.Dialer that uses a different Dialer for
// each controller. The DialerMap is keyed by controller name.
type DialerMap map[string]juju.Dialer

// Dial implements juju.Dialer.
func (m DialerMap) Dial(ctx context.Context, ctl *dbmodel.Controller, mt names.ModelTag, u *openfga.User) (juju.API, error) {
	if d, ok := m[ctl.Name]; ok {
		return d.Dial(ctx, ctl, mt, u)
	}
	return nil, fmt.Errorf("dialer not configured for controller %s", ctl.Name)
}

// API is a default implementation of the juju.API interface. Every method
// has a corresponding function field. Whenever the method is called it
// will delegate to the requested function or if the function is nil return
// a NotImplemented error.
type API struct {
	base.APICaller

	Activate_                          func(modelUUID string, sourceInfo coremigration.SourceControllerInfo, relatedModels []string) error
	Abort_                             func(modelUUID string) error
	AddCloud_                          func(names.CloudTag, jujucloud.Cloud, bool) error
	AdoptResources_                    func(modelUUID string, controllerVersion version.Number) error
	ChangeModelCredential_             func(context.Context, names.ModelTag, names.CloudCredentialTag) error
	CheckCredentialModels_             func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error)
	CheckMachines_                     func(modelUUID string) ([]error, error)
	Close_                             func() error
	Cloud_                             func(names.CloudTag) (jujucloud.Cloud, error)
	Clouds_                            func() (map[names.CloudTag]jujucloud.Cloud, error)
	CloudSpec_                         func(context.Context) (cloudspec.CloudSpec, error)
	ControllerConfig_                  func(context.Context) (jujucontroller.Config, error)
	CreateModel_                       func(context.Context, *jujuclient.CreateModelArgs) (base.ModelInfo, error)
	DestroyApplicationOffer_           func(context.Context, string, bool) error
	DestroyModel_                      func(context.Context, names.ModelTag, *bool, *bool, *time.Duration, *time.Duration) error
	DumpModel_                         func(context.Context, names.ModelTag, bool) (map[string]interface{}, error)
	DumpModelDB_                       func(context.Context, names.ModelTag) (map[string]interface{}, error)
	FindApplicationOffers_             func(context.Context, []crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error)
	GetApplicationOffer_               func(context.Context, string) (*crossmodel.ApplicationOfferDetails, error)
	GetApplicationOfferConsumeDetails_ func(context.Context, string) (jujuparams.ConsumeOfferDetails, error)
	GrantJIMMModelAdmin_               func(context.Context, names.ModelTag) error
	Import_                            func(bytes []byte) error
	IsBroken_                          bool
	LatestLogTime_                     func(string) (time.Time, error)
	ListApplicationOffers_             func(context.Context, []crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error)
	ModelInfo_                         func(context.Context, names.ModelTag) (jujuclient.ModelInfo, error)
	ModelStatus_                       func(context.Context, names.ModelTag) (base.ModelStatus, error)
	ListModelSummaries_                func(context.Context, jujuparams.ModelSummariesRequest) ([]base.UserModelSummary, error)
	Offer_                             func(context.Context, jujuclient.OfferParams) error
	Ping_                              func(context.Context) error
	RemoveCloud_                       func(names.CloudTag) error
	Prechecks_                         func(jujuparams.MigrationModelInfo) error
	RevokeCredential_                  func(context.Context, names.CloudCredentialTag) error
	SupportsModelSummaryWatcher_       bool
	Status_                            func(context.Context, []string) (*jujuparams.FullStatus, error)
	UpdateCloud_                       func(names.CloudTag, jujucloud.Cloud) error
	UpdateCloudsCredentialForce_       func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error)
	ValidateModelUpgrade_              func(context.Context, names.ModelTag, bool) error
	WatchAllModelSummaries_            func(context.Context) (jujuclient.SummaryWatcher, error)
	ListFilesystems_                   func(ctx context.Context, machines []string) ([]jujuparams.FilesystemDetailsListResult, error)
	ListVolumes_                       func(ctx context.Context, machines []string) ([]jujuparams.VolumeDetailsListResult, error)
	ListStorageDetails_                func(ctx context.Context) ([]jujuparams.StorageDetails, error)
	ListModels_                        func(context.Context) ([]base.UserModel, error)
	CredentialContents_                func(cloud string, credential string, withSecrets bool) ([]jujuparams.CredentialContentResult, error)
	UpgradeModel_                      func(modelUUID string, targetVersion version.Number, stream string, ignoreAgentVersions bool, dryRun bool) (version.Number, error)
}

func (a *API) Activate(modelUUID string, sourceInfo coremigration.SourceControllerInfo, relatedModels []string) error {
	if a.Activate_ == nil {
		return errors.New("not implemented")
	}
	return a.Activate_(modelUUID, sourceInfo, relatedModels)
}

// Abort aborts the current operation on the controller.
func (a *API) Abort(modelUUID string) error {
	if a.Abort_ == nil {
		return errors.New("not implemented")
	}
	return a.Abort_(modelUUID)
}

func (a *API) AddCloud(tag names.CloudTag, cld jujucloud.Cloud, force bool) error {
	if a.AddCloud_ == nil {
		return errors.New("not implemented")
	}
	return a.AddCloud_(tag, cld, force)
}

func (a *API) AdoptResources(modelUUID string, controllerVersion version.Number) error {
	if a.AdoptResources_ == nil {
		return errors.New("not implemented")
	}
	return a.AdoptResources_(modelUUID, controllerVersion)
}

func (a *API) CheckCredentialModels(ctx context.Context, cred jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
	if a.CheckCredentialModels_ == nil {
		return nil, errors.New("not implemented")
	}
	return a.CheckCredentialModels_(ctx, cred)
}

func (a *API) CheckMachines(modelUUID string) ([]error, error) {
	if a.CheckMachines_ == nil {
		return nil, errors.New("not implemented")
	}
	return a.CheckMachines_(modelUUID)
}

func (a *API) Close() error {
	if a.Close_ == nil {
		return nil
	}
	return a.Close_()
}

func (a *API) Cloud(tag names.CloudTag) (jujucloud.Cloud, error) {
	if a.Cloud_ == nil {
		return jujucloud.Cloud{}, errors.New("not implemented")
	}
	return a.Cloud_(tag)
}

func (a *API) Clouds() (map[names.CloudTag]jujucloud.Cloud, error) {
	if a.Clouds_ == nil {
		return nil, errors.New("not implemented")
	}
	return a.Clouds_()
}

func (a *API) CloudSpec(ctx context.Context) (cloudspec.CloudSpec, error) {
	if a.CloudSpec_ == nil {
		return cloudspec.CloudSpec{}, errors.New("not implemented")
	}
	return a.CloudSpec_(ctx)
}

func (a *API) ControllerConfig(ctx context.Context) (jujucontroller.Config, error) {
	if a.ControllerConfig_ == nil {
		return jujucontroller.Config{}, errors.New("not implemented")
	}
	return a.ControllerConfig_(ctx)
}

func (a *API) CreateModel(ctx context.Context, args *jujuclient.CreateModelArgs) (base.ModelInfo, error) {
	if a.CreateModel_ == nil {
		return base.ModelInfo{}, errors.New("not implemented")
	}
	return a.CreateModel_(ctx, args)
}

func (a *API) DestroyApplicationOffer(ctx context.Context, offerURL string, force bool) error {
	if a.DestroyApplicationOffer_ == nil {
		return errors.New("not implemented")
	}
	return a.DestroyApplicationOffer_(ctx, offerURL, force)
}

func (a *API) DestroyModel(ctx context.Context, tag names.ModelTag, destroyStorage *bool, force *bool, maxWait, timeout *time.Duration) error {
	if a.DestroyModel_ == nil {
		return errors.New("not implemented")
	}
	return a.DestroyModel_(ctx, tag, destroyStorage, force, maxWait, timeout)
}

func (a *API) DumpModel(ctx context.Context, tag names.ModelTag, simplified bool) (map[string]interface{}, error) {
	if a.DumpModel_ == nil {
		return nil, errors.New("not implemented")
	}
	return a.DumpModel_(ctx, tag, simplified)
}

func (a *API) DumpModelDB(ctx context.Context, tag names.ModelTag) (map[string]interface{}, error) {
	if a.DumpModelDB_ == nil {
		return nil, errors.New("not implemented")
	}
	return a.DumpModelDB_(ctx, tag)
}

func (a *API) FindApplicationOffers(ctx context.Context, f []crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error) {
	if a.FindApplicationOffers_ == nil {
		return nil, errors.New("not implemented")
	}
	return a.FindApplicationOffers_(ctx, f)
}

func (a *API) GetApplicationOffer(ctx context.Context, urlStr string) (*crossmodel.ApplicationOfferDetails, error) {
	if a.GetApplicationOffer_ == nil {
		return nil, errors.New("not implemented")
	}
	return a.GetApplicationOffer_(ctx, urlStr)
}

func (a *API) GetApplicationOfferConsumeDetails(ctx context.Context, url string) (jujuparams.ConsumeOfferDetails, error) {
	if a.GetApplicationOfferConsumeDetails_ == nil {
		return jujuparams.ConsumeOfferDetails{}, errors.New("not implemented")
	}
	return a.GetApplicationOfferConsumeDetails_(ctx, url)
}

func (a *API) GrantJIMMModelAdmin(ctx context.Context, tag names.ModelTag) error {
	if a.GrantJIMMModelAdmin_ == nil {
		return errors.New("not implemented")
	}
	return a.GrantJIMMModelAdmin_(ctx, tag)
}

func (a *API) IsBroken() bool {
	return a.IsBroken_
}

func (a *API) ListApplicationOffers(ctx context.Context, f []crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error) {
	if a.ListApplicationOffers_ == nil {
		return nil, errors.New("not implemented")
	}
	return a.ListApplicationOffers_(ctx, f)
}

func (a *API) ModelInfo(ctx context.Context, model names.ModelTag) (jujuclient.ModelInfo, error) {
	if a.ModelInfo_ == nil {
		return jujuclient.ModelInfo{}, errors.New("not implemented")
	}
	return a.ModelInfo_(ctx, model)
}

func (a *API) ModelStatus(ctx context.Context, modelTag names.ModelTag) (base.ModelStatus, error) {
	if a.ModelStatus_ == nil {
		return base.ModelStatus{}, errors.New("not implemented")
	}
	return a.ModelStatus_(ctx, modelTag)
}

func (a *API) LatestLogTime(modelUUID string) (time.Time, error) {
	if a.LatestLogTime_ == nil {
		return time.Time{}, errors.New("not implemented")
	}
	return a.LatestLogTime_(modelUUID)
}

func (a *API) ListModelSummaries(ctx context.Context, ms jujuparams.ModelSummariesRequest) ([]base.UserModelSummary, error) {
	if a.ListModelSummaries_ == nil {
		return nil, errors.New("not implemented")
	}
	return a.ListModelSummaries_(ctx, ms)
}

func (a *API) Offer(ctx context.Context, offer jujuclient.OfferParams) error {
	if a.Offer_ == nil {
		return errors.New("not implemented")
	}
	return a.Offer_(ctx, offer)
}

func (a *API) Ping(ctx context.Context) error {
	if a.Ping_ == nil {
		return nil
	}
	return a.Ping_(ctx)
}

func (a *API) Prechecks(model jujuparams.MigrationModelInfo) error {
	if a.Prechecks_ == nil {
		return errors.New("not implemented")
	}
	return a.Prechecks_(model)
}

func (a *API) RemoveCloud(tag names.CloudTag) error {
	if a.RemoveCloud_ == nil {
		return errors.New("not implemented")
	}
	return a.RemoveCloud_(tag)
}

func (a *API) RevokeCredential(ctx context.Context, tag names.CloudCredentialTag) error {
	if a.RevokeCredential_ == nil {
		return errors.New("not implemented")
	}
	return a.RevokeCredential_(ctx, tag)
}

func (a *API) SupportsModelSummaryWatcher() bool {
	return a.SupportsModelSummaryWatcher_
}

func (a *API) Status(ctx context.Context, patterns []string) (*jujuparams.FullStatus, error) {
	if a.Status_ == nil {
		return nil, errors.New("not implemented")
	}
	return a.Status_(ctx, patterns)
}

func (a *API) UpdateCloud(tag names.CloudTag, cloud jujucloud.Cloud) error {
	if a.UpdateCloud_ == nil {
		return errors.New("not implemented")
	}
	return a.UpdateCloud_(tag, cloud)
}

func (a *API) UpdateCloudsCredentialForce(ctx context.Context, cred jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
	if a.UpdateCloudsCredentialForce_ == nil {
		return nil, errors.New("not implemented")
	}
	return a.UpdateCloudsCredentialForce_(ctx, cred)
}

func (a *API) ValidateModelUpgrade(ctx context.Context, model names.ModelTag, force bool) error {
	if a.ValidateModelUpgrade_ == nil {
		return errors.New("not implemented")
	}
	return a.ValidateModelUpgrade_(ctx, model, force)
}

func (a *API) WatchAllModelSummaries(ctx context.Context) (jujuclient.SummaryWatcher, error) {
	if a.WatchAllModelSummaries_ == nil {
		return nil, errors.New("not implemented")
	}
	return a.WatchAllModelSummaries_(ctx)
}

func (a *API) ChangeModelCredential(ctx context.Context, model names.ModelTag, credential names.CloudCredentialTag) error {
	if a.ChangeModelCredential_ == nil {
		return errors.New("not implemented")
	}
	return a.ChangeModelCredential_(ctx, model, credential)
}

func (a *API) ListFilesystems(ctx context.Context, machines []string) ([]jujuparams.FilesystemDetailsListResult, error) {
	if a.ListFilesystems_ == nil {
		return nil, errors.New("not implemented")
	}
	return a.ListFilesystems_(ctx, machines)
}

func (a *API) ListVolumes(ctx context.Context, machines []string) ([]jujuparams.VolumeDetailsListResult, error) {
	if a.ListVolumes_ == nil {
		return nil, errors.New("not implemented")
	}
	return a.ListVolumes_(ctx, machines)
}

func (a *API) ListStorageDetails(ctx context.Context) ([]jujuparams.StorageDetails, error) {
	if a.ListStorageDetails_ == nil {
		return nil, errors.New("not implemented")
	}
	return a.ListStorageDetails_(ctx)
}

func (a *API) ListModels(ctx context.Context) ([]base.UserModel, error) {
	if a.ListModels_ == nil {
		return nil, errors.New("not implemented")
	}
	return a.ListModels_(ctx)
}

func (a *API) Import(bytes []byte) error {
	if a.Import_ == nil {
		return errors.New("not implemented")
	}
	return a.Import_(bytes)
}

func (a *API) CredentialContents(cloud string, credential string, withSecrets bool) ([]jujuparams.CredentialContentResult, error) {
	if a.CredentialContents_ == nil {
		return nil, errors.New("not implemented")
	}
	return a.CredentialContents(cloud, credential, withSecrets)
}

func (a *API) UpgradeModel(modelUUID string, targetVersion version.Number, stream string, ignoreAgentVersions bool, dryRun bool) (version.Number, error) {
	if a.UpgradeModel_ == nil {
		return version.Number{}, errors.New("not implemented")
	}
	return a.UpgradeModel_(modelUUID, targetVersion, stream, ignoreAgentVersions, dryRun)
}

var _ juju.API = &API{}
