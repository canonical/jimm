// Copyright 2020 Canonical Ltd.

package jem

import (
	"context"
	"fmt"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
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
		Credential:             cred.Path,
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
	if _, err := j.DB.Models().FindId(modelDoc.Id).Apply(mgo.Change{
		Update: bson.D{{"$set", bson.D{
			{"uuid", info.UUID},
			{"controller", ctlPath},
			{"cloud", p.Cloud},
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
