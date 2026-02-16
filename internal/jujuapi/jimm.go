// Copyright 2026 Canonical.

package jujuapi

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/core/network"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	jujuversion "github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuapi/rpc"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/servermon"
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
		addModelToControllerMethod := rpc.Method(r.AddModelToController)
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
		getBootstrapInfo := rpc.Method(r.GetBootstrapInfo)
		stopBootstrap := rpc.Method(r.StopBootstrap)
		startBootstrap := rpc.Method(r.StartBootstrap)
		startDestroyController := rpc.Method(r.StartDestroyController)
		upgradeToMethod := rpc.Method(r.UpgradeTo)
		listUserCloudsMethod := rpc.Method(r.ListUserClouds)
		modelControllerInfoMethod := rpc.Method(r.ModelControllerInfo)

		// JIMM Generic RPC
		r.AddMethod("JIMM", 4, "AddCloudToController", addCloudToControllerMethod)
		r.AddMethod("JIMM", 4, "AddController", addControllerMethod)
		r.AddMethod("JIMM", 4, "AddModelToController", addModelToControllerMethod)
		r.AddMethod("JIMM", 4, "DisableControllerUUIDMasking", disableControllerUUIDMaskingMethod)
		r.AddMethod("JIMM", 4, "FindAuditEvents", findAuditEventsMethod)
		r.AddMethod("JIMM", 4, "FullModelStatus", fullModelStatusMethod)
		r.AddMethod("JIMM", 4, "GrantAuditLogAccess", grantAuditLogAccessMethod)
		r.AddMethod("JIMM", 4, "ImportModel", importModelMethod)
		r.AddMethod("JIMM", 4, "ListControllers", listControllersMethod)
		r.AddMethod("JIMM", 4, "ListUserClouds", listUserCloudsMethod)
		r.AddMethod("JIMM", 4, "MigrateModel", migrateModel)
		r.AddMethod("JIMM", 4, "ModelControllerInfo", modelControllerInfoMethod)
		r.AddMethod("JIMM", 4, "PurgeLogs", purgeLogsMethod)
		r.AddMethod("JIMM", 4, "RemoveCloudFromController", removeCloudFromControllerMethod)
		r.AddMethod("JIMM", 4, "RemoveController", removeControllerMethod)
		r.AddMethod("JIMM", 4, "RevokeAuditLogAccess", revokeAuditLogAccessMethod)
		r.AddMethod("JIMM", 4, "SetControllerDeprecated", setControllerDeprecatedMethod)
		r.AddMethod("JIMM", 4, "UpdateMigratedModel", updateMigratedModelMethod)

		// JIMM ReBAC RPC
		r.AddMethod("JIMM", 4, "AddGroup", addGroupMethod)
		r.AddMethod("JIMM", 4, "AddRelation", addRelationMethod)
		r.AddMethod("JIMM", 4, "AddRole", addRoleMethod)
		r.AddMethod("JIMM", 4, "CheckRelation", checkRelationMethod)
		r.AddMethod("JIMM", 4, "CheckRelations", checkRelationsMethod)
		r.AddMethod("JIMM", 4, "GetGroup", getGroupMethod)
		r.AddMethod("JIMM", 4, "GetRole", getRoleMethod)
		r.AddMethod("JIMM", 4, "ListGroups", listGroupsMethod)
		r.AddMethod("JIMM", 4, "ListRelationshipTuples", listRelationshipTuplesMethod)
		r.AddMethod("JIMM", 4, "ListRoles", listRolesMethod)
		r.AddMethod("JIMM", 4, "RemoveGroup", removeGroupMethod)
		r.AddMethod("JIMM", 4, "RemoveRelation", removeRelationMethod)
		r.AddMethod("JIMM", 4, "RemoveRole", removeRoleMethod)
		r.AddMethod("JIMM", 4, "RenameGroup", renameGroupMethod)
		r.AddMethod("JIMM", 4, "RenameRole", renameRoleMethod)
		// JIMM Cross-model queries
		r.AddMethod("JIMM", 4, "CrossModelQuery", crossModelQueryMethod)
		r.AddMethod("JIMM", 4, "Version", version)
		// JIMM Model Migrations
		r.AddMethod("JIMM", 4, "ListMigrationTargets", listMigrationTargetsMethod)
		r.AddMethod("JIMM", 4, "PrepareModelMigration", prepareModelMigration)
		// JIMM Bootstrap
		r.AddMethod("JIMM", 4, "BootstrapInfo", getBootstrapInfo)
		r.AddMethod("JIMM", 4, "StartBootstrap", startBootstrap)
		r.AddMethod("JIMM", 4, "StartDestroyController", startDestroyController)
		r.AddMethod("JIMM", 4, "StopBootstrap", stopBootstrap)
		// JIMM Upgrades
		r.AddMethod("JIMM", 4, "UpgradeTo", upgradeToMethod)

		return []int{4}
	}
}

