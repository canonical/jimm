// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	jujupermission "github.com/juju/juju/core/permission"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"go.uber.org/zap"

	"github.com/canonical/jimm/internal/constants"
	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm/workers"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
)

// shuffle is used to randomize the order in which possible controllers
// are tried. It is a variable so it can be replaced in tests.
var shuffle func(int, func(int, int)) = rand.Shuffle

func shuffleRegionControllers(controllers []dbmodel.CloudRegionControllerPriority) {
	shuffle(len(controllers), func(i, j int) {
		controllers[i], controllers[j] = controllers[j], controllers[i]
	})
	sort.SliceStable(controllers, func(i, j int) bool {
		return controllers[i].Priority > controllers[j].Priority
	})
}

// ModelCreateArgs contains parameters used to add a new model.
type ModelCreateArgs struct {
	Name            string
	Owner           names.UserTag
	Config          map[string]interface{}
	Cloud           names.CloudTag
	CloudRegion     string
	CloudCredential names.CloudCredentialTag
}

// FromJujuModelCreateArgs converts jujuparams.ModelCreateArgs into AddModelArgs.
func (a *ModelCreateArgs) FromJujuModelCreateArgs(args *jujuparams.ModelCreateArgs) error {
	if args.Name == "" {
		return errors.E("name not specified")
	}
	a.Name = args.Name
	a.Config = args.Config
	a.CloudRegion = args.CloudRegion
	if args.CloudTag == "" {
		return errors.E("no cloud specified for model; please specify one")
	}
	ct, err := names.ParseCloudTag(args.CloudTag)
	if err != nil {
		return errors.E(err, errors.CodeBadRequest)
	}
	a.Cloud = ct

	if args.OwnerTag == "" {
		return errors.E("owner tag not specified")
	}
	ot, err := names.ParseUserTag(args.OwnerTag)
	if err != nil {
		return errors.E(err, errors.CodeBadRequest)
	}
	a.Owner = ot

	if args.CloudCredentialTag != "" {
		ct, err := names.ParseCloudCredentialTag(args.CloudCredentialTag)
		if err != nil {
			return errors.E(err, "invalid cloud credential tag")
		}
		if a.Cloud.Id() != "" && ct.Cloud().Id() != a.Cloud.Id() {
			return errors.E("cloud credential cloud mismatch")
		}

		a.CloudCredential = ct
	}
	return nil
}

func newModelBuilder(ctx context.Context, j *JIMM) *modelBuilder {
	return &modelBuilder{
		ctx:  ctx,
		jimm: j,
	}
}

func newModelBuilderFromModel(ctx context.Context, j *JIMM, owner *dbmodel.User, modelId uint, config map[string]interface{}) (*modelBuilder, error) {
	const op = errors.Op("jimm.newModelBuilderFromModel")
	b := &modelBuilder{
		ctx:   ctx,
		jimm:  j,
		owner: owner,
	}
	b = b.WithConfig(config)
	b.model = &dbmodel.Model{
		ID: modelId,
	}
	if err := j.Database.GetModel(ctx, b.model); err != nil {
		return nil, errors.E(err, fmt.Sprintf("failed to fetch model information, err: %s", err))
	}
	b.name = b.model.Name
	b.controller = &b.model.Controller
	b.cloud = &b.model.CloudRegion.Cloud
	if err := j.Database.GetCloud(ctx, b.cloud); err != nil {
		return nil, errors.E(err, "cloud not found")
	}
	b = b.WithCloudRegion(b.model.CloudRegion.Name)
	b.credential = &b.model.CloudCredential

	// fetch cloud region defaults
	if b.cloud != nil && names.NewCloudTag(b.cloud.Name) != (names.CloudTag{}) && b.cloudRegion != "" {
		cloudRegionDefaults := dbmodel.CloudDefaults{
			Username: owner.Username,
			Cloud: dbmodel.Cloud{
				Name: names.NewCloudTag(b.cloud.Name).Id(),
			},
			Region: b.cloudRegion,
		}
		err := j.Database.CloudDefaults(ctx, &cloudRegionDefaults)
		if err != nil && errors.ErrorCode(err) != errors.CodeNotFound {
			return nil, errors.E(op, "failed to fetch cloud defaults")
		}
		b = b.WithConfig(cloudRegionDefaults.Defaults)
	}
	return b, b.err
}

