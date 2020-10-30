// Copyright 2020 Canonical Ltd.

package jem

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/names/v4"
	"github.com/juju/version"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jem/jimmdb"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
)

// ValidateModelUpgrade validates if a model is allowed to perform an upgrade.
func (j *JEM) ValidateModelUpgrade(ctx context.Context, id identchecker.ACLIdentity, modelUUID string, force bool) error {
	model := mongodoc.Model{UUID: modelUUID}
	if err := j.GetModel(ctx, id, jujuparams.ModelAdminAccess, &model); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}

	conn, err := j.OpenAPI(ctx, model.Controller)
	if err != nil {
		return errgo.Notef(err, "cannot connect to controller")
	}
	defer conn.Close()

	return errgo.Mask(conn.ValidateModelUpgrade(ctx, names.NewModelTag(model.UUID), force), apiconn.IsAPIError)
}

// DestroyModel destroys the specified model. The model will have its
// Life set to dying, but won't be removed until it is removed from the
// controller.
func (j *JEM) DestroyModel(ctx context.Context, id identchecker.ACLIdentity, model *mongodoc.Model, destroyStorage *bool, force *bool, maxWait *time.Duration) error {
	if err := j.GetModel(ctx, id, jujuparams.ModelAdminAccess, model); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	conn, err := j.OpenAPI(ctx, model.Controller)
	if err != nil {
		return errgo.Mask(err)
	}
	if err := conn.DestroyModel(ctx, model.UUID, destroyStorage, force, maxWait); err != nil {
		return errgo.Mask(err, apiconn.IsAPIError)
	}
	if err := j.SetModelLife(ctx, model.Controller, model.UUID, "dying"); err != nil {
		// If this update fails then don't worry as the watcher
		// will detect the state change and update as appropriate.
		zapctx.Warn(ctx, "error updating model life", zap.Error(err), zap.String("model", model.UUID))
	}
	j.DB.AppendAudit(ctx, id, &params.AuditModelDestroyed{
		ID:   model.Id,
		UUID: model.UUID,
	})
	return nil
}

// SetModelDefaults writes new default model setting values for the specified cloud/region.
func (j *JEM) SetModelDefaults(ctx context.Context, id identchecker.ACLIdentity, cloud, region string, configs map[string]interface{}) error {
	return errgo.Mask(j.DB.UpsertModelDefaultConfig(ctx, &mongodoc.CloudRegionDefaults{
		User:     id.Id(),
		Cloud:    cloud,
		Region:   region,
		Defaults: configs,
	}))
}

// UnsetModelDefaults resets  default model setting values for the specified cloud/region.
func (j *JEM) UnsetModelDefaults(ctx context.Context, id identchecker.ACLIdentity, cloud, region string, keys []string) error {
	u := new(jimmdb.Update)
	for _, k := range keys {
		u.Unset("defaults." + k)
	}
	d := mongodoc.CloudRegionDefaults{
		User:   id.Id(),
		Cloud:  cloud,
		Region: region,
	}

	return errgo.Mask(j.DB.UpdateModelDefaultConfig(ctx, &d, u, true))
}

// ModelDefaultsForCloud returns the default config values for the specified cloud.
func (j *JEM) ModelDefaultsForCloud(ctx context.Context, id identchecker.ACLIdentity, cloud params.Cloud) (jujuparams.ModelDefaultsResult, error) {
	result := jujuparams.ModelDefaultsResult{
		Config: make(map[string]jujuparams.ModelDefaults),
	}
	q := jimmdb.And(jimmdb.Eq("user", id.Id()), jimmdb.Eq("cloud", cloud))
	err := j.DB.ForEachModelDefaultConfig(ctx, q, []string{"region"}, func(config *mongodoc.CloudRegionDefaults) error {
		zapctx.Debug(ctx, "XXX###XXX", zap.Any("config", config))
		for k, v := range config.Defaults {
			d := result.Config[k]
			if config.Region == "" {
				d.Default = v
			} else {
				d.Regions = append(d.Regions, jujuparams.RegionDefaults{
					RegionName: config.Region,
					Value:      v,
				})
			}
			result.Config[k] = d
		}
		return nil
	})
	if err != nil {
		return jujuparams.ModelDefaultsResult{}, errgo.Mask(err)
	}
	return result, nil
}

