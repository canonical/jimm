// Copyright 2016 Canonical Ltd.

package v2

import (
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/juju/httprequest"
	cloudapi "github.com/juju/juju/api/cloud"
	modelmanager "github.com/juju/juju/controller/modelmanager"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/loggo"
	jujuschema "github.com/juju/schema"
	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jem/internal/auth"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/jemerror"
	"github.com/CanonicalLtd/jem/internal/jemserver"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/internal/servermon"
	"github.com/CanonicalLtd/jem/params"
)

var logger = loggo.GetLogger("jem.internal.v1")

type Handler struct {
	jem     *jem.JEM
	context context.Context
	config  jemserver.Params
	monReq  servermon.Request
}

func NewAPIHandler(jp *jem.Pool, ap *auth.Pool, sp jemserver.Params) ([]httprequest.Handler, error) {
	return jemerror.Mapper.Handlers(func(p httprequest.Params) (*Handler, error) {
		// All requests require an authenticated client.
		a := ap.Authenticator()
		defer a.Close()
		ctx, err := a.AuthenticateRequest(context.Background(), p.Request)
		if err != nil {
			return nil, errgo.Mask(err, errgo.Any)
		}
		h := &Handler{
			jem:     jp.JEM(),
			context: ctx,
			config:  sp,
		}
		h.monReq.Start(p.PathPattern)
		return h, nil
	}), nil
}

// Close implements io.Closer and is called by httprequest
// when the request is complete.
func (h *Handler) Close() error {
	h.jem.Close()
	h.jem = nil
	h.monReq.End()
	return nil
}

// WhoAmI returns authentication information on the client that is
// making the WhoAmI call.
func (h *Handler) WhoAmI(arg *params.WhoAmI) (params.WhoAmIResponse, error) {
	return params.WhoAmIResponse{
		User: auth.Username(h.context),
	}, nil
}

// AddController adds a new controller.
func (h *Handler) AddController(arg *params.AddController) error {
	if err := auth.CheckIsUser(h.context, arg.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	if !arg.Info.Public {
		return errgo.WithCausef(nil, params.ErrForbidden, "cannot add private controller")
	}
	if err := auth.CheckIsUser(h.context, h.jem.ControllerAdmin()); err != nil {
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

	location := make(map[string]string, 2)
	if arg.Info.Cloud != "" {
		location["cloud"] = string(arg.Info.Cloud)
	}
	if arg.Info.Region != "" {
		location["region"] = arg.Info.Region
	}

	ctl := &mongodoc.Controller{
		Path:          arg.EntityPath,
		CACert:        arg.Info.CACert,
		HostPorts:     [][]mongodoc.HostPort{hps},
		AdminUser:     arg.Info.User,
		AdminPassword: arg.Info.Password,
		UUID:          arg.Info.ControllerUUID,
		Public:        arg.Info.Public,
		Location:      location,
	}
	logger.Infof("dialling model")
	// Attempt to connect to the controller before accepting it.
	conn, err := h.jem.OpenAPIFromDoc(ctl)
	if err != nil {
		logger.Infof("cannot open API: %v", err)
		return badRequestf(err, "cannot connect to controller")
	}
	defer conn.Close()
	ctl.UUID = conn.ControllerTag().Id()

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

	err = h.jem.DB.AddController(ctl)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrAlreadyExists))
	}
	return nil
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
	ctl, err := h.jem.Controller(h.context, arg.EntityPath)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	neSchema, err := h.schemaForNewModel(arg.EntityPath, params.User(auth.Username(h.context)))
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return &params.ControllerResponse{
		Path:             arg.EntityPath,
		ProviderType:     neSchema.providerType,
		Schema:           neSchema.schema,
		Location:         ctl.Location,
		Public:           ctl.Public,
		UnavailableSince: newTime(ctl.UnavailableSince.UTC()),
	}, nil
}

// GetSchema returns the schema that should be used for
// the model configuration when starting a controller
// with a location matching p.Location.
//
//
// If controllers of more than one provider type
// are matched, it will return an error with a params.ErrAmbiguousLocation
// cause.
//
// If no controllers are matched, it will return an error with
// a params.ErrNotFound cause.
func (h *Handler) GetSchema(p httprequest.Params, arg *params.GetSchema) (*params.SchemaResponse, error) {
	lp, err := parseFormLocations(p.Request.Form)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	if len(lp.other) > 0 {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "no matching controllers")
	}
	return h.schemaForLocation(lp.cloud, lp.region)
}

