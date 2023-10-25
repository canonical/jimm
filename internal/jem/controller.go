// Copyright 2015 Canonical Ltd.

package jem

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/names/v4"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jem/jimmdb"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
)

// GetController retrieves the given controller from the database,
// validating that the current user is allowed to read the controller.
func (j *JEM) GetController(ctx context.Context, id identchecker.ACLIdentity, ctl *mongodoc.Controller) error {
	if err := j.DB.GetController(ctx, ctl); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if err := auth.CheckCanRead(ctx, id, ctl); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	return nil
}

// ForEachController iterates through all controllers that the given user
// has access to and calls the given function with each one. If the given
// function returns an error iteration will immediately stop and the error
// will be returned with the cause unamasked.
func (j *JEM) ForEachController(ctx context.Context, id identchecker.ACLIdentity, f func(ctl *mongodoc.Controller) error) error {
	var ferr error
	err := j.DB.ForEachController(ctx, nil, []string{"path.user", "path.name"}, func(ctl *mongodoc.Controller) error {
		if err := auth.CheckCanRead(ctx, id, ctl); err != nil {
			if errgo.Cause(err) == params.ErrUnauthorized {
				err = nil
			}
			return errgo.Mask(err)
		}
		if err := f(ctl); err != nil {
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

// AddController adds the given controller to the system.
func (j *JEM) AddController(ctx context.Context, id identchecker.ACLIdentity, ctl *mongodoc.Controller) error {
	// Users can only create controllers in their namespace.
	if err := auth.CheckIsUser(ctx, id, ctl.Path.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}

	if ctl.Public {
		// Only controller admins can create public controllers.
		if err := auth.CheckACL(ctx, id, j.ControllerAdmins()); err != nil {
			return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
		}
	} else {
		return errgo.WithCausef(nil, params.ErrForbidden, "cannot add private controller")
	}

	conn, err := j.OpenAPIFromDoc(ctx, ctl)
	if err != nil {
		return errgo.Mask(err, errgo.Is(ErrAPIConnection))
	}
	// The connection will have been cached with a key of "". Avoid errors
	// when adding subsequent controllers by evicting it as soon as we're
	// finished.
	defer conn.Evict()

	// Fill out controller details from the connection.
	ctl.UUID = conn.ControllerTag().Id()
	if v, ok := conn.ServerVersion(); ok {
		ctl.Version = &v
	}

	var mi jujuparams.ModelSummary
	if err := conn.ControllerModelSummary(ctx, &mi); err != nil {
		return errgo.Mask(err, apiconn.IsAPIError, errgo.Is(params.ErrNotFound))
	}

	cloud, err := names.ParseCloudTag(mi.CloudTag)
	if err != nil {
		return errgo.Notef(err, "bad data from controller")
	}
	location := map[string]string{
		"cloud": cloud.Id(),
	}
	if mi.CloudRegion != "" {
		location["region"] = mi.CloudRegion
	}
	ctl.Location = location

	// Update addresses from latest known in controller. Note that
	// conn.APIHostPorts is always guaranteed to include the actual
	// address we succeeded in connecting to.
	ctl.HostPorts = mongodocAPIHostPorts(conn.APIHostPorts())

	if err := j.DB.InsertController(ctx, ctl); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrAlreadyExists))
	}

	if err := j.updateControllerClouds(ctx, conn, ctl); err != nil {
		// TODO(mhilton) mark this error so that it can be considered non-fatal.
		return errgo.Mask(err)
	}

	return nil
}

// mongodocAPIHostPorts returns the given API addresses prepared
// for storage in the database.
//
// It removes unusable addresses and marks any scope-unknown
// addresses as public so that the clients using only public-scoped
// addresses will use them.
func mongodocAPIHostPorts(nmhpss []network.MachineHostPorts) [][]mongodoc.HostPort {
	hpss := make([][]mongodoc.HostPort, 0, len(nmhpss))
	for _, nmhps := range nmhpss {
		nhps := nmhps.HostPorts().FilterUnusable()
		if len(nhps) == 0 {
			continue
		}
		hps := make([]mongodoc.HostPort, len(nhps))
		for i, nhp := range nhps {
			hps[i].SetJujuHostPort(nhp)
			if hps[i].Scope == string(network.ScopeUnknown) {
				// This is needed because network.NewHostPort returns
				// scope unknown for DNS names.
				hps[i].Scope = string(network.ScopePublic)
			}
		}
		hpss = append(hpss, hps)
	}
	return hpss
}

func (j *JEM) updateControllerClouds(ctx context.Context, conn *apiconn.Conn, ctl *mongodoc.Controller) error {
	clouds, err := conn.Clouds(ctx)
	if err != nil {
		return errgo.Mask(err, apiconn.IsAPIError)
	}
	acl := params.ACL{Read: []string{identchecker.Everyone}}
	for name, cloud := range clouds {
		isPrimaryRegion := func(string) bool { return false }
		if string(name) == ctl.Location["cloud"] {
			isPrimaryRegion = func(r string) bool { return r == "" || r == ctl.Location["region"] }
		}
		if err := j.updateControllerCloud(ctx, ctl.Path, name, cloud, isPrimaryRegion, acl); err != nil {
			return errgo.Mask(err)
		}
	}
	return nil
}

func (j *JEM) updateControllerCloud(
	ctx context.Context,
	ctlPath params.EntityPath,
	name params.Cloud,
	cloud jujuparams.Cloud,
	isPrimaryRegion func(string) bool,
	acl params.ACL,
) error {
	if isPrimaryRegion == nil {
		isPrimaryRegion = func(string) bool { return true }
	}
	for _, cr := range conv.FromCloud(name, cloud) {
		if isPrimaryRegion(cr.Region) {
			cr.PrimaryControllers = []params.EntityPath{ctlPath}
		} else {
			cr.SecondaryControllers = []params.EntityPath{ctlPath}
		}
		cr.ACL = acl
		if err := j.DB.UpsertCloudRegion(ctx, &cr); err != nil {
			return errgo.Mask(err)
		}
	}
	return nil
}

// SetControllerDeprecated sets whether the given controller is deprecated.
func (j *JEM) SetControllerDeprecated(ctx context.Context, id identchecker.ACLIdentity, ctlPath params.EntityPath, deprecated bool) error {
	// Only a controller admin can mark a controller deprecated.
	if err := auth.CheckACL(ctx, id, j.ControllerAdmins()); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}

	u := new(jimmdb.Update)
	if deprecated {
		u.Set("deprecated", true)
	} else {
		// A controller that's not deprecated is stored with no deprecated
		// field for backward compatibility and consistency.
		u.Unset("deprecated")
	}
	return errgo.Mask(j.DB.UpdateController(ctx, &mongodoc.Controller{Path: ctlPath}, u, true), errgo.Is(params.ErrNotFound))
}