type modelBuilder struct {
	ctx context.Context
	err error

	jimm *JIMM

	name          string
	config        map[string]interface{}
	owner         *dbmodel.User
	credential    *dbmodel.CloudCredential
	controller    *dbmodel.Controller
	cloud         *dbmodel.Cloud
	cloudRegion   string
	cloudRegionID uint
	model         *dbmodel.Model
	modelInfo     *jujuparams.ModelInfo
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

	return &jujuparams.ModelCreateArgs{
		Name:               b.name,
		OwnerTag:           b.owner.Tag().String(),
		Config:             b.config,
		CloudTag:           b.cloud.Tag().String(),
		CloudRegion:        b.cloudRegion,
		CloudCredentialTag: b.credential.Tag().String(),
	}, nil
}

// WithOwner returns a builder with the specified owner.
func (b *modelBuilder) WithOwner(owner *dbmodel.User) *modelBuilder {
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
func (b *modelBuilder) WithCloud(cloud names.CloudTag) *modelBuilder {
	if b.err != nil {
		return b
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
		b.cloudRegionID = regionControllers[0].CloudRegionID
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
		Name:          credentialTag.Name(),
		CloudName:     credentialTag.Cloud().Id(),
		OwnerUsername: credentialTag.Owner().Id(),
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
			b.err = errors.E(err, fmt.Sprintf("model %s/%s already exists", b.owner.Username, b.name))
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
		zapctx.Error(ctx, "failed to delete model", zap.String("model", b.model.Name), zap.String("owner", b.model.Owner.Username), zaputil.Error(derr))
	}
}

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
	credentials, err := b.jimm.Database.GetUserCloudCredentials(b.ctx, b.owner, b.cloud.Name)
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
	b.modelInfo = &jujuparams.ModelInfo{}
	if err := api.CreateModel(b.ctx, args, b.modelInfo); err != nil {
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
			if miErr := api.ModelInfo(b.ctx, b.modelInfo); miErr != nil {
				b.err = errors.E(err, fmt.Sprintf("model already exists, but failed to read its model info: %s", miErr))
			}
		case jujuparams.CodeUpgradeInProgress:
			b.err = errors.E(err, "upgrade in progress")
		default:
			// The model couldn't be created because of an
			// error in the request, don't try another
			// controller.
			b.err = errors.E(err, errors.CodeBadRequest)
		}
	}
	return b
}

func (b *modelBuilder) updateCredential(ctx context.Context, api API, cred *dbmodel.CloudCredential) error {
	var err error
	cred1 := *cred
	cred1.Attributes, err = b.jimm.getCloudCredentialAttributes(ctx, cred)
	if err != nil {
		return err
	}

	_, err = b.jimm.updateControllerCloudCredential(ctx, &cred1, api.UpdateCredential)
	return err
}

// JujuModelInfo returns model information returned by the controller.
func (b *modelBuilder) JujuModelInfo() *jujuparams.ModelInfo {
	return b.modelInfo
}

// RiverAddModelArgs holds the river job arguments for building models.
type RiverAddModelArgs struct {
	ModelId   uint                   `json:"model_id"`
	OwnerName string                 `json:"owner_name"`
	Config    map[string]interface{} `json:"config"`
}

// Kind is a string that uniquely identifies the type of job to be picked up by the appropriate river workers.
// This is required by river and must be provided on the job arguments struct to implement the JobArgs interface.
func (RiverAddModelArgs) Kind() string { return "AddModel" }

// RiverAddModelWorker is the river worker that would run the job.
type RiverAddModelWorker struct {
	river.WorkerDefaults[RiverAddModelArgs]
	Database   *db.Database
	JIMM       *JIMM
	OfgaClient *openfga.OFGAClient
}

// Timeout returns the timeout duration for the RiverAddModelWorker.
// This is needed to avoid using the default timeout on the client.
func (w *RiverAddModelWorker) Timeout(job *river.Job[RiverAddModelArgs]) time.Duration {
	return workers.AddModelTimeout
}

