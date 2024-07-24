// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/juju/juju/core/network"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jujuapi/rpc"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	apiparams "github.com/canonical/jimmapi/params"
)

func init() {
	facadeInit["JIMM"] = func(r *controllerRoot) []int {
		addControllerMethod := rpc.Method(r.AddController)
		disableControllerUUIDMaskingMethod := rpc.Method(r.DisableControllerUUIDMasking)
		findAuditEventsMethod := rpc.Method(r.FindAuditEvents)
		grantAuditLogAccessMethod := rpc.Method(r.GrantAuditLogAccess)
		importModelMethod := rpc.Method(r.ImportModel)
		listControllersMethod := rpc.Method(r.ListControllers)
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
		purgeLogsMethod := rpc.Method(r.PurgeLogs)
		migrateModel := rpc.Method(r.MigrateModel)
		addServiceAccountMethod := rpc.Method(r.AddServiceAccount)
		copyServiceAccountCredentialMethod := rpc.Method(r.CopyServiceAccountCredential)
		updateServiceAccountCredentials := rpc.Method(r.UpdateServiceAccountCredentials)
		listServiceAccountCredentials := rpc.Method(r.ListServiceAccountCredentials)
		grantServiceAccountAccess := rpc.Method(r.GrantServiceAccountAccess)

		// JIMM Generic RPC
		r.AddMethod("JIMM", 4, "AddController", addControllerMethod)
		r.AddMethod("JIMM", 4, "DisableControllerUUIDMasking", disableControllerUUIDMaskingMethod)
		r.AddMethod("JIMM", 4, "FindAuditEvents", findAuditEventsMethod)
		r.AddMethod("JIMM", 4, "FullModelStatus", fullModelStatusMethod)
		r.AddMethod("JIMM", 4, "GrantAuditLogAccess", grantAuditLogAccessMethod)
		r.AddMethod("JIMM", 4, "ImportModel", importModelMethod)
		r.AddMethod("JIMM", 4, "ListControllers", listControllersMethod)
		r.AddMethod("JIMM", 4, "RemoveController", removeControllerMethod)
		r.AddMethod("JIMM", 4, "RevokeAuditLogAccess", revokeAuditLogAccessMethod)
		r.AddMethod("JIMM", 4, "SetControllerDeprecated", setControllerDeprecatedMethod)
		r.AddMethod("JIMM", 4, "UpdateMigratedModel", updateMigratedModelMethod)
		r.AddMethod("JIMM", 4, "AddCloudToController", addCloudToControllerMethod)
		r.AddMethod("JIMM", 4, "RemoveCloudFromController", removeCloudFromControllerMethod)
		r.AddMethod("JIMM", 4, "PurgeLogs", purgeLogsMethod)
		r.AddMethod("JIMM", 4, "MigrateModel", migrateModel)
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
		// JIMM Service Accounts
		r.AddMethod("JIMM", 4, "AddServiceAccount", addServiceAccountMethod)
		r.AddMethod("JIMM", 4, "CopyServiceAccountCredential", copyServiceAccountCredentialMethod)
		r.AddMethod("JIMM", 4, "UpdateServiceAccountCredentials", updateServiceAccountCredentials)
		r.AddMethod("JIMM", 4, "ListServiceAccountCredentials", listServiceAccountCredentials)
		r.AddMethod("JIMM", 4, "GrantServiceAccountAccess", grantServiceAccountAccess)

		return []int{4}
	}
}

