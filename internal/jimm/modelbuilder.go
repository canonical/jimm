// Copyright 2025 Canonical.

package jimm

import (
	"context"
	"fmt"

	jujupermission "github.com/juju/juju/core/permission"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

func newModelBuilder(ctx context.Context, j *JIMM) *modelBuilder {
	return &modelBuilder{
		ctx:  ctx,
		jimm: j,
	}
}

type modelBuilder struct {
	ctx context.Context
	err error

	jimm *JIMM

	name               string
	config             map[string]interface{}
	owner              *dbmodel.Identity
	credential         *dbmodel.CloudCredential
	controller         *dbmodel.Controller
	cloud              *dbmodel.Cloud
	cloudRegion        string
	cloudRegionID      uint
	cloudRegionVirtual bool
	model              *dbmodel.Model
	modelInfo          *jujuparams.ModelInfo
}

// Error returns the error that occurred in the process
// of adding a new model.
func (b *modelBuilder) Error() error {
	return b.err
}

func (b *modelBuilder) jujuModelCreateArgs() (*jujuparams.ModelCreateArgs, error) {
	if b.name == "" {
		return nil, errors.E("model name not specified")
	}
	if b.owner == nil {
		return nil, errors.E("model owner not specified")
	}
	if b.cloud == nil {
		return nil, errors.E("cloud not specified")
	}
	if b.cloudRegionID == 0 {
		return nil, errors.E("cloud region not specified")
	}
	if b.credential == nil {
		return nil, errors.E("credentials not specified")
	}

	args := &jujuparams.ModelCreateArgs{
		Name:               b.name,
		OwnerTag:           b.owner.Tag().String(),
		Config:             b.config,
		CloudTag:           b.cloud.Tag().String(),
		CloudRegion:        b.cloudRegion,
		CloudCredentialTag: b.credential.Tag().String(),
	}
	// if this cloud region is a virtual one (cloud did not report
	// any regions and we added a "default" region), we will
	// not send a cloud region to the controller.
	if b.cloudRegionVirtual {
		args.CloudRegion = ""
	}
	return args, nil
}

// WithOwner returns a builder with the specified owner.
func (b *modelBuilder) WithOwner(owner *dbmodel.Identity) *modelBuilder {
	if b.err != nil {
		return b
	}
	b.owner = owner
	return b
}

// WithName returns a builder with the specified model name.
func (b *modelBuilder) WithName(name string) *modelBuilder {
	if b.err != nil {
		return b
	}
	b.name = name
	return b
}

// WithConfig returns a builder with the specified model config.
func (b *modelBuilder) WithConfig(cfg map[string]interface{}) *modelBuilder {
	if b.config == nil {
		b.config = make(map[string]interface{})
	}
	for key, value := range cfg {
		b.config[key] = value
	}
	return b
}

// WithCloud returns a builder with the specified cloud.
func (b *modelBuilder) WithCloud(user *openfga.User, cloud names.CloudTag) *modelBuilder {
	if b.err != nil {
		return b
	}

	// if cloud was not specified then we try to determine if
	// JIMM knows of only one cloud and use that one
	if cloud.Id() == "" {
		return b.withImplicitCloud(user)
	}

	c := dbmodel.Cloud{
		Name: cloud.Id(),
	}

	if err := b.jimm.Database.GetCloud(b.ctx, &c); err != nil {
		b.err = err
		return b
	}
	b.cloud = &c

	return b
}

// withImplicitCloud returns a builder with the only cloud known to JIMM. Should JIMM
// know of multiple clouds an error will be raised.
func (b *modelBuilder) withImplicitCloud(user *openfga.User) *modelBuilder {
	if b.err != nil {
		return b
	}
	var clouds []*dbmodel.Cloud
	err := b.jimm.ForEachUserCloud(b.ctx, user, func(c *dbmodel.Cloud) error {
		clouds = append(clouds, c)
		return nil
	})
	if err != nil {
		b.err = err
		return b
	}
	if len(clouds) == 0 {
		b.err = fmt.Errorf("no available clouds")
		return b
	}
	if len(clouds) != 1 {
		b.err = fmt.Errorf("no cloud specified for model; please specify one")
		return b
	}
	b.cloud = clouds[0]

	return b
}

// WithCloudRegion returns a builder with the specified cloud region.
func (b *modelBuilder) WithCloudRegion(region string) *modelBuilder {
	if b.err != nil {
		return b
	}
	if b.cloud == nil {
		b.err = errors.E("cloud not specified")
		return b
	}
	// if the region is not specified, we pick the first cloud region
	// with any associated controllers
	if region == "" {
		for _, r := range b.cloud.Regions {
			regionControllers := r.Controllers
			if len(regionControllers) == 0 {
				continue
			}
			region = r.Name
			break
		}
	}
	// loop through all cloud regions
	for _, r := range b.cloud.Regions {
		// if the region matches
		if r.Name != region {
			continue
		}
		// consider all possible controllers for that region
		regionControllers := r.Controllers
		if len(regionControllers) == 0 {
			b.err = errors.E(errors.CodeBadRequest, fmt.Sprintf("unsupported cloud region %s/%s", b.cloud.Name, region))
			return b
		}
		// shuffle controllers
		shuffleRegionControllers(regionControllers)

		// and select the first controller in the slice
		b.cloudRegion = region
		b.cloudRegionID = r.ID
		b.cloudRegionVirtual = r.Virtual
		b.controller = &regionControllers[0].Controller

		break
	}
	// we looped through all cloud regions and could not find a match
	if b.cloudRegionID == 0 {
		b.err = errors.E("cloudregion not found", errors.CodeNotFound)
	}
	return b
}

// WithCloudCredential returns a builder with the specified cloud credentials.
func (b *modelBuilder) WithCloudCredential(credentialTag names.CloudCredentialTag) *modelBuilder {
	if b.err != nil {
		return b
	}
	credential := dbmodel.CloudCredential{
		Name:              credentialTag.Name(),
		CloudName:         credentialTag.Cloud().Id(),
		OwnerIdentityName: credentialTag.Owner().Id(),
	}
	err := b.jimm.Database.GetCloudCredential(b.ctx, &credential)
	if err != nil {
		b.err = errors.E(err, fmt.Sprintf("failed to fetch cloud credentials %s", credential.Path()))
	}
	b.credential = &credential

	return b
}

// CreateDatabaseModel stores temporary model information.
func (b *modelBuilder) CreateDatabaseModel() *modelBuilder {
	if b.err != nil {
		return b
	}

	// if model name is not specified we error and abort
	if b.name == "" {
		b.err = errors.E("model name not specified")
		return b
	}
	// if the model owner is not specified we error and abort
	if b.owner == nil {
		b.err = errors.E("owner not specified")
		return b
	}
	// if at this point the cloud region is not specified we
	// try to select a region/controller among the available
	// regions/controllers for the specified cloud
	if b.cloudRegionID == 0 {
		// if selectCloudRegion returns an error that means we have
		// no regions/controllers for the specified cloud - we
		// error and abort
		if err := b.selectCloudRegion(); err != nil {
			b.err = errors.E(err)
			return b
		}
	}
	// if controller is still not selected, there's nothing
	// we can do - either a cloud or a cloud region was specified
	// by this point and a controller should've been selected
	if b.controller == nil {
		b.err = errors.E("unable to determine a suitable controller")
		return b
	}

	if b.credential == nil {
		// try to select a valid credential
		if err := b.selectCloudCredentials(); err != nil {
			b.err = errors.E(err, "could not select cloud credentials")
			return b
		}
	}

	b.model = &dbmodel.Model{
		Name:              b.name,
		ControllerID:      b.controller.ID,
		Owner:             *b.owner,
		CloudCredentialID: b.credential.ID,
		CloudRegionID:     b.cloudRegionID,
	}

	err := b.jimm.Database.AddModel(b.ctx, b.model)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeAlreadyExists {
			b.err = errors.E(err, fmt.Sprintf("model %s/%s already exists", b.owner.Name, b.name))
			return b
		} else {
			zapctx.Error(b.ctx, "failed to store model information", zaputil.Error(err))
			b.err = errors.E(err, "failed to store model information")
			return b
		}
	}
	return b
}