// schemaForLocation returns the schema for the controllers matching
// the given location as a SchemaResponse.
// If the controllers selected by the location are not compatible,
// it returns an error with a params.ErrAmbiguousLocation cause.
// If there are no controllers selected, it returns an error with a
// params.ErrNotFound cause.
func (h *Handler) schemaForLocation(cloud params.Cloud, region string) (*params.SchemaResponse, error) {
	// TODO This will be insufficient when we can have several servers with the
	// same provider type but different versions that could potentially have
	// different configuration schemas. In that case, we could return a schema
	// that's the intersection of all the matched schemas and check that it's
	// valid for all of them before returning it.
	providerType := ""
	err := h.jem.DoControllers(h.context, cloud, region, func(ctl *mongodoc.Controller) error {

		if providerType != "" && ctl.Cloud.ProviderType != providerType {
			return errgo.WithCausef(nil, params.ErrAmbiguousLocation, "ambiguous location matches controller of more than one type")
		}
		providerType = ctl.Cloud.ProviderType
		return nil
	})
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrAmbiguousLocation), errgo.Is(params.ErrBadRequest))
	}
	if providerType == "" {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "no matching controllers")
	}
	schema, err := schemaForProviderType(providerType)
	if err != nil {
		return nil, errgo.Notef(err, "cannot get schema for provider type %q", providerType)
	}
	return &params.SchemaResponse{
		ProviderType: providerType,
		Schema:       schema,
	}, nil
}

// DeleteController removes an existing controller.
func (h *Handler) DeleteController(arg *params.DeleteController) error {
	// Check if user has permissions.
	if err := auth.CheckIsUser(h.context, arg.EntityPath.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	if !arg.Force {
		ctl, err := h.jem.DB.Controller(arg.EntityPath)
		if err != nil {
			return errgo.Mask(err, errgo.Is(params.ErrNotFound))
		}
		if ctl.UnavailableSince.IsZero() {
			return errgo.WithCausef(nil, params.ErrStillAlive, "cannot delete controller while it is still alive")
		}
	}
	if err := h.jem.DB.DeleteController(arg.EntityPath); err != nil {
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
	if err := h.jem.DB.CheckReadACL(h.context, h.jem.DB.Models(), arg.EntityPath); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}

	m, err := h.jem.DB.Model(arg.EntityPath)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	ctl, err := h.jem.DB.Controller(m.Controller)
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
	}
	return r, nil
}

// DeleteModel deletes an model from JEM.
func (h *Handler) DeleteModel(arg *params.DeleteModel) error {
	if err := auth.CheckIsUser(h.context, arg.EntityPath.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	if err := h.jem.DB.DeleteModel(arg.EntityPath); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrForbidden))
	}
	return nil
}

