// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/names/v4"

	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
	"github.com/CanonicalLtd/jimm/params"
)

func init() {
	facadeInit["JIMM"] = func(r *controllerRoot) []int {
		addControllerMethod := rpc.Method(r.AddController)
		disableControllerUUIDMaskingMethod := rpc.Method(r.DisableControllerUUIDMasking)
		findAuditEventsMethod := rpc.Method(r.FindAuditEvents)
		grantAuditLogAccessMethod := rpc.Method(r.GrantAuditLogAccess)
		listControllersMethod := rpc.Method(r.ListControllers)
		listControllersV3Method := rpc.Method(r.ListControllersV3)
		removeControllerMethod := rpc.Method(r.RemoveController)
		revokeAuditLogAccessMethod := rpc.Method(r.RevokeAuditLogAccess)
		setControllerDeprecatedMethod := rpc.Method(r.SetControllerDeprecated)

		r.AddMethod("JIMM", 2, "DisableControllerUUIDMasking", disableControllerUUIDMaskingMethod)
		r.AddMethod("JIMM", 2, "ListControllers", listControllersMethod)

		r.AddMethod("JIMM", 3, "AddController", addControllerMethod)
		r.AddMethod("JIMM", 3, "DisableControllerUUIDMasking", disableControllerUUIDMaskingMethod)
		r.AddMethod("JIMM", 3, "FindAuditEvents", findAuditEventsMethod)
		r.AddMethod("JIMM", 3, "GrantAuditLogAccess", grantAuditLogAccessMethod)
		r.AddMethod("JIMM", 3, "ListControllers", listControllersV3Method)
		r.AddMethod("JIMM", 3, "RemoveController", removeControllerMethod)
		r.AddMethod("JIMM", 3, "RevokeAuditLogAccess", revokeAuditLogAccessMethod)
		r.AddMethod("JIMM", 3, "SetControllerDeprecated", setControllerDeprecatedMethod)

		return []int{2, 3}
	}
}