// NextRetry returns the time that is used to schedule the next retry in case of a failure.
func (w *RiverAddModelWorker) NextRetry(job *river.Job[RiverAddModelArgs]) time.Time {
	return time.Now().Add(20 * time.Second)
}

// Work is the function executed by the worker when it picks up the job.
func (w *RiverAddModelWorker) Work(ctx context.Context, job *river.Job[RiverAddModelArgs]) error {
	const op = errors.Op("jimm.AddModel")
	args := job.Args
	j := w.JIMM
	owner := &dbmodel.User{Username: args.OwnerName}
	if err := j.Database.GetUser(ctx, owner); err != nil {
		return errors.E(op, err)
	}
	builder, err := newModelBuilderFromModel(ctx, w.JIMM, owner, args.ModelId, args.Config)
	if err != nil {
		return errors.E(op, err)
	}
	builder = builder.CreateControllerModel()
	if err := builder.Error(); err != nil {
		return errors.E(op, err)
	}
	// update builder to construct the builder state from the model id.
	builder = builder.UpdateDatabaseModel()
	if err := builder.Error(); err != nil {
		return errors.E(op, err)
	}

	mi := builder.JujuModelInfo()
	if err := w.OfgaClient.AddControllerModel(
		ctx,
		builder.controller.ResourceTag(),
		builder.model.ResourceTag(),
	); err != nil {
		zapctx.Error(
			ctx,
			"failed to add controller-model relation",
			zap.String("controller", builder.controller.UUID),
			zap.String("model", builder.model.UUID.String),
		)
		return errors.E(err, "failed to add the controller-model relation from the river job.")
	}
	err = openfga.NewUser(owner, w.OfgaClient).SetModelAccess(ctx, names.NewModelTag(mi.UUID), ofganames.AdministratorRelation)
	if err != nil {
		zapctx.Error(
			ctx,
			"failed to add administrator relation",
			zap.String("user", owner.Tag().String()),
			zap.String("model", builder.model.UUID.String),
		)
		return errors.E(err, "failed to add the administrator relation from the river job.")
	}
	return nil
}