// CreateModelParams specifies the parameters needed to create a new
// model using CreateModel.
type CreateModelParams struct {
	// Path contains the path of the new model.
	Path params.EntityPath

	// ControllerPath contains the path of the owning
	// controller.
	ControllerPath params.EntityPath

	// Credential contains the name of the credential to use to
	// create the model.
	Credential params.CredentialPath

	// Cloud contains the name of the cloud in which the
	// model will be created.
	Cloud params.Cloud

	// Region contains the name of the region in which the model will
	// be created. This may be empty if the cloud does not support
	// regions.
	Region string

	// Attributes contains the attributes to assign to the new model.
	Attributes map[string]interface{}
}

// CreateModel creates a new model as specified by p.
func (j *JEM) CreateModel(ctx context.Context, id identchecker.ACLIdentity, p CreateModelParams, info *jujuparams.ModelInfo) (err error) {
	// Only the owner can create a new model in their namespace.
	if err := auth.CheckIsUser(ctx, id, p.Path.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}

	var usageSenderCredentials []byte
	if j.pool.config.UsageSenderAuthorizationClient != nil {
		usageSenderCredentials, err = j.pool.config.UsageSenderAuthorizationClient.GetCredentials(
			ctx,
			string(p.Path.User),
		)
		if err != nil {
			zapctx.Warn(ctx, "failed to obtain credentials for model", zaputil.Error(err), zap.String("user", string(p.Path.User)))
		}
	}

	cred, err := j.selectCredential(ctx, id, p.Credential, p.Path.User, p.Cloud)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrAmbiguousChoice))
	}

	controllers, err := j.possibleControllers(
		ctx,
		id,
		p.ControllerPath,
		&mongodoc.CloudRegion{
			Cloud:  p.Cloud,
			Region: p.Region,
		},
	)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}

	var credPath mongodoc.CredentialPath
	if cred != nil {
		credPath = cred.Path
	}
	// Create the model record in the database before actually
	// creating the model on the controller. It will have an invalid
	// UUID because it doesn't exist but that's better than creating
	// a model that we can't add locally because the name
	// already exists.
	modelDoc := &mongodoc.Model{
		Path:                   p.Path,
		CreationTime:           wallClock.Now(),
		Creator:                id.Id(),
		UsageSenderCredentials: usageSenderCredentials,
		Credential:             credPath,
		// Use a temporary UUID so that we can create two at the
		// same time, because the uuid field must always be
		// unique.
		UUID: fmt.Sprintf("creating-%x", j.pool.uuidGenerator.Next()),
	}

	if err := j.DB.InsertModel(ctx, modelDoc); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrAlreadyExists))
	}

	defer func() {
		if err == nil {
			return
		}

		// We're returning an error, so remove the model from the
		// database. Note that this might leave the model around
		// in the controller, but this should be rare and we can
		// deal with it at model creation time later (see TODO below).
		if err := j.DB.RemoveModel(ctx, modelDoc); err != nil {
			zapctx.Error(ctx, "cannot remove model from database after error; leaked model", zaputil.Error(err))
		}
	}()

	if info == nil {
		info = new(jujuparams.ModelInfo)
	}
	cmp := createModelParams{
		CreateModelParams: p,
		cred:              cred,
	}
	var ctlPath params.EntityPath
	for _, controller := range controllers {
		ctx = zapctx.WithFields(ctx, zap.Stringer("controller", controller))
		cmp.controller = &mongodoc.Controller{Path: controller}
		if err = j.DB.GetController(ctx, cmp.controller); err != nil {
			zapctx.Error(ctx, "cannot get controller", zap.Error(err))
			continue
		}
		if cmp.controller.Deprecated {
			zapctx.Warn(ctx, "controller deprecated")
			continue
		}
		if !cmp.controller.Public {
			if err := auth.CheckCanRead(ctx, id, cmp.controller); err != nil {
				zapctx.Warn(ctx, "not authorized for controller")
				continue
			}
		}
		err := j.createModel(ctx, cmp, info)
		if err == nil {
			ctlPath = controller
			break
		}
		if errgo.Cause(err) == errInvalidModelParams {
			return errgo.Notef(err, "cannot create model")
		}
		zapctx.Error(ctx, "cannot create model on controller", zaputil.Error(err))
	}

	if ctlPath.Name == "" {
		return errgo.New("cannot find suitable controller")
	}

	// Now set the UUID to that of the actually created model,
	// and update other attributes from the response too.
	// Use Apply so that we can return a result that's consistent
	// with Database.Model.
	update := new(jimmdb.Update)
	update.Set("uuid", info.UUID)
	update.Set("controller", ctlPath)
	update.Set("controlleruuid", info.ControllerUUID)
	ct, err := names.ParseCloudTag(info.CloudTag)
	if err != nil {
		zapctx.Error(ctx, "bad data returned from controller", zap.Error(err))
	} else {
		update.Set("cloud", ct.Id())
	}
	update.Set("cloudregion", info.CloudRegion)
	update.Set("defaultseries", info.DefaultSeries)

	cfg := make(map[string]interface{}, len(p.Attributes)+1)
	for k, v := range p.Attributes {
		cfg[k] = v
	}
	if info.AgentVersion != nil {
		cfg[config.AgentVersionKey] = info.AgentVersion.String()
	}
	var since time.Time
	if info.Status.Since != nil {
		since = *info.Status.Since
	}
	update.Set("info", mongodoc.ModelInfo{
		Life:   string(info.Life),
		Config: cfg,
		Status: mongodoc.ModelStatus{
			Status:  string(info.Status.Status),
			Message: info.Status.Info,
			Data:    info.Status.Data,
			Since:   since,
		},
	})
	update.Set("type", info.Type)
	update.Set("providertype", info.ProviderType)
	if err := j.DB.UpdateModel(ctx, modelDoc, update, true); err != nil {
		return errgo.Notef(err, "cannot update model %s in database", modelDoc.UUID)
	}
	j.DB.AppendAudit(ctx, id, &params.AuditModelCreated{
		ID:             modelDoc.Id,
		UUID:           modelDoc.UUID,
		Owner:          string(modelDoc.Owner()),
		Creator:        modelDoc.Creator,
		ControllerPath: ctlPath.String(),
		Cloud:          string(modelDoc.Cloud),
		Region:         modelDoc.CloudRegion,
	})
	return nil
}