// DisableControllerUUIDMasking ensures that the controller UUID returned
// with any model information is the UUID of the juju controller that is
// hosting the model, and not JAAS.
func (r *controllerRoot) DisableControllerUUIDMasking(ctx context.Context) error {
	const op = errors.Op("jujuapi.DisableControllerUUIDMasking")

	if r.user.ControllerAccess != "superuser" {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	r.controllerUUIDMasking = false
	return nil
}

// ListControllers returns the list of juju controllers hosting models
// as part of this JAAS system.
func (r *controllerRoot) ListControllers(ctx context.Context) (params.ListControllerResponse, error) {
	const op = errors.Op("jujuapi.ListControllers")

	if r.user.ControllerAccess != "superuser" {
		// if the user isn't a controller admin return JAAS
		// itself as the only controller.
		srvVersion, err := r.jimm.EarliestControllerVersion(ctx)
		if err != nil {
			return params.ListControllerResponse{}, errors.E(op, err)
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

	var controllers []params.ControllerResponse
	err := r.jimm.Database.ForEachController(ctx, func(ctl *dbmodel.Controller) error {
		var cr params.ControllerResponse
		cr.Path = params.EntityPath{User: "admin", Name: params.Name(ctl.Name)}
		cr.Public = true
		cr.Location = map[string]string{
			"cloud":  ctl.CloudName,
			"region": ctl.CloudRegion,
		}
		if ctl.UnavailableSince.Valid {
			cr.UnavailableSince = &ctl.UnavailableSince.Time
		}
		cr.UUID = ctl.UUID
		cr.Version = ctl.AgentVersion
		controllers = append(controllers, cr)
		return nil
	})
	if err != nil {
		return params.ListControllerResponse{}, errors.E(op, err)
	}
	return params.ListControllerResponse{
		Controllers: controllers,
	}, nil
}

// AddController allows adds a controller to the pool of controllers
// available to JIMM.
func (r *controllerRoot) AddController(ctx context.Context, req apiparams.AddControllerRequest) (apiparams.ControllerInfo, error) {
	const op = errors.Op("jujuapi.AddController")

	ctl := dbmodel.Controller{
		Name:          req.Name,
		PublicAddress: req.PublicAddress,
		CACertificate: req.CACertificate,
		AdminUser:     req.Username,
		AdminPassword: req.Password,
	}
	nphps, err := network.ParseProviderHostPorts(req.APIAddresses...)
	if err != nil {
		return apiparams.ControllerInfo{}, errors.E(op, errors.CodeBadRequest, err)
	}
	for i := range nphps {
		// Mark all the unknown scopes public.
		if nphps[i].Scope == network.ScopeUnknown {
			nphps[i].Scope = network.ScopePublic
		}
	}
	ctl.Addresses = dbmodel.HostPorts{jujuparams.FromProviderHostPorts(nphps)}
	if err := r.jimm.AddController(ctx, r.user, &ctl); err != nil {
		return apiparams.ControllerInfo{}, errors.E(op, err)
	}
	return ctl.ToAPIControllerInfo(), nil
}

// ListControllersV3 returns the list of juju controllers hosting models
// as part of this JAAS system.
func (r *controllerRoot) ListControllersV3(ctx context.Context) (apiparams.ListControllersResponse, error) {
	const op = errors.Op("jujuapi.ListControllersV3")

	if r.user.ControllerAccess != "superuser" {
		// if the user isn't a controller admin return JAAS
		// itself as the only controller.
		srvVersion, err := r.jimm.EarliestControllerVersion(ctx)
		if err != nil {
			return apiparams.ListControllersResponse{}, errors.E(op, err)
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

	var controllers []apiparams.ControllerInfo
	err := r.jimm.Database.ForEachController(ctx, func(ctl *dbmodel.Controller) error {
		controllers = append(controllers, ctl.ToAPIControllerInfo())
		return nil
	})
	if err != nil {
		return apiparams.ListControllersResponse{}, errors.E(op, err)
	}
	return apiparams.ListControllersResponse{
		Controllers: controllers,
	}, nil
}

// RemoveController removes a controller.
func (r *controllerRoot) RemoveController(ctx context.Context, req apiparams.RemoveControllerRequest) (apiparams.ControllerInfo, error) {
	const op = errors.Op("jujuapi.RemoveController")

	ctl := dbmodel.Controller{
		Name: req.Name,
	}
	if err := r.jimm.Database.GetController(ctx, &ctl); err != nil {
		return apiparams.ControllerInfo{}, errors.E(op, err)
	}

	if err := r.jimm.RemoveController(ctx, r.user, req.Name, req.Force); err != nil {
		return apiparams.ControllerInfo{}, errors.E(op, err)
	}
	return ctl.ToAPIControllerInfo(), nil
}

// SetControllerDeprecated sets the deprecated status of a controller.
func (r *controllerRoot) SetControllerDeprecated(ctx context.Context, req apiparams.SetControllerDeprecatedRequest) (apiparams.ControllerInfo, error) {
	const op = errors.Op("jujuapi.SetControllerDeprecated")

	if err := r.jimm.SetControllerDeprecated(ctx, r.user, req.Name, req.Deprecated); err != nil {
		return apiparams.ControllerInfo{}, errors.E(op, err)
	}
	ctl := dbmodel.Controller{
		Name: req.Name,
	}
	if err := r.jimm.Database.GetController(ctx, &ctl); err != nil {
		return apiparams.ControllerInfo{}, errors.E(op, err)
	}
	return ctl.ToAPIControllerInfo(), nil
}

// maxLimit is the maximum number of audit-log entries that will be
// returned from the audit log, no matter how many are requested.
const maxLimit = 50

// FindAuditEvents finds the audit-log entries that match the given filter.
func (r *controllerRoot) FindAuditEvents(ctx context.Context, req apiparams.FindAuditEventsRequest) (apiparams.AuditEvents, error) {
	const op = errors.Op("jujuapi.FindAuditEvents")

	var filter db.AuditLogFilter
	var err error
	if req.After != "" {
		filter.Start, err = time.Parse(time.RFC3339, req.After)
		if err != nil {
			return apiparams.AuditEvents{}, errors.E(op, err, errors.CodeBadRequest, `invalid "after" filter`)
		}
	}
	if req.Before != "" {
		filter.End, err = time.Parse(time.RFC3339, req.Before)
		if err != nil {
			return apiparams.AuditEvents{}, errors.E(op, err, errors.CodeBadRequest, `invalid "before" filter`)
		}
	}
	if req.Tag != "" {
		tag, err := names.ParseTag(req.Tag)
		if err != nil {
			return apiparams.AuditEvents{}, errors.E(op, err, errors.CodeBadRequest, `invalid "tag" filter`)
		}
		filter.Tag = tag.String()
	}
	if req.UserTag != "" {
		tag, err := names.ParseUserTag(req.UserTag)
		if err != nil {
			return apiparams.AuditEvents{}, errors.E(op, err, errors.CodeBadRequest, `invalid "user-tag" filter`)
		}
		filter.UserTag = tag.String()
	}
	filter.Action = req.Action

	limit := int(req.Limit)
	if limit < 1 || limit > maxLimit {
		limit = maxLimit
	}

	entries, err := r.jimm.FindAuditEvents(ctx, r.user, filter)
	if err != nil {
		return apiparams.AuditEvents{}, errors.E(op, err)
	}

	if len(entries) > limit {
		entries = entries[:limit]
	}

	events := make([]apiparams.AuditEvent, len(entries))
	for i, ent := range entries {
		events[i] = ent.ToAPIAuditEvent()
	}
	return apiparams.AuditEvents{
		Events: events,
	}, nil
}

// GrantAuditLogAccess grants access to the audit log at the specified
// level to the specified user. The only currently supported level is
// "read". Only controller admin users can grant access to the audit log.
func (r *controllerRoot) GrantAuditLogAccess(ctx context.Context, req apiparams.AuditLogAccessRequest) error {
	const op = errors.Op("jujuapi.GrantAuditLogAccess")

	_, err := parseUserTag(req.UserTag)
	if err != nil {
		return errors.E(op, err, errors.CodeBadRequest)
	}
	if r.user.ControllerAccess != "superuser" {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	// TODO(mhilton) actually grant access to the user.
	return nil
}

// RevokeAuditLogAccess revokes access to the audit log at the specified
// level from the specified user. The only currently supported level is
// "read". Only controller admin users can revoke access to the audit log.
func (r *controllerRoot) RevokeAuditLogAccess(ctx context.Context, req apiparams.AuditLogAccessRequest) error {
	const op = errors.Op("jujuapi.RevokeAuditLogAccess")

	_, err := parseUserTag(req.UserTag)
	if err != nil {
		return errors.E(op, err, errors.CodeBadRequest)
	}
	if r.user.ControllerAccess != "superuser" {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	// TODO(mhilton) actually revoke access from the user.
	return nil
}
