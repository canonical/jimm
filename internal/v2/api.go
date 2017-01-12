// Copyright 2016 Canonical Ltd.

package v2

import (
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/juju/httprequest"
	cloudapi "github.com/juju/juju/api/cloud"
	modelmanagerapi "github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/uber-go/zap"
	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jem/internal/apiconn"
	"github.com/CanonicalLtd/jem/internal/auth"
	"github.com/CanonicalLtd/jem/internal/ctxutil"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/jemerror"
	"github.com/CanonicalLtd/jem/internal/jemserver"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/internal/servermon"
	"github.com/CanonicalLtd/jem/internal/zapctx"
	"github.com/CanonicalLtd/jem/internal/zaputil"
	"github.com/CanonicalLtd/jem/params"
)

type Handler struct {
	jem                      *jem.JEM
	context                  context.Context
	cancel                   context.CancelFunc
	config                   jemserver.Params
	monReq                   servermon.Request
	usageSenderAuthorization func(applicationUser string) ([]byte, error)
}

func NewAPIHandler(ctx context.Context, jp *jem.Pool, ap *auth.Pool, sp jemserver.Params) ([]httprequest.Handler, error) {
	return jemerror.Mapper.Handlers(func(p httprequest.Params) (*Handler, error) {
		// Time out all requests after 30s. Do this before joining
		// the contexts because p.Context is likely to have a done
		// channel already and context.WithTimeout will be more efficient
		// in that case than working on the custom context type returned
		// from ctxutil.Join.
		ctx, cancel := context.WithTimeout(p.Context, 30*time.Second)
		ctx = ctxutil.Join(ctx, p.Context)
		ctx = zapctx.WithFields(ctx, zap.String("req-id", httprequest.RequestUUID(ctx)))
		zapctx.Debug(ctx, "HTTP request", zap.String("method", p.Request.Method), zap.Stringer("url", p.Request.URL))

		// All requests require an authenticated client.
		a := ap.Authenticator(ctx)
		defer a.Close()
		ctx, err := a.AuthenticateRequest(ctx, p.Request)
		if err != nil {
			return nil, errgo.Mask(err, errgo.Any)
		}
		h := &Handler{
			jem:     jp.JEM(ctx),
			context: ctx,
			config:  sp,
			cancel:  cancel,
			usageSenderAuthorization: jp.UsageSenderAuthorization,
		}

		h.monReq.Start(p.PathPattern)
		return h, nil
	}), nil
}

// Close implements io.Closer and is called by httprequest
// when the request is complete.
func (h *Handler) Close() error {
	h.cancel()
	h.jem.Close()
	h.jem = nil
	h.monReq.End()
	zapctx.Debug(h.context, "HTTP request done")
	return nil
}

// WhoAmI returns authentication information on the client that is
// making the WhoAmI call.
func (h *Handler) WhoAmI(arg *params.WhoAmI) (params.WhoAmIResponse, error) {
	ctx := h.context
	return params.WhoAmIResponse{
		User: auth.Username(ctx),
	}, nil
}

