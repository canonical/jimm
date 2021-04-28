// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
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

// FromJujuModelCreateArgs convers jujuparams.ModelCreateArgs into AddModelArgs.
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

type modelBuilder struct {
	ctx context.Context
	err error

	jimm *JIMM

	name          string
	config        map[string]interface{}
	user          *dbmodel.User
	owner         *dbmodel.User
	credential    *dbmodel.CloudCredential
	controller    *dbmodel.Controller
	cloud         *dbmodel.Cloud
	cloudRegion   string
	cloudRegionID uint
	model         *dbmodel.Model
	modelInfo     *jujuparams.ModelInfo
}

// Error returns the error that occured in the process
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
	if b.cloud != nil {
		// loop through all cloud regions
		for _, r := range b.cloud.Regions {
			// if the region matches
			if r.Name == region {
				// consider all possible controlers for that region
				regionControllers := r.Controllers
				if len(regionControllers) == 0 {
					b.err = errors.E(errors.CodeBadRequest, fmt.Sprintf("unsupported cloud region %s/%s", b.cloud.Name, region))
				}
				// shuffle controllers
				shuffleRegionControllers(regionControllers)

				// and sellect the first controller in the slice
				b.cloudRegion = region
				b.cloudRegionID = regionControllers[0].CloudRegionID
				b.controller = &regionControllers[0].Controller

				break
			}
		}
		// we looped through all cloud regions and could not find a match
		if b.cloudRegionID == 0 {
			b.err = errors.E("cloudregion not found", errors.CodeNotFound)
		}
	} else {
		b.err = errors.E("cloud not specified")
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
	}

	if b.credential == nil {
		// try to select a valid credential
		if err := b.selectCloudCredentials(); err != nil {
			b.err = errors.E(err, "could not select cloud credentials")
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
	if derr := b.jimm.Database.DeleteModel(b.ctx, b.model); derr != nil {
		zapctx.Error(b.ctx, "failed to delete model", zap.String("model", b.model.Name), zap.String("owner", b.model.Owner.Username), zaputil.Error(derr))
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

	err = b.filterModelUserAccesses()
	if err != nil {
		b.err = errors.E(err)
		return b
	}

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
	if b.user == nil {
		return errors.E("user not specified")
	}
	if b.cloud == nil {
		return errors.E("cloud not specified")
	}
	credentials, err := b.jimm.Database.GetUserCloudCredentials(b.ctx, b.user, b.cloud.Name)
	if err != nil {
		return errors.E(err, "failed to fetch user cloud credentials")
	}
	for _, credential := range credentials {
		// consider only valid credentials
		if credential.Valid.Valid && credential.Valid.Bool == true {
			b.credential = &credential
			return nil
		}
	}
	return errors.E("valid cloud credentials not found")
}

func (b *modelBuilder) filterModelUserAccesses() error {
	a := []dbmodel.UserModelAccess{}
	for _, access := range b.model.Users {
		access := access

		// JIMM users will contain an @ sign in the username
		if !strings.Contains(access.User.Username, "@") {
			continue
		}

		// fetch user information
		if err := b.jimm.Database.GetUser(b.ctx, &access.User); err != nil {
			return errors.E(err)
		}
		a = append(a, access)
	}
	b.model.Users = a
	return nil
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

	api, err := b.jimm.dial(b.ctx, b.controller, names.ModelTag{})
	if err != nil {
		b.err = errors.E(err)
		return b
	}
	defer api.Close()

	if b.credential != nil {
		if err := b.updateCredential(b.ctx, api, b.credential); err != nil {
			b.err = errors.E("failed to update cloud credential", err)
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

// AddModel adds the specified model to JIMM.
func (j *JIMM) AddModel(ctx context.Context, u *dbmodel.User, args *ModelCreateArgs) (_ *jujuparams.ModelInfo, err error) {
	const op = errors.Op("jimm.AddModel")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		UserTag: u.Tag().String(),
		Action:  "create",
		Params: dbmodel.StringMap{
			"name":  args.Name,
			"owner": args.Owner.String(),
		},
	}
	defer j.addAuditLogEntry(&ale)

	fail := func(err error) (*jujuparams.ModelInfo, error) {
		ale.Params["err"] = err.Error()
		return nil, err
	}
	owner := dbmodel.User{
		Username: args.Owner.Id(),
	}
	err = j.Database.GetUser(ctx, &owner)
	if err != nil {
		return fail(errors.E(op, err))
	}

	if owner.Username != u.Username && u.ControllerAccess != "superuser" {
		return fail(errors.E(op, errors.CodeUnauthorized, "unauthorized"))
	}

	builder := newModelBuilder(ctx, j)
	builder = builder.WithOwner(&owner)
	builder = builder.WithName(args.Name)
	if err := builder.Error(); err != nil {
		return fail(errors.E(op, err))
	}

	// fetch user model defaults
	userConfig, err := j.UserModelDefaults(ctx, u)
	if err != nil && errors.ErrorCode(err) != errors.CodeNotFound {
		return fail(errors.E(op, "failed to fetch cloud defaults"))
	}
	builder = builder.WithConfig(userConfig)

	// fetch cloud defaults
	if args.Cloud != (names.CloudTag{}) {
		cloudDefaults := dbmodel.CloudDefaults{
			Username: u.Username,
			Cloud: dbmodel.Cloud{
				Name: args.Cloud.Id(),
			},
		}
		err = j.Database.CloudDefaults(ctx, &cloudDefaults)
		if err != nil && errors.ErrorCode(err) != errors.CodeNotFound {
			return fail(errors.E(op, "failed to fetch cloud defaults"))
		}
		builder = builder.WithConfig(cloudDefaults.Defaults)
	}

	// fetch cloud region defaults
	if args.Cloud != (names.CloudTag{}) && args.CloudRegion != "" {
		cloudRegionDefaults := dbmodel.CloudDefaults{
			Username: u.Username,
			Cloud: dbmodel.Cloud{
				Name: args.Cloud.Id(),
			},
			Region: args.CloudRegion,
		}
		err = j.Database.CloudDefaults(ctx, &cloudRegionDefaults)
		if err != nil && errors.ErrorCode(err) != errors.CodeNotFound {
			return fail(errors.E(op, "failed to fetch cloud defaults"))
		}
		builder = builder.WithConfig(cloudRegionDefaults.Defaults)
	}

	// last but not least, use the provided config values
	// overriding all defaults
	builder = builder.WithConfig(args.Config)

	if args.Cloud != (names.CloudTag{}) {
		ale.Params["cloud"] = args.Cloud.String()
		builder = builder.WithCloud(args.Cloud)
		if err := builder.Error(); err != nil {
			return fail(errors.E(op, err))
		}
	}
	if args.CloudRegion != "" {
		ale.Params["region"] = args.CloudRegion
		builder = builder.WithCloudRegion(args.CloudRegion)
		if err := builder.Error(); err != nil {
			return fail(errors.E(op, err))
		}
	}
	if args.CloudCredential != (names.CloudCredentialTag{}) {
		ale.Params["cloud-credential"] = args.CloudCredential.String()
		builder = builder.WithCloudCredential(args.CloudCredential)
		if err := builder.Error(); err != nil {
			return fail(errors.E(op, err))
		}
	}
	builder = builder.CreateDatabaseModel()
	if err := builder.Error(); err != nil {
		return fail(errors.E(op, err))
	}
	defer builder.Cleanup()

	builder = builder.CreateControllerModel()
	if err := builder.Error(); err != nil {
		return fail(errors.E(op, err))
	}

	builder = builder.UpdateDatabaseModel()
	if err := builder.Error(); err != nil {
		return fail(errors.E(op, err))
	}

	mi := builder.JujuModelInfo()
	ale.Tag = names.NewModelTag(mi.UUID).String()
	ale.Success = true

	return mi, nil
}

// ModelInfo returns the model info for the model with the given ModelTag.
// The returned ModelInfo will be appropriate for the given user's
// access-level on the model. If the model does not exist then the returned
// error will have the code CodeNotFound. If the given user does not have
// access to the model then the returned error will have the code
// CodeUnauthorized.
func (j *JIMM) ModelInfo(ctx context.Context, u *dbmodel.User, mt names.ModelTag) (*jujuparams.ModelInfo, error) {
	const op = errors.Op("jimm.ModelInfo")

	m := dbmodel.Model{
		UUID: sql.NullString{
			String: mt.Id(),
			Valid:  true,
		},
	}

	if err := j.Database.GetModel(ctx, &m); err != nil {
		return nil, errors.E(op, err)
	}

	mi := m.ToJujuModelInfo()
	var modelUser jujuparams.ModelUserInfo
	for _, user := range mi.Users {
		if user.UserName == u.Username {
			modelUser = user
		}
	}

	if u.ControllerAccess == "superuser" || modelUser.Access == "admin" {
		// Admin users have access to all data unmodified.
		return &mi, nil
	}

	if modelUser.Access == "" {
		// If the user doesn't have any access on the model return an
		// unauthorized error
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	// Non-admin users can only see their own user.
	mi.Users = []jujuparams.ModelUserInfo{modelUser}

	if modelUser.Access != "write" {
		// Users need "write" level access (or above) to see machine
		// information. Note "admin" level users will have already
		// returned data above.
		mi.Machines = nil
	}

	return &mi, nil
}

// ModelStatus returns a jujuparams.ModelStatus for the given model. If
// the model doesn't exist then the returned error will have the code
// CodeNotFound, If the given user does not have admin access to the model
// then the returned error will have the code CodeUnauthorized.
func (j *JIMM) ModelStatus(ctx context.Context, u *dbmodel.User, mt names.ModelTag) (*jujuparams.ModelStatus, error) {
	const op = errors.Op("jimm.ModelStatus")

	var ms jujuparams.ModelStatus
	err := j.doModelAdmin(ctx, u, mt, func(_ *dbmodel.Model, api API) error {
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
func (j *JIMM) ForEachUserModel(ctx context.Context, u *dbmodel.User, f func(*dbmodel.UserModelAccess) error) error {
	const op = errors.Op("jimm.ForEachUserModel")

	models, err := j.Database.GetUserModels(ctx, u)
	if err != nil {
		return errors.E(op, err)
	}

	for _, m := range models {
		switch m.Access {
		default:
			continue
		case "read", "write", "admin":
		}
		if err := f(&m); err != nil {
			return err
		}
	}
	return nil
}

// ForEachModel calls the given function once for each model in the system.
// The UserModelAccess object passed to f will always specify that the
// user's Access is "admin" and will not include the LastConnection time.
// ForEachModel will return an error with the code CodeUnauthorized when
// the user is not a controller admin. If the given function returns an
// error the error will be returned unmodified and iteration will stop
// immediately. The given function should not update the database.
func (j *JIMM) ForEachModel(ctx context.Context, u *dbmodel.User, f func(*dbmodel.UserModelAccess) error) error {
	const op = errors.Op("jimm.ForEachUserModel")

	if u.ControllerAccess != "superuser" {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	errStop := errors.E("stop")
	var iterErr error
	err := j.Database.ForEachModel(ctx, func(m *dbmodel.Model) error {
		uma := dbmodel.UserModelAccess{
			Access: "admin",
			Model_: *m,
		}
		if err := f(&uma); err != nil {
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
// is returned. If the ModifyModelAccess API call retuns an error the
// error code is not masked.
func (j *JIMM) GrantModelAccess(ctx context.Context, u *dbmodel.User, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error {
	const op = errors.Op("jimm.GrantModelAccess")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		Tag:     mt.String(),
		UserTag: u.Tag().String(),
		Action:  "grant",
		Params: dbmodel.StringMap{
			"user":   ut.String(),
			"access": string(access),
		},
	}
	defer j.addAuditLogEntry(&ale)

	err := j.doModelAdmin(ctx, u, mt, func(m *dbmodel.Model, api API) error {
		targetUser := dbmodel.User{
			Username: ut.Id(),
		}
		if err := j.Database.GetUser(ctx, &targetUser); err != nil {
			return err
		}
		if err := api.GrantModelAccess(ctx, mt, ut, access); err != nil {
			return err
		}
		var uma dbmodel.UserModelAccess
		for _, a := range m.Users {
			if a.Username == targetUser.Username {
				uma = a
				break
			}
		}
		uma.User = targetUser
		uma.Model_ = *m
		uma.Access = string(access)

		if err := j.Database.UpdateUserModelAccess(ctx, &uma); err != nil {
			return errors.E(op, err, "cannot update database after updating controller")
		}
		return nil
	})
	if err != nil {
		ale.Params["err"] = err.Error()
		return errors.E(op, err)
	}
	ale.Success = true
	return nil

}

// RevokeModelAccess revokes the given access level on the given model
// from the given user. If the model is not found then an error with the
// code CodeNotFound is returned. If the authenticated user does not
// have admin access to the model then an error with the code
// CodeUnauthorized is returned. If the ModifyModelAccess API call
// retuns an error the error code is not masked.
func (j *JIMM) RevokeModelAccess(ctx context.Context, u *dbmodel.User, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error {
	const op = errors.Op("jimm.RevokeModelAccess")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		Tag:     mt.String(),
		UserTag: u.Tag().String(),
		Action:  "revoke",
		Params: dbmodel.StringMap{
			"user":   ut.String(),
			"access": string(access),
		},
	}
	defer j.addAuditLogEntry(&ale)

	err := j.doModelAdmin(ctx, u, mt, func(m *dbmodel.Model, api API) error {
		targetUser := dbmodel.User{
			Username: ut.Id(),
		}
		if err := j.Database.GetUser(ctx, &targetUser); err != nil {
			return err
		}
		if err := api.RevokeModelAccess(ctx, mt, ut, access); err != nil {
			return err
		}
		var uma dbmodel.UserModelAccess
		for _, a := range m.Users {
			if a.Username == targetUser.Username {
				uma = a
				break
			}
		}
		uma.User = targetUser
		uma.Model_ = *m
		switch access {
		case "admin":
			uma.Access = "write"
		case "write":
			uma.Access = "read"
		default:
			uma.Access = ""
		}

		if err := j.Database.UpdateUserModelAccess(ctx, &uma); err != nil {
			return errors.E(op, err, "cannot update database after updating controller")
		}
		return nil
	})
	if err != nil {
		ale.Params["err"] = err.Error()
		return errors.E(op, err)
	}
	ale.Success = true
	return nil
}

// DestroyModel starts the process of destroying the given model. If the
// given user is not a controller superuser or a model admin an error
// with a code of CodeUnauthorized is returned. Any error returned from
// the juju API will not have it's code masked.
func (j *JIMM) DestroyModel(ctx context.Context, u *dbmodel.User, mt names.ModelTag, destroyStorage, force *bool, maxWait *time.Duration) error {
	const op = errors.Op("jimm.DestroyModel")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		Tag:     mt.String(),
		UserTag: u.Tag().String(),
		Action:  "destroy",
		Params:  dbmodel.StringMap{},
	}
	defer j.addAuditLogEntry(&ale)

	if destroyStorage != nil {
		ale.Params["destroy-storage"] = strconv.FormatBool(*destroyStorage)
	}
	if force != nil {
		ale.Params["force"] = strconv.FormatBool(*force)
	}

	err := j.doModelAdmin(ctx, u, mt, func(m *dbmodel.Model, api API) error {
		if err := api.DestroyModel(ctx, mt, destroyStorage, force, maxWait); err != nil {
			return err
		}
		m.Life = "dying"
		if err := j.Database.UpdateModel(ctx, m); err != nil {
			// If the database fails to update don't worry too much the
			// monitor should catch it.
			zapctx.Error(ctx, "failed to store model change", zaputil.Error(err))
		}

		return nil
	})
	if err != nil {
		ale.Params["err"] = err.Error()
		return errors.E(op, err)
	}
	ale.Success = true
	return nil
}

// DumpModel retrieves a database-agnostic dump of the given model from its
// juju controller. If simplified is true a simpllified dump is requested.
// If the given user is not a controller superuser or a model admin an
// error with the code CodeUnauthorized is returned.
func (j *JIMM) DumpModel(ctx context.Context, u *dbmodel.User, mt names.ModelTag, simplified bool) (string, error) {
	const op = errors.Op("jimm.DumpModel")

	var dump string
	err := j.doModelAdmin(ctx, u, mt, func(m *dbmodel.Model, api API) error {
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
func (j *JIMM) DumpModelDB(ctx context.Context, u *dbmodel.User, mt names.ModelTag) (map[string]interface{}, error) {
	const op = errors.Op("jimm.DumpModelDB")

	var dump map[string]interface{}
	err := j.doModelAdmin(ctx, u, mt, func(m *dbmodel.Model, api API) error {
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
// CodeNotImplemented error code will be propergated back to the client.
func (j *JIMM) ValidateModelUpgrade(ctx context.Context, u *dbmodel.User, mt names.ModelTag, force bool) error {
	const op = errors.Op("jimm.ValidateModelUpgrade")

	err := j.doModelAdmin(ctx, u, mt, func(_ *dbmodel.Model, api API) error {
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
func (j *JIMM) doModelAdmin(ctx context.Context, u *dbmodel.User, mt names.ModelTag, f func(*dbmodel.Model, API) error) error {
	const op = errors.Op("jimm.doModelAdmin")

	var m dbmodel.Model
	m.SetTag(mt)

	if err := j.Database.GetModel(ctx, &m); err != nil {
		return errors.E(op, err)
	}
	if u.ControllerAccess != "superuser" && m.UserAccess(u) != "admin" {
		// If the user doesn't have admin access on the model return
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

// ChangeModelCredential changes the credential used with a model on both
// the controller and the local database.
func (j *JIMM) ChangeModelCredential(ctx context.Context, user *dbmodel.User, modelTag names.ModelTag, cloudCredentialTag names.CloudCredentialTag) error {
	const op = errors.Op("jimm.ChangeModelCredential")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		Tag:     modelTag.String(),
		UserTag: user.Tag().String(),
		Action:  "change-credential",
		Params: dbmodel.StringMap{
			"cloud-credential": cloudCredentialTag.String(),
		},
	}
	defer j.addAuditLogEntry(&ale)

	fail := func(err error) error {
		ale.Params["err"] = err.Error()
		return err
	}

	if user.ControllerAccess != "superuser" && user.Tag() != cloudCredentialTag.Owner() {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	credential := dbmodel.CloudCredential{}
	credential.SetTag(cloudCredentialTag)

	err := j.Database.GetCloudCredential(ctx, &credential)
	if err != nil {
		return fail(errors.E(op, err))
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
		return fail(errors.E(op, err))
	}

	m.CloudCredential = credential
	m.CloudCredentialID = credential.ID
	err = j.Database.UpdateModel(ctx, m)
	if err != nil {
		return fail(errors.E(op, err))
	}

	ale.Success = true
	return nil
}