// Cleanup deletes temporary model information if there was an
// error in the process of creating model.
func (b *modelBuilder) Cleanup() {
	if b.err == nil {
		return
	}
	if b.model == nil {
		return
	}
	// the model should be deleted from the database regardless of the request
	// context expiration
	ctx := context.Background()
	if derr := b.jimm.Database.DeleteModel(ctx, b.model); derr != nil {
		zapctx.Error(ctx, "failed to delete model", zap.String("model", b.model.Name), zap.String("owner", b.model.Owner.Name), zaputil.Error(derr))
	}
}

// UpdateDatabaseModel persists the information about the model
// retrieved from Juju to our database.
func (b *modelBuilder) UpdateDatabaseModel() *modelBuilder {
	if b.err != nil {
		return b
	}
	err := b.model.FromJujuModelInfo(*b.modelInfo)
	if err != nil {
		b.err = errors.E(err, "failed to convert model info")
		return b
	}
	b.model.ControllerID = b.controller.ID
	// we know which credentials and cloud region was used
	// - ignore this information returned by the controller
	//   because we need IDs to properly update the model
	b.model.CloudCredentialID = b.credential.ID
	b.model.CloudRegionID = b.cloudRegionID
	b.model.CloudCredential = dbmodel.CloudCredential{}
	b.model.CloudRegion = dbmodel.CloudRegion{}

	err = b.jimm.Database.UpdateModel(b.ctx, b.model)
	if err != nil {
		b.err = errors.E(err, "failed to store model information")
		return b
	}
	return b
}