// AddController adds a new controller.
func (h *Handler) AddController(arg *params.AddController) error {
	ctx := h.context
	if err := auth.CheckIsUser(ctx, arg.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	if !arg.Info.Public {
		return errgo.WithCausef(nil, params.ErrForbidden, "cannot add private controller")
	}
	if err := auth.CheckIsUser(ctx, h.jem.ControllerAdmin()); err != nil {
		if errgo.Cause(err) == params.ErrUnauthorized {
			return errgo.WithCausef(nil, params.ErrUnauthorized, "admin access required to add public controllers")
		}
		return errgo.Mask(err)
	}
	if len(arg.Info.HostPorts) == 0 {
		return badRequestf(nil, "no host-ports in request")
	}

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
		return badRequestf(nil, "no user in request")
	}
	if !names.IsValidModel(arg.Info.ControllerUUID) {
		return badRequestf(nil, "bad model UUID in request")
	}

	ctl := &mongodoc.Controller{
		Path:          arg.EntityPath,
		CACert:        arg.Info.CACert,
		HostPorts:     [][]mongodoc.HostPort{hps},
		AdminUser:     arg.Info.User,
		AdminPassword: arg.Info.Password,
		UUID:          arg.Info.ControllerUUID,
		Public:        arg.Info.Public,
	}
	zapctx.Debug(ctx, "dialling controller")
	// Attempt to connect to the controller before accepting it.
	conn, err := h.jem.OpenAPIFromDoc(ctx, ctl)
	if err != nil {
		zapctx.Info(ctx, "cannot open API", zaputil.Error(err))
		return badRequestf(err, "cannot connect to controller")
	}
	defer conn.Close()
	ctl.UUID = conn.ControllerTag().Id()

	// Find out where the controller model is.
	mi, err := controllerModelInfo(conn, arg.Info.User)
	if err != nil {
		return badRequestf(err, "cannot get controller model details")
	}
	cloud, err := names.ParseCloudTag(mi.CloudTag)
	if err != nil {
		return badRequestf(err, "bad data from controller")
	}
	location := map[string]string{
		"cloud": cloud.Id(),
	}
	if mi.CloudRegion != "" {
		location["region"] = mi.CloudRegion
	}
	ctl.Location = location

	// Find out the cloud information.
	clouds, err := cloudapi.NewClient(conn).Clouds()
	if err != nil {
		return errgo.Notef(err, "cannot get clouds")
	}
	for k, v := range clouds {
		ctl.Cloud.Name = params.Cloud(k.Id())
		ctl.Cloud.ProviderType = v.Type
		for _, at := range v.AuthTypes {
			ctl.Cloud.AuthTypes = append(ctl.Cloud.AuthTypes, string(at))
		}
		ctl.Cloud.Endpoint = v.Endpoint
		ctl.Cloud.IdentityEndpoint = v.IdentityEndpoint
		ctl.Cloud.StorageEndpoint = v.StorageEndpoint
		for _, reg := range v.Regions {
			ctl.Cloud.Regions = append(ctl.Cloud.Regions, mongodoc.Region{
				Name:             reg.Name,
				Endpoint:         reg.Endpoint,
				IdentityEndpoint: reg.IdentityEndpoint,
				StorageEndpoint:  reg.StorageEndpoint,
			})
		}
		break
	}

	// Update addresses from latest known in controller. Note that
	// conn.APIHostPorts is always guaranteed to include the actual
	// address we succeeded in connecting to.
	ctl.HostPorts = mongodocAPIHostPorts(conn.APIHostPorts())

	err = h.jem.DB.AddController(ctx, ctl)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrAlreadyExists))
	}
	return nil
}

// controllerModelInfo returns the model info for the controller model.
func controllerModelInfo(conn *apiconn.Conn, user string) (*jujuparams.ModelInfo, error) {
	client := modelmanagerapi.NewClient(conn)
	models, err := client.ListModels(user)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	for _, m := range models {
		if m.Name != "controller" {
			continue
		}
		mir, err := client.ModelInfo([]names.ModelTag{names.NewModelTag(m.UUID)})
		if err != nil {
			return nil, errgo.Mask(err)
		}
		if mir[0].Error != nil {
			return nil, errgo.Mask(mir[0].Error)
		}
		return mir[0].Result, nil
	}
	return nil, errgo.New("controller model not found")
}

// mongodocAPIHostPorts returns the given API addresses prepared
// for storage in the database.
//
// It removes unusable addresses and marks any scope-unknown
// addresses as public so that the clients using only public-scoped
// addresses will use them.
func mongodocAPIHostPorts(nhpss [][]network.HostPort) [][]mongodoc.HostPort {
	hpss := make([][]mongodoc.HostPort, 0, len(nhpss))
	for _, nhps := range nhpss {
		nhps = network.FilterUnusableHostPorts(nhps)
		if len(nhps) == 0 {
			continue
		}
		hps := make([]mongodoc.HostPort, len(nhps))
		for i, nhp := range nhps {
			if nhp.Scope == network.ScopeUnknown {
				// This is needed because network.NewHostPort returns
				// scope unknown for DNS names.
				nhp.Scope = network.ScopePublic
			}
			hps[i].SetJujuHostPort(nhp)
		}
		hpss = append(hpss, hps)
	}
	return hpss
}

