// Copyright 2024 Canonical.

package jimmtest

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/crossmodel"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/version"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
)

// DefaultControllerUUID is the controller UUID returned by Dialer if
// the is no configured controller UUID.
const DefaultControllerUUID = "982b16d9-a945-4762-b684-fd4fd885aa10"

// A Dialer is a jimm.Dialer that either returns an error if Err is
// non-zero, or returns the value of API. The number of open API
// connections is tracked.
type Dialer struct {
	// API contains the API implementation to return, if Err is nil.
	API jimm.API

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

// Dialer implements jimm.Dialer.
func (d *Dialer) Dial(_ context.Context, ctl *dbmodel.Controller, _ names.ModelTag, _ map[string]string) (jimm.API, error) {
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
		ctl.AgentVersion = version.Current.String()
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
	jimm.API
	open *int64
}

// Close closes the API and decrements the open count.
func (w apiWrapper) Close() error {
	atomic.AddInt64(w.open, -1)
	return w.API.Close()
}

// ModelDialerMap enables the dialing of many models on the same controller,
// it is designed such that should you need to query multiple models, you can.
type ModelDialerMap map[string]jimm.Dialer

// Dial implements jimm.Dialer.
func (m ModelDialerMap) Dial(ctx context.Context, ctl *dbmodel.Controller, mt names.ModelTag, _ map[string]string) (jimm.API, error) {
	if d, ok := m[mt.Id()]; ok {
		return d.Dial(ctx, ctl, mt, nil)
	}
	return nil, errors.E(fmt.Sprintf("dialer not configured for controller %s", ctl.Name))
}

// A DialerMap implements a jimm.Dialer that uses a different Dialer for
// each controller. The DialerMap is keyed by controller name.
type DialerMap map[string]jimm.Dialer

// Dial implements jimm.Dialer.
func (m DialerMap) Dial(ctx context.Context, ctl *dbmodel.Controller, mt names.ModelTag, _ map[string]string) (jimm.API, error) {
	if d, ok := m[ctl.Name]; ok {
		return d.Dial(ctx, ctl, mt, nil)
	}
	return nil, errors.E(fmt.Sprintf("dialer not configured for controller %s", ctl.Name))
}

// API is a default implementation of the jimm.API interface. Every method
// has a corresponding function field. Whenever the method is called it
// will delegate to the requested function or if the function is nil return
// a NotImplemented error.
type API struct {
	base.APICaller

	AddCloud_                          func(context.Context, names.CloudTag, jujuparams.Cloud, bool) error
	AllModelWatcherNext_               func(context.Context, string) ([]jujuparams.Delta, error)
	AllModelWatcherStop_               func(context.Context, string) error
	ChangeModelCredential_             func(context.Context, names.ModelTag, names.CloudCredentialTag) error
	CheckCredentialModels_             func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error)
	Close_                             func() error
	Cloud_                             func(context.Context, names.CloudTag, *jujuparams.Cloud) error
	CloudInfo_                         func(context.Context, names.CloudTag, *jujuparams.CloudInfo) error
	Clouds_                            func(context.Context) (map[names.CloudTag]jujuparams.Cloud, error)
	ControllerModelSummary_            func(context.Context, *jujuparams.ModelSummary) error
	CreateModel_                       func(context.Context, *jujuparams.ModelCreateArgs, *jujuparams.ModelInfo) error
	DestroyApplicationOffer_           func(context.Context, string, bool) error
	DestroyModel_                      func(context.Context, names.ModelTag, *bool, *bool, *time.Duration, *time.Duration) error
	DumpModel_                         func(context.Context, names.ModelTag, bool) (string, error)
	DumpModelDB_                       func(context.Context, names.ModelTag) (map[string]interface{}, error)
	FindApplicationOffers_             func(context.Context, []jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error)
	GetApplicationOffer_               func(context.Context, *jujuparams.ApplicationOfferAdminDetailsV5) error
	GetApplicationOfferConsumeDetails_ func(context.Context, names.UserTag, *jujuparams.ConsumeOfferDetails, bakery.Version) error
	GrantApplicationOfferAccess_       func(context.Context, string, names.UserTag, jujuparams.OfferAccessPermission) error
	GrantCloudAccess_                  func(context.Context, names.CloudTag, names.UserTag, string) error
	GrantJIMMModelAdmin_               func(context.Context, names.ModelTag) error
	GrantModelAccess_                  func(context.Context, names.ModelTag, names.UserTag, jujuparams.UserAccessPermission) error
	IsBroken_                          bool
	ListApplicationOffers_             func(context.Context, []jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error)
	ModelInfo_                         func(context.Context, *jujuparams.ModelInfo) error
	ModelStatus_                       func(context.Context, *jujuparams.ModelStatus) error
	ModelSummaryWatcherNext_           func(context.Context, string) ([]jujuparams.ModelAbstract, error)
	ModelSummaryWatcherStop_           func(context.Context, string) error
	ModelWatcherNext_                  func(ctx context.Context, id string) ([]jujuparams.Delta, error)
	ModelWatcherStop_                  func(ctx context.Context, id string) error
	Offer_                             func(context.Context, crossmodel.OfferURL, jujuparams.AddApplicationOffer) error
	Ping_                              func(context.Context) error
	RemoveCloud_                       func(context.Context, names.CloudTag) error
	RevokeApplicationOfferAccess_      func(context.Context, string, names.UserTag, jujuparams.OfferAccessPermission) error
	RevokeCloudAccess_                 func(context.Context, names.CloudTag, names.UserTag, string) error
	RevokeCredential_                  func(context.Context, names.CloudCredentialTag) error
	RevokeModelAccess_                 func(context.Context, names.ModelTag, names.UserTag, jujuparams.UserAccessPermission) error
	SupportsCheckCredentialModels_     bool
	SupportsModelSummaryWatcher_       bool
	Status_                            func(context.Context, []string) (*jujuparams.FullStatus, error)
	UpdateCloud_                       func(context.Context, names.CloudTag, jujuparams.Cloud) error
	UpdateCredential_                  func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error)
	ValidateModelUpgrade_              func(context.Context, names.ModelTag, bool) error
	WatchAll_                          func(context.Context) (string, error)
	WatchAllModelSummaries_            func(context.Context) (string, error)
	WatchAllModels_                    func(context.Context) (string, error)
	ListFilesystems_                   func(ctx context.Context, machines []string) ([]jujuparams.FilesystemDetailsListResult, error)
	ListVolumes_                       func(ctx context.Context, machines []string) ([]jujuparams.VolumeDetailsListResult, error)
	ListStorageDetails_                func(ctx context.Context) ([]jujuparams.StorageDetails, error)
}