// DisableControllerUUIDMasking ensures that the controller UUID returned
// with any model information is the UUID of the juju controller that is
// hosting the model, and not JAAS.
func (r *controllerRoot) DisableControllerUUIDMasking(ctx context.Context) error {

	if !r.user.JimmAdmin {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
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
	force := req.Force != nil && *req.Force

	cloud := cloudFromParams(req.Name, req.Cloud)
	if err := r.jimm.JujuManager().AddCloudToController(ctx, r.user, req.ControllerName, names.NewCloudTag(req.Name), cloud, force); err != nil {
		return errors.E(err)
	}
	return nil
}

// AddModelToController adds a new model to a specific controller.
func (r *controllerRoot) AddModelToController(ctx context.Context, req apiparams.AddModelToControllerRequest) (jujuparams.ModelInfo, error) {
	mca, err := toAddModelArgs(req.ModelCreateArgs, r.user.ResourceTag())
	if err != nil {
		return jujuparams.ModelInfo{}, errors.E(err)
	}
	// Add JIMM specific field.
	mca.ControllerName = req.ControllerName

	info, err := r.jimm.JujuManager().AddModel(ctx, r.user, mca)
	if err != nil {
		servermon.ModelsCreatedFailCount.Inc()
		return jujuparams.ModelInfo{}, errors.E(err)
	}

	servermon.ModelsCreatedCount.Inc()
	if r.controllerUUIDMasking {
		info.ControllerUUID = r.params.ControllerUUID
	}
	return toModelInfo(info), nil
}

// AddController allows adds a controller to the pool of controllers
// available to JIMM.
func (r *controllerRoot) AddController(ctx context.Context, req apiparams.AddControllerRequest) (apiparams.ControllerInfo, error) {
	if !r.user.JimmAdmin {
		return apiparams.ControllerInfo{}, errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	if req.Name == jimmControllerName {
		return apiparams.ControllerInfo{}, errors.E(errors.CodeBadRequest, fmt.Sprintf("cannot add a controller with name %q", jimmControllerName))
	}
	if req.PublicAddress != "" {
		host, port, err := net.SplitHostPort(req.PublicAddress)
		if err != nil {
			return apiparams.ControllerInfo{}, errors.E(err, errors.CodeBadRequest)
		}
		if host == "" {
			return apiparams.ControllerInfo{}, errors.E(fmt.Sprintf("address %s: host not specified in public address", req.PublicAddress), errors.CodeBadRequest)
		}
		if port == "" {
			return apiparams.ControllerInfo{}, errors.E(fmt.Sprintf("address %s: port not specified in public address", req.PublicAddress), errors.CodeBadRequest)
		}
	}

	nphps, err := network.ParseProviderHostPorts(req.APIAddresses...)
	if err != nil {
		return apiparams.ControllerInfo{}, errors.E(errors.CodeBadRequest, err)
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
		return apiparams.ControllerInfo{}, errors.E(fmt.Errorf("failed to add controller: %w", err))
	}
	return ctl.ToAPIControllerInfo(), nil
}

// ListControllers returns the list of juju controllers the user has can_addmodel access to on the controller.
func (r *controllerRoot) ListControllers(ctx context.Context) (apiparams.ListControllersResponse, error) {
	dbControllers, err := r.jimm.JujuManager().ListControllers(ctx, r.user)
	if err != nil {
		return apiparams.ListControllersResponse{}, errors.E(err)
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
	if !r.user.JimmAdmin {
		return apiparams.ControllerInfo{}, errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	ctl, err := r.jimm.JujuManager().ControllerInfo(ctx, req.Name)
	if err != nil {
		return apiparams.ControllerInfo{}, errors.E(err)
	}

	if err := r.jimm.JujuManager().RemoveController(ctx, r.user, req.Name, req.Force); err != nil {
		return apiparams.ControllerInfo{}, errors.E(err)
	}
	return ctl.ToAPIControllerInfo(), nil
}

// SetControllerDeprecated sets the deprecated status of a controller.
func (r *controllerRoot) SetControllerDeprecated(ctx context.Context, req apiparams.SetControllerDeprecatedRequest) (apiparams.ControllerInfo, error) {

	if err := r.jimm.JujuManager().SetControllerDeprecated(ctx, r.user, req.Name, req.Deprecated); err != nil {
		return apiparams.ControllerInfo{}, errors.E(err)
	}
	ctl, err := r.jimm.JujuManager().ControllerInfo(ctx, req.Name)
	if err != nil {
		return apiparams.ControllerInfo{}, errors.E(err)
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

	filter, err := auditParamsToFilter(req)
	if err != nil {
		return apiparams.AuditEvents{}, errors.E(err)
	}
	entries, err := r.jimm.AuditLogManager().FindAuditEvents(ctx, r.user, filter)
	if err != nil {
		return apiparams.AuditEvents{}, errors.E(err)
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

	ut, err := parseUserTag(req.UserTag)
	if err != nil {
		return errors.E(err, errors.CodeBadRequest)
	}

	err = r.jimm.PermissionManager().GrantAuditLogAccess(ctx, r.user, ut)
	if err != nil {
		return errors.E(err)
	}
	return nil
}

// RevokeAuditLogAccess revokes access to the audit log at the specified
// level from the specified user. The only currently supported level is
// "read". Only controller admin users can revoke access to the audit log.
func (r *controllerRoot) RevokeAuditLogAccess(ctx context.Context, req apiparams.AuditLogAccessRequest) error {

	ut, err := parseUserTag(req.UserTag)
	if err != nil {
		return errors.E(err, errors.CodeBadRequest)
	}

	err = r.jimm.PermissionManager().RevokeAuditLogAccess(ctx, r.user, ut)
	if err != nil {
		return errors.E(err)
	}
	return nil
}

// FullModelStatus returns the full status of the juju model.
func (r *controllerRoot) FullModelStatus(ctx context.Context, req apiparams.FullModelStatusRequest) (jujuparams.FullStatus, error) {

	mt, err := names.ParseModelTag(req.ModelTag)
	if err != nil {
		return jujuparams.FullStatus{}, errors.E(err, errors.CodeBadRequest)
	}

	status, err := r.jimm.JujuManager().FullModelStatus(ctx, r.user, mt, req.Patterns)
	if err != nil {
		return jujuparams.FullStatus{}, errors.E(err)
	}

	return *status, nil
}

// UpdateMigratedModel checks that the model has been migrated to the specified controller
// and updates internal representation of the model.
func (r *controllerRoot) UpdateMigratedModel(ctx context.Context, req apiparams.UpdateMigratedModelRequest) error {

	if !r.user.JimmAdmin {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	mt, err := names.ParseModelTag(req.ModelTag)
	if err != nil {
		return errors.E(err, errors.CodeBadRequest)
	}
	err = r.jimm.JujuManager().UpdateMigratedModel(ctx, r.user, mt, req.TargetController)
	if err != nil {
		return errors.E(err)
	}
	return nil
}

// ImportModel imports a model already attached to a controller allowing
// management of that model in JIMM.
func (r *controllerRoot) ImportModel(ctx context.Context, req apiparams.ImportModelRequest) error {

	mt, err := names.ParseModelTag(req.ModelTag)
	if err != nil {
		return errors.E(err, errors.CodeBadRequest)
	}

	err = r.jimm.JujuManager().ImportModel(ctx, r.user, req.Controller, mt, req.Owner)
	if err != nil {
		return errors.E(err)
	}
	return nil
}

// RemoveCloudFromController removes the specified cloud from a specific controller.
func (r *controllerRoot) RemoveCloudFromController(ctx context.Context, req apiparams.RemoveCloudFromControllerRequest) error {

	ct, err := names.ParseCloudTag(req.CloudTag)
	if err != nil {
		return errors.E(err, errors.CodeBadRequest)
	}
	if err := r.jimm.JujuManager().RemoveCloudFromController(ctx, r.user, req.ControllerName, ct); err != nil {
		return errors.E(err)
	}
	return nil
}

// CrossModelQuery enables users to query all of their available models and each entity within the model.
//
// The query will run against output exactly like "juju status --format json", but for each of their models.
func (r *controllerRoot) CrossModelQuery(ctx context.Context, req apiparams.CrossModelQueryRequest) (apiparams.CrossModelQueryResponse, error) {

	modelUUIDs, err := r.user.ListModels(ctx, ofganames.ReaderRelation)
	if err != nil {
		return apiparams.CrossModelQueryResponse{}, errors.E(errors.Code("failed to list user's model access"))
	}

	switch strings.TrimSpace(strings.ToLower(req.Type)) {
	case "jq":
		return r.jimm.JujuManager().QueryModelsJq(ctx, modelUUIDs, req.Query)
	case "jimmsql":
		return apiparams.CrossModelQueryResponse{}, errors.E(errors.CodeNotImplemented)
	default:
		return apiparams.CrossModelQueryResponse{}, errors.E(errors.Code("invalid query type"), "unable to query models")
	}
}

// PurgeLogs removes all audit log entries older than the specified date.
func (r *controllerRoot) PurgeLogs(ctx context.Context, req apiparams.PurgeLogsRequest) (apiparams.PurgeLogsResponse, error) {

	deleted_count, err := r.jimm.AuditLogManager().PurgeLogs(ctx, r.user, req.Date)
	if err != nil {
		return apiparams.PurgeLogsResponse{}, errors.E(err)
	}
	return apiparams.PurgeLogsResponse{
		DeletedCount: deleted_count,
	}, nil
}

// MigrateModel is a JIMM specific method for migrating models between two controllers that
// are already attached to JIMM. See InitiateMigration in controller.go to migrate a model
// in a controller attached to JIMM to one not managed by JIMM.
func (r *controllerRoot) MigrateModel(ctx context.Context, args apiparams.MigrateModelRequest) (jujuparams.InitiateMigrationResults, error) {

	results := make([]jujuparams.InitiateMigrationResult, len(args.Specs))

	for i, arg := range args.Specs {
		result, err := r.jimm.JujuManager().InitiateInternalMigration(ctx, r.user, arg.TargetModelNameOrUUID, arg.TargetController)
		if err != nil {
			result.Error = r.mapError(ctx, errors.E(err))
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

	resp := apiparams.PrepareModelMigrationResponse{}

	if !r.user.JimmAdmin {
		return resp, errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	mt, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return resp, errors.E("invalid model tag", err)
	}

	if !names.IsValidControllerName(args.BackingControllerName) {
		return resp, errors.E("invalid controller name")
	}

	// Check each key is a valid local user and each value is a valid user and has a domain
	for local, external := range args.UserMapping {
		if !names.IsValidUserName(local) {
			return resp, errors.E(fmt.Sprintf("%s is not a valid local user name", local))
		}

		if external == "" {
			// The external user can be empty meaning that we are
			// intentionally skipping the mapping for this local user.
			continue
		}

		if !names.IsValidUser(external) || !strings.Contains(external, "@") {
			return resp, errors.E(fmt.Sprintf("%s is not a valid external user name", external))
		}
	}

	resp.Token, err = r.jimm.JujuManager().PrepareModelMigration(ctx, r.user, mt.Id(), args.BackingControllerName, args.UserMapping)
	if err != nil {
		return resp, errors.E(err)
	}

	return resp, nil
}

// ListMigrationTargets returns the list of juju controllers that the given internal
// model could be migrated to. This includes controllers that support the model's
// cloud region and version, but excludes the controller the model is already on.
func (r *controllerRoot) ListMigrationTargets(ctx context.Context, req apiparams.ListMigrationTargetsRequest) (apiparams.ListControllersResponse, error) {

	mt, err := names.ParseModelTag(req.ModelTag)
	if err != nil {
		return apiparams.ListControllersResponse{}, errors.E(err, errors.CodeBadRequest)
	}

	dbControllers, err := r.jimm.JujuManager().ListMigrationTargets(ctx, r.user, mt)
	if err != nil {
		return apiparams.ListControllersResponse{}, errors.E(err)
	}
	controllersInfo := make([]apiparams.ControllerInfo, 0, len(dbControllers))
	for _, ctl := range dbControllers {
		controllersInfo = append(controllersInfo, ctl.ToAPIControllerInfo())
	}

	return apiparams.ListControllersResponse{
		Controllers: controllersInfo,
	}, nil
}

// GetBootstrapInfo retrieves the status of a bootstrap job, its logs and the watermark
// for the logs.
func (r *controllerRoot) GetBootstrapInfo(ctx context.Context, req apiparams.GetBootstrapInfoRequest) (apiparams.GetBootstrapInfoResponse, error) {

	if !r.user.JimmAdmin {
		return apiparams.GetBootstrapInfoResponse{}, errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	jobID, err := strconv.ParseInt(req.JobID, 10, 64)
	if err != nil {
		return apiparams.GetBootstrapInfoResponse{}, errors.E(fmt.Sprintf("invalid job ID: %s", req.JobID), errors.CodeBadRequest)
	}

	return r.jimm.BootstrapManager().GetJobInfo(ctx, r.user, jobID, req.Watermark)
}

// StopBootstrap stops a bootstrap job.
func (r *controllerRoot) StopBootstrap(ctx context.Context, req apiparams.StopBootstrapRequest) error {

	if !r.user.JimmAdmin {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	jobID, err := strconv.ParseInt(req.JobID, 10, 64)
	if err != nil {
		return errors.E(fmt.Sprintf("invalid job ID: %s", req.JobID), errors.CodeBadRequest)
	}

	err = r.jimm.BootstrapManager().StopJob(ctx, r.user, jobID)
	if err != nil {
		return errors.E(fmt.Errorf("failed to stop job: %v", err))
	}
	return nil
}

// StartBootstrap starts a bootstrap job.
func (r *controllerRoot) StartBootstrap(ctx context.Context, req apiparams.BootstrapParams) (apiparams.StartBootstrapResponse, error) {

	if !r.user.JimmAdmin {
		return apiparams.StartBootstrapResponse{}, errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	// Check built in clouds like localhost (lxd).
	builtinClouds, err := common.BuiltInClouds()
	if err != nil {
		return apiparams.StartBootstrapResponse{}, errors.E(errors.CodeIncompatibleClouds, "unauthorized")
	}

	if _, isABuiltinCloud := builtinClouds[req.CloudName]; isABuiltinCloud {
		return apiparams.StartBootstrapResponse{},
			errors.E(errors.CodeIncompatibleClouds, fmt.Errorf("bootstrap via JIMM does not support built-in clouds like %q", req.CloudName))
	}

	cloudNameAndRegion := req.CloudName

	if req.RegionName != "" {
		cloudNameAndRegion = fmt.Sprintf("%s/%s", req.CloudName, req.RegionName)
	}

	params := bootstrap.BootstrapParams{
		CLIVersion: req.ControllerVersion,

		CloudNameAndRegion: cloudNameAndRegion,
		ControllerName:     req.ControllerName,

		Cloud: cloudFromParams(req.CloudName, req.Cloud),
		CloudCred: cloud.NewNamedCredential(
			"bootstrap-credential",
			cloud.AuthType(req.Credential.AuthType),
			req.Credential.Attributes,
			false,
		),

		UserConfig: req.Config,
	}

	jobID, err := r.jimm.BootstrapManager().StartBootstrapJob(ctx, r.user, params)
	if err != nil {
		return apiparams.StartBootstrapResponse{}, errors.E(fmt.Errorf("failed to start bootstrap job: %v", err))
	}
	return apiparams.StartBootstrapResponse{
		JobID: strconv.FormatInt(jobID, 10),
	}, nil
}

// StartDestroyController starts a destroy-controller job.
func (r *controllerRoot) StartDestroyController(ctx context.Context, req apiparams.DestroyControllerRequest) (apiparams.StartBootstrapResponse, error) {

	if !r.user.JimmAdmin {
		return apiparams.StartBootstrapResponse{}, errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	ctrl, err := r.jimm.JujuManager().ControllerInfo(ctx, req.ControllerName)
	if err != nil {
		return apiparams.StartBootstrapResponse{}, errors.E(fmt.Errorf("failed to fetch controller info: %w", err))
	}

	if len(ctrl.Models) != 0 {
		return apiparams.StartBootstrapResponse{}, errors.E(errors.CodeBadRequest, "cannot destroy controller with models")
	}

	jobID, err := r.jimm.BootstrapManager().StartDestroyControllerJob(ctx, r.user, bootstrap.DestroyControllerParams{
		ControllerName: req.ControllerName,
		ControllerUUID: ctrl.UUID,
		AgentVersion:   ctrl.AgentVersion,
		CloudName:      ctrl.CloudName,
		CloudRegion:    ctrl.CloudRegion,
		APIEndpoints:   ctrl.ToAPIControllerInfo().APIAddresses,
		PublicAddress:  ctrl.PublicAddress,
		CACertificate:  ctrl.CACertificate,
	})
	if err != nil {
		return apiparams.StartBootstrapResponse{}, errors.E(fmt.Errorf("failed to start destroy-controller job: %v", err))
	}

	return apiparams.StartBootstrapResponse{
		JobID: strconv.FormatInt(jobID, 10),
	}, nil
}

// UpgradeTo upgrades the controller hosting the given model by cloning a new controller
// at the requested version and migrating the model to it (phase 1 automated upgrade).
func (r *controllerRoot) UpgradeTo(ctx context.Context, req apiparams.UpgradeToRequest) (apiparams.UpgradeToResponse, error) {
	if !r.user.JimmAdmin {
		return apiparams.UpgradeToResponse{}, errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	mt, err := names.ParseModelTag(req.ModelTag)
	if err != nil {
		return apiparams.UpgradeToResponse{}, errors.E(errors.CodeBadRequest, fmt.Errorf("invalid model tag %q: %w", req.ModelTag, err))
	}

	targetControllerVersion, err := jujuversion.Parse(req.TargetControllerVersion)
	if err != nil {
		return apiparams.UpgradeToResponse{}, errors.E(errors.CodeBadRequest, fmt.Errorf("invalid target controller version %q: %w", req.TargetControllerVersion, err))
	}

	_, err = r.jimm.UpgradeManager().UpgradeTo(ctx, r.user, mt.Id(), targetControllerVersion)
	if err != nil {
		return apiparams.UpgradeToResponse{}, errors.E(errors.CodeBadRequest, fmt.Errorf("failed to run upgrade to: %w", err))
	}

	return apiparams.UpgradeToResponse{
		Success: true,
	}, nil
}

// ListUserClouds lists the clouds accessible to the user.
func (r *controllerRoot) ListUserClouds(ctx context.Context, req apiparams.ListUserCloudsRequest) (jujuparams.CloudsResult, error) {
	if !r.user.JimmAdmin && r.user.ResourceTag().String() != req.UserTag {
		return jujuparams.CloudsResult{}, errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	user := r.user
	if r.user.ResourceTag().String() != req.UserTag {
		ut, err := names.ParseUserTag(req.UserTag)
		if err != nil {
			return jujuparams.CloudsResult{}, errors.E(err, errors.CodeBadRequest, "invalid user tag")
		}
		u, err := r.jimm.IdentityManager().FetchIdentity(ctx, ut.Id())
		if err != nil {
			return jujuparams.CloudsResult{}, errors.E(err)
		}

		user = u
	}
	res := jujuparams.CloudsResult{
		Clouds: make(map[string]jujuparams.Cloud),
	}
	err := r.jimm.JujuManager().ForEachUserCloud(ctx, user, func(cld *dbmodel.Cloud) error {
		res.Clouds[cld.Tag().String()] = cld.ToJujuCloud()
		return nil
	})
	if err != nil {
		return res, errors.E(err)
	}
	return res, nil
}

// ModelControllerInfo returns controller information about a model.
// The model can be specified either by model UUID,
// or by the combination of ownerName and modelName parameters.
func (r *controllerRoot) ModelControllerInfo(ctx context.Context, req apiparams.ModelControllerInfoRequest) (apiparams.ModelControllerInfo, error) {
	if !r.user.JimmAdmin {
		return apiparams.ModelControllerInfo{}, errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	var qualifier juju.ModelControllerInfoQualifier

	tokens := strings.SplitN(req.ModelQualifier, "/", 2)
	if len(tokens) == 2 && tokens[0] != "" && tokens[1] != "" {
		qualifier = juju.WithOwnerAndModelName(tokens[0], tokens[1])
	} else {
		modelUUID, err := uuid.Parse(req.ModelQualifier)
		if err != nil {
			return apiparams.ModelControllerInfo{}, errors.E(fmt.Errorf("invalid model UUID: %w", err), errors.CodeBadRequest)
		}
		qualifier = juju.WithModelUUID(modelUUID.String())
	}

	response, err := r.jimm.JujuManager().ModelControllerInfo(ctx, r.user, qualifier)
	if err != nil {
		return apiparams.ModelControllerInfo{}, errors.E(err)
	}

	return *response, nil
}
