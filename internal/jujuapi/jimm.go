// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"strings"
	"time"

	"github.com/juju/juju/core/network"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"

	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
	"github.com/CanonicalLtd/jimm/internal/openfga"
)

func init() {
	facadeInit["JIMM"] = func(r *controllerRoot) []int {
		addControllerMethod := rpc.Method(r.AddController)
		disableControllerUUIDMaskingMethod := rpc.Method(r.DisableControllerUUIDMasking)
		findAuditEventsMethod := rpc.Method(r.FindAuditEvents)
		grantAuditLogAccessMethod := rpc.Method(r.GrantAuditLogAccess)
		importModelMethod := rpc.Method(r.ImportModel)
		listControllersMethod := rpc.Method(r.ListControllers)
		listControllersV3Method := rpc.Method(r.ListControllersV3)
		removeControllerMethod := rpc.Method(r.RemoveController)
		revokeAuditLogAccessMethod := rpc.Method(r.RevokeAuditLogAccess)
		setControllerDeprecatedMethod := rpc.Method(r.SetControllerDeprecated)
		fullModelStatusMethod := rpc.Method(r.FullModelStatus)
		updateMigratedModelMethod := rpc.Method(r.UpdateMigratedModel)
		addCloudToControllerMethod := rpc.Method(r.AddCloudToController)
		removeCloudFromControllerMethod := rpc.Method(r.RemoveCloudFromController)
		addGroupMethod := rpc.Method(r.AddGroup)
		renameGroupMethod := rpc.Method(r.RenameGroup)
		removeGroupMethod := rpc.Method(r.RemoveGroup)
		listGroupsMethod := rpc.Method(r.ListGroups)
		addRelationMethod := rpc.Method(r.AddRelation)
		removeRelationMethod := rpc.Method(r.RemoveRelation)
		checkRelationMethod := rpc.Method(r.CheckRelation)
		listRelationshipTuplesMethod := rpc.Method(r.ListRelationshipTuples)
		crossModelQueryMethod := rpc.Method(r.CrossModelQuery)

		r.AddMethod("JIMM", 2, "DisableControllerUUIDMasking", disableControllerUUIDMaskingMethod)
		r.AddMethod("JIMM", 2, "ListControllers", listControllersMethod)

		r.AddMethod("JIMM", 3, "AddController", addControllerMethod)
		r.AddMethod("JIMM", 3, "DisableControllerUUIDMasking", disableControllerUUIDMaskingMethod)
		r.AddMethod("JIMM", 3, "FindAuditEvents", findAuditEventsMethod)
		r.AddMethod("JIMM", 3, "FullModelStatus", fullModelStatusMethod)
		r.AddMethod("JIMM", 3, "GrantAuditLogAccess", grantAuditLogAccessMethod)
		r.AddMethod("JIMM", 3, "ImportModel", importModelMethod)
		r.AddMethod("JIMM", 3, "ListControllers", listControllersV3Method)
		r.AddMethod("JIMM", 3, "RemoveController", removeControllerMethod)
		r.AddMethod("JIMM", 3, "RevokeAuditLogAccess", revokeAuditLogAccessMethod)
		r.AddMethod("JIMM", 3, "SetControllerDeprecated", setControllerDeprecatedMethod)
		r.AddMethod("JIMM", 3, "UpdateMigratedModel", updateMigratedModelMethod)
		r.AddMethod("JIMM", 3, "AddCloudToController", addCloudToControllerMethod)
		r.AddMethod("JIMM", 3, "RemoveCloudFromController", removeCloudFromControllerMethod)
		r.AddMethod("JIMM", 3, "CrossModelQuery", crossModelQueryMethod)

		// JIMM Generic RPC
		r.AddMethod("JIMM", 4, "AddController", addControllerMethod)
		r.AddMethod("JIMM", 4, "DisableControllerUUIDMasking", disableControllerUUIDMaskingMethod)
		r.AddMethod("JIMM", 4, "FindAuditEvents", findAuditEventsMethod)
		r.AddMethod("JIMM", 4, "FullModelStatus", fullModelStatusMethod)
		r.AddMethod("JIMM", 4, "GrantAuditLogAccess", grantAuditLogAccessMethod)
		r.AddMethod("JIMM", 4, "ImportModel", importModelMethod)
		r.AddMethod("JIMM", 4, "ListControllers", listControllersV3Method)
		r.AddMethod("JIMM", 4, "RemoveController", removeControllerMethod)
		r.AddMethod("JIMM", 4, "RevokeAuditLogAccess", revokeAuditLogAccessMethod)
		r.AddMethod("JIMM", 4, "SetControllerDeprecated", setControllerDeprecatedMethod)
		r.AddMethod("JIMM", 4, "UpdateMigratedModel", updateMigratedModelMethod)
		r.AddMethod("JIMM", 4, "AddCloudToController", addCloudToControllerMethod)
		r.AddMethod("JIMM", 4, "RemoveCloudFromController", removeCloudFromControllerMethod)
		// JIMM ReBAC RPC
		r.AddMethod("JIMM", 4, "AddGroup", addGroupMethod)
		r.AddMethod("JIMM", 4, "RenameGroup", renameGroupMethod)
		r.AddMethod("JIMM", 4, "RemoveGroup", removeGroupMethod)
		r.AddMethod("JIMM", 4, "ListGroups", listGroupsMethod)
		r.AddMethod("JIMM", 4, "AddRelation", addRelationMethod)
		r.AddMethod("JIMM", 4, "RemoveRelation", removeRelationMethod)
		r.AddMethod("JIMM", 4, "CheckRelation", checkRelationMethod)
		r.AddMethod("JIMM", 4, "ListRelationshipTuples", listRelationshipTuplesMethod)
		// JIMM Cross-model queries
		r.AddMethod("JIMM", 4, "CrossModelQuery", crossModelQueryMethod)

		return []int{2, 3, 4}
	}
}