func (a *API) AddCloud(ctx context.Context, tag names.CloudTag, cld jujuparams.Cloud, force bool) error {
	if a.AddCloud_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.AddCloud_(ctx, tag, cld, force)
}

func (a *API) AllModelWatcherNext(ctx context.Context, id string) ([]jujuparams.Delta, error) {
	if a.AllModelWatcherNext_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return a.AllModelWatcherNext_(ctx, id)
}

func (a *API) AllModelWatcherStop(ctx context.Context, id string) error {
	if a.AllModelWatcherStop_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.AllModelWatcherStop_(ctx, id)
}

func (a *API) CheckCredentialModels(ctx context.Context, cred jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
	if a.CheckCredentialModels_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return a.CheckCredentialModels_(ctx, cred)
}

func (a *API) Close() error {
	if a.Close_ == nil {
		return nil
	}
	return a.Close_()
}

func (a *API) Cloud(ctx context.Context, tag names.CloudTag, ci *jujuparams.Cloud) error {
	if a.Cloud_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.Cloud_(ctx, tag, ci)
}

func (a *API) CloudInfo(ctx context.Context, tag names.CloudTag, ci *jujuparams.CloudInfo) error {
	if a.CloudInfo_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.CloudInfo_(ctx, tag, ci)
}

func (a *API) Clouds(ctx context.Context) (map[names.CloudTag]jujuparams.Cloud, error) {
	if a.Clouds_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return a.Clouds_(ctx)
}

