// Copyright 2025 Canonical.

package jujuapi

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/core/network"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuapi/rpc"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/pkg/api/params"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
	"github.com/canonical/jimm/v3/version"
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
		getGroupMethod := rpc.Method(r.GetGroup)
		renameGroupMethod := rpc.Method(r.RenameGroup)
		removeGroupMethod := rpc.Method(r.RemoveGroup)
		listGroupsMethod := rpc.Method(r.ListGroups)
		addRelationMethod := rpc.Method(r.AddRelation)
		removeRelationMethod := rpc.Method(r.RemoveRelation)
		checkRelationMethod := rpc.Method(r.CheckRelation)
		checkRelationsMethod := rpc.Method(r.CheckRelations)
		listRelationshipTuplesMethod := rpc.Method(r.ListRelationshipTuples)
		addRoleMethod := rpc.Method(r.AddRole)
		getRoleMethod := rpc.Method(r.GetRole)
		renameRoleMethod := rpc.Method(r.RenameRole)
		removeRoleMethod := rpc.Method(r.RemoveRole)
		listRolesMethod := rpc.Method(r.ListRoles)
		crossModelQueryMethod := rpc.Method(r.CrossModelQuery)
		purgeLogsMethod := rpc.Method(r.PurgeLogs)
		migrateModel := rpc.Method(r.MigrateModel)
		version := rpc.Method(r.Version)
		prepareModelMigration := rpc.Method(r.PrepareModelMigration)
		listMigrationTargetsMethod := rpc.Method(r.ListMigrationTargets)
		bootstrapStatus := rpc.Method(r.BootstrapStatus)
		bootstrapStart := rpc.Method(r.BootstrapStart)
		bootstrapStop := rpc.Method(r.BootstrapStop)

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
		r.AddMethod("JIMM", 4, "GetGroup", getGroupMethod)
		r.AddMethod("JIMM", 4, "RenameGroup", renameGroupMethod)
		r.AddMethod("JIMM", 4, "RemoveGroup", removeGroupMethod)
		r.AddMethod("JIMM", 4, "ListGroups", listGroupsMethod)
		r.AddMethod("JIMM", 4, "AddRelation", addRelationMethod)
		r.AddMethod("JIMM", 4, "RemoveRelation", removeRelationMethod)
		r.AddMethod("JIMM", 4, "CheckRelation", checkRelationMethod)
		r.AddMethod("JIMM", 4, "CheckRelations", checkRelationsMethod)
		r.AddMethod("JIMM", 4, "ListRelationshipTuples", listRelationshipTuplesMethod)
		r.AddMethod("JIMM", 4, "AddRole", addRoleMethod)
		r.AddMethod("JIMM", 4, "GetRole", getRoleMethod)
		r.AddMethod("JIMM", 4, "RenameRole", renameRoleMethod)
		r.AddMethod("JIMM", 4, "RemoveRole", removeRoleMethod)
		r.AddMethod("JIMM", 4, "ListRoles", listRolesMethod)
		// JIMM Cross-model queries
		r.AddMethod("JIMM", 4, "CrossModelQuery", crossModelQueryMethod)
		r.AddMethod("JIMM", 4, "Version", version)
		// JIMM Model Migrations
		r.AddMethod("JIMM", 4, "PrepareModelMigration", prepareModelMigration)
		r.AddMethod("JIMM", 4, "ListMigrationTargets", listMigrationTargetsMethod)
		// JIMM Bootstrap
		r.AddMethod("JIMM", 4, "BootstrapStatus", bootstrapStatus)
		r.AddMethod("JIMM", 4, "BootstrapStart", bootstrapStart)
		r.AddMethod("JIMM", 4, "BootstrapStop", bootstrapStop)

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
	cloud := cloudFromParams(req.Name, req.Cloud)
	if err := r.jimm.JujuManager().AddCloudToController(ctx, r.user, req.ControllerName, names.NewCloudTag(req.Name), cloud, force); err != nil {
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

	// TODO(ale8k): Don't build dbmodel here, do it as params to AddController.
	ctl := dbmodel.Controller{
		UUID:          req.UUID,
		Name:          req.Name,
		PublicAddress: req.PublicAddress,
		CACertificate: req.CACertificate,
		TLSHostname:   req.TLSHostname,
		Addresses:     dbmodel.HostPorts{jujuparams.FromProviderHostPorts(nphps)},
	}
	ctlCreds := juju.ControllerCreds{
		AdminIdentityName: req.Username,
		AdminPassword:     req.Password,
	}
	if err := r.jimm.JujuManager().AddController(ctx, r.user, &ctl, ctlCreds); err != nil {
		zapctx.Error(ctx, "failed to add controller", zaputil.Error(err))
		return apiparams.ControllerInfo{}, errors.E(op, err)
	}
	return ctl.ToAPIControllerInfo(), nil
}