const errInvalidModelParams params.ErrorCode = "invalid CreateModel request"

// A createModelParams value is an internal version of CreateModelParams
// containing additional values.
type createModelParams struct {
	CreateModelParams

	controller *mongodoc.Controller
	cred       *mongodoc.Credential
}

func (j *JEM) createModel(ctx context.Context, p createModelParams, info *jujuparams.ModelInfo) error {
	conn, err := j.OpenAPIFromDoc(ctx, p.controller)
	if err != nil {
		return errgo.Notef(err, "cannot connect to controller")
	}
	defer conn.Close()

	var cloudCredentialTag string
	if p.cred != nil {

		if _, err := j.updateControllerCredential(ctx, conn, p.controller.Path, p.cred); err != nil {
			return errgo.WithCausef(err, errInvalidModelParams, "cannot add credential")
		}
		if err := j.credentialAddController(ctx, p.cred, p.controller.Path); err != nil {
			return errgo.WithCausef(err, errInvalidModelParams, "cannot add credential")
		}
		cloudCredentialTag = conv.ToCloudCredentialTag(p.cred.Path.ToParams()).String()
	}

	args := jujuparams.ModelCreateArgs{
		Name:               string(p.Path.Name),
		OwnerTag:           conv.ToUserTag(p.Path.User).String(),
		Config:             p.Attributes,
		CloudRegion:        p.Region,
		CloudCredentialTag: cloudCredentialTag,
	}
	if p.Cloud != "" {
		args.CloudTag = conv.ToCloudTag(p.Cloud).String()
	}

	if err := conn.CreateModel(ctx, &args, info); err != nil {
		switch jujuparams.ErrCode(err) {
		case jujuparams.CodeAlreadyExists:
			// The model already exists in the controller but it didn't
			// exist in the database. This probably means that it's
			// been abortively created previously, but left around because
			// of connection failure.
			// TODO initiate cleanup of the model, first checking that
			// it's empty, but return an error to the user because
			// the operation to delete a model isn't synchronous even
			// for empty models. We could also have a worker that deletes
			// empty models that don't appear in the database.
			return errgo.WithCausef(err, errInvalidModelParams, "model name in use")
		case jujuparams.CodeUpgradeInProgress:
			return errgo.Notef(err, "upgrade in progress")
		default:
			// The model couldn't be created because of an
			// error in the request, don't try another
			// controller.
			return errgo.WithCausef(err, errInvalidModelParams, "")
		}
	}
	// TODO should we try to delete the model from the controller
	// on error here?

	// Grant JIMM admin access to the model. Note that if this fails,
	// the local database entry will be deleted but the model
	// will remain on the controller and will trigger the "already exists
	// in the backend controller" message above when the user
	// attempts to create a model with the same name again.
	if err := conn.GrantJIMMModelAdmin(ctx, info.UUID); err != nil {
		// TODO (mhilton) ensure that this is flagged in some admin interface somewhere.
		zapctx.Error(ctx, "leaked model", zap.Stringer("model", p.Path), zaputil.Error(err), zap.String("model-uuid", info.UUID))
		return errgo.Notef(err, "cannot grant model access")
	}

	return nil
}