// AddModel adds the specified model to JIMM.
func (j *JIMM) AddModel(ctx context.Context, user *openfga.User, args *ModelCreateArgs) (_ *jujuparams.ModelInfo, err error) {
	const op = errors.Op("jimm.AddModel")
	owner := &dbmodel.User{
		Username: args.Owner.Id(),
	}
	err = j.Database.GetUser(ctx, owner)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// Only JIMM admins are able to add models on behalf of other users.
	if owner.Username != user.Username && !user.JimmAdmin {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	builder := newModelBuilder(ctx, j)
	builder = builder.WithOwner(owner)
	builder = builder.WithName(args.Name)
	if err := builder.Error(); err != nil {
		return nil, errors.E(op, err)
	}

	// fetch user model defaults
	userConfig, err := j.UserModelDefaults(ctx, user.User)
	if err != nil && errors.ErrorCode(err) != errors.CodeNotFound {
		return nil, errors.E(op, "failed to fetch cloud defaults")
	}
	builder = builder.WithConfig(userConfig)

	// fetch cloud defaults
	if args.Cloud != (names.CloudTag{}) {
		cloudDefaults := dbmodel.CloudDefaults{
			Username: user.Username,
			Cloud: dbmodel.Cloud{
				Name: args.Cloud.Id(),
			},
		}
		err = j.Database.CloudDefaults(ctx, &cloudDefaults)
		if err != nil && errors.ErrorCode(err) != errors.CodeNotFound {
			return nil, errors.E(op, "failed to fetch cloud defaults")
		}
		builder = builder.WithConfig(cloudDefaults.Defaults)
		builder = builder.WithCloud(args.Cloud)
		if err := builder.Error(); err != nil {
			return nil, errors.E(op, err)
		}
	}

	builder = builder.WithCloudRegion(args.CloudRegion)
	if err := builder.Error(); err != nil {
		return nil, errors.E(op, err)
	}

	// at this point we know which cloud will host the model and
	// we must check the user has add-model permission on the cloud
	ownerOfgaUser := openfga.NewUser(owner, j.OpenFGAClient)
	canAddModel, err := ownerOfgaUser.IsAllowedAddModel(ctx, builder.cloud.ResourceTag())
	if err != nil {
		return nil, errors.E(op, "permission check failed")
	}
	if !canAddModel {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	// fetch cloud region defaults
	if args.Cloud != (names.CloudTag{}) && builder.cloudRegion != "" {
		cloudRegionDefaults := dbmodel.CloudDefaults{
			Username: user.Username,
			Cloud: dbmodel.Cloud{
				Name: args.Cloud.Id(),
			},
			Region: builder.cloudRegion,
		}
		err = j.Database.CloudDefaults(ctx, &cloudRegionDefaults)
		if err != nil && errors.ErrorCode(err) != errors.CodeNotFound {
			return nil, errors.E(op, "failed to fetch cloud defaults")
		}
		builder = builder.WithConfig(cloudRegionDefaults.Defaults)
	}

	// last but not least, use the provided config values
	// overriding all defaults
	builder = builder.WithConfig(args.Config)

	if args.CloudCredential != (names.CloudCredentialTag{}) {
		builder = builder.WithCloudCredential(args.CloudCredential)
		if err := builder.Error(); err != nil {
			return nil, errors.E(op, err)
		}
	}

	builder = builder.CreateDatabaseModel()
	if err := builder.Error(); err != nil {
		return nil, errors.E(op, err)
	}
	defer builder.Cleanup()

	riverAddModelArgs := RiverAddModelArgs{
		Config:    builder.config,
		ModelId:   builder.model.ID,
		OwnerName: owner.Username,
	}

	waitConfig := &workers.WaitConfig{Duration: time.Duration(j.River.MaxAttempts) * workers.AddModelTimeout}
	err = InsertJob(ctx, waitConfig, j.River, func() (*rivertype.JobRow, error) {
		return j.River.Client.Insert(ctx, riverAddModelArgs, &river.InsertOpts{MaxAttempts: j.River.MaxAttempts})
	})
	if err != nil {
		builder.err = err
		return nil, errors.E(err, fmt.Sprintf("failed to insert and wait for the river job, err: %s", err))
	}
	model := &dbmodel.Model{
		ID: builder.model.ID,
	}
	if err = j.Database.GetModel(ctx, model); err != nil {
		return nil, errors.E(err, fmt.Sprintf("failed to fetch model information, err: %s", err))
	}
	modelInfo, err := j.ModelInfo(ctx, ownerOfgaUser, names.NewModelTag(model.UUID.String))
	if err != nil {
		return nil, errors.E(err, fmt.Sprintf("failed to read model info, err: %s", err))
	}
	return modelInfo, nil
}

// ModelInfo returns the model info for the model with the given ModelTag.
// The returned ModelInfo will be appropriate for the given user's
// access-level on the model. If the model does not exist then the returned
// error will have the code CodeNotFound. If the given user does not have
// access to the model then the returned error will have the code
// CodeUnauthorized.
func (j *JIMM) ModelInfo(ctx context.Context, user *openfga.User, mt names.ModelTag) (*jujuparams.ModelInfo, error) {
	const op = errors.Op("jimm.ModelInfo")

	var m dbmodel.Model
	m.SetTag(mt)
	if err := j.Database.GetModel(ctx, &m); err != nil {
		return nil, errors.E(op, err)
	}

	modelAccess, err := j.GetUserModelAccess(ctx, user, mt)
	if err != nil {
		return nil, errors.E(op, err)
	}
	if modelAccess == "" {
		// If the user doesn't have any access on the model return an
		// unauthorized error
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	api, err := j.dial(ctx, &m.Controller, names.ModelTag{})
	if err != nil {
		return nil, errors.E(op, err)
	}
	defer api.Close()

	mi := &jujuparams.ModelInfo{
		UUID: mt.Id(),
	}
	if err := api.ModelInfo(ctx, mi); err != nil {
		return nil, errors.E(op, err)
	}

	userAccess := make(map[string]string)

	for _, relation := range []openfga.Relation{
		// Here we list possible relation in decreasing level
		// of access privilege.
		ofganames.AdministratorRelation,
		ofganames.WriterRelation,
		ofganames.ReaderRelation,
	} {
		usersWithSpecifiedRelation, err := openfga.ListUsersWithAccess(ctx, j.OpenFGAClient, mt, relation)
		if err != nil {
			return nil, errors.E(op, err)
		}
		for _, u := range usersWithSpecifiedRelation {
			// Since we are checking user relations in decreasing level of
			// access privilege, we want to make sure the user has not
			// already been recorded with a higher access level.
			if _, ok := userAccess[u.Username]; !ok {
				userAccess[u.Username] = ToModelAccessString(relation)
			}
		}
	}

	users := make([]jujuparams.ModelUserInfo, 0, len(userAccess))
	for username, access := range userAccess {
		// If the user does not contain an "@" sign (no domain), it means
		// this is a local user of this controller and JIMM does not
		// care or know about local users - only Candid users are
		// relevant.
		if !strings.Contains(username, "@") {
			continue
		}
		if modelAccess == "admin" || username == user.Username || username == ofganames.EveryoneUser {
			users = append(users, jujuparams.ModelUserInfo{
				UserName: username,
				Access:   jujuparams.UserAccessPermission(access),
			})
		}
	}
	mi.Users = users

	if modelAccess != "admin" && modelAccess != "write" {
		// Users need "write" level access (or above) to see machine
		// information.
		mi.Machines = nil
	}

	return mi, nil
}

// ModelStatus returns a jujuparams.ModelStatus for the given model. If
// the model doesn't exist then the returned error will have the code
// CodeNotFound, If the given user does not have admin access to the model
// then the returned error will have the code CodeUnauthorized.
func (j *JIMM) ModelStatus(ctx context.Context, user *openfga.User, mt names.ModelTag) (*jujuparams.ModelStatus, error) {
	const op = errors.Op("jimm.ModelStatus")

	var ms jujuparams.ModelStatus
	err := j.doModelAdmin(ctx, user, mt, func(_ *dbmodel.Model, api API) error {
		ms.ModelTag = mt.String()
		return api.ModelStatus(ctx, &ms)
	})
	if err != nil {
		return nil, errors.E(op, err)
	}
	return &ms, nil
}

// ForEachUserModel calls the given function once for each model that the
// given user has been granted explicit access to. The UserModelAccess
// object passed to f will always include the Model_, Access, and
// LastConnection fields populated. ForEachUserModel ignores a user's
// controller access when determining the set of models to return, for
// superusers the ForEachModel method should be used to get every model in
// the system. If the given function returns an error the error will be
// returned unmodified and iteration will stop immediately. The given
// function should not update the database.
func (j *JIMM) ForEachUserModel(ctx context.Context, user *openfga.User, f func(*dbmodel.Model, jujuparams.UserAccessPermission) error) error {
	const op = errors.Op("jimm.ForEachUserModel")

	errStop := errors.E("stop")
	var iterErr error
	err := j.Database.ForEachModel(ctx, func(m *dbmodel.Model) error {
		model := *m

		access, err := j.GetUserModelAccess(ctx, user, model.ResourceTag())
		if err != nil {
			return errors.E(op, err)
		}
		if access == "read" || access == "write" || access == "admin" {
			if err := f(&model, jujuparams.UserAccessPermission(access)); err != nil {
				iterErr = err
				return errStop
			}
			return nil
		}
		return nil
	})
	switch err {
	case nil:
		return nil
	case errStop:
		return iterErr
	default:
		return errors.E(op, err)
	}
}

// ForEachModel calls the given function once for each model in the system.
// The UserModelAccess object passed to f will always specify that the
// user's Access is "admin" and will not include the LastConnection time.
// ForEachModel will return an error with the code CodeUnauthorized when
// the user is not a controller admin. If the given function returns an
// error the error will be returned unmodified and iteration will stop
// immediately. The given function should not update the database.
func (j *JIMM) ForEachModel(ctx context.Context, user *openfga.User, f func(*dbmodel.Model, jujuparams.UserAccessPermission) error) error {
	const op = errors.Op("jimm.ForEachModel")

	if !user.JimmAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	errStop := errors.E("stop")
	var iterErr error
	err := j.Database.ForEachModel(ctx, func(m *dbmodel.Model) error {
		if err := f(m, jujuparams.UserAccessPermission("admin")); err != nil {
			iterErr = err
			return errStop
		}
		return nil
	})
	switch err {
	case nil:
		return nil
	case errStop:
		return iterErr
	default:
		return errors.E(op, err)
	}
}

// GrantModelAccess grants the given access level on the given model to
// the given user. If the model is not found then an error with the code
// CodeNotFound is returned. If the authenticated user does not have
// admin access to the model then an error with the code CodeUnauthorized
// is returned.
func (j *JIMM) GrantModelAccess(ctx context.Context, user *openfga.User, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error {
	const op = errors.Op("jimm.GrantModelAccess")

	targetRelation, err := ToModelRelation(string(access))
	if err != nil {
		zapctx.Debug(
			ctx,
			"failed to recognize given access",
			zaputil.Error(err),
			zap.String("access", string(access)),
		)
		return errors.E(op, errors.CodeBadRequest, fmt.Sprintf("failed to recognize given access: %q", access), err)
	}

	err = j.doModelAdmin(ctx, user, mt, func(_ *dbmodel.Model, _ API) error {
		targetUser := &dbmodel.User{}
		targetUser.SetTag(ut)
		if err := j.Database.GetUser(ctx, targetUser); err != nil {
			return err
		}
		targetOfgaUser := openfga.NewUser(targetUser, j.OpenFGAClient)

		currentRelation := targetOfgaUser.GetModelAccess(ctx, mt)
		switch targetRelation {
		case ofganames.ReaderRelation:
			switch currentRelation {
			case ofganames.NoRelation:
				break
			default:
				return nil
			}
		case ofganames.WriterRelation:
			switch currentRelation {
			case ofganames.NoRelation, ofganames.ReaderRelation:
				break
			default:
				return nil
			}
		case ofganames.AdministratorRelation:
			switch currentRelation {
			case ofganames.NoRelation, ofganames.ReaderRelation, ofganames.WriterRelation:
				break
			default:
				return nil
			}
		}

		if err := targetOfgaUser.SetModelAccess(ctx, mt, targetRelation); err != nil {
			return errors.E(err, op, "failed to set model access")
		}
		return nil
	})

	if err != nil {
		zapctx.Error(
			ctx,
			"failed to grant model access",
			zaputil.Error(err),
			zap.String("targetUser", string(ut.Id())),
			zap.String("model", string(mt.Id())),
			zap.String("access", string(access)),
		)
		return errors.E(op, err)
	}
	return nil
}

// RevokeModelAccess revokes the given access level on the given model from
// the given user. If the model is not found then an error with the code
// CodeNotFound is returned. If the authenticated user does not have admin
// access to the model, and is not attempting to revoke their own access,
// then an error with the code CodeUnauthorized is returned.
func (j *JIMM) RevokeModelAccess(ctx context.Context, user *openfga.User, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error {
	const op = errors.Op("jimm.RevokeModelAccess")

	targetRelation, err := ToModelRelation(string(access))
	if err != nil {
		zapctx.Debug(
			ctx,
			"failed to recognize given access",
			zaputil.Error(err),
			zap.String("access", string(access)),
		)
		return errors.E(op, errors.CodeBadRequest, fmt.Sprintf("failed to recognize given access: %q", access), err)
	}

	requiredAccess := "admin"
	if user.Tag() == ut {
		// If the user is attempting to revoke their own access.
		requiredAccess = "read"
	}

	err = j.doModel(ctx, user, mt, requiredAccess, func(_ *dbmodel.Model, _ API) error {
		targetUser := &dbmodel.User{}
		targetUser.SetTag(ut)
		if err := j.Database.GetUser(ctx, targetUser); err != nil {
			return err
		}
		targetOfgaUser := openfga.NewUser(targetUser, j.OpenFGAClient)

		currentRelation := targetOfgaUser.GetModelAccess(ctx, mt)

		var relationsToRevoke []openfga.Relation
		switch targetRelation {
		case ofganames.ReaderRelation:
			switch currentRelation {
			case ofganames.NoRelation:
				return nil
			default:
				relationsToRevoke = []openfga.Relation{
					ofganames.ReaderRelation,
					ofganames.WriterRelation,
					ofganames.AdministratorRelation,
				}
			}
		case ofganames.WriterRelation:
			switch currentRelation {
			case ofganames.NoRelation, ofganames.ReaderRelation:
				return nil
			default:
				relationsToRevoke = []openfga.Relation{
					ofganames.WriterRelation,
					ofganames.AdministratorRelation,
				}
			}
		case ofganames.AdministratorRelation:
			switch currentRelation {
			case ofganames.NoRelation, ofganames.ReaderRelation, ofganames.WriterRelation:
				return nil
			default:
				relationsToRevoke = []openfga.Relation{
					ofganames.AdministratorRelation,
				}
			}
		}

		if err := targetOfgaUser.UnsetModelAccess(ctx, mt, relationsToRevoke...); err != nil {
			return errors.E(err, op, "failed to unset model access")
		}
		return nil
	})

	if err != nil {
		zapctx.Error(
			ctx,
			"failed to revoke model access",
			zaputil.Error(err),
			zap.String("targetUser", string(ut.Id())),
			zap.String("model", string(mt.Id())),
			zap.String("access", string(access)),
		)
		return errors.E(op, err)
	}
	return nil
}

// DestroyModel starts the process of destroying the given model. If the
// given user is not a controller superuser or a model admin an error
// with a code of CodeUnauthorized is returned. Any error returned from
// the juju API will not have it's code masked.
func (j *JIMM) DestroyModel(ctx context.Context, user *openfga.User, mt names.ModelTag, destroyStorage, force *bool, maxWait, timeout *time.Duration) error {
	const op = errors.Op("jimm.DestroyModel")

	if destroyStorage != nil {
	}
	if force != nil {
	}

	err := j.doModelAdmin(ctx, user, mt, func(m *dbmodel.Model, api API) error {
		if err := api.DestroyModel(ctx, mt, destroyStorage, force, maxWait, timeout); err != nil {
			return err
		}
		m.Life = constants.DYING.String()
		if err := j.Database.UpdateModel(ctx, m); err != nil {
			// If the database fails to update don't worry too much the
			// monitor should catch it.
			zapctx.Error(ctx, "failed to store model change", zaputil.Error(err))
		}
		return nil
	})
	if err != nil {
		return errors.E(op, err)
	}

	// NOTE (alesstimec) If we remove OpenFGA relation now, the user
	// will no longer be authorised to check for model status (which
	// will show the model as dying for a bit, until the Juju controller
	// completes the model destuction).

	return nil
}

// DumpModel retrieves a database-agnostic dump of the given model from its
// juju controller. If simplified is true a simpllified dump is requested.
// If the given user is not a controller superuser or a model admin an
// error with the code CodeUnauthorized is returned.
func (j *JIMM) DumpModel(ctx context.Context, user *openfga.User, mt names.ModelTag, simplified bool) (string, error) {
	const op = errors.Op("jimm.DumpModel")

	var dump string
	err := j.doModelAdmin(ctx, user, mt, func(m *dbmodel.Model, api API) error {
		var err error
		dump, err = api.DumpModel(ctx, mt, simplified)
		return err
	})
	if err != nil {
		return "", errors.E(op, err)
	}
	return dump, nil
}

// DumpModelDB retrieves a database dump of the given model from its juju
// controller. If the given user is not a controller superuser or a model
// admin an error with the code CodeUnauthorized is returned.
func (j *JIMM) DumpModelDB(ctx context.Context, user *openfga.User, mt names.ModelTag) (map[string]interface{}, error) {
	const op = errors.Op("jimm.DumpModelDB")

	var dump map[string]interface{}
	err := j.doModelAdmin(ctx, user, mt, func(m *dbmodel.Model, api API) error {
		var err error
		dump, err = api.DumpModelDB(ctx, mt)
		return err
	})
	if err != nil {
		return nil, errors.E(op, err)
	}
	return dump, nil
}

// ValidateModelUpgrade validates that a model is in a state that can be
// upgraded. If the given user is not a controller superuser or a model
// admin then an error with the code CodeUnauthorized is returned. Any
// error returned from the API will have the code maintained therefore if
// the controller doesn't support the ValidateModelUpgrades command the
// CodeNotImplemented error code will be propagated back to the client.
func (j *JIMM) ValidateModelUpgrade(ctx context.Context, user *openfga.User, mt names.ModelTag, force bool) error {
	const op = errors.Op("jimm.ValidateModelUpgrade")

	err := j.doModelAdmin(ctx, user, mt, func(_ *dbmodel.Model, api API) error {
		return api.ValidateModelUpgrade(ctx, mt, force)
	})
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// doModelAdmin is a simple wrapper that provides the common parts of model
// administration commands. doModelAdmin finds the model with the given tag
// and validates that the given user has admin access to the model.
// doModelAdmin then connects to the controller hosting the model and calls
// the given function with the model and API connection to perform the
// operation specific commands. If the model cannot be found then an error
// with the code CodeNotFound is returned. If the given user does not have
// admin access to the model then an error with the code CodeUnauthorized
// is returned. If there is an error connecting to the controller hosting
// the model then the returned error will have the same code as the error
// returned from the dial operation. If the given function returns an error
// that error will be returned with the code unmasked.
func (j *JIMM) doModelAdmin(ctx context.Context, user *openfga.User, mt names.ModelTag, f func(*dbmodel.Model, API) error) error {
	return j.doModel(ctx, user, mt, "admin", f)
}

// GetUserModelAccess returns the access level a user has against a specific model.
func (j *JIMM) GetUserModelAccess(ctx context.Context, user *openfga.User, model names.ModelTag) (string, error) {
	accessLevel := user.GetModelAccess(ctx, model)
	return ToModelAccessString(accessLevel), nil
}

func (j *JIMM) doModel(ctx context.Context, user *openfga.User, mt names.ModelTag, access string, f func(*dbmodel.Model, API) error) error {
	const op = errors.Op("jimm.doModel")

	var m dbmodel.Model
	m.SetTag(mt)

	if err := j.Database.GetModel(ctx, &m); err != nil {
		return errors.E(op, err)
	}

	accessLevel, err := j.GetUserModelAccess(ctx, user, mt)
	if err != nil {
		return errors.E(op, err)
	}
	if !allowedModelAccess[access][accessLevel] {
		// If the user doesn't have correct access on the model return
		// an unauthorized error.
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	api, err := j.dial(ctx, &m.Controller, names.ModelTag{})
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()
	if err := f(&m, api); err != nil {
		return errors.E(op, err)
	}
	return nil
}

var allowedModelAccess = map[string]map[string]bool{
	"admin": {
		"admin": true,
	},
	"write": {
		"admin": true,
		"write": true,
	},
	"read": {
		"admin": true,
		"write": true,
		"read":  true,
	},
}

// ChangeModelCredential changes the credential used with a model on both
// the controller and the local database.
func (j *JIMM) ChangeModelCredential(ctx context.Context, user *openfga.User, modelTag names.ModelTag, cloudCredentialTag names.CloudCredentialTag) error {
	const op = errors.Op("jimm.ChangeModelCredential")

	if !user.JimmAdmin && user.Tag() != cloudCredentialTag.Owner() {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	credential := dbmodel.CloudCredential{}
	credential.SetTag(cloudCredentialTag)

	err := j.Database.GetCloudCredential(ctx, &credential)
	if err != nil {
		return errors.E(op, err)
	}

	var m *dbmodel.Model
	err = j.doModelAdmin(ctx, user, modelTag, func(model *dbmodel.Model, api API) error {
		_, err = j.updateControllerCloudCredential(ctx, &credential, api.UpdateCredential)
		if err != nil {
			return errors.E(op, err)
		}

		err = api.ChangeModelCredential(ctx, modelTag, cloudCredentialTag)
		if err != nil {
			return errors.E(op, err)
		}
		m = model
		return nil
	})
	if err != nil {
		return errors.E(op, err)
	}

	m.CloudCredential = credential
	m.CloudCredentialID = credential.ID
	err = j.Database.UpdateModel(ctx, m)
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}