// ListControllers returns the list of juju controllers hosting models
// as part of this JAAS system.
// If the user is not an admin, they will only receive information about
// JIMM itself - note that the controller name returned is "jaas".
func (r *controllerRoot) ListControllers(ctx context.Context) (apiparams.ListControllersResponse, error) {
	const op = errors.Op("jujuapi.ListControllersV3")

	if !r.user.JimmAdmin {
		// if the user isn't a controller admin return JAAS
		// itself as the only controller.
		srvVersion, err := r.jimm.JujuManager().EarliestControllerVersion(ctx)
		if err != nil {
			return apiparams.ListControllersResponse{}, errors.E(op, err)
		}
		jimmCtl := params.ControllerInfo{
			Name: "jaas",
			UUID: r.params.ControllerUUID,
			// TODO(mhilton)enable setting the public address.
			AgentVersion: srvVersion.String(),
			Status: jujuparams.EntityStatus{
				Status: "available",
			},
		}
		controllers := []apiparams.ControllerInfo{jimmCtl}
		return apiparams.ListControllersResponse{Controllers: controllers}, nil
	}
	dbControllers, err := r.jimm.JujuManager().ListControllers(ctx, r.user)
	if err != nil {
		return apiparams.ListControllersResponse{}, errors.E(op, err)
	}
	controllersInfo := make([]apiparams.ControllerInfo, 0, len(dbControllers))
	for _, ctl := range dbControllers {
		controllersInfo = append(controllersInfo, ctl.ToAPIControllerInfo())
	}
	return apiparams.ListControllersResponse{
		Controllers: controllersInfo,
	}, nil
}

// RemoveController removes a controller.
func (r *controllerRoot) RemoveController(ctx context.Context, req apiparams.RemoveControllerRequest) (apiparams.ControllerInfo, error) {
	const op = errors.Op("jujuapi.RemoveController")

	ctl, err := r.jimm.JujuManager().ControllerInfo(ctx, req.Name)
	if err != nil {
		return apiparams.ControllerInfo{}, errors.E(op, err)
	}

	if err := r.jimm.JujuManager().RemoveController(ctx, r.user, req.Name, req.Force); err != nil {
		return apiparams.ControllerInfo{}, errors.E(op, err)
	}
	return ctl.ToAPIControllerInfo(), nil
}