// selectCredential chooses a credential appropriate for the given user that can
// be used when starting a model in the given cloud.
//
// If there's more than one such credential, it returns a params.ErrAmbiguousChoice error.
//
// If there are no credentials found, a zero credential path is returned.
func (j *JEM) selectCredential(ctx context.Context, id identchecker.ACLIdentity, path params.CredentialPath, user params.User, cloud params.Cloud) (*mongodoc.Credential, error) {
	if !path.IsZero() {
		cred := mongodoc.Credential{Path: mongodoc.CredentialPathFromParams(path)}
		if err := j.GetCredential(ctx, id, &cred); err != nil {
			return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized), errgo.Is(params.ErrNotFound))
		}
		if cred.Revoked {
			return nil, errgo.Newf("credential %v has been revoked", path)
		}
		return &cred, nil
	}
	var cred *mongodoc.Credential
	err := j.ForEachCredential(ctx, id, user, cloud, func(c *mongodoc.Credential) error {
		if cred != nil {
			return errgo.WithCausef(nil, params.ErrAmbiguousChoice, "more than one possible credential to use")
		}
		cred = c
		return nil
	})
	return cred, errgo.Mask(err, errgo.Is(params.ErrAmbiguousChoice))
}

// GetModelInfo completes the given ModelInfo, which must have a non-zero
// UUID. If the queryController parameter is true then ModelInfo will be
// retrieved from the controller, otherwise only information available from
// the  local database will be returned. If the model cannot be found then
// an  error with a cause of params.ErrNotFound will be returned, if the
// given user does not have read access to the model then an error with a
// cause of params.ErrUnauthorized will be returned.
func (j *JEM) GetModelInfo(ctx context.Context, id identchecker.ACLIdentity, info *jujuparams.ModelInfo, queryController bool) error {
	if info == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "")
	}
	m := mongodoc.Model{
		UUID: info.UUID,
	}
	if err := j.GetModel(ctx, id, jujuparams.ModelReadAccess, &m); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	if queryController {
		ctx := zapctx.WithFields(ctx, zap.Stringer("controller", m.Controller))
		conn, err := j.OpenAPI(ctx, m.Controller)
		if err == nil {
			err = conn.ModelInfo(ctx, info)
			if jujuparams.IsCodeUnauthorized(err) && m.Life() == "dying" {
				zapctx.Info(ctx, "could not get ModelInfo for dying model, marking dead", zap.Error(err))
				// The model was dying and now cannot be accessed, assume it is now dead.
				if err := j.DB.RemoveModel(ctx, &m); err != nil {
					// If this update fails then don't worry as the watcher
					// will detect the state change and update as appropriate.
					zapctx.Warn(ctx, "error deleting model", zap.Error(err))
				}
				// return the error with the an appropriate cause.
				return errgo.Mask(err, apiconn.IsAPIError)
			}
		}
		if err != nil {
			zapctx.Error(ctx, "cannot get model info from controller", zap.Error(err))
		}
	}

	if info.Name == "" {
		// We couldn't populate the ModelInfo from the controller, so use
		// the local database.
		info.Name = string(m.Path.Name)
		info.Type = m.Type
		info.ControllerUUID = m.ControllerUUID
		info.IsController = false
		info.ProviderType = m.ProviderType
		info.DefaultSeries = m.DefaultSeries
		info.CloudTag = conv.ToCloudTag(m.Cloud).String()
		info.CloudRegion = m.CloudRegion
		info.CloudCredentialTag = conv.ToCloudCredentialTag(m.Credential.ToParams()).String()
		info.CloudCredentialValidity = nil
		info.OwnerTag = conv.ToUserTag(m.Path.User).String()
		info.Life = life.Value(m.Life())
		info.Status = modelStatus(m.Info)

		// Add all the possible users in priority order, they will be
		// filtered later.
		info.Users = []jujuparams.ModelUserInfo{userInfo(m.Path.User, jujuparams.ModelAdminAccess)}
		for _, u := range m.ACL.Admin {
			info.Users = append(info.Users, userInfo(params.User(u), jujuparams.ModelAdminAccess))
		}
		for _, u := range m.ACL.Write {
			info.Users = append(info.Users, userInfo(params.User(u), jujuparams.ModelWriteAccess))
		}
		for _, u := range m.ACL.Read {
			info.Users = append(info.Users, userInfo(params.User(u), jujuparams.ModelReadAccess))
		}

		var err error
		info.Machines, err = j.modelMachineInfo(ctx, info.UUID)
		if err != nil {
			zapctx.Error(ctx, "cannot get machine information", zap.Error(err))
		}

		info.Migration = nil
		// TODO(mhilton) we should store SLA information
		info.SLA = nil
		info.AgentVersion = modelVersion(ctx, m.Info)
	}

	var canSeeUsers, canSeeMachines bool
	adminACL := append(m.ACL.Admin, string(m.Path.User), string(j.ControllerAdmin()))
	if err := auth.CheckACL(ctx, id, adminACL); err == nil {
		canSeeUsers = true
		canSeeMachines = true
	} else if err := auth.CheckACL(ctx, id, m.ACL.Write); err == nil {
		canSeeMachines = true
	}

	// Filter users to remove local users (from controller) and duplicates
	// (from database).
	var users []jujuparams.ModelUserInfo
	seen := make(map[string]bool)
	for _, u := range info.Users {
		if seen[u.UserName] {
			continue
		}
		seen[u.UserName] = true

		ut := names.NewUserTag(u.UserName)
		uid, err := conv.FromUserTag(ut)
		if err != nil {
			// This will be an error if the user is a controller-local
			// user which does not make sense in a JAAS environement.
			continue
		}
		if !canSeeUsers {
			if err := auth.CheckIsUser(ctx, id, uid); err != nil {
				continue
			}
		}
		// The authenticated user is allowed to know about this user.
		users = append(users, u)
	}
	info.Users = users

	sort.Slice(info.Users, func(i, j int) bool { return info.Users[i].UserName < info.Users[j].UserName })

	if !canSeeMachines {
		info.Machines = nil
	}
	sort.Slice(info.Machines, func(i, j int) bool { return info.Machines[i].Id < info.Machines[j].Id })

	return nil
}