// DeleteController deletes existing controller and all of its
// associated models from the database. It returns an error if
// either deletion fails. If there is no matching controller then the
// error will have the cause params.ErrNotFound.
//
// Note that this operation is not atomic.
func (j *JEM) DeleteController(ctx context.Context, id identchecker.ACLIdentity, ctl *mongodoc.Controller, force bool) error {
	// Only a controller admin can delete a controller.
	if err := auth.CheckIsUser(ctx, id, ctl.Path.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}

	// TODO (urosj) make this operation atomic.
	if err := j.DB.GetController(ctx, ctl); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}

	if !force && ctl.UnavailableSince.IsZero() {
		return errgo.WithCausef(nil, params.ErrStillAlive, "cannot delete controller while it is still alive")
	}

	// Delete controller from credentials.
	if err := j.credentialsRemoveController(ctx, ctl.Path); err != nil {
		return errgo.Notef(err, "error deleting controller from credentials")
	}

	// Delete controller from cloud regions.
	u := new(jimmdb.Update)
	u.Pull("primarycontrollers", ctl.Path)
	u.Pull("secondarycontrollers", ctl.Path)
	if _, err := j.DB.UpdateCloudRegions(ctx, nil, u); err != nil {
		return errgo.Mask(err)
	}

	// Delete its models first.
	removed, err := j.DB.RemoveModels(ctx, jimmdb.Eq("controller", ctl.Path))
	if err != nil {
		errgo.Notef(err, "error deleting controller models")
	}

	// Then delete the controller.
	if err := j.DB.RemoveController(ctx, ctl); err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			return errgo.Mask(err, errgo.Is(params.ErrNotFound))
		}
		zapctx.Error(ctx, "could not delete controller after removing models",
			zap.Int("model-count", removed),
			zaputil.Error(err),
		)
		return errgo.Notef(err, "cannot delete controller")
	}
	zapctx.Info(ctx, "deleted controller",
		zap.Stringer("controller", ctl.Path),
		zap.Int("model-count", removed),
	)
	return nil
}

// ConnectMonitor creates a connection to the given controller for use by
// monitors. On a successful connection the cloud information will be read
// from the controller and the local database updated, also any outstanding
// changes that are scheduled to be made on the controller will be
// performed. If the specified controlled cannot be found then an error
// with a cause of params.ErrNotFound will be returned. If there is an
// error connecting to the controller then an error with a cause of
// ErrAPIConnection will be returned.
func (j *JEM) ConnectMonitor(ctx context.Context, path params.EntityPath) (*apiconn.Conn, error) {
	ctl := mongodoc.Controller{Path: path}
	if err := j.DB.GetController(ctx, &ctl); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}

	conn, err := j.OpenAPIFromDoc(ctx, &ctl)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(ErrAPIConnection))
	}

	if v, ok := conn.ServerVersion(); ok {
		if err := j.SetControllerVersion(ctx, path, v); err != nil {
			zapctx.Warn(ctx, "cannot update controller version", zap.Error(err))
		}
	}

	if err := j.updateControllerClouds(ctx, conn, &ctl); err != nil {
		zapctx.Warn(ctx, "cannot update controller clouds", zap.Error(err))
	}
	j.controllerUpdateCredentials(ctx, conn, &ctl)
	return conn, nil
}