// ListModels returns all the models stored in JEM.
// Note that the models returned don't include the username or password.
// To gain access to a specific model, that model should be retrieved
// explicitly.
func (h *Handler) ListModels(arg *params.ListModels) (*params.ListModelsResponse, error) {
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
	modelIter := h.jem.DB.NewCanReadIter(h.context, h.jem.DB.Models().Find(nil).Sort("_id").Iter())
	var m mongodoc.Model
	for modelIter.Next(&m) {
		ctl, ok := controllers[m.Controller]
		if !ok {
			logger.Errorf("model %s has invalid controller value %s; omitting from result", m.Path, m.Controller)
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
	var controllers []params.ControllerResponse

	// TODO populate ProviderType and Schema fields when we have a cache
	// for the schemaForNewModel results.
	iter := h.jem.DB.NewCanReadIter(h.context, h.jem.DB.Controllers().Find(nil).Sort("_id").Iter())
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
	err = h.jem.DoControllers(h.context, lp.cloud, lp.region, func(ctl *mongodoc.Controller) error {
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
	err = h.jem.DoControllers(h.context, lp.cloud, lp.region, func(ctl *mongodoc.Controller) error {
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
	ctl, err := h.jem.Controller(h.context, arg.EntityPath)
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
	_, _, err = h.jem.CreateModel(h.context, jem.CreateModelParams{
		Path:           modelPath,
		ControllerPath: ctlPath,
		Credential:     args.Info.Credential,
		Cloud:          lp.cloud,
		Region:         lp.region,
		Attributes:     args.Info.Config,
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
func (h *Handler) SetModelPerm(arg *params.SetModelPerm) error {
	// TODO revoke access from all the users that currently
	// have access to the model that should not have access
	// now.
	return h.setPerm(h.jem.DB.Models(), arg.EntityPath, arg.ACL)
}

func (h *Handler) setPerm(coll *mgo.Collection, path params.EntityPath, acl params.ACL) error {
	// Only path.User (or members thereof) can change permissions.
	if err := auth.CheckIsUser(h.context, path.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	logger.Infof("set perm %s %s to %#v", coll.Name, path, acl)
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
	// Only the owner can read permissions.
	if err := auth.CheckIsUser(h.context, path.User); err != nil {
		return params.ACL{}, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	acl, err := h.jem.DB.GetACL(coll, path)
	if err != nil {
		return params.ACL{}, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return acl, nil
}

// UpdateCredential stores the provided credential under the provided,
// user, cloud and name. If there is already a credential with that name
// it is overwritten.
func (h *Handler) UpdateCredential(arg *params.UpdateCredential) error {
	// Only the owner can set credentials.
	if err := auth.CheckIsUser(h.context, arg.EntityPath.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	// TODO(mhilton) validate the credentials.
	err := h.jem.UpdateCredential(h.context, &mongodoc.Credential{
		Path:       arg.CredentialPath,
		Type:       arg.Credential.AuthType,
		Attributes: arg.Credential.Attributes,
	})
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}

type schemaForNewModel struct {
	providerType string
	schema       environschema.Fields
	checker      jujuschema.Checker
	skeleton     map[string]interface{}
}

// schemaForNewModel returns the schema for the configuration options
// for creating new models on the controller with the given id.
func (h *Handler) schemaForNewModel(ctlPath params.EntityPath, user params.User) (*schemaForNewModel, error) {
	st, err := h.jem.OpenAPI(ctlPath)
	if err != nil {
		return nil, errgo.NoteMask(err, "cannot open API", errgo.Is(params.ErrNotFound))
	}
	defer st.Close()

	var neSchema schemaForNewModel

	client := cloudapi.NewClient(st)
	defaultCloud, err := client.DefaultCloud()
	if err != nil {
		return nil, errgo.Notef(err, "cannot get base configuration")
	}
	cloudInfo, err := client.Cloud(defaultCloud)
	if err != nil {
		return nil, errgo.Notef(err, "cannot get base configuration")
	}
	neSchema.providerType = cloudInfo.Type
	neSchema.schema, err = schemaForProviderType(neSchema.providerType)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	fields, defaults, err := neSchema.schema.ValidationSchema()
	if err != nil {
		return nil, errgo.Notef(err, "cannot create validation schema for provider %s", neSchema.providerType)
	}
	neSchema.checker = jujuschema.FieldMap(fields, defaults)
	return &neSchema, nil
}

// schemaForProviderType returns the schema for the given
// provider type. This works currently because we link in
// all the provider code so we can do it locally.
//
// It's defined as a variable so it can be overridden in tests.
//
// TODO get the model schema over the juju API. We'll
// need make GetSchema be cleverer about mapping
// from provider type to schema in that case.
var schemaForProviderType = func(providerType string) (environschema.Fields, error) {
	provider, err := environs.Provider(providerType)
	if err != nil {
		return nil, errgo.Notef(err, "cannot get provider type %q", providerType)
	}
	schp, ok := provider.(interface {
		Schema() environschema.Fields
	})
	if !ok {
		return nil, errgo.Notef(err, "provider %q does not provide schema", providerType)
	}
	schema := schp.Schema()

	restrictedFields, err := modelmanager.RestrictedProviderFields(provider)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	// Remove everything from the schema that's restricted.
	for _, attr := range restrictedFields {
		delete(schema, attr)
	}
	// Also remove any attributes ending in "-path" because
	// they're only applicable locally.
	for name := range schema {
		if strings.HasSuffix(name, "-path") {
			delete(schema, name)
		}
	}
	// We're going to set the model name from the
	// JEM model path, so remove it from
	// the schema.
	delete(schema, "name")
	// TODO Delete admin-secret too, because it's never a valid
	// attribute for the client to provide. We can't do that right
	// now because it's the only secret attribute in the dummy
	// provider and we need it to test secret template attributes.
	// When Juju provides the schema over its API, that API call
	// should delete it before returning.
	return schema, nil
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
	hps = network.DropDuplicatedHostPorts(hps)
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