// GetController returns information on a controller.
func (h *Handler) GetController(arg *params.GetController) (*params.ControllerResponse, error) {
	ctx := h.context
	ctl, err := h.jem.Controller(ctx, arg.EntityPath)
	if err != nil {
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
func (h *Handler) DeleteController(arg *params.DeleteController) error {
	ctx := h.context
	// Check if user has permissions.
	if err := auth.CheckIsUser(ctx, arg.EntityPath.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	if !arg.Force {
		ctl, err := h.jem.DB.Controller(ctx, arg.EntityPath)
		if err != nil {
			return errgo.Mask(err, errgo.Is(params.ErrNotFound))
		}
		if ctl.UnavailableSince.IsZero() {
			return errgo.WithCausef(nil, params.ErrStillAlive, "cannot delete controller while it is still alive")
		}
	}
	if err := h.jem.DB.DeleteController(ctx, arg.EntityPath); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return nil
}

// isAlreadyGrantedError reports whether the error
// (as returned from modelmanager.Client.GrantModel)
// represents the condition that the user has already
// been granted access.
//
// We have to use string comparison because of
// https://bugs.launchpad.net/juju-core/+bug/1564880.
func isAlreadyGrantedError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.HasPrefix(s, "user already has ") &&
		strings.HasSuffix(s, " access or greater")
}

// GetModel returns information on a given model.
func (h *Handler) GetModel(arg *params.GetModel) (*params.ModelResponse, error) {
	ctx := h.context
	if err := h.jem.DB.CheckReadACL(ctx, h.jem.DB.Models(), arg.EntityPath); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}

	m, err := h.jem.DB.Model(ctx, arg.EntityPath)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	ctl, err := h.jem.DB.Controller(ctx, m.Controller)
	if err != nil {
		return nil, errgo.Mask(err)
	}

	r := &params.ModelResponse{
		Path:             arg.EntityPath,
		UUID:             m.UUID,
		ControllerUUID:   ctl.UUID,
		CACert:           ctl.CACert,
		HostPorts:        mongodoc.Addresses(ctl.HostPorts),
		ControllerPath:   m.Controller,
		Life:             m.Life,
		UnavailableSince: newTime(ctl.UnavailableSince.UTC()),
		Counts:           m.Counts,
		Creator:          m.Creator,
	}
	return r, nil
}

// DeleteModel deletes an model from JEM.
func (h *Handler) DeleteModel(arg *params.DeleteModel) error {
	ctx := h.context
	if err := auth.CheckIsUser(ctx, arg.EntityPath.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	if err := h.jem.DB.DeleteModel(ctx, arg.EntityPath); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrForbidden))
	}
	return nil
}

// ListModels returns all the models stored in JEM.
// Note that the models returned don't include the username or password.
// To gain access to a specific model, that model should be retrieved
// explicitly.
func (h *Handler) ListModels(arg *params.ListModels) (*params.ListModelsResponse, error) {
	ctx := h.context
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
	models := make([]params.ModelResponse, 0, len(controllers))
	modelIter := h.jem.DB.NewCanReadIter(ctx, h.jem.DB.Models().Find(nil).Sort("_id").Iter())
	var m mongodoc.Model
	for modelIter.Next(&m) {
		ctl, ok := controllers[m.Controller]
		if !ok {
			zapctx.Error(ctx, "model has invalid controller value", zap.Stringer("model", m.Path), zap.Stringer("controller", m.Controller))
			continue
		}
		// TODO We could ensure that the currently authenticated user has
		// access to the model and return their username and password,
		// but that would mean we'd have to ensure the user in every
		// returned model which currently we can't do efficiently,
		// so given that most uses of this endpoint won't actually want
		// to connect to all of the models, we leave out the username and
		// password for now.
		models = append(models, params.ModelResponse{
			Path:             m.Path,
			UUID:             m.UUID,
			ControllerUUID:   ctl.UUID,
			CACert:           ctl.CACert,
			HostPorts:        mongodoc.Addresses(ctl.HostPorts),
			ControllerPath:   m.Controller,
			Life:             m.Life,
			UnavailableSince: newTime(ctl.UnavailableSince.UTC()),
			Counts:           m.Counts,
			Creator:          m.Creator,
		})
	}
	if err := modelIter.Err(); err != nil {
		return nil, errgo.Notef(err, "cannot get models")
	}
	return &params.ListModelsResponse{
		Models: models,
	}, nil
}