// SetControllerDeprecated sets the deprecated status of a controller.
func (r *controllerRoot) SetControllerDeprecated(ctx context.Context, req apiparams.SetControllerDeprecatedRequest) (apiparams.ControllerInfo, error) {
	const op = errors.Op("jujuapi.SetControllerDeprecated")

	if err := r.jimm.JujuManager().SetControllerDeprecated(ctx, r.user, req.Name, req.Deprecated); err != nil {
		return apiparams.ControllerInfo{}, errors.E(op, err)
	}
	ctl, err := r.jimm.JujuManager().ControllerInfo(ctx, req.Name)
	if err != nil {
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
	entries, err := r.jimm.AuditLogManager().FindAuditEvents(ctx, r.user, filter)
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

	err = r.jimm.PermissionManager().GrantAuditLogAccess(ctx, r.user, ut)
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

	err = r.jimm.PermissionManager().RevokeAuditLogAccess(ctx, r.user, ut)
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

	status, err := r.jimm.JujuManager().FullModelStatus(ctx, r.user, mt, req.Patterns)
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
	err = r.jimm.JujuManager().UpdateMigratedModel(ctx, r.user, mt, req.TargetController)
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

	err = r.jimm.JujuManager().ImportModel(ctx, r.user, req.Controller, mt, req.Owner)
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
	if err := r.jimm.JujuManager().RemoveCloudFromController(ctx, r.user, req.ControllerName, ct); err != nil {
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

	switch strings.TrimSpace(strings.ToLower(req.Type)) {
	case "jq":
		return r.jimm.JujuManager().QueryModelsJq(ctx, modelUUIDs, req.Query)
	case "jimmsql":
		return apiparams.CrossModelQueryResponse{}, errors.E(op, errors.CodeNotImplemented)
	default:
		return apiparams.CrossModelQueryResponse{}, errors.E(op, errors.Code("invalid query type"), "unable to query models")
	}
}

// PurgeLogs removes all audit log entries older than the specified date.
func (r *controllerRoot) PurgeLogs(ctx context.Context, req apiparams.PurgeLogsRequest) (apiparams.PurgeLogsResponse, error) {
	const op = errors.Op("jujuapi.PurgeLogs")

	deleted_count, err := r.jimm.AuditLogManager().PurgeLogs(ctx, r.user, req.Date)
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
		result, err := r.jimm.JujuManager().InitiateInternalMigration(ctx, r.user, arg.TargetModelNameOrUUID, arg.TargetController)
		if err != nil {
			result.Error = mapError(errors.E(op, err))
		}
		results[i] = result
	}

	return jujuparams.InitiateMigrationResults{
		Results: results,
	}, nil
}

// Version is a method on the JIMM facade that returns information on the version of JIMM.
func (r *controllerRoot) Version(ctx context.Context) (apiparams.VersionResponse, error) {
	versionInfo := apiparams.VersionResponse{
		Version: version.VersionInfo.Version,
		Commit:  version.VersionInfo.GitCommit,
	}
	return versionInfo, nil
}

// PrepareModelMigration prepares JIMM for an incoming migration.
func (r *controllerRoot) PrepareModelMigration(ctx context.Context, args apiparams.PrepareModelMigrationRequest) (apiparams.PrepareModelMigrationResponse, error) {
	const op = errors.Op("jujuapi.PrepareModelMigration")
	resp := apiparams.PrepareModelMigrationResponse{}

	if !r.user.JimmAdmin {
		return resp, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	mt, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return resp, errors.E(op, "invalid model tag", err)
	}

	if !names.IsValidControllerName(args.BackingControllerName) {
		return resp, errors.E(op, "invalid controller name")
	}

	// Check each key is a valid local user and each value is a valid user and has a domain
	for local, external := range args.UserMapping {
		if !names.IsValidUserName(local) {
			return resp, errors.E(op, fmt.Sprintf("%s is not a valid local user name", local))
		}

		if external == "" {
			// The external user can be empty meaning that we are
			// intentionally skipping the mapping for this local user.
			continue
		}

		if !names.IsValidUser(external) || !strings.Contains(external, "@") {
			return resp, errors.E(op, fmt.Sprintf("%s is not a valid external user name", external))
		}
	}

	resp.Token, err = r.jimm.JujuManager().PrepareModelMigration(ctx, r.user, mt.Id(), args.BackingControllerName, args.UserMapping)
	if err != nil {
		return resp, errors.E(op, err)
	}

	return resp, nil
}

// ListMigrationTargets returns the list of juju controllers that the given internal
// model could be migrated to. This includes controllers that support the model's
// cloud region and version, but excludes the controller the model is already on.
func (r *controllerRoot) ListMigrationTargets(ctx context.Context, req apiparams.ListMigrationTargetsRequest) (apiparams.ListControllersResponse, error) {
	const op = errors.Op("jujuapi.ListMigrationTargets")

	mt, err := names.ParseModelTag(req.ModelTag)
	if err != nil {
		return apiparams.ListControllersResponse{}, errors.E(op, err, errors.CodeBadRequest)
	}

	dbControllers, err := r.jimm.JujuManager().ListMigrationTargets(ctx, r.user, mt)
	if err != nil {
		return apiparams.ListControllersResponse{}, errors.E(op, err)
	}
	controllersInfo := make([]apiparams.ControllerInfo, 0, len(dbControllers))
	for _, ctl := range dbControllers {
		controllersInfo = append(controllersInfo, ctl.ToAPIControllerInfo())
	}
	return apiparams.ListControllersResponse{
		Controllers: controllersInfo,
	}, nil
}

// BootstrapStatus retrieves the status of a bootstrap job, its logs and the watermark
// for the logs.
func (r *controllerRoot) BootstrapStatus(ctx context.Context, req apiparams.BootstrapStatusRequest) (apiparams.BootstrapStatusResponse, error) {
	const op = errors.Op("jujuapi.BootstrapStatus")

	if !r.user.JimmAdmin {
		return apiparams.BootstrapStatusResponse{}, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	jobId, err := uuid.Parse(req.JobID)
	if err != nil {
		return apiparams.BootstrapStatusResponse{}, errors.E(op, errors.CodeBadRequest, "invalid job ID", err)
	}

	return r.jimm.BootstrapManager().GetBootstrapStatusAndLogs(ctx, r.user, jobId, req.Watermark)
}

// BootstrapStart starts a bootstrap job.
func (r *controllerRoot) BootstrapStart(ctx context.Context, req apiparams.BootstrapStartParams) (apiparams.BootstrapStartResponse, error) {
	const op = errors.Op("jujuapi.BootstrapStart")

	if !r.user.JimmAdmin {
		return apiparams.BootstrapStartResponse{}, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	// Check built in clouds like localhost (lxd).
	builtinClouds, err := common.BuiltInClouds()
	if err != nil {
		return apiparams.BootstrapStartResponse{}, errors.E(op, errors.CodeIncompatibleClouds, "unauthorized")
	}

	if _, isABuiltinCloud := builtinClouds[req.CloudName]; isABuiltinCloud {
		return apiparams.BootstrapStartResponse{},
			errors.E(op, errors.CodeIncompatibleClouds, fmt.Errorf("bootstrap via JIMM does not support built-in clouds like %q", req.CloudName))
	}

	cloudNameAndRegion := req.CloudName

	if req.RegionName != "" {
		cloudNameAndRegion = fmt.Sprintf("%s/%s", req.CloudName, req.RegionName)
	}

	params := bootstrap.BootstrapParams{
		CLIVersion: "3.6.8",

		CloudNameAndRegion: cloudNameAndRegion,
		ControllerName:     req.ControllerName,
		BootstrapTimeout:   req.Flags.Timeout,

		PersonalCloud: cloudFromParams(req.CloudName, req.Cloud),
		CloudCred:     req.Credential,

		PublicDNSAddress:       req.Flags.PublicDNSAddress,
		ControllerServiceType:  req.Flags.ControllerServiceType,
		ControllerExternalIPs:  req.Flags.ControllerExternalIPs,
		ControllerExternalName: req.Flags.ControllerExternalName,
	}

	jobID, err := r.jimm.BootstrapManager().StartBootstrap(ctx, r.user, params)
	if err != nil {
		return apiparams.BootstrapStartResponse{}, errors.E(op, fmt.Errorf("failed to start bootstrap job: %v", err))
	}
	return apiparams.BootstrapStartResponse{
		JobID: jobID,
	}, nil
}

// BootstrapStop stops a bootstrap job.
func (r *controllerRoot) BootstrapStop(ctx context.Context, req apiparams.BootstrapStopRequest) error {
	const op = errors.Op("jujuapi.BootstrapStop")

	if !r.user.JimmAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	jobID, err := uuid.Parse(req.JobID)
	if err != nil {
		return errors.E(op, errors.CodeBadRequest, "invalid job ID", err)
	}

	err = r.jimm.BootstrapManager().StopBootstrap(ctx, r.user, jobID)
	if err != nil {
		return errors.E(op, fmt.Errorf("failed to stop bootstrap job: %v", err))
	}
	return nil
}
