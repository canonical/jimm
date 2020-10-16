// Copyright 2015 Canonical Ltd.

package jem

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/names/v4"
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

// AddController adds the given controller to the system.
func (j *JEM) AddController(ctx context.Context, id identchecker.ACLIdentity, ctl *mongodoc.Controller) error {
	// Users can only create controllers in their namespace.
	if err := auth.CheckIsUser(ctx, id, ctl.Path.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}

	if ctl.Public {
		// Only controller admins can create public controllers.
		if err := auth.CheckIsUser(ctx, id, j.ControllerAdmin()); err != nil {
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
	regions := conv.FromCloud(name, cloud)
	for i := range regions {
		if isPrimaryRegion(regions[i].Region) {
			regions[i].PrimaryControllers = []params.EntityPath{ctlPath}
		} else {
			regions[i].SecondaryControllers = []params.EntityPath{ctlPath}
		}
		regions[i].ACL = acl
	}
	return errgo.Mask(j.DB.UpdateCloudRegions(ctx, regions))
}

// SetControllerDeprecated sets whether the given controller is deprecated.
func (j *JEM) SetControllerDeprecated(ctx context.Context, id identchecker.ACLIdentity, ctlPath params.EntityPath, deprecated bool) error {
	// Only a controller admin can mark a controller deprecated.
	if err := auth.CheckIsUser(ctx, id, j.ControllerAdmin()); err != nil {
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
	if err := j.DB.DeleteControllerFromCloudRegions(ctx, ctl.Path); err != nil {
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
			if err := j.revokeControllerCredential(ctx, conn, ctl.Path, cred.Path.ToParams()); err != nil {
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