// DisableControllerUUIDMasking ensures that the controller UUID returned
// with any model information is the UUID of the juju controller that is
// hosting the model, and not JAAS.
func (r *controllerRoot) DisableControllerUUIDMasking(ctx context.Context) error {
	const op = errors.Op("jujuapi.DisableControllerUUIDMasking")

	if !r.user.JimmAdmin {
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

// AddCloudToController adds the specified cloud to a specific controller.
func (r *controllerRoot) AddCloudToController(ctx context.Context, req apiparams.AddCloudToControllerRequest) error {
	const op = errors.Op("jujuapi.AddCloudToController")
	force := false
	if req.Force != nil && *req.Force {
		force = true
	}
	if err := r.jimm.AddCloudToController(ctx, r.user, req.ControllerName, names.NewCloudTag(req.Name), req.Cloud, force); err != nil {
		return errors.E(op, err)
	}
	return nil
}

// AddController allows adds a controller to the pool of controllers
// available to JIMM.
func (r *controllerRoot) AddController(ctx context.Context, req apiparams.AddControllerRequest) (apiparams.ControllerInfo, error) {
	const op = errors.Op("jujuapi.AddController")

	if req.Name == jimmControllerName {
		return apiparams.ControllerInfo{}, errors.E(op, errors.CodeBadRequest, fmt.Sprintf("cannot add a controller with name %q", jimmControllerName))
	}
	if req.PublicAddress != "" {
		host, port, err := net.SplitHostPort(req.PublicAddress)
		if err != nil {
			return apiparams.ControllerInfo{}, errors.E(op, err, errors.CodeBadRequest)
		}
		if host == "" {
			return apiparams.ControllerInfo{}, errors.E(op, fmt.Sprintf("address %s: host not specified in public address", req.PublicAddress), errors.CodeBadRequest)
		}
		if port == "" {
			return apiparams.ControllerInfo{}, errors.E(op, fmt.Sprintf("address %s: port not specified in public address", req.PublicAddress), errors.CodeBadRequest)
		}
	}

	ctl := dbmodel.Controller{
		UUID:              req.UUID,
		Name:              req.Name,
		PublicAddress:     req.PublicAddress,
		CACertificate:     req.CACertificate,
		AdminIdentityName: req.Username,
		AdminPassword:     req.Password,
		TLSHostname:       req.TLSHostname,
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

// ListControllers returns the list of juju controllers hosting models
// as part of this JAAS system.
func (r *controllerRoot) ListControllers(ctx context.Context) (apiparams.ListControllersResponse, error) {
	const op = errors.Op("jujuapi.ListControllersV3")

	if !r.user.JimmAdmin {
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
	err := r.jimm.DB().ForEachController(ctx, func(ctl *dbmodel.Controller) error {
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
	if err := r.jimm.DB().GetController(ctx, &ctl); err != nil {
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
	if err := r.jimm.DB().GetController(ctx, &ctl); err != nil {
		return apiparams.ControllerInfo{}, errors.E(op, err)
	}
	return ctl.ToAPIControllerInfo(), nil
}

// maxLimit is the maximum number of audit-log entries that will be
// returned from the audit log, no matter how many are requested.
const maxLimit = 1000
const limitDefault = 50

func auditParamsToFilter(req apiparams.FindAuditEventsRequest) (db.AuditLogFilter, error) {
	var filter db.AuditLogFilter
	var err error
	filter.Method = req.Method
	filter.Model = req.Model
	filter.SortTime = req.SortTime

	if req.After != "" {
		filter.Start, err = time.Parse(time.RFC3339, req.After)
		if err != nil {
			return filter, errors.E(err, errors.CodeBadRequest, `invalid "after" filter`)
		}
	}
	if req.Before != "" {
		filter.End, err = time.Parse(time.RFC3339, req.Before)
		if err != nil {
			return filter, errors.E(err, errors.CodeBadRequest, `invalid "before" filter`)
		}
	}
	if req.UserTag != "" {
		tag, err := names.ParseUserTag(req.UserTag)
		if err != nil {
			return filter, errors.E(err, errors.CodeBadRequest, `invalid "user-tag" filter`)
		}
		filter.IdentityTag = tag.String()
	}

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
	return filter, nil
}

// FindAuditEvents finds the audit-log entries that match the given filter.
func (r *controllerRoot) FindAuditEvents(ctx context.Context, req apiparams.FindAuditEventsRequest) (apiparams.AuditEvents, error) {
	const op = errors.Op("jujuapi.FindAuditEvents")
	filter, err := auditParamsToFilter(req)
	if err != nil {
		return apiparams.AuditEvents{}, errors.E(op, err)
	}
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
		return errors.E(op, err, errors.CodeBadRequest)
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
		return errors.E(op, err, errors.CodeBadRequest)
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

	if !r.user.JimmAdmin {
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

	err = r.jimm.ImportModel(ctx, r.user, req.Controller, mt, req.Owner)
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

	modelUUIDs, err := r.user.ListModels(ctx, ofganames.ReaderRelation)
	if err != nil {
		return apiparams.CrossModelQueryResponse{}, errors.E(op, errors.Code("failed to list user's model access"))
	}
	models, err := r.jimm.DB().GetModelsByUUID(ctx, modelUUIDs)
	if err != nil {
		return apiparams.CrossModelQueryResponse{}, errors.E(op, errors.Code("failed to get models for user"))
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

// PurgeLogs removes all audit log entries older than the specified date.
func (r *controllerRoot) PurgeLogs(ctx context.Context, req apiparams.PurgeLogsRequest) (apiparams.PurgeLogsResponse, error) {
	const op = errors.Op("jujuapi.PurgeLogs")

	deleted_count, err := r.jimm.PurgeLogs(ctx, r.user, req.Date)
	if err != nil {
		return apiparams.PurgeLogsResponse{}, errors.E(op, err)
	}
	return apiparams.PurgeLogsResponse{
		DeletedCount: deleted_count,
	}, nil
}

// MigrateModel is a JIMM specific method for migrating models between two controllers that
// are already attached to JIMM. See InitiateMigration in controller.go to migrate a model
// in a controller attached to JIMM to one not managed by JIMM.
func (r *controllerRoot) MigrateModel(ctx context.Context, args apiparams.MigrateModelRequest) (jujuparams.InitiateMigrationResults, error) {
	const op = errors.Op("jujuapi.MigrateModel")

	results := make([]jujuparams.InitiateMigrationResult, len(args.Specs))
	for i, arg := range args.Specs {
		mt, err := names.ParseModelTag(arg.ModelTag)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err))
			continue
		}
		result, err := r.jimm.InitiateInternalMigration(ctx, r.user, mt, arg.TargetController)
		if err != nil {
			result.Error = mapError(errors.E(op, err))
		}
		results[i] = result
	}

	return jujuparams.InitiateMigrationResults{
		Results: results,
	}, nil
}