// controllerUpdateCredentials updates the given controller by updating
// all outstanding UpdateCredentials. Note that if these updates fail they
// are not considered fatal. Any failures are likely to persist and the
// connection retried. In that case the updates will be tried again.
func (j *JEM) controllerUpdateCredentials(ctx context.Context, conn *apiconn.Conn, ctl *mongodoc.Controller) {
	for _, credPath := range ctl.UpdateCredentials {
		cred := mongodoc.Credential{
			Path: credPath,
		}
		if err := j.DB.GetCredential(ctx, &cred); err != nil {
			zapctx.Warn(ctx,
				"cannot get credential for controller",
				zap.Stringer("cred", credPath),
				zap.Stringer("controller", ctl.Path),
				zaputil.Error(err),
			)
			continue
		}
		if cred.Revoked {
			if err := j.revokeControllerCredential(ctx, conn, ctl.Path, cred.Path); err != nil {
				zapctx.Warn(ctx,
					"cannot revoke credential",
					zap.Stringer("cred", credPath),
					zap.Stringer("controller", ctl.Path),
					zaputil.Error(err),
				)
			}
		} else {
			if _, err := j.updateControllerCredential(ctx, conn, ctl.Path, &cred); err != nil {
				zapctx.Warn(ctx,
					"cannot update credential",
					zap.Stringer("cred", credPath),
					zap.Stringer("controller", ctl.Path),
					zaputil.Error(err),
				)
			}
		}
	}
}

// setCredentialUpdates marks all the controllers in the given ctlPaths
// as requiring an update to the credential with the given credPath.
func (j *JEM) setCredentialUpdates(ctx context.Context, ctlPaths []params.EntityPath, credPath mongodoc.CredentialPath) error {
	in := make([]interface{}, len(ctlPaths))
	for i, p := range ctlPaths {
		in[i] = p
	}
	_, err := j.DB.UpdateControllers(ctx, jimmdb.In("path", in...), new(jimmdb.Update).AddToSet("updatecredentials", credPath))
	return errgo.Mask(err)
}

// ClearCredentialUpdate removes the record indicating that the given
// controller needs to update the given credential.
func (j *JEM) clearCredentialUpdate(ctx context.Context, ctlPath params.EntityPath, credPath mongodoc.CredentialPath) error {
	c := &mongodoc.Controller{Path: ctlPath}
	err := j.DB.UpdateController(ctx, c, new(jimmdb.Update).Pull("updatecredentials", credPath), true)
	return errgo.Mask(err, errgo.Is(params.ErrNotFound))
}

// UpdateMigratedModel asserts that the model has been migrated to the
// specified controller and updates the internal model representation.
func (j *JEM) UpdateMigratedModel(ctx context.Context, id identchecker.ACLIdentity, modelTag names.ModelTag, targetControllerName params.Name) error {
	if err := auth.CheckACL(ctx, id, j.ControllerAdmins()); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	model := mongodoc.Model{
		UUID: modelTag.Id(),
	}

	if err := j.DB.GetModel(ctx, &model); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}

	targetController := mongodoc.Controller{
		Path: params.EntityPath{
			Name: targetControllerName,
		},
	}

	if err := j.DB.GetController(ctx, &targetController); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}

	conn, err := j.OpenAPIFromDoc(ctx, &targetController)
	if err != nil {
		return errgo.Notef(err, "cannot connect to controller")
	}
	defer conn.Close()

	err = conn.ModelInfo(ctx, &jujuparams.ModelInfo{
		UUID: modelTag.Id(),
	})
	if err != nil {
		return errgo.Mask(err, apiconn.IsAPIError)
	}

	update := new(jimmdb.Update).Set("controller", targetController.Path)
	err = j.DB.UpdateModel(ctx, &model, update, false)
	if err != nil {
		zapctx.Error(ctx, "failed to update model", zap.String("model", model.UUID), zaputil.Error(err))
		return errgo.Mask(err)
	}

	return nil
}

func (j *JEM) InitiateMigration(ctx context.Context, id identchecker.ACLIdentity, spec jujuparams.MigrationSpec) (*jujuparams.InitiateMigrationResult, error) {
	mt, err := names.ParseModelTag(spec.ModelTag)
	if err != nil {
		return nil, errgo.Mask(err)
	}

	model := mongodoc.Model{UUID: mt.Id()}
	if err := j.GetModel(ctx, id, jujuparams.ModelAdminAccess, &model); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}

	ctl := mongodoc.Controller{Path: model.Controller}
	if err := j.DB.GetController(ctx, &ctl); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}

	conn, err := j.OpenAPIFromDoc(ctx, &ctl)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(ErrAPIConnection))
	}
	defer conn.Close()

	return conn.InitiateMigration(ctx, spec)
}
