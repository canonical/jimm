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
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
)

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

	if err := j.DB.AddController(ctx, ctl); err != nil {
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

// ConnectMonitor creates a connection to the given controller for use by
// monitors. On a successful connection the cloud information will be read
// from the controller and the local database updated, also any outstanding
// changes that are scheduled to be made on the controller will be
// performed. If the specified controlled cannot be found then an error
// with a cause of params.ErrNotFound will be returned. If there is an
// error connecting to the controller then an error with a cause of
// ErrAPIConnection will be returned.
func (j *JEM) ConnectMonitor(ctx context.Context, path params.EntityPath) (*apiconn.Conn, error) {
	ctl, err := j.DB.Controller(ctx, path)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}

	conn, err := j.OpenAPIFromDoc(ctx, ctl)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(ErrAPIConnection))
	}

	if v, ok := conn.ServerVersion(); ok {
		if err := j.DB.SetControllerVersion(ctx, path, v); err != nil {
			zapctx.Warn(ctx, "cannot update controller version", zap.Error(err))
		}
	}

	if err := j.updateControllerClouds(ctx, conn, ctl); err != nil {
		zapctx.Warn(ctx, "cannot update controller clouds", zap.Error(err))
	}
	j.controllerUpdateCredentials(ctx, conn, ctl)
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