func (j *JEM) modelMachineInfo(ctx context.Context, uuid string) ([]jujuparams.ModelMachineInfo, error) {
	var mmis []jujuparams.ModelMachineInfo
	err := j.DB.ForEachMachine(ctx, jimmdb.Eq("info.modeluuid", uuid), []string{"_id"}, func(m *mongodoc.Machine) error {
		if m.Info == nil || m.Info.Life == "dead" {
			return nil
		}
		mmi := jujuparams.ModelMachineInfo{
			Id:         m.Info.Id,
			InstanceId: m.Info.InstanceId,
			Status:     string(m.Info.AgentStatus.Current),
			HasVote:    m.Info.HasVote,
			WantsVote:  m.Info.WantsVote,
		}
		if m.Info.HardwareCharacteristics != nil {
			mmi.Hardware = &jujuparams.MachineHardware{
				Arch:             m.Info.HardwareCharacteristics.Arch,
				Mem:              m.Info.HardwareCharacteristics.Mem,
				RootDisk:         m.Info.HardwareCharacteristics.RootDisk,
				Cores:            m.Info.HardwareCharacteristics.CpuCores,
				CpuPower:         m.Info.HardwareCharacteristics.CpuPower,
				Tags:             m.Info.HardwareCharacteristics.Tags,
				AvailabilityZone: m.Info.HardwareCharacteristics.AvailabilityZone,
			}
		}
		mmis = append(mmis, mmi)
		return nil
	})
	return mmis, errgo.Mask(err)
}