func (a *API) ControllerModelSummary(ctx context.Context, ms *jujuparams.ModelSummary) error {
	if a.ControllerModelSummary_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.ControllerModelSummary_(ctx, ms)
}

func (a *API) CreateModel(ctx context.Context, args *jujuparams.ModelCreateArgs, mi *jujuparams.ModelInfo) error {
	if a.CreateModel_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.CreateModel_(ctx, args, mi)
}

func (a *API) DestroyApplicationOffer(ctx context.Context, offerURL string, force bool) error {
	if a.DestroyApplicationOffer_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.DestroyApplicationOffer_(ctx, offerURL, force)
}

func (a *API) DestroyModel(ctx context.Context, tag names.ModelTag, destroyStorage *bool, force *bool, maxWait, timeout *time.Duration) error {
	if a.DestroyModel_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.DestroyModel_(ctx, tag, destroyStorage, force, maxWait, timeout)
}

func (a *API) DumpModel(ctx context.Context, mt names.ModelTag, simplified bool) (string, error) {
	if a.DumpModel_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return a.DumpModel_(ctx, mt, simplified)
}

func (a *API) DumpModelDB(ctx context.Context, mt names.ModelTag) (map[string]interface{}, error) {
	if a.DumpModelDB_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return a.DumpModelDB_(ctx, mt)
}

func (a *API) FindApplicationOffers(ctx context.Context, f []jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error) {
	if a.FindApplicationOffers_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return a.FindApplicationOffers_(ctx, f)
}

func (a *API) GetApplicationOffer(ctx context.Context, offer *jujuparams.ApplicationOfferAdminDetailsV5) error {
	if a.GetApplicationOffer_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.GetApplicationOffer_(ctx, offer)
}

func (a *API) GetApplicationOfferConsumeDetails(ctx context.Context, tag names.UserTag, cod *jujuparams.ConsumeOfferDetails, v bakery.Version) error {
	if a.GetApplicationOfferConsumeDetails_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.GetApplicationOfferConsumeDetails_(ctx, tag, cod, v)
}

func (a *API) GrantApplicationOfferAccess(ctx context.Context, offerURL string, tag names.UserTag, p jujuparams.OfferAccessPermission) error {
	if a.GrantApplicationOfferAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.GrantApplicationOfferAccess_(ctx, offerURL, tag, p)
}

func (a *API) GrantCloudAccess(ctx context.Context, ct names.CloudTag, ut names.UserTag, access string) error {
	if a.GrantCloudAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.GrantCloudAccess_(ctx, ct, ut, access)
}

func (a *API) GrantJIMMModelAdmin(ctx context.Context, tag names.ModelTag) error {
	if a.GrantJIMMModelAdmin_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.GrantJIMMModelAdmin_(ctx, tag)
}

func (a *API) GrantModelAccess(ctx context.Context, mt names.ModelTag, ut names.UserTag, p jujuparams.UserAccessPermission) error {
	if a.GrantModelAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.GrantModelAccess_(ctx, mt, ut, p)
}

func (a *API) IsBroken() bool {
	return a.IsBroken_
}

func (a *API) ListApplicationOffers(ctx context.Context, f []jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetailsV5, error) {
	if a.ListApplicationOffers_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return a.ListApplicationOffers_(ctx, f)
}

func (a *API) ModelInfo(ctx context.Context, mi *jujuparams.ModelInfo) error {
	if a.ModelInfo_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.ModelInfo_(ctx, mi)
}

func (a *API) ModelStatus(ctx context.Context, ms *jujuparams.ModelStatus) error {
	if a.ModelStatus_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.ModelStatus_(ctx, ms)
}

func (a *API) ModelSummaryWatcherNext(ctx context.Context, id string) ([]jujuparams.ModelAbstract, error) {
	if a.ModelSummaryWatcherNext_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return a.ModelSummaryWatcherNext_(ctx, id)
}

func (a *API) ModelSummaryWatcherStop(ctx context.Context, id string) error {
	if a.ModelSummaryWatcherStop_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.ModelSummaryWatcherStop_(ctx, id)
}

