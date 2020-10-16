// Copyright 2016 Canonical Ltd.

package v2

import (
	"context"
	"time"

	"github.com/juju/aclstore"
	controllerapi "github.com/juju/juju/api/controller"
	modelmanagerapi "github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/errgo.v1"
	"gopkg.in/httprequest.v1"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jem/jimmdb"
	"github.com/CanonicalLtd/jimm/internal/jemerror"
	"github.com/CanonicalLtd/jimm/internal/jemserver"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/servermon"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
)

const (
	// ACL Names
	auditLogACL = "audit-log"
	logLevelACL = "log-level"
)

// controllerClientInitiateMigration is defined as a variable so that
// it can be overridden for tests.
var controllerClientInitiateMigration = (*controllerapi.Client).InitiateMigration

type Handler struct {
	id         identchecker.ACLIdentity
	jem        *jem.JEM
	cancel     context.CancelFunc
	config     jemserver.Params
	monReq     servermon.Request
	aclManager *aclstore.Manager
}

func NewAPIHandler(ctx context.Context, params jemserver.HandlerParams) ([]httprequest.Handler, error) {
	// Ensure the required ACLs exist.
	if err := params.ACLManager.CreateACL(ctx, auditLogACL, string(params.ControllerAdmin)); err != nil {
		return nil, errgo.Mask(err)
	}
	if err := params.ACLManager.CreateACL(ctx, logLevelACL, string(params.ControllerAdmin)); err != nil {
		return nil, errgo.Mask(err)
	}

	srv := &httprequest.Server{
		ErrorMapper: jemerror.Mapper,
	}

	return srv.Handlers(func(p httprequest.Params) (*Handler, context.Context, error) {
		// Time out all requests after 30s.
		ctx, cancel := context.WithTimeout(p.Context, 30*time.Second)
		zapctx.Debug(ctx, "HTTP request", zap.String("method", p.Request.Method), zap.Stringer("url", p.Request.URL))

		// All requests require an authenticated client.
		id, err := params.Authenticator.AuthenticateRequest(ctx, p.Request)
		if err != nil {
			return nil, ctx, errgo.Mask(err, errgo.Any)
		}
		h := &Handler{
			id:         id,
			jem:        params.JEMPool.JEM(ctx),
			config:     params.Params,
			cancel:     cancel,
			aclManager: params.ACLManager,
		}

		h.monReq.Start(p.PathPattern)
		return h, ctx, nil
	}), nil
}

// Close implements io.Closer and is called by httprequest
// when the request is complete.
func (h *Handler) Close() error {
	h.cancel()
	h.jem.Close()
	h.jem = nil
	h.monReq.End()
	return nil
}

// WhoAmI returns authentication information on the client that is
// making the WhoAmI call.
func (h *Handler) WhoAmI(p httprequest.Params, arg *params.WhoAmI) (params.WhoAmIResponse, error) {
	return params.WhoAmIResponse{
		User: h.id.Id(),
	}, nil
}

// AddController adds a new controller.
func (h *Handler) AddController(p httprequest.Params, arg *params.AddController) error {
	hps, err := mongodoc.ParseAddresses(arg.Info.HostPorts)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "")
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
	if arg.Info.User == "" {
		return errgo.WithCausef(nil, params.ErrBadRequest, "no user in request")
	}
	ctl := mongodoc.Controller{
		Path:          arg.EntityPath,
		CACert:        arg.Info.CACert,
		HostPorts:     [][]mongodoc.HostPort{hps},
		AdminUser:     arg.Info.User,
		AdminPassword: arg.Info.Password,
		UUID:          arg.Info.ControllerUUID,
		Public:        arg.Info.Public,
	}
	err = h.jem.AddController(p.Context, h.id, &ctl)
	switch errgo.Cause(err) {
	case jem.ErrAPIConnection:
		return errgo.WithCausef(err, params.ErrBadRequest, "")
	default:
		return errgo.Mask(
			err,
			errgo.Is(params.ErrAlreadyExists),
			errgo.Is(params.ErrForbidden),
			errgo.Is(params.ErrUnauthorized),
		)
	}
}

