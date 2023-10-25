// Copyright 2015 Canonical Ltd.

package jem

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/conv"
	"github.com/canonical/jimm/internal/jem/jimmdb"
	"github.com/canonical/jimm/internal/mongodoc"
	"github.com/canonical/jimm/internal/zapctx"
	"github.com/canonical/jimm/params"
)

// GetModel retrieves the given model from the database using
// Database.GetModel. It then checks that the given user has the given
// access level on the model. If the model cannot be found then an error
// with a cause of params.ErrNotFound is returned. If the given user
// does not have the correct access level on the model then an error of
// type params.ErrUnauthorized will be returned.
func (j *JEM) GetModel(ctx context.Context, id identchecker.ACLIdentity, access jujuparams.UserAccessPermission, m *mongodoc.Model) error {
	if err := j.DB.GetModel(ctx, m); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if err := j.checkModelAccess(ctx, id, access, m, true); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	if err := j.updateModelContent(ctx, m); err != nil {
		// Log the failure, but return what we have to the caller.
		zapctx.Error(ctx, "cannot update model info", zap.Error(err))
	}
	return nil
}

// ForEachModel iterates through all models that the authorized user has
// the given access level for. The given function will be called for each
// model. If the given function returns an error iteration will immediately
// stop and the error will be returned with the cause unamasked.
func (j *JEM) ForEachModel(ctx context.Context, id identchecker.ACLIdentity, access jujuparams.UserAccessPermission, f func(*mongodoc.Model) error) error {
	var ferr error
	err := j.DB.ForEachModel(ctx, nil, []string{"path.user", "path.name"}, func(m *mongodoc.Model) error {
		if err := j.checkModelAccess(ctx, id, access, m, false); err != nil {
			if errgo.Cause(err) == params.ErrUnauthorized {
				err = nil
			}
			return errgo.Mask(err)
		}
		if err := j.updateModelContent(ctx, m); err != nil {
			// Log the failure, but use what we have.
			zapctx.Error(ctx, "cannot update model info", zap.Error(err))
		}
		if err := f(m); err != nil {
			ferr = err
			return errStop
		}
		return nil
	})
	if errgo.Cause(err) == errStop {
		return errgo.Mask(ferr, errgo.Any)
	}
	return errgo.Mask(err)
}

// check model access checks that that authenticated user has the given
// access level on the given model.
func (j *JEM) checkModelAccess(ctx context.Context, id identchecker.ACLIdentity, access jujuparams.UserAccessPermission, m *mongodoc.Model, allowControllerAdmin bool) error {
	// Currently in JAAS the namespace user has full access to the model.
	acl := []string{string(m.Path.User)}
	if allowControllerAdmin {
		acl = append(acl, j.ControllerAdmins()...)
	}
	switch access {
	case jujuparams.ModelReadAccess:
		acl = append(acl, m.ACL.Read...)
		fallthrough
	case jujuparams.ModelWriteAccess:
		acl = append(acl, m.ACL.Write...)
		fallthrough
	case jujuparams.ModelAdminAccess:
		acl = append(acl, m.ACL.Admin...)
	}
	return errgo.Mask(auth.CheckACL(ctx, id, acl), errgo.Is(params.ErrUnauthorized))
}

// updateModelContent retrieves model parameters missing in the current
// database from the controller.
func (j *JEM) updateModelContent(ctx context.Context, model *mongodoc.Model) error {
	u := new(jimmdb.Update)
	cloud := model.Cloud
	if cloud == "" {
		// The model does not currently store its cloud information so go
		// and fetch it from the model itself. This happens if the model
		// was created with a JIMM version older than 0.9.5.
		conn, err := j.OpenAPI(ctx, model.Controller)
		if err != nil {
			return errgo.Mask(err)
		}
		defer conn.Close()
		info := jujuparams.ModelInfo{UUID: model.UUID}
		if err := conn.ModelInfo(ctx, &info); err != nil {
			return errgo.Mask(err)
		}
		cloudTag, err := names.ParseCloudTag(info.CloudTag)
		if err != nil {
			return errgo.Notef(err, "bad data from controller")
		}
		cloud = params.Cloud(cloudTag.Id())
		credentialTag, err := names.ParseCloudCredentialTag(info.CloudCredentialTag)
		if err != nil {
			return errgo.Notef(err, "bad data from controller")
		}
		owner, err := conv.FromUserTag(credentialTag.Owner())
		if err != nil {
			return errgo.Mask(err, errgo.Is(conv.ErrLocalUser))
		}
		u.Set("cloud", cloud)
		u.Set("credential", mongodoc.CredentialPath{
			Cloud: string(params.Cloud(credentialTag.Cloud().Id())),
			EntityPath: mongodoc.EntityPath{
				User: string(owner),
				Name: credentialTag.Name(),
			},
		})
		u.Set("defaultseries", info.DefaultSeries)
		if info.CloudRegion != "" {
			u.Set("cloudregion", info.CloudRegion)
		}
	}
	if model.ProviderType == "" {
		cr := mongodoc.CloudRegion{Cloud: cloud}
		if err := j.DB.GetCloudRegion(ctx, &cr); err != nil {
			return errgo.Mask(err)
		}
		u.Set("providertype", cr.ProviderType)
	}
	if model.ControllerUUID == "" {
		ctl := mongodoc.Controller{Path: model.Controller}
		if err := j.DB.GetController(ctx, &ctl); err != nil {
			return errgo.Mask(err)
		}
		u.Set("controlleruuid", ctl.UUID)
	}
	if u.IsZero() {
		return nil
	}
	return errgo.Mask(j.DB.UpdateModel(ctx, model, u, true))
}
