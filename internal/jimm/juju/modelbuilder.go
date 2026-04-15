// Copyright 2026 Canonical.

package juju

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"

	"github.com/juju/juju/api/base"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jujuclient"
	"github.com/canonical/jimm/v3/internal/openfga"
)

type candidateController struct {
	// controller is a candidate controller for the new model.
	controller dbmodel.Controller
	// priority decides an ordering for controller selection
	// based on the cloud-region.
	priority uint
}

// shuffleCandidateControllers shuffles the candidate controllers slice
// based on their priority (higher priority first).
func shuffleCandidateControllers(controllers []candidateController) {
	shuffle(len(controllers), func(i, j int) {
		controllers[i], controllers[j] = controllers[j], controllers[i]
	})
	sort.SliceStable(controllers, func(i, j int) bool {
		return controllers[i].priority > controllers[j].priority
	})
}

func newModelBuilder(ctx context.Context, j *JujuManager) *modelBuilder {
	return &modelBuilder{
		ctx:         ctx,
		jujuManager: j,
	}
}

type modelBuilder struct {
	ctx context.Context
	err error

	jujuManager *JujuManager
	ofgaUser    *openfga.User

	name               string
	candidates         []candidateController
	config             map[string]any
	owner              *dbmodel.Identity
	credential         *dbmodel.CloudCredential
	controller         *dbmodel.Controller
	cloud              *dbmodel.Cloud
	cloudRegion        string
	cloudRegionID      uint
	cloudRegionVirtual bool
	model              *dbmodel.Model
	modelInfo          base.ModelInfo
}

// Error returns the error that occurred in the process
// of adding a new model.
func (b *modelBuilder) Error() error {
	return b.err
}