// GetController returns information on a controller.
func (h *Handler) GetController(p httprequest.Params, arg *params.GetController) (*params.ControllerResponse, error) {
	ctl := &mongodoc.Controller{Path: arg.EntityPath}
	if err := h.jem.GetController(p.Context, h.id, ctl); err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			if !(auth.CheckIsUser(p.Context, h.id, ctl.Path.User) == nil) {
				err = params.ErrUnauthorized
			}
		}
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	return &params.ControllerResponse{
		Path:             arg.EntityPath,
		Location:         ctl.Location,
		Public:           ctl.Public,
		UnavailableSince: newTime(ctl.UnavailableSince.UTC()),
	}, nil
}

// DeleteController removes an existing controller.
func (h *Handler) DeleteController(p httprequest.Params, arg *params.DeleteController) error {
	err := h.jem.DeleteController(p.Context, h.id, &mongodoc.Controller{Path: arg.EntityPath}, arg.Force)
	return errgo.Mask(err,
		errgo.Is(params.ErrUnauthorized),
		errgo.Is(params.ErrNotFound),
		errgo.Is(params.ErrStillAlive),
	)
}

// GetModel returns information on a given model.
func (h *Handler) GetModel(p httprequest.Params, arg *params.GetModel) (*params.ModelResponse, error) {
	m := mongodoc.Model{Path: arg.EntityPath}
	if err := h.jem.GetModel(p.Context, h.id, jujuparams.ModelReadAccess, &m); err != nil {
		if errgo.Cause(err) == params.ErrNotFound && auth.CheckIsUser(p.Context, h.id, arg.EntityPath.User) != nil {
			err = errgo.WithCausef(nil, params.ErrUnauthorized, "")
		}
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	ctl := mongodoc.Controller{Path: m.Controller}
	if err := h.jem.DB.GetController(p.Context, &ctl); err != nil {
		return nil, errgo.Mask(err)
	}

	r := &params.ModelResponse{
		Path:             arg.EntityPath,
		UUID:             m.UUID,
		ControllerUUID:   ctl.UUID,
		CACert:           ctl.CACert,
		HostPorts:        mongodoc.Addresses(ctl.HostPorts),
		ControllerPath:   m.Controller,
		Life:             m.Life(),
		UnavailableSince: newTime(ctl.UnavailableSince.UTC()),
		Counts:           m.Counts,
		Creator:          m.Creator,
	}
	return r, nil
}

// DeleteModel deletes an model from JEM.
func (h *Handler) DeleteModel(p httprequest.Params, arg *params.DeleteModel) error {
	m := mongodoc.Model{Path: arg.EntityPath}
	if err := h.jem.GetModel(p.Context, h.id, jujuparams.ModelAdminAccess, &m); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	return errgo.Mask(h.jem.DB.RemoveModel(p.Context, &m))
}

// ListModels returns all the models stored in JEM.
// Note that the models returned don't include the username or password.
// To gain access to a specific model, that model should be retrieved
// explicitly.
func (h *Handler) ListModels(p httprequest.Params, arg *params.ListModels) (*params.ListModelsResponse, error) {
	// TODO provide a way of restricting the results.

	// We get all controllers first, because many models will be
	// sharing the same controllers.
	// TODO we could do better than this and avoid gathering all the
	// controllers into memory. Possiblities include caching
	// controllers, and gathering results to do only a few
	// concurrent queries.
	controllers := make(map[params.EntityPath]mongodoc.Controller)
	iter := h.jem.DB.Controllers().Find(nil).Sort("_id").Iter()
	var ctl mongodoc.Controller
	for iter.Next(&ctl) {
		controllers[ctl.Path] = ctl
	}
	if err := iter.Err(); err != nil {
		return nil, errgo.Notef(err, "cannot get controllers")
	}

	iter = h.jem.DB.Models().Find(nil).Sort("_id").Iter()
	var modelIter entityIter
	if arg.All {
		if err := auth.CheckIsUser(p.Context, h.id, h.jem.ControllerAdmin()); err != nil {
			if errgo.Cause(err) == params.ErrUnauthorized {
				return nil, errgo.WithCausef(nil, params.ErrUnauthorized, "admin access required to list all models")
			}
			return nil, errgo.Mask(err)
		}
		modelIter = mgoIter{iter}
	} else {
		modelIter = h.jem.DB.NewCanReadIter(h.id, iter)
	}
	var models []params.ModelResponse
	var m mongodoc.Model
	for modelIter.Next(p.Context, &m) {
		ctl, ok := controllers[m.Controller]
		if !ok {
			zapctx.Error(p.Context, "model has invalid controller value", zap.Stringer("model", m.Path), zap.Stringer("controller", m.Controller))
			continue
		}
		models = append(models, params.ModelResponse{
			Path:             m.Path,
			UUID:             m.UUID,
			ControllerUUID:   ctl.UUID,
			CACert:           ctl.CACert,
			HostPorts:        mongodoc.Addresses(ctl.HostPorts),
			ControllerPath:   m.Controller,
			Life:             m.Life(),
			UnavailableSince: newTime(ctl.UnavailableSince.UTC()),
			Counts:           m.Counts,
			Creator:          m.Creator,
		})
	}
	if err := modelIter.Err(p.Context); err != nil {
		return nil, errgo.Notef(err, "cannot get models")
	}
	return &params.ListModelsResponse{
		Models: models,
	}, nil
}

// ListController returns all the controllers stored in JEM.
// Currently the ProviderType field in each ControllerResponse is not
// populated.
func (h *Handler) ListController(p httprequest.Params, arg *params.ListController) (*params.ListControllerResponse, error) {
	var controllers []params.ControllerResponse

	iter := h.jem.DB.NewCanReadIter(h.id, h.jem.DB.Controllers().Find(nil).Sort("_id").Iter())
	var ctl mongodoc.Controller
	for iter.Next(p.Context, &ctl) {
		controllers = append(controllers, params.ControllerResponse{
			Path:             ctl.Path,
			Public:           ctl.Public,
			UnavailableSince: newTime(ctl.UnavailableSince.UTC()),
			Location:         ctl.Location,
		})
	}
	if err := iter.Err(p.Context); err != nil {
		return nil, errgo.Notef(err, "cannot get controllers")
	}
	return &params.ListControllerResponse{
		Controllers: controllers,
	}, nil
}

// newTime returns a pointer to t if it's non-zero,
// or nil otherwise.
func newTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// NewModel creates a new model inside an existing Controller.
func (h *Handler) NewModel(p httprequest.Params, args *params.NewModel) (*params.ModelResponse, error) {
	var ctlPath params.EntityPath
	if args.Info.Controller != nil {
		ctlPath = *args.Info.Controller
	}
	lp, err := cloudAndRegion(args.Info.Location)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	if len(lp.other) > 0 {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "cannot select controller: no matching controllers found")
	}

	modelPath := params.EntityPath{args.User, args.Info.Name}
	err = h.jem.CreateModel(p.Context, h.id, jem.CreateModelParams{
		Path:           modelPath,
		ControllerPath: ctlPath,
		Credential:     args.Info.Credential,
		Cloud:          lp.cloud,
		Region:         lp.region,
		Attributes:     args.Info.Config,
	}, nil)
	if err != nil {
		return nil, errgo.Mask(err,
			errgo.Is(params.ErrNotFound),
			errgo.Is(params.ErrBadRequest),
			errgo.Is(params.ErrAlreadyExists),
			errgo.Is(params.ErrUnauthorized),
		)
	}

	// Use GetModel so that we're sure to get exactly
	// the same semantics, including ensuring that
	// the user exists. This does a bit more work
	// than necessary - we could optimize the ACL checking etc
	// out if it's becoming a bottleneck.
	return h.GetModel(p, &params.GetModel{
		EntityPath: modelPath,
	})
}

