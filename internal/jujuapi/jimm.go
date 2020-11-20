// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/names/v4"
	"gopkg.in/errgo.v1"

	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

func init() {
	facadeInit["JIMM"] = func(r *controllerRoot) []int {
		addControllerMethod := rpc.Method(r.AddController)
		disableControllerUUIDMaskingMethod := rpc.Method(r.DisableControllerUUIDMasking)
		listControllersMethod := rpc.Method(r.ListControllers)
		listControllersV3Method := rpc.Method(r.ListControllersV3)
		userModelStatsMethod := rpc.Method(r.UserModelStats)

		r.AddMethod("JIMM", 1, "UserModelStats", userModelStatsMethod)

		r.AddMethod("JIMM", 2, "DisableControllerUUIDMasking", disableControllerUUIDMaskingMethod)
		r.AddMethod("JIMM", 2, "ListControllers", listControllersMethod)
		r.AddMethod("JIMM", 2, "UserModelStats", userModelStatsMethod)

		r.AddMethod("JIMM", 3, "AddController", addControllerMethod)
		r.AddMethod("JIMM", 3, "DisableControllerUUIDMasking", disableControllerUUIDMaskingMethod)
		r.AddMethod("JIMM", 3, "ListControllers", listControllersV3Method)

		return []int{1, 2, 3}
	}
}

// UserModelStats returns statistics about all the models that were created
// by the currently authenticated user.
func (r *controllerRoot) UserModelStats(ctx context.Context) (params.UserModelStatsResponse, error) {
	models := make(map[string]params.ModelStats)
	err := r.jem.ForEachModel(ctx, r.identity, jujuparams.ModelReadAccess, func(m *mongodoc.Model) error {
		if m.Creator != r.identity.Id() {
			return nil
		}
		models[m.UUID] = params.ModelStats{
			Model:  userModelForModelDoc(m),
			Counts: m.Counts,
		}
		return nil
	})
	if err != nil {
		return params.UserModelStatsResponse{}, errgo.Mask(err)
	}
	return params.UserModelStatsResponse{
		Models: models,
	}, nil
}

// DisableControllerUUIDMasking ensures that the controller UUID returned
// with any model information is the UUID of the juju controller that is
// hosting the model, and not JAAS.
func (r *controllerRoot) DisableControllerUUIDMasking(ctx context.Context) error {
	err := auth.CheckACL(ctx, r.identity, []string{string(r.jem.ControllerAdmin())})
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	r.controllerUUIDMasking = false
	return nil
}

// ListControllers returns the list of juju controllers hosting models
// as part of this JAAS system.
func (r *controllerRoot) ListControllers(ctx context.Context) (params.ListControllerResponse, error) {
	err := auth.CheckACL(ctx, r.identity, []string{string(r.jem.ControllerAdmin())})
	if errgo.Cause(err) == params.ErrUnauthorized {
		// if the user isn't a controller admin return JAAS
		// itself as the only controller.

		srvVersion, err := r.jem.EarliestControllerVersion(ctx, r.identity)
		if err != nil {
			return params.ListControllerResponse{}, errgo.Mask(err)
		}
		return params.ListControllerResponse{
			Controllers: []params.ControllerResponse{{
				Path:    params.EntityPath{User: "admin", Name: "jaas"},
				Public:  true,
				UUID:    r.params.ControllerUUID,
				Version: srvVersion.String(),
			}},
		}, nil
	}
	if err != nil {
		return params.ListControllerResponse{}, errgo.Mask(err)
	}

	var controllers []params.ControllerResponse
	err = r.jem.ForEachController(ctx, r.identity, func(ctl *mongodoc.Controller) error {
		controllers = append(controllers, params.ControllerResponse{
			Path:             ctl.Path,
			Public:           ctl.Public,
			UnavailableSince: newTime(ctl.UnavailableSince.UTC()),
			Location:         ctl.Location,
			UUID:             ctl.UUID,
			Version:          ctl.Version.String(),
		})
		return nil
	})
	if err != nil {
		return params.ListControllerResponse{}, errgo.Mask(err)
	}
	return params.ListControllerResponse{
		Controllers: controllers,
	}, nil
}