func (b *modelBuilder) jujuModelCreateArgs() (*jujuclient.CreateModelArgs, error) {
	if b.name == "" {
		return nil, errors.New("model name not specified")
	}
	if b.owner == nil {
		return nil, errors.New("model owner not specified")
	}
	if b.cloud == nil {
		return nil, errors.New("cloud not specified")
	}
	if b.cloudRegionID == 0 {
		return nil, errors.New("cloud region not specified")
	}
	if b.credential == nil {
		return nil, errors.New("credentials not specified")
	}

	args := &jujuclient.CreateModelArgs{
		Name:               b.name,
		Owner:              b.owner.Name,
		Config:             b.config,
		Cloud:              b.cloud.Name,
		CloudRegion:        b.cloudRegion,
		CloudCredentialTag: b.credential.ResourceTag(),
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

// WithAuthorizer returns a builder with the specified OpenFGA user
// that will be used to authorize the model creation.
func (b *modelBuilder) WithAuthorizer(ofgaUser *openfga.User) *modelBuilder {
	if b.err != nil {
		return b
	}
	b.ofgaUser = ofgaUser
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
func (b *modelBuilder) WithConfig(cfg map[string]any) *modelBuilder {
	if b.config == nil {
		b.config = make(map[string]any)
	}
	maps.Copy(b.config, cfg)
	return b
}

// WithController returns a builder with the specified target controller
// if it exists and the user has access to it.
func (b *modelBuilder) WithController(controllerName string) *modelBuilder {
	if b.err != nil {
		return b
	}
	if b.ofgaUser == nil {
		b.err = errors.New("authorizer not specified")
		return b
	}
	targetController := dbmodel.Controller{
		Name: controllerName,
	}
	err := b.jujuManager.Database.GetController(b.ctx, &targetController)
	if err != nil {
		b.err = fmt.Errorf("controller %q not found: %w", controllerName, err)
		return b
	}
	ok, err := b.ofgaUser.IsAllowedAddModelToController(b.ctx, targetController.ResourceTag())
	if err != nil {
		b.err = fmt.Errorf("failed to verify permissions for adding model to controller: %w", err)
		return b
	}
	if !ok {
		b.err = errors.Codef(errors.CodeUnauthorized, "not authorized to add model to controller %q", controllerName)
		return b
	}
	b.candidates = append(b.candidates, candidateController{
		controller: targetController,
		priority:   0, // priority is unknown until we select the cloud-region
	})
	return b
}

// WithAnyController returns a builder with all available controllers
// that the user has access to.
func (b *modelBuilder) WithAnyController() *modelBuilder {
	if b.err != nil {
		return b
	}
	if b.ofgaUser == nil {
		b.err = errors.New("authorizer not specified")
		return b
	}
	var candidateControllers []candidateController
	err := b.jujuManager.Database.ForEachController(b.ctx, func(c *dbmodel.Controller) error {
		if c.Deprecated {
			return nil
		}
		ok, err := b.ofgaUser.IsAllowedAddModelToController(b.ctx, c.ResourceTag())
		if err != nil {
			return fmt.Errorf("failed to verify permissions for adding model to controller: %v", err)
		}
		if ok {
			candidateControllers = append(candidateControllers, candidateController{
				controller: *c,
				priority:   0, // priority is unknown until we select the cloud-region
			})
		}
		return nil
	})
	if err != nil {
		b.err = err
		return b
	}
	b.candidates = candidateControllers
	// Error early with a helpful message if no
	// candidate controllers are available.
	if len(b.candidates) == 0 {
		b.err = errors.New("no available controllers - check permissions to controllers and list of available controllers")
	}
	return b
}

// WithCloud returns a builder with the specified cloud.
// Based on the cloud, the candidate controllers are filtered
// to only those that support the specified cloud.
func (b *modelBuilder) WithCloud(user *openfga.User, cloud names.CloudTag) *modelBuilder {
	if b.err != nil {
		return b
	}
	if b.ofgaUser == nil {
		b.err = errors.New("authorizer not specified")
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

	if err := b.jujuManager.Database.GetCloud(b.ctx, &c); err != nil {
		b.err = err
		return b
	}
	ok, err := b.ofgaUser.IsAllowedAddModelToCloud(b.ctx, c.ResourceTag())
	if err != nil {
		b.err = fmt.Errorf("failed to verify permissions for adding model to cloud: %w", err)
		return b
	}
	if !ok {
		b.err = errors.Codef(errors.CodeUnauthorized, "not authorized to add model to cloud %q", cloud.Id())
		return b
	}
	b.cloud = &c

	// Filter the candidate controllers based on the cloud.
	matchCloud := func(r dbmodel.CloudRegionControllerPriority) bool {
		return r.CloudRegion.CloudName == c.Name
	}
	var candidateControllers []candidateController
	for _, controller := range b.candidates {
		if slices.ContainsFunc(controller.controller.CloudRegions, matchCloud) {
			candidateControllers = append(candidateControllers, controller)
		}
	}
	b.candidates = candidateControllers

	return b
}

// withImplicitCloud returns a builder with the only cloud known to JIMM. Should JIMM
// know of multiple clouds an error will be raised.
func (b *modelBuilder) withImplicitCloud(user *openfga.User) *modelBuilder {
	if b.err != nil {
		return b
	}
	var clouds []*dbmodel.Cloud
	err := b.jujuManager.ForEachUserCloud(b.ctx, user, func(c *dbmodel.Cloud) error {
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
// It filters the candidate controllers based on the specified region.
// If the region is not specified, we pick the first cloud region
// with any associated candidate controller
func (b *modelBuilder) WithCloudRegion(region string) *modelBuilder {
	if b.err != nil {
		return b
	}
	if b.cloud == nil {
		b.err = errors.New("cloud not specified")
		return b
	}

	if region == "" {
		// Make a map of supported regions in the cloud from among the candidate controllers.
		supported := make(map[string]struct{})
		for _, c := range b.candidates {
			for _, cr := range c.controller.CloudRegions {
				if cr.CloudRegion.CloudName == b.cloud.Name {
					supported[cr.CloudRegion.Name] = struct{}{}
				}
			}
		}

		// Pick the first supported region from the cloud's regions.
		for _, r := range b.cloud.Regions {
			if _, ok := supported[r.Name]; ok {
				region = r.Name
				break
			}
		}
	}

	// If there is no such region we will get a zero valued object below.
	cloudRegion := b.cloud.Region(region)
	if cloudRegion.Name == "" {
		b.err = errors.Codef(errors.CodeNotFound, "cloud region %q not found in cloud %q", region, b.cloud.Name)
		return b
	}
	b.cloudRegion = region
	b.cloudRegionID = cloudRegion.ID
	b.cloudRegionVirtual = cloudRegion.Virtual

	// Filter candidate controllers based on the specified region.
	var candidateControllers []candidateController
	for _, candidate := range b.candidates {
		if i := slices.IndexFunc(cloudRegion.Controllers, func(crp dbmodel.CloudRegionControllerPriority) bool {
			return crp.ControllerID == candidate.controller.ID
		}); i != -1 {
			candidateControllers = append(candidateControllers, candidateController{
				controller: candidate.controller,
				priority:   cloudRegion.Controllers[i].Priority,
			})
		}
	}
	b.candidates = candidateControllers
	return b
}

// WithCloudCredential returns a builder with the specified cloud credentials.
func (b *modelBuilder) WithCloudCredential(credentialTag names.CloudCredentialTag) *modelBuilder {
	if b.err != nil {
		return b
	}

	// Verify ownership of cloud credential
	if b.owner == nil || b.owner.Name != credentialTag.Owner().Id() {
		b.err = errors.Codef(errors.CodeUnauthorized, "model owner doesn't match cloud-credential owner")
		return b
	}

	credential := dbmodel.CloudCredential{
		Name:              credentialTag.Name(),
		CloudName:         credentialTag.Cloud().Id(),
		OwnerIdentityName: credentialTag.Owner().Id(),
	}
	err := b.jujuManager.Database.GetCloudCredential(b.ctx, &credential)
	if err != nil {
		b.err = fmt.Errorf("failed to fetch cloud credentials %s: %w", credential.Path(), err)
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
		b.err = errors.New("model name not specified")
		return b
	}
	// if the model owner is not specified we error and abort
	if b.owner == nil {
		b.err = errors.New("owner not specified")
		return b
	}

	if err := b.selectController(); err != nil {
		b.err = err
		return b
	}
	// if controller is still not selected, there's nothing
	// we can do - either a cloud or a cloud region was specified
	// by this point and a controller should've been selected
	if b.controller == nil {
		b.err = errors.New("unable to determine a suitable controller")
		return b
	}

	if b.credential == nil {
		// try to select a valid credential
		if err := b.selectCloudCredentials(); err != nil {
			b.err = fmt.Errorf("could not select cloud credentials: %w", err)
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

	err := b.jujuManager.Database.AddModel(b.ctx, b.model)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeAlreadyExists {
			b.err = errors.Codef(errors.CodeAlreadyExists, "model %s/%s already exists", b.owner.Name, b.name)
			return b
		} else {
			b.err = fmt.Errorf("failed to store model information: %w", err)
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
	if derr := b.jujuManager.Database.DeleteModel(ctx, b.model); derr != nil {
		zapctx.Error(ctx, "failed to delete model", zap.String("model", b.model.Name), zap.String("owner", b.model.Owner.Name), zaputil.Error(derr))
	}
}

// UpdateDatabaseModel persists the information about the model
// retrieved from Juju to our database.
func (b *modelBuilder) UpdateDatabaseModel() *modelBuilder {
	if b.err != nil {
		return b
	}
	err := b.model.FromJujuModelInfo(b.modelInfo)
	if err != nil {
		b.err = fmt.Errorf("failed to convert model info: %w", err)
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

	err = b.jujuManager.Database.UpdateModel(b.ctx, b.model)
	if err != nil {
		b.err = fmt.Errorf("failed to store model information: %w", err)
		return b
	}
	return b
}

// selectController selects a controller to use for the model.
// It assumes that the candidates slice is already filtered
// based on the specified cloud and region and/or the user's
// specified target controller.
func (b *modelBuilder) selectController() error {
	// if no controllers are found, we return an error
	if len(b.candidates) == 0 {
		return fmt.Errorf("unsupported cloud region %s/%s - confirm access to the cloud/controller", b.cloud.Name, b.cloudRegion)
	}

	// shuffle controllers according to their priority
	shuffleCandidateControllers(b.candidates)

	b.controller = &b.candidates[0].controller

	return nil
}

func (b *modelBuilder) selectCloudCredentials() error {
	if b.owner == nil {
		return errors.New("user not specified")
	}
	if b.cloud == nil {
		return errors.New("cloud not specified")
	}
	credentials, err := b.jujuManager.Database.GetIdentityCloudCredentials(b.ctx, b.owner, b.cloud.Name)
	if err != nil {
		return fmt.Errorf("failed to fetch user cloud credentials: %w", err)
	}
	for _, credential := range credentials {
		// skip any credentials known to be invalid.
		if credential.Valid.Valid && !credential.Valid.Bool {
			continue
		}
		b.credential = &credential
		return nil
	}
	return errors.New("valid cloud credentials not found")
}

// CreateControllerModel uses provided information to create a new
// model on the selected controller.
func (b *modelBuilder) CreateControllerModel() *modelBuilder {
	if b.err != nil {
		return b
	}

	if b.model == nil {
		b.err = errors.New("model not specified")
		return b
	}

	api, err := b.jujuManager.dial(b.ctx, b.controller, names.ModelTag{}, b.ofgaUser)
	if err != nil {
		b.err = err
		return b
	}
	defer api.Close()

	if b.credential != nil {
		if err := b.updateCredential(b.ctx, api, b.credential); err != nil {
			b.err = fmt.Errorf("failed to update cloud credential: %w", err)
			return b
		}
	}

	args, err := b.jujuModelCreateArgs()
	if err != nil {
		b.err = err
		return b
	}

	info, err := api.CreateModel(b.ctx, args)
	if err != nil {
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
			b.err = errors.Codef(errors.CodeAlreadyExists, "model name in use: %w", err)
		case jujuparams.CodeUpgradeInProgress:
			b.err = fmt.Errorf("upgrade in progress: %w", err)
		default:
			// The model couldn't be created because of an
			// error in the request, don't try another
			// controller.
			b.err = errors.Codef(errors.CodeBadRequest, "%w", err)
		}
		return b
	}

	// Grant JIMM admin access to the model. Note that if this fails,
	// the local database entry will be deleted but the model
	// will remain on the controller and will trigger the "already exists
	// in the backend controller" message above when the user
	// attempts to create a model with the same name again.
	// TODO(JUJU-8869): We need to keep this despite encoding permissions in
	// JWTs because Juju returns a different result on migrated models otherwise.
	if err := api.GrantJIMMModelAdmin(b.ctx, names.NewModelTag(info.UUID)); err != nil {
		zapctx.Error(b.ctx, "leaked model", zap.String("model", info.UUID), zaputil.Error(err))
		b.err = err
		return b
	}

	b.modelInfo = info
	return b
}

func (b *modelBuilder) updateCredential(ctx context.Context, api API, cred *dbmodel.CloudCredential) error {
	var err error

	_, err = b.jujuManager.updateControllerCloudCredential(ctx, cred, api.UpdateCloudsCredentialForce)
	return err
}

// JujuModelInfo returns model information returned by the controller.
func (b *modelBuilder) JujuModelInfo() base.ModelInfo {
	return b.modelInfo
}