func (b *modelBuilder) selectCloudRegion() error {
	if b.cloudRegionID != 0 {
		return nil
	}
	if b.cloud == nil {
		return errors.E("cloud not specified")
	}

	var regionControllers []dbmodel.CloudRegionControllerPriority
	for _, r := range b.cloud.Regions {
		regionControllers = append(regionControllers, r.Controllers...)
	}

	// if no controllers are found, we return an error
	if len(regionControllers) == 0 {
		return errors.E(fmt.Sprintf("unsupported cloud %s", b.cloud.Name))
	}

	// shuffle controllers according to their priority
	shuffleRegionControllers(regionControllers)

	b.cloudRegionID = regionControllers[0].CloudRegionID
	b.controller = &regionControllers[0].Controller

	return nil
}

func (b *modelBuilder) selectCloudCredentials() error {
	if b.owner == nil {
		return errors.E("user not specified")
	}
	if b.cloud == nil {
		return errors.E("cloud not specified")
	}
	credentials, err := b.jimm.Database.GetIdentityCloudCredentials(b.ctx, b.owner, b.cloud.Name)
	if err != nil {
		return errors.E(err, "failed to fetch user cloud credentials")
	}
	for _, credential := range credentials {
		// skip any credentials known to be invalid.
		if credential.Valid.Valid && !credential.Valid.Bool {
			continue
		}
		b.credential = &credential
		return nil
	}
	return errors.E("valid cloud credentials not found")
}

// CreateControllerModel uses provided information to create a new
// model on the selected controller.
func (b *modelBuilder) CreateControllerModel() *modelBuilder {
	if b.err != nil {
		return b
	}

	if b.model == nil {
		b.err = errors.E("model not specified")
		return b
	}

	api, err := b.jimm.dial(
		b.ctx,
		b.controller,
		names.ModelTag{},
		permission{
			resource: b.cloud.ResourceTag().String(),
			relation: string(jujupermission.AddModelAccess),
		},
	)
	if err != nil {
		b.err = errors.E(err)
		return b
	}
	defer api.Close()

	if b.credential != nil {
		if err := b.updateCredential(b.ctx, api, b.credential); err != nil {
			b.err = errors.E(fmt.Sprintf("failed to update cloud credential: %s", err), err)
			return b
		}
	}

	args, err := b.jujuModelCreateArgs()
	if err != nil {
		b.err = errors.E(err)
		return b
	}

	var info jujuparams.ModelInfo
	if err := api.CreateModel(b.ctx, args, &info); err != nil {
		switch jujuparams.ErrCode(err) {
		case jujuparams.CodeAlreadyExists:
			// The model already exists in the controller but it didn't
			// exist in the database. This probably means that it's
			// been abortively created previously, but left around because
			// of connection failure.
			// it's empty, but return an error to the user because
			// TODO initiate cleanup of the model, first checking that
			// the operation to delete a model isn't synchronous even
			// for empty models. We could also have a worker that deletes
			// empty models that don't appear in the database.
			b.err = errors.E(err, errors.CodeAlreadyExists, "model name in use")
		case jujuparams.CodeUpgradeInProgress:
			b.err = errors.E(err, "upgrade in progress")
		default:
			// The model couldn't be created because of an
			// error in the request, don't try another
			// controller.
			b.err = errors.E(err, errors.CodeBadRequest)
		}
		return b
	}

	// Grant JIMM admin access to the model. Note that if this fails,
	// the local database entry will be deleted but the model
	// will remain on the controller and will trigger the "already exists
	// in the backend controller" message above when the user
	// attempts to create a model with the same name again.
	if err := api.GrantJIMMModelAdmin(b.ctx, names.NewModelTag(info.UUID)); err != nil {
		zapctx.Error(b.ctx, "leaked model", zap.String("model", info.UUID), zaputil.Error(err))
		b.err = errors.E(err)
		return b
	}

	b.modelInfo = &info
	return b
}

func (b *modelBuilder) updateCredential(ctx context.Context, api API, cred *dbmodel.CloudCredential) error {
	var err error

	_, err = b.jimm.updateControllerCloudCredential(ctx, cred, api.UpdateCredential)
	return err
}

// JujuModelInfo returns model information returned by the controller.
func (b *modelBuilder) JujuModelInfo() *jujuparams.ModelInfo {
	return b.modelInfo
}