// AddController allows adds a controller to the pool of controllers
// available to JIMM.
func (r *controllerRoot) AddController(ctx context.Context, req apiparams.AddControllerRequest) (apiparams.ControllerInfo, error) {
	var addresses []string
	if req.PublicAddress != "" {
		addresses = append(addresses, req.PublicAddress)
	}
	addresses = append(addresses, req.APIAddresses...)

	hps, err := mongodoc.ParseAddresses(addresses)
	if err != nil {
		return apiparams.ControllerInfo{}, errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	for i, hp := range hps {
		if network.DeriveAddressType(hp.Host) != network.HostName {
			continue
		}
		if hp.Host != "localhost" {
			// As it won't have been specified we'll assume that any DNS name, except
			// localhost, is public.
			hps[i].Scope = string(network.ScopePublic)
		}
	}

	ctl := mongodoc.Controller{
		Path: params.EntityPath{
			User: r.jem.ControllerAdmin(),
			Name: params.Name(req.Name),
		},
		CACert:        req.CACertificate,
		HostPorts:     [][]mongodoc.HostPort{hps},
		AdminUser:     req.Username,
		AdminPassword: req.Password,
		Public:        true,
	}

	err = r.jem.AddController(ctx, r.identity, &ctl)
	if errgo.Cause(err) == jem.ErrAPIConnection {
		return apiparams.ControllerInfo{}, errgo.WithCausef(err, params.ErrBadRequest, "")
	} else if err != nil {
		return apiparams.ControllerInfo{}, errgo.Mask(err,
			errgo.Is(params.ErrAlreadyExists),
			errgo.Is(params.ErrForbidden),
			errgo.Is(params.ErrUnauthorized),
		)
	}
	var ci apiparams.ControllerInfo
	writeControllerInfo(&ci, &ctl)
	return ci, nil
}

// ListControllersV3 returns the list of juju controllers hosting models
// as part of this JAAS system.
func (r *controllerRoot) ListControllersV3(ctx context.Context) (apiparams.ListControllersResponse, error) {
	err := auth.CheckACL(ctx, r.identity, []string{string(r.jem.ControllerAdmin())})
	if errgo.Cause(err) == params.ErrUnauthorized {
		// if the user isn't a controller admin return JAAS
		// itself as the only controller.

		srvVersion, err := r.jem.EarliestControllerVersion(ctx, r.identity)
		if err != nil {
			return apiparams.ListControllersResponse{}, errgo.Mask(err)
		}
		return apiparams.ListControllersResponse{
			Controllers: []apiparams.ControllerInfo{{
				Name: "jaas", // TODO(mhilton) make this configurable.
				UUID: r.params.ControllerUUID,
				// TODO(mhilton)enable setting the public address.
				AgentVersion: srvVersion.String(),
				Status: jujuparams.EntityStatus{
					Status: "available",
				},
			}},
		}, nil
	}
	if err != nil {
		return apiparams.ListControllersResponse{}, errgo.Mask(err)
	}

	var controllers []apiparams.ControllerInfo
	err = r.jem.ForEachController(ctx, r.identity, func(ctl *mongodoc.Controller) error {
		var ci apiparams.ControllerInfo
		writeControllerInfo(&ci, ctl)

		controllers = append(controllers, ci)
		return nil
	})
	if err != nil {
		return apiparams.ListControllersResponse{}, errgo.Mask(err)
	}
	return apiparams.ListControllersResponse{
		Controllers: controllers,
	}, nil
}

func writeControllerInfo(ci *apiparams.ControllerInfo, ctl *mongodoc.Controller) {
	ci.Name = string(ctl.Path.Name)
	ci.UUID = ctl.UUID

	// Assume the first hostname we find is the public address.
OUTER:
	for _, hps := range ctl.HostPorts {
		for _, hp := range hps {
			if network.DeriveAddressType(hp.Host) != network.HostName {
				continue
			}
			if hp.Host == "localhost" {
				continue
			}
			ci.PublicAddress = hp.Address()
			break OUTER
		}
	}
	ci.APIAddresses = mongodoc.Addresses(ctl.HostPorts)
	ci.CACertificate = ctl.CACert
	ci.CloudTag = names.NewCloudTag(ctl.Location["cloud"]).String()
	ci.CloudRegion = ctl.Location["region"]
	ci.Username = ctl.AdminUser
	ci.AgentVersion = ctl.Version.String()
	t := ctl.UnavailableSince.UTC()
	if !t.IsZero() {
		ci.Status = jujuparams.EntityStatus{
			Status: "unavailable",
			Since:  &t,
		}
	} else if ctl.Deprecated {
		ci.Status = jujuparams.EntityStatus{
			Status: "deprecated",
		}
	} else {
		ci.Status = jujuparams.EntityStatus{
			Status: "available",
		}
	}
}