// ListController returns all the controllers stored in JEM.
// Currently the ProviderType field in each ControllerResponse is not
// populated.
func (h *Handler) ListController(arg *params.ListController) (*params.ListControllerResponse, error) {
	ctx := h.context
	var controllers []params.ControllerResponse

	iter := h.jem.DB.NewCanReadIter(ctx, h.jem.DB.Controllers().Find(nil).Sort("_id").Iter())
	var ctl mongodoc.Controller
	for iter.Next(&ctl) {
		controllers = append(controllers, params.ControllerResponse{
			Path:             ctl.Path,
			Public:           ctl.Public,
			UnavailableSince: newTime(ctl.UnavailableSince.UTC()),
			Location:         ctl.Location,
		})
	}
	if err := iter.Err(); err != nil {
		return nil, errgo.Notef(err, "cannot get models")
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

// GetControllerLocations returns all the available values for a given controller
// location attribute. The set of controllers is constrained by the URL query
// parameters.
func (h *Handler) GetControllerLocations(p httprequest.Params, arg *params.GetControllerLocations) (*params.ControllerLocationsResponse, error) {
	ctx := h.context
	attr := arg.Attr
	if !params.IsValidLocationAttr(attr) {
		return nil, badRequestf(nil, "invalid location %q", attr)
	}
	lp, err := parseFormLocations(p.Request.Form)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	if len(lp.other) > 0 {
		return &params.ControllerLocationsResponse{
			Values: []string{},
		}, nil
	}
	// TODO(mhilton) this method may select many more controllers
	// than necessary. Re-evaluate the method if we start seeing
	// problems.
	found := make(map[string]bool)
	err = h.jem.DoControllers(ctx, lp.cloud, lp.region, func(ctl *mongodoc.Controller) error {
		switch attr {
		case "cloud":
			found[string(ctl.Cloud.Name)] = true
		case "region":
			for _, r := range ctl.Cloud.Regions {
				found[r.Name] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}

	// Build the result slice and sort it so we get deterministic results.
	results := make([]string, 0, len(found))
	for val := range found {
		results = append(results, val)
	}
	sort.Strings(results)
	return &params.ControllerLocationsResponse{
		Values: results,
	}, nil
}

// GetAllControllerLocations returns all the available
// sets of controller location attributes, restricting
// the search by any provided location attributes.
func (h *Handler) GetAllControllerLocations(p httprequest.Params, arg *params.GetAllControllerLocations) (*params.AllControllerLocationsResponse, error) {
	ctx := h.context
	lp, err := parseFormLocations(p.Request.Form)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	if len(lp.other) > 0 {
		return &params.AllControllerLocationsResponse{
			Locations: []map[string]string{},
		}, nil
	}
	locSet := make(map[cloudRegion]bool)
	err = h.jem.DoControllers(ctx, lp.cloud, lp.region, func(ctl *mongodoc.Controller) error {
		if len(ctl.Cloud.Regions) == 0 {
			locSet[cloudRegion{ctl.Cloud.Name, ""}] = true
			return nil
		}
		for _, reg := range ctl.Cloud.Regions {
			locSet[cloudRegion{ctl.Cloud.Name, reg.Name}] = true
		}
		return nil
	})
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	ordered := make(cloudRegions, 0, len(locSet))
	for k := range locSet {
		ordered = append(ordered, k)
	}
	sort.Sort(ordered)
	return &params.AllControllerLocationsResponse{
		Locations: ordered.locations(),
	}, nil
}

type cloudRegion struct {
	cloud  params.Cloud
	region string
}

type cloudRegions []cloudRegion

// Len implements sort.Interface.Len
func (c cloudRegions) Len() int {
	return len(c)
}

// Less implements sort.Interface.Less
func (c cloudRegions) Less(i, j int) bool {
	if c[i].cloud == c[j].cloud {
		return c[i].region < c[j].region
	}
	return c[i].cloud < c[j].cloud
}

// Swap implements sort.Interface.Swap
func (c cloudRegions) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (c cloudRegions) locations() []map[string]string {
	locs := make([]map[string]string, 0, len(c))
	for _, cr := range c {
		m := map[string]string{
			"cloud": string(cr.cloud),
		}
		if cr.region != "" {
			m["region"] = cr.region
		}
		locs = append(locs, m)
	}
	return locs
}

// GetControllerLocation returns a map of location attributes for a given controller.
func (h *Handler) GetControllerLocation(arg *params.GetControllerLocation) (params.ControllerLocation, error) {
	ctx := h.context
	ctl, err := h.jem.Controller(ctx, arg.EntityPath)
	if err != nil {
		return params.ControllerLocation{}, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	loc := map[string]string{
		"cloud": string(ctl.Cloud.Name),
	}
	if len(ctl.Cloud.Regions) > 0 {
		loc["region"] = ctl.Cloud.Regions[0].Name
	}
	return params.ControllerLocation{
		Location: loc,
	}, nil
}

// NewModel creates a new model inside an existing Controller.
func (h *Handler) NewModel(args *params.NewModel) (*params.ModelResponse, error) {
	ctx := h.context
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
	var credentials []byte
	if h.usageSenderAuthorization != nil {
		credentials, err = h.usageSenderAuthorization(string(args.User))
		if err != nil {
			return nil, errgo.Notef(err, "failed to obtain metrics credentials for user: %v", args.User)
		}
	}

	modelPath := params.EntityPath{args.User, args.Info.Name}
	_, err = h.jem.CreateModel(ctx, jem.CreateModelParams{
		Path:               modelPath,
		ControllerPath:     ctlPath,
		Credential:         args.Info.Credential,
		Cloud:              lp.cloud,
		Region:             lp.region,
		Attributes:         args.Info.Config,
		MetricsCredentials: credentials,
	})
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
	return h.GetModel(&params.GetModel{
		EntityPath: modelPath,
	})
}

// SetControllerPerm sets the permissions on a controller entity.
// Only the owner (arg.EntityPath.User) can change the permissions
// on an an entity. The owner can always read an entity, even
// if it has empty ACL.
func (h *Handler) SetControllerPerm(arg *params.SetControllerPerm) error {
	return h.setPerm(h.jem.DB.Controllers(), arg.EntityPath, arg.ACL)
}

// SetModelPerm sets the permissions on a controller entity.
// Only the owner (arg.EntityPath.User) can change the permissions
// on an an entity. The owner can always read an entity, even
// if it has empty ACL.
// TODO remove this.
func (h *Handler) SetModelPerm(arg *params.SetModelPerm) error {
	// TODO revoke access from all the users that currently
	// have access to the model that should not have access
	// now.
	return h.setPerm(h.jem.DB.Models(), arg.EntityPath, arg.ACL)
}

func (h *Handler) setPerm(coll *mgo.Collection, path params.EntityPath, acl params.ACL) error {
	ctx := h.context
	// Only path.User (or members thereof) can change permissions.
	if err := auth.CheckIsUser(ctx, path.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	zapctx.Info(ctx, "set perm", zap.String("collection", coll.Name), zap.Stringer("entity", path), zap.Object("acl", acl))
	if err := coll.UpdateId(path.String(), bson.D{{"$set", bson.D{{"acl", acl}}}}); err != nil {
		if err == mgo.ErrNotFound {
			return params.ErrNotFound
		}
		return errgo.Notef(err, "cannot update %v", path)
	}
	return nil
}

// GetControllerPerm returns the ACL for a given controller.
// Only the owner (arg.EntityPath.User) can read the ACL.
func (h *Handler) GetControllerPerm(arg *params.GetControllerPerm) (params.ACL, error) {
	return h.getPerm(h.jem.DB.Controllers(), arg.EntityPath)
}

// GetModelPerm returns the ACL for a given model.
// Only the owner (arg.EntityPath.User) can read the ACL.
func (h *Handler) GetModelPerm(arg *params.GetModelPerm) (params.ACL, error) {
	return h.getPerm(h.jem.DB.Models(), arg.EntityPath)
}

func (h *Handler) getPerm(coll *mgo.Collection, path params.EntityPath) (params.ACL, error) {
	ctx := h.context
	// Only the owner can read permissions.
	if err := auth.CheckIsUser(ctx, path.User); err != nil {
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
func (h *Handler) UpdateCredential(arg *params.UpdateCredential) error {
	ctx := h.context
	// Only the owner can set credentials.
	if err := auth.CheckIsUser(ctx, arg.EntityPath.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	// TODO(mhilton) validate the credentials.
	err := h.jem.UpdateCredential(ctx, &mongodoc.Credential{
		Path:       arg.CredentialPath,
		Type:       arg.Credential.AuthType,
		Attributes: arg.Credential.Attributes,
	})
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func badRequestf(underlying error, f string, a ...interface{}) error {
	err := errgo.WithCausef(underlying, params.ErrBadRequest, f, a...)
	err.(*errgo.Err).SetLocation(1)
	return err
}

// collapseHostPorts collapses a list of host-port lists
// into a single list suitable for passing to api.Open.
// It preserves ordering because api.State.APIHostPorts
// makes sure to return the first-connected address
// first in the slice.
// See juju.PrepareEndpointsForCaching for a more
// comprehensive version of this function.
func collapseHostPorts(hpss [][]network.HostPort) []string {
	hps := network.CollapseHostPorts(hpss)
	hps = network.FilterUnusableHostPorts(hps)
	hps = network.UniqueHostPorts(hps)
	return network.HostPortsToStrings(hps)
}

// formToLocationAttrs converts a set of location attributes
// specified as URL query paramerters into the usual
// location attribute map form.
func formToLocationAttrs(form url.Values) (map[string]string, error) {
	attrs := make(map[string]string)
	for attr, vals := range form {
		if !params.IsValidLocationAttr(attr) {
			return nil, badRequestf(nil, "invalid location attribute %q", attr)
		}
		if len(vals) > 0 {
			attrs[attr] = vals[0]
		}
	}
	return attrs, nil
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

func parseFormLocations(form url.Values) (locationParams, error) {
	loc, err := formToLocationAttrs(form)
	if err != nil {
		return locationParams{}, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	return cloudAndRegion(loc)
}