func (a *API) Offer(ctx context.Context, offerURL crossmodel.OfferURL, aao jujuparams.AddApplicationOffer) error {
	if a.Offer_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.Offer_(ctx, offerURL, aao)
}

func (a *API) Ping(ctx context.Context) error {
	if a.Ping_ == nil {
		return nil
	}
	return a.Ping_(ctx)
}

func (a *API) RemoveCloud(ctx context.Context, tag names.CloudTag) error {
	if a.RemoveCloud_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.RemoveCloud_(ctx, tag)
}

func (a *API) RevokeApplicationOfferAccess(ctx context.Context, offerURL string, tag names.UserTag, p jujuparams.OfferAccessPermission) error {
	if a.RevokeApplicationOfferAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.RevokeApplicationOfferAccess_(ctx, offerURL, tag, p)
}

func (a *API) RevokeCloudAccess(ctx context.Context, ct names.CloudTag, ut names.UserTag, access string) error {
	if a.RevokeCloudAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.RevokeCloudAccess_(ctx, ct, ut, access)
}

func (a *API) RevokeCredential(ctx context.Context, tag names.CloudCredentialTag) error {
	if a.RevokeCredential_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.RevokeCredential_(ctx, tag)
}

func (a *API) RevokeModelAccess(ctx context.Context, mt names.ModelTag, ut names.UserTag, p jujuparams.UserAccessPermission) error {
	if a.RevokeModelAccess_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.RevokeModelAccess_(ctx, mt, ut, p)
}

func (a *API) SupportsCheckCredentialModels() bool {
	return a.SupportsCheckCredentialModels_
}

func (a *API) SupportsModelSummaryWatcher() bool {
	return a.SupportsModelSummaryWatcher_
}

func (a *API) Status(ctx context.Context, patterns []string) (*jujuparams.FullStatus, error) {
	if a.Status_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return a.Status_(ctx, patterns)
}

func (a *API) UpdateCloud(ctx context.Context, tag names.CloudTag, cloud jujuparams.Cloud) error {
	if a.UpdateCloud_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.UpdateCloud_(ctx, tag, cloud)
}

func (a *API) UpdateCredential(ctx context.Context, cred jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
	if a.UpdateCredential_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return a.UpdateCredential_(ctx, cred)
}

func (a *API) ValidateModelUpgrade(ctx context.Context, tag names.ModelTag, force bool) error {
	if a.ValidateModelUpgrade_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.ValidateModelUpgrade_(ctx, tag, force)
}

func (a *API) WatchAllModelSummaries(ctx context.Context) (string, error) {
	if a.WatchAllModelSummaries_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return a.WatchAllModelSummaries_(ctx)
}

func (a *API) WatchAllModels(ctx context.Context) (string, error) {
	if a.WatchAllModels_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return a.WatchAllModels_(ctx)
}

func (a *API) ChangeModelCredential(ctx context.Context, model names.ModelTag, cred names.CloudCredentialTag) error {
	if a.ChangeModelCredential_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.ChangeModelCredential_(ctx, model, cred)
}

func (a *API) ModelWatcherNext(ctx context.Context, id string) ([]jujuparams.Delta, error) {
	if a.ModelWatcherNext_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return a.ModelWatcherNext_(ctx, id)
}

func (a *API) ModelWatcherStop(ctx context.Context, id string) error {
	if a.ModelWatcherStop_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.ModelWatcherStop_(ctx, id)
}

func (a *API) WatchAll(ctx context.Context) (string, error) {
	if a.WatchAll_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return a.WatchAll_(ctx)
}

func (a *API) ListFilesystems(ctx context.Context, machines []string) ([]jujuparams.FilesystemDetailsListResult, error) {
	if a.ListFilesystems_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return a.ListFilesystems_(ctx, machines)
}

func (a *API) ListVolumes(ctx context.Context, machines []string) ([]jujuparams.VolumeDetailsListResult, error) {
	if a.ListVolumes_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return a.ListVolumes_(ctx, machines)
}

func (a *API) ListStorageDetails(ctx context.Context) ([]jujuparams.StorageDetails, error) {
	if a.ListStorageDetails_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return a.ListStorageDetails_(ctx)
}

var _ jimm.API = &API{}
