// Copyright 2020 Canonical Ltd.

package jem

import (
	"context"
	"fmt"
	"sort"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/names/v4"
	"github.com/juju/version"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
)

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
	if err := j.DB.SetModelLife(ctx, model.Controller, model.UUID, "dying"); err != nil {
		// If this update fails then don't worry as the watcher
		// will detect the state change and update as appropriate.
		zapctx.Warn(ctx, "error updating model life", zap.Error(err), zap.String("model", model.UUID))
	}
	j.DB.AppendAudit(ctx, &params.AuditModelDestroyed{
		ID:   model.Id,
		UUID: model.UUID,
	})
	return nil
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
	if j.usageSenderAuthorizationClient != nil {
		usageSenderCredentials, err = j.usageSenderAuthorizationClient.GetCredentials(
			ctx,
			string(p.Path.User))
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

	if err := j.DB.AddModel(ctx, modelDoc); err != nil {
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
		if err := j.DB.DeleteModel(ctx, modelDoc.Path); err != nil {
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

		cmp.controller, err = j.DB.Controller(ctx, controller)
		if err != nil {
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
	var since time.Time
	if info.Status.Since != nil {
		since = *info.Status.Since
	}
	cfg := make(map[string]interface{}, len(p.Attributes)+1)
	for k, v := range p.Attributes {
		cfg[k] = v
	}
	if info.AgentVersion != nil {
		cfg[config.AgentVersionKey] = info.AgentVersion.String()
	}
	ct, err := names.ParseCloudTag(info.CloudTag)
	if err != nil {
		zapctx.Error(ctx, "bad data returned from controller", zap.Error(err))
	}
	if _, err := j.DB.Models().FindId(modelDoc.Id).Apply(mgo.Change{
		Update: bson.D{{"$set", bson.D{
			{"uuid", info.UUID},
			{"controller", ctlPath},
			{"cloud", ct.Id()},
			{"cloudregion", info.CloudRegion},
			{"defaultseries", info.DefaultSeries},
			{"info", mongodoc.ModelInfo{
				Life:   string(info.Life),
				Config: cfg,
				Status: mongodoc.ModelStatus{
					Status:  string(info.Status.Status),
					Message: info.Status.Info,
					Data:    info.Status.Data,
					Since:   since,
				},
			}},
			{"type", info.Type},
			{"providertype", info.ProviderType},
		}}},
		ReturnNew: true,
	}, &modelDoc); err != nil {
		j.DB.checkError(ctx, &err)
		return errgo.Notef(err, "cannot update model %s in database", modelDoc.UUID)
	}
	j.DB.AppendAudit(ctx, &params.AuditModelCreated{
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
		if err := j.DB.credentialAddController(ctx, p.cred.Path, p.controller.Path); err != nil {
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
	p := mongodoc.CredentialPathFromParams(path)
	query := bson.D{{"path", p}}
	if p.IsZero() {
		query = bson.D{
			{"path.entitypath.user", user},
			{"path.cloud", cloud},
			{"revoked", false},
		}
	}
	var creds []mongodoc.Credential
	iter := j.DB.NewCanReadIter(auth.ContextWithIdentity(ctx, id), j.DB.Credentials().Find(query).Iter())
	var cred mongodoc.Credential
	for iter.Next(ctx, &cred) {
		creds = append(creds, cred)
	}
	if err := iter.Err(ctx); err != nil {
		return nil, errgo.Notef(err, "cannot query credentials")
	}
	switch len(creds) {
	case 0:
		var err error
		if !p.IsZero() {
			err = errgo.WithCausef(nil, params.ErrNotFound, "credential %q not found", path)
		}
		return nil, err
	case 1:
		cred := &creds[0]
		if cred.Revoked {
			// The credential (which must have been specifically selected by
			// path, because if the path wasn't set, we will never select
			// a revoked credential) has been revoked - we can't use it.
			return nil, errgo.Newf("credential %v has been revoked", creds[0].Path)
		}
		return cred, nil
	default:
		return nil, errgo.WithCausef(nil, params.ErrAmbiguousChoice, "more than one possible credential to use")
	}
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
		return errgo.WithCausef(nil, params.ErrUnauthorized, "")
	}
	m := mongodoc.Model{
		UUID: info.UUID,
	}
	if err := j.GetModel(ctx, id, jujuparams.ModelReadAccess, &m); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}

	ctl, err := j.DB.Controller(ctx, m.Controller)
	if err != nil {
		return errgo.Mask(err)
	}

	if queryController {
		ctx := zapctx.WithFields(ctx, zap.Stringer("controller", m.Controller))
		conn, err := j.OpenAPIFromDoc(ctx, ctl)
		if err == nil {
			err = conn.ModelInfo(ctx, info)
			if jujuparams.IsCodeUnauthorized(err) && m.Life() == "dying" {
				zapctx.Info(ctx, "could not get ModelInfo for dying model, marking dead", zap.Error(err))
				// The model was dying and now cannot be accessed, assume it is now dead.
				if err := j.DB.DeleteModelWithUUID(ctx, m.Controller, m.UUID); err != nil {
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
		info.ControllerUUID = ctl.UUID
		info.IsController = false
		info.ProviderType = m.ProviderType
		if info.ProviderType == "" {
			info.ProviderType, err = j.DB.ProviderType(ctx, m.Cloud)
			if err != nil {
				zapctx.Error(ctx, "cannot get provider type", zap.Error(err))
			}
		}
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

		machines, err := j.DB.MachinesForModel(ctx, info.UUID)
		if err == nil {
			info.Machines = make([]jujuparams.ModelMachineInfo, 0, len(machines))
			for _, machine := range machines {
				if machine.Info.Life == "dead" {
					continue
				}
				mi := jujuparams.ModelMachineInfo{
					Id:         machine.Info.Id,
					InstanceId: machine.Info.InstanceId,
					Status:     string(machine.Info.AgentStatus.Current),
					HasVote:    machine.Info.HasVote,
					WantsVote:  machine.Info.WantsVote,
				}
				if machine.Info.HardwareCharacteristics != nil {
					mi.Hardware = &jujuparams.MachineHardware{
						Arch:             machine.Info.HardwareCharacteristics.Arch,
						Mem:              machine.Info.HardwareCharacteristics.Mem,
						RootDisk:         machine.Info.HardwareCharacteristics.RootDisk,
						Cores:            machine.Info.HardwareCharacteristics.CpuCores,
						CpuPower:         machine.Info.HardwareCharacteristics.CpuPower,
						Tags:             machine.Info.HardwareCharacteristics.Tags,
						AvailabilityZone: machine.Info.HardwareCharacteristics.AvailabilityZone,
					}
				}
				info.Machines = append(info.Machines, mi)
			}
		} else {
			zapctx.Error(ctx, "cannot get machine information", zap.Error(err))
		}

		info.Migration = nil
		// TODO(mhilton) we should store SLA information
		info.SLA = nil
		info.AgentVersion = modelVersion(ctx, m.Info)
	}

	var canSeeUsers, canSeeMachines bool
	if err := auth.CheckIsUser(ctx, id, m.Path.User); err == nil {
		canSeeUsers = true
		canSeeMachines = true
	} else if err := auth.CheckACL(ctx, id, m.ACL.Admin); err == nil {
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