// SetControllerPerm sets the permissions on a controller entity.
// Only the owner (arg.EntityPath.User) can change the permissions
// on an an entity. The owner can always read an entity, even
// if it has empty ACL.
func (h *Handler) SetControllerPerm(p httprequest.Params, arg *params.SetControllerPerm) error {
	return h.setPerm(p.Context, h.jem.DB.Controllers(), arg.EntityPath, arg.ACL)
}

// SetModelPerm sets the permissions on a model entity. Only the owner
// (arg.EntityPath.User) can change the permissions on an entity. The
// owner can always read an entity, even if it has empty ACL.
// TODO remove this.
func (h *Handler) SetModelPerm(p httprequest.Params, arg *params.SetModelPerm) error {
	// TODO revoke access from all the users that currently
	// have access to the model that should not have access
	// now.
	return h.setPerm(p.Context, h.jem.DB.Models(), arg.EntityPath, arg.ACL)
}

func (h *Handler) setPerm(ctx context.Context, coll *mgo.Collection, path params.EntityPath, acl params.ACL) error {
	// Only path.User (or members thereof) can change permissions.
	if err := auth.CheckIsUser(ctx, h.id, path.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	zapctx.Info(ctx, "set perm", zap.String("collection", coll.Name), zap.Stringer("entity", path), zap.Any("acl", acl))
	if err := coll.UpdateId(path.String(), new(jimmdb.Update).Set("acl", acl)); err != nil {
		if err == mgo.ErrNotFound {
			return params.ErrNotFound
		}
		return errgo.Notef(err, "cannot update %v", path.String())
	}
	return nil
}

// GetControllerPerm returns the ACL for a given controller.
// Only the owner (arg.EntityPath.User) can read the ACL.
func (h *Handler) GetControllerPerm(p httprequest.Params, arg *params.GetControllerPerm) (params.ACL, error) {
	return h.getPerm(p.Context, h.jem.DB.Controllers(), arg.EntityPath)
}

// GetModelPerm returns the ACL for a given model.
// Only the owner (arg.EntityPath.User) can read the ACL.
func (h *Handler) GetModelPerm(p httprequest.Params, arg *params.GetModelPerm) (params.ACL, error) {
	return h.getPerm(p.Context, h.jem.DB.Models(), arg.EntityPath)
}

func (h *Handler) getPerm(ctx context.Context, coll *mgo.Collection, path params.EntityPath) (params.ACL, error) {
	// Only the owner can read permissions.
	if err := auth.CheckIsUser(ctx, h.id, path.User); err != nil {
		return params.ACL{}, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	acl, err := h.jem.DB.GetACL(ctx, coll, path)
	if err != nil {
		return params.ACL{}, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return acl, nil
}

// UpdateCredential stores the provided credential under the provided,
// user, cloud and name. If there is already a credential with that name
// it is overwritten.
func (h *Handler) UpdateCredential(p httprequest.Params, arg *params.UpdateCredential) error {
	// Only the owner can set credentials.
	if err := auth.CheckIsUser(p.Context, h.id, arg.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	_, err := h.jem.UpdateCredential(p.Context, &mongodoc.Credential{
		Path:       mongodoc.CredentialPathFromParams(arg.CredentialPath),
		Type:       arg.Credential.AuthType,
		Attributes: arg.Credential.Attributes,
	}, 0)
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// JujuStatus retrieves and returns the status of the specifed model.
func (h *Handler) JujuStatus(p httprequest.Params, arg *params.JujuStatus) (*params.JujuStatusResponse, error) {
	if err := auth.CheckIsUser(p.Context, h.id, h.config.ControllerAdmin); err != nil {
		if err := h.jem.DB.CheckReadACL(p.Context, h.id, h.jem.DB.Models(), arg.EntityPath); err != nil {
			return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
		}
	}
	conn, err := h.jem.OpenModelAPI(p.Context, arg.EntityPath)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	defer conn.Close()
	client := conn.Client()
	status, err := client.Status(nil)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return &params.JujuStatusResponse{
		Status: *status,
	}, nil
}

// Migrate starts a migration of a model from its current
// controller to a different one. The migration will not have
// completed by the time the Migrate call returns.
func (h *Handler) Migrate(p httprequest.Params, arg *params.Migrate) error {
	if err := auth.CheckIsUser(p.Context, h.id, h.config.ControllerAdmin); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	model := mongodoc.Model{Path: arg.EntityPath}
	if err := h.jem.DB.GetModel(p.Context, &model); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	conn, err := h.jem.OpenAPI(p.Context, model.Controller)
	if err != nil {
		return errgo.Mask(err)
	}
	defer conn.Close()
	ctl := mongodoc.Controller{Path: arg.Controller}
	if err := h.jem.GetController(p.Context, h.id, &ctl); err != nil {
		return errgo.NoteMask(err, "cannot access destination controller", errgo.Is(params.ErrNotFound))
	}
	zapctx.Debug(p.Context, "about to call InitiateMigration")
	api := controllerapi.NewClient(conn)
	_, err = controllerClientInitiateMigration(api, controllerapi.MigrationSpec{
		ModelUUID:            model.UUID,
		TargetControllerUUID: ctl.UUID,
		TargetAddrs:          mongodoc.Addresses(ctl.HostPorts),
		TargetCACert:         ctl.CACert,
		TargetUser:           ctl.AdminUser,
		TargetPassword:       ctl.AdminPassword,
	})
	if err != nil {
		return errgo.Notef(err, "cannot initiate migration")
	}
	if err := h.jem.DB.SetModelController(p.Context, arg.EntityPath, arg.Controller); err != nil {
		// This is a problem, because we can't undo the migration now,
		// so just shout about it.
		zapctx.Error(p.Context, "cannot update model database entry", zap.Stringer("model", arg.EntityPath), zap.Stringer("controller", arg.Controller))
		return errgo.Notef(err, "cannot update model database entry (manual intervention required!)")
	}

	// TODO return the migration id?
	return nil
}

// LogLevel returns the current logging level of the running service.
func (h *Handler) LogLevel(p httprequest.Params, _ *params.LogLevel) (params.Level, error) {
	if err := h.checkACL(p.Context, logLevelACL); err != nil {
		return params.Level{}, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	return params.Level{
		Level: zapctx.LogLevel.String(),
	}, nil
}

func (h *Handler) SetControllerDeprecated(p httprequest.Params, req *params.SetControllerDeprecated) error {
	if err := h.jem.SetControllerDeprecated(p.Context, h.id, req.EntityPath, req.Body.Deprecated); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	return nil
}

func (h *Handler) GetControllerDeprecated(p httprequest.Params, req *params.GetControllerDeprecated) (*params.DeprecatedBody, error) {
	ctl := mongodoc.Controller{Path: req.EntityPath}
	if err := h.jem.GetController(p.Context, h.id, &ctl); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	return &params.DeprecatedBody{
		Deprecated: ctl.Deprecated,
	}, nil
}

// SetLogLevel configures the logging level of the running service.
func (h *Handler) SetLogLevel(p httprequest.Params, req *params.SetLogLevel) error {
	if err := h.checkACL(p.Context, logLevelACL); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(req.Level.Level)); err != nil {
		return badRequestf(err, "")
	}
	zapctx.LogLevel.SetLevel(level)
	zaputil.SetLoggoLogLevel(level)
	return nil
}

func badRequestf(underlying error, f string, a ...interface{}) error {
	err := errgo.WithCausef(underlying, params.ErrBadRequest, f, a...)
	err.(*errgo.Err).SetLocation(1)
	return err
}

type locationParams struct {
	cloud  params.Cloud
	region string
	other  map[string]string
}

// cloudAndRegion extracts the cloud and region from the location
// parameters, if present.
func cloudAndRegion(loc map[string]string) (locationParams, error) {
	var p locationParams
	for k, v := range loc {
		switch k {
		case "cloud":
			if err := p.cloud.UnmarshalText([]byte(v)); err != nil {
				return locationParams{}, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
			}
		case "region":
			p.region = v
		default:
			if p.other == nil {
				p.other = make(map[string]string)
			}
			p.other[k] = v
		}
	}
	return p, nil
}

// entityIter is an iterator over a set of entities.
type entityIter interface {
	Next(ctx context.Context, item auth.ACLEntity) bool
	Close(ctx context.Context) error
	Err(ctx context.Context) error
}

// mgoIter is an adapter to convert a *mgo.Iter into an entityIter.
type mgoIter struct {
	*mgo.Iter
}

// Next implements entityIter.Next by wrapping *mgo.Next using the
// auth.ACLEntity type.
func (it mgoIter) Next(_ context.Context, item auth.ACLEntity) bool {
	return it.Iter.Next(item)
}

func (it mgoIter) Close(_ context.Context) error {
	return it.Iter.Close()
}

func (it mgoIter) Err(_ context.Context) error {
	return it.Iter.Err()
}

// GetModelName returns the name of the model identified by the provided uuid.
func (h *Handler) GetModelName(p httprequest.Params, arg *params.ModelNameRequest) (params.ModelNameResponse, error) {
	m := mongodoc.Model{UUID: arg.UUID}
	if err := h.jem.GetModel(p.Context, h.id, jujuparams.ModelReadAccess, &m); err != nil {
		return params.ModelNameResponse{}, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}

	return params.ModelNameResponse{
		Name: string(m.Path.Name),
	}, nil
}

// GetAuditEntries return the list of audit log entries based on the requested query.
func (h *Handler) GetAuditEntries(p httprequest.Params, arg *params.AuditLogRequest) (params.AuditLogEntries, error) {
	if err := h.checkACL(p.Context, auditLogACL); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	entries, err := h.jem.DB.GetAuditEntries(p.Context, arg.Start.Time, arg.End.Time, arg.Type)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return entries, nil
}

// GetModelStatuses return the list of all models created between 2 dates (or all).
func (h *Handler) GetModelStatuses(p httprequest.Params, arg *params.ModelStatusesRequest) (params.ModelStatuses, error) {
	entries, err := h.jem.GetModelStatuses(p.Context, h.id)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	return entries, nil
}

func (h *Handler) checkACL(ctx context.Context, aclName string) error {
	acl, err := h.aclManager.ACL(ctx, aclName)
	if err != nil {
		return errgo.Mask(err)
	}
	return errgo.Mask(auth.CheckACL(ctx, h.id, acl), errgo.Is(params.ErrUnauthorized))
}

// MissingModels returns a list of models present on the given controller
// that are not in the local database.
func (h *Handler) MissingModels(p httprequest.Params, arg *params.MissingModelsRequest) (params.MissingModels, error) {
	var resp params.MissingModels

	if err := auth.CheckIsUser(p.Context, h.id, h.jem.ControllerAdmin()); err != nil {
		if errgo.Cause(err) == params.ErrUnauthorized {
			return resp, errgo.WithCausef(nil, params.ErrUnauthorized, "admin access required")
		}
		return resp, errgo.Mask(err)
	}

	conn, err := h.jem.OpenAPI(p.Context, arg.EntityPath)
	if err != nil {
		return resp, errgo.Mask(err)
	}
	defer conn.Close()

	user := conn.Info.Tag.Id()
	client := modelmanagerapi.NewClient(conn)
	mss, err := client.ListModelSummaries(user, true)
	if err != nil {
		return resp, errgo.Mask(err)
	}

	for _, ms := range mss {
		// Check that a model with the same UUID exists.
		m := mongodoc.Model{UUID: ms.UUID}
		err := h.jem.DB.GetModel(p.Context, &m)
		if err == nil {
			continue
		}
		if errgo.Cause(err) != params.ErrNotFound {
			return resp, errgo.Mask(err)
		}
		resp.Models = append(resp.Models, params.ModelStatus{
			ID:         ms.Owner + "/" + ms.Name,
			UUID:       ms.UUID,
			Cloud:      ms.Cloud,
			Region:     ms.CloudRegion,
			Status:     string(ms.Status.Status),
			Controller: arg.EntityPath.String(),
		})
	}

	return resp, nil
}