func userInfo(u params.User, access jujuparams.UserAccessPermission) jujuparams.ModelUserInfo {
	t := conv.ToUserTag(u)
	return jujuparams.ModelUserInfo{
		UserName:    t.Id(),
		DisplayName: t.Name(),
		Access:      access,
	}
}

func modelStatus(info *mongodoc.ModelInfo) jujuparams.EntityStatus {
	var st jujuparams.EntityStatus
	if info == nil {
		return st
	}
	st.Status = status.Status(info.Status.Status)
	st.Info = info.Status.Message
	st.Data = info.Status.Data
	if !info.Status.Since.IsZero() {
		st.Since = &info.Status.Since
	}
	return st
}

func modelVersion(ctx context.Context, info *mongodoc.ModelInfo) *version.Number {
	if info == nil {
		return nil
	}
	versionString, _ := info.Config[config.AgentVersionKey].(string)
	if versionString == "" {
		return nil
	}
	v, err := version.Parse(versionString)
	if err != nil {
		zapctx.Warn(ctx, "cannot parse agent-version", zap.String("agent-version", versionString), zap.Error(err))
		return nil
	}
	return &v
}

// GetModelStatus writes the status of the model with the given uuid into
// the given ModelStatus. If queryController is true then the status will
// be read from the controller, otherwise it will be created from data in
// the local database. If a model with the given UUID cannot be found then
// an error with a cause of params.ErrNotFound will be returned. If the
// given identity does not have admin level access to the model then an
// error with a cause of params.ErrUnauthorized will be returned.
func (j *JEM) GetModelStatus(ctx context.Context, id identchecker.ACLIdentity, uuid string, status *jujuparams.ModelStatus, queryController bool) error {
	m := mongodoc.Model{
		UUID: uuid,
	}
	if err := j.GetModel(ctx, id, jujuparams.ModelAdminAccess, &m); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}

	status.ModelTag = names.NewModelTag(uuid).String()

	if queryController {
		ctx := zapctx.WithFields(ctx, zap.Stringer("controller", m.Controller))
		conn, err := j.OpenAPI(ctx, m.Controller)
		if err == nil {
			err = conn.ModelStatus(ctx, status)
			if jujuparams.IsCodeNotFound(err) && m.Life() == "dying" {
				zapctx.Info(ctx, "could not get ModelStatus for dying model, marking dead", zap.Error(err))
				// The model was dying and now cannot be accessed, assume it is now dead.
				if err := j.DB.RemoveModel(ctx, &m); err != nil {
					// If this update fails then don't worry as the watcher
					// will detect the state change and update as appropriate.
					zapctx.Warn(ctx, "error deleting model", zap.Error(err))
				}
				// return the error with the an appropriate cause.
				return errgo.Mask(err, apiconn.IsAPIError)
			}
		}
		if err != nil {
			zapctx.Error(ctx, "cannot get model status from controller", zap.Error(err))
		}
	}

	if status.OwnerTag != "" {
		// We got a response from the controller.
		return nil
	}
	// Fill out the ModelStatus from the Model as best we can.
	status.Life = life.Value(m.Life())
	status.Type = m.Type
	status.OwnerTag = conv.ToUserTag(m.Path.User).String()
	status.HostedMachineCount = m.Counts[params.MachineCount].Current
	status.ApplicationCount = m.Counts[params.ApplicationCount].Current
	status.UnitCount = m.Counts[params.UnitCount].Current
	var err error
	status.Machines, err = j.modelMachineInfo(ctx, uuid)
	if err != nil {
		zapctx.Error(ctx, "cannot get machine information", zap.Error(err))
	}
	// TODO(mhilton) store and populate Volume and FileSystem information.
	return nil
}