// DisableControllerUUIDMasking ensures that the controller UUID returned
// with any model information is the UUID of the juju controller that is
// hosting the model, and not JAAS.
func (r *controllerRoot) DisableControllerUUIDMasking(ctx context.Context) error {
	const op = errors.Op("jujuapi.DisableControllerUUIDMasking")

	isControllerAdmin, err := openfga.IsAdministrator(ctx, r.user, names.NewControllerTag(r.jimm.UUID))
	if err != nil {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	if !isControllerAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	r.controllerUUIDMasking = false
	return nil
}

// LegacyListControllerResponse holds a list of controllers as returned
// by the legacy JIMM.ListControllers API.
type LegacyListControllerResponse struct {
	Controllers []LegacyControllerResponse `json:"controllers"`
}

// LegacyControllerResponse holds information on a given Controller as
// returned by the legacy JIMM.ListControllers API.
type LegacyControllerResponse struct {
	// Path holds the path of the controller.
	Path string `json:"path"`

	// ProviderType holds the kind of provider used
	// by the Controller.
	ProviderType string `json:"provider-type,omitempty"`

	// Location holds location attributes associated with the controller.
	Location map[string]string `json:"location,omitempty"`

	// Public holds whether the controller is part of the public
	// pool of controllers.
	Public bool

	// UnavailableSince holds the time that the JEM server
	// noticed that the model's controller could not be
	// contacted. It is empty when the model is available.
	UnavailableSince *time.Time `json:"unavailable-since,omitempty"`

	// UUID holds the controller's UUID.
	UUID string `json:"uuid,omitempty"`

	// Version holds the version of the controller.
	Version string `json:"version,omitempty"`
}

// ListControllers returns the list of juju controllers hosting models
// as part of this JAAS system.
func (r *controllerRoot) ListControllers(ctx context.Context) (LegacyListControllerResponse, error) {
	const op = errors.Op("jujuapi.ListControllers")

	isControllerAdmin, err := openfga.IsAdministrator(ctx, r.user, names.NewControllerTag(r.jimm.UUID))
	if err != nil {
		return LegacyListControllerResponse{}, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	if !isControllerAdmin {
		// if the user isn't a controller admin return JAAS
		// itself as the only controller.
		srvVersion, err := r.jimm.EarliestControllerVersion(ctx)
		if err != nil {
			return LegacyListControllerResponse{}, errors.E(op, err)
		}
		return LegacyListControllerResponse{
			Controllers: []LegacyControllerResponse{{
				Path:    "admin/jaas",
				Public:  true,
				UUID:    r.params.ControllerUUID,
				Version: srvVersion.String(),
			}},
		}, nil
	}

	var controllers []LegacyControllerResponse
	err = r.jimm.Database.ForEachController(ctx, func(ctl *dbmodel.Controller) error {
		var cr LegacyControllerResponse
		cr.Path = "admin/" + ctl.Name
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
		return LegacyListControllerResponse{}, errors.E(op, err)
	}
	return LegacyListControllerResponse{
		Controllers: controllers,
	}, nil
}

// AddCloudToController adds the specified cloud to a specific controller.
func (r *controllerRoot) AddCloudToController(ctx context.Context, req apiparams.AddCloudToControllerRequest) error {
	const op = errors.Op("jujuapi.AddCloudToController")
	if err := r.jimm.AddCloudToController(ctx, r.user, req.ControllerName, names.NewCloudTag(req.Name), req.Cloud); err != nil {
		return errors.E(op, err)
	}
	return nil
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
		zapctx.Error(ctx, "failed to add controller", zaputil.Error(err))
		return apiparams.ControllerInfo{}, errors.E(op, err)
	}
	return ctl.ToAPIControllerInfo(), nil
}

// ListControllersV3 returns the list of juju controllers hosting models
// as part of this JAAS system.
func (r *controllerRoot) ListControllersV3(ctx context.Context) (apiparams.ListControllersResponse, error) {
	const op = errors.Op("jujuapi.ListControllersV3")

	isControllerAdmin, err := openfga.IsAdministrator(ctx, r.user, names.NewControllerTag(r.jimm.UUID))
	if err != nil {
		return apiparams.ListControllersResponse{}, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	if !isControllerAdmin {
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
	err = r.jimm.Database.ForEachController(ctx, func(ctl *dbmodel.Controller) error {
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
const maxLimit = 1000
const limitDefault = 50

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
	if req.UserTag != "" {
		tag, err := names.ParseUserTag(req.UserTag)
		if err != nil {
			return apiparams.AuditEvents{}, errors.E(op, err, errors.CodeBadRequest, `invalid "user-tag" filter`)
		}
		filter.UserTag = tag.String()
	}
	filter.Model = req.Model

	limit := int(req.Limit)
	if limit < 1 {
		limit = limitDefault
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	filter.Limit = limit
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	filter.Offset = offset

	entries, err := r.jimm.FindAuditEvents(ctx, r.user, filter)
	if err != nil {
		return apiparams.AuditEvents{}, errors.E(op, err)
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

	ut, err := parseUserTag(req.UserTag)
	if err != nil {
		return errors.E(op, err, errors.CodeBadRequest, "invalid user tag")
	}

	err = r.jimm.GrantAuditLogAccess(ctx, r.user, ut)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// RevokeAuditLogAccess revokes access to the audit log at the specified
// level from the specified user. The only currently supported level is
// "read". Only controller admin users can revoke access to the audit log.
func (r *controllerRoot) RevokeAuditLogAccess(ctx context.Context, req apiparams.AuditLogAccessRequest) error {
	const op = errors.Op("jujuapi.RevokeAuditLogAccess")

	ut, err := parseUserTag(req.UserTag)
	if err != nil {
		return errors.E(op, err, errors.CodeBadRequest, "invalid user tag")
	}

	err = r.jimm.RevokeAuditLogAccess(ctx, r.user, ut)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// FullModelStatus returns the full status of the juju model.
func (r *controllerRoot) FullModelStatus(ctx context.Context, req apiparams.FullModelStatusRequest) (jujuparams.FullStatus, error) {
	const op = errors.Op("jujuapi.FullModelStatus")

	mt, err := names.ParseModelTag(req.ModelTag)
	if err != nil {
		return jujuparams.FullStatus{}, errors.E(op, err, errors.CodeBadRequest)
	}

	status, err := r.jimm.FullModelStatus(ctx, r.user, mt, req.Patterns)
	if err != nil {
		return jujuparams.FullStatus{}, errors.E(op, err)
	}

	return *status, nil
}

// UpdateMigratedModel checks that the model has been migrated to the specified controller
// and updates internal representation of the model.
func (r *controllerRoot) UpdateMigratedModel(ctx context.Context, req apiparams.UpdateMigratedModelRequest) error {
	const op = errors.Op("jujuapi.UpdateMigratedModel")

	if r.user.ControllerAccess != "superuser" {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	mt, err := names.ParseModelTag(req.ModelTag)
	if err != nil {
		return errors.E(op, err, errors.CodeBadRequest)
	}
	err = r.jimm.UpdateMigratedModel(ctx, r.user, mt, req.TargetController)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// ImportModel imports a model already attached to a controller allowing
// management of that model in JIMM.
func (r *controllerRoot) ImportModel(ctx context.Context, req apiparams.ImportModelRequest) error {
	const op = errors.Op("jujuapi.ImportModel")

	mt, err := names.ParseModelTag(req.ModelTag)
	if err != nil {
		return errors.E(op, err, errors.CodeBadRequest)
	}

	err = r.jimm.ImportModel(ctx, r.user, req.Controller, mt)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// RemoveCloudFromController removes the specified cloud from a specific controller.
func (r *controllerRoot) RemoveCloudFromController(ctx context.Context, req apiparams.RemoveCloudFromControllerRequest) error {
	const op = errors.Op("jujuapi.RemoveCloudFromController")
	ct, err := names.ParseCloudTag(req.CloudTag)
	if err != nil {
		return errors.E(op, err, errors.CodeBadRequest)
	}
	if err := r.jimm.RemoveCloudFromController(ctx, r.user, req.ControllerName, ct); err != nil {
		return errors.E(op, err)
	}
	return nil
}

// CrossModelQuery enables users to query all of their available models and each entity within the model.
//
// The query will run against output exactly like "juju status --format json", but for each of their models.
func (r *controllerRoot) CrossModelQuery(ctx context.Context, req apiparams.CrossModelQueryRequest) (apiparams.CrossModelQueryResponse, error) {
	const op = errors.Op("jujuapi.CrossModelQuery")

	usersModels, err := r.jimm.Database.GetUserModels(ctx, r.user.User)
	if err != nil {
		return apiparams.CrossModelQueryResponse{}, errors.E(op, errors.Code("failed to get models for user"))
	}
	models := make([]dbmodel.Model, len(usersModels))
	for i, m := range usersModels {
		models[i] = m.Model_
	}
	switch strings.TrimSpace(strings.ToLower(req.Type)) {
	case "jq":
		return r.jimm.QueryModelsJq(ctx, models, req.Query)
	case "jimmsql":
		return apiparams.CrossModelQueryResponse{}, errors.E(op, errors.CodeNotImplemented)
	default:
		return apiparams.CrossModelQueryResponse{}, errors.E(op, errors.Code("invalid query type"), "unable to query models")
	}
}