// GrantModel grants the given access for the given user on the given model
// and updates the JEM database.
func (j *JEM) GrantModel(ctx context.Context, id identchecker.ACLIdentity, m *mongodoc.Model, user params.User, access jujuparams.UserAccessPermission) error {
	if err := j.GetModel(ctx, id, jujuparams.ModelAdminAccess, m); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	conn, err := j.OpenAPI(ctx, m.Controller)
	if err != nil {
		return errgo.Mask(err)
	}
	defer conn.Close()
	if err := conn.GrantModelAccess(ctx, m.UUID, user, access); err != nil {
		return errgo.Mask(err, apiconn.IsAPIError)
	}
	u := new(jimmdb.Update)
	switch access {
	case jujuparams.ModelAdminAccess:
		u.AddToSet("acl.admin", user)
		fallthrough
	case jujuparams.ModelWriteAccess:
		u.AddToSet("acl.write", user)
		fallthrough
	case jujuparams.ModelReadAccess:
		u.AddToSet("acl.read", user)
	}
	return errgo.Mask(j.DB.UpdateModel(ctx, m, u, false))
}

// RevokeModel revokes the given access for the given user on the given
// model and updates the JEM database.
func (j *JEM) RevokeModel(ctx context.Context, id identchecker.ACLIdentity, m *mongodoc.Model, user params.User, access jujuparams.UserAccessPermission) error {
	if err := j.GetModel(ctx, id, jujuparams.ModelAdminAccess, m); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	u := new(jimmdb.Update)
	switch access {
	case jujuparams.ModelReadAccess:
		u.Pull("acl.read", user)
		fallthrough
	case jujuparams.ModelWriteAccess:
		u.Pull("acl.write", user)
		fallthrough
	case jujuparams.ModelAdminAccess:
		u.Pull("acl.admin", user)
	}
	if err := j.DB.UpdateModel(ctx, m, u, false); err != nil {
		return errgo.Mask(err)
	}

	conn, err := j.OpenAPI(ctx, m.Controller)
	if err != nil {
		return errgo.Mask(err)
	}
	defer conn.Close()
	if err := conn.RevokeModelAccess(ctx, m.UUID, user, access); err != nil {
		// TODO (mhilton) What should be done with the changes already made to the database.
		return errgo.Mask(err, apiconn.IsAPIError)
	}
	return nil
}

// GetModelStatuses retrieves the model status from all models. If the
// given user is not a controller admin then an error with a cause of
// params.ErrUnauthorized will be retuned.
func (j *JEM) GetModelStatuses(ctx context.Context, id identchecker.ACLIdentity) (params.ModelStatuses, error) {
	if err := auth.CheckIsUser(ctx, id, j.ControllerAdmin()); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	var mss params.ModelStatuses
	err := j.DB.ForEachModel(ctx, nil, []string{"-creationtime"}, func(m *mongodoc.Model) error {
		status := "unknown"
		if m.Info != nil {
			status = m.Info.Status.Status
		}
		mss = append(mss, params.ModelStatus{
			ID:         m.Id,
			UUID:       m.UUID,
			Cloud:      string(m.Cloud),
			Region:     m.CloudRegion,
			Created:    m.CreationTime,
			Controller: m.Controller.String(),
			Status:     status,
		})
		return nil
	})
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return mss, nil
}

// UpdateModelCredential updates the credential used with a model on both
// the controller and the local database.
func (j *JEM) UpdateModelCredential(ctx context.Context, conn *apiconn.Conn, model *mongodoc.Model, cred *mongodoc.Credential) error {
	if _, err := j.updateControllerCredential(ctx, conn, model.Controller, cred); err != nil {
		return errgo.Notef(err, "cannot add credential")
	}
	if err := j.credentialAddController(ctx, cred, model.Controller); err != nil {
		return errgo.Notef(err, "cannot add credential")
	}

	client := modelmanager.NewClient(conn)
	if err := client.ChangeModelCredential(names.NewModelTag(model.UUID), conv.ToCloudCredentialTag(cred.Path.ToParams())); err != nil {
		return errgo.Mask(err)
	}

	if err := j.DB.UpdateModel(ctx, model, new(jimmdb.Update).Set("credential", cred.Path), true); err != nil {
		return errgo.Mask(err)
	}
	return nil
}
