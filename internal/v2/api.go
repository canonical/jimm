// Copyright 2016 Canonical Ltd.

package v2

import (
	"net/url"
	"sort"
	"time"

	"github.com/juju/httprequest"
	"github.com/juju/loggo"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/jemerror"
	"github.com/CanonicalLtd/jem/internal/jemserver"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/internal/servermon"
	"github.com/CanonicalLtd/jem/params"
)

var logger = loggo.GetLogger("jem.internal.v1")

type Handler struct {
	jem    *jem.JEM
	config jemserver.Params
	monReq servermon.Request
}

func NewAPIHandler(jp *jem.Pool, sp jemserver.Params) ([]httprequest.Handler, error) {
	return jemerror.Mapper.Handlers(func(p httprequest.Params) (*Handler, error) {
		// All requests require an authenticated client.
		h := &Handler{
			jem:    jp.JEM(),
			config: sp,
		}
		h.monReq.Start(p.PathPattern)
		if err := h.jem.Authenticate(p.Request); err != nil {
			h.Close()
			return nil, errgo.Mask(err, errgo.Any)
		}
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
		User: h.jem.Auth.Username,
	}, nil
}

// AddController adds a new controller.
func (h *Handler) AddController(arg *params.AddController) error {
	err := h.jem.AddController(&mongodoc.Controller{
		Path:          arg.EntityPath,
		CACert:        arg.Info.CACert,
		HostPorts:     arg.Info.HostPorts,
		AdminUser:     arg.Info.User,
		AdminPassword: arg.Info.Password,
		UUID:          arg.Info.ControllerUUID,
		Public:        arg.Info.Public,
	})
	return errgo.Mask(err, errgo.Is(params.ErrUnauthorized), errgo.Is(params.ErrBadRequest), errgo.Is(params.ErrAlreadyExists))
}

// GetController returns information on a controller.
func (h *Handler) GetController(arg *params.GetController) (*params.ControllerResponse, error) {
	ctl, err := h.jem.Controller(arg.EntityPath)
	if err != nil {
		if errgo.Cause(err) == params.ErrNotFound && h.jem.Auth.Username != string(arg.EntityPath.User) {
			err = params.ErrUnauthorized
		}
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized), errgo.Is(params.ErrNotFound))
	}
	loc := make(map[string]string, 2)
	loc["cloud"] = string(ctl.Cloud.Name)
	if len(ctl.Cloud.Regions) > 0 {
		loc["region"] = ctl.Cloud.Regions[0].Name
	}
	return &params.ControllerResponse{
		Path:             arg.EntityPath,
		ProviderType:     ctl.Cloud.ProviderType,
		Location:         loc,
		Public:           ctl.Public,
		UnavailableSince: newTime(ctl.UnavailableSince.UTC()),
	}, nil
}

// DeleteController removes an existing controller.
func (h *Handler) DeleteController(arg *params.DeleteController) error {
	if !arg.Force {
		ctl, err := h.jem.Controller(arg.EntityPath)
		if err != nil {
			return errgo.Mask(err, errgo.Is(params.ErrNotFound))
		}
		if ctl.UnavailableSince.IsZero() {
			return errgo.WithCausef(nil, params.ErrStillAlive, "cannot delete controller while it is still alive")
		}
	}
	if err := h.jem.DeleteController(arg.EntityPath); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return nil
}

// GetModel returns information on a given model.
func (h *Handler) GetModel(arg *params.GetModel) (*params.ModelResponse, error) {
	m, err := h.jem.Model(arg.EntityPath)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
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
		HostPorts:        ctl.HostPorts,
		ControllerPath:   m.Controller,
		Life:             m.Life,
		UnavailableSince: newTime(ctl.UnavailableSince.UTC()),
	}
	return r, nil
}

// DeleteModel deletes an model from JEM.
func (h *Handler) DeleteModel(arg *params.DeleteModel) error {
	if err := h.jem.DestroyModel(arg.EntityPath); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrForbidden), errgo.Is(params.ErrUnauthorized))
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
	modelIter := h.jem.CanReadIter(h.jem.DB.Models().Find(nil).Sort("_id").Iter())
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
			HostPorts:        ctl.HostPorts,
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

	iter := h.jem.CanReadIter(h.jem.DB.Controllers().Find(nil).Sort("_id").Iter())
	var ctl mongodoc.Controller
	for iter.Next(&ctl) {
		loc := map[string]string{"cloud": string(ctl.Cloud.Name)}
		if len(ctl.Cloud.Regions) > 0 {
			loc["region"] = ctl.Cloud.Regions[0].Name
		}
		controllers = append(controllers, params.ControllerResponse{
			Path:             ctl.Path,
			Public:           ctl.Public,
			UnavailableSince: newTime(ctl.UnavailableSince.UTC()),
			Location:         loc,
			ProviderType:     ctl.Cloud.ProviderType,
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
	err = h.jem.DoControllers(lp.cloud, lp.region, func(ctl *mongodoc.Controller) error {
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
	err = h.jem.DoControllers(lp.cloud, lp.region, func(ctl *mongodoc.Controller) error {
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
	ctl, err := h.jem.Controller(arg.EntityPath)
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
	modelPath := params.EntityPath{args.User, args.Info.Name}
	loc, err := cloudAndRegion(args.Info.Location)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	if len(loc.other) > 0 {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "no matching controllers found")
	}
	var controllerPath params.EntityPath
	if args.Info.Controller != nil {
		controllerPath = *args.Info.Controller
	}
	_, _, err = h.jem.CreateModel(jem.CreateModelParams{
		Path:           modelPath,
		ControllerPath: controllerPath,
		Credential:     args.Info.Credential,
		Cloud:          loc.cloud,
		Region:         loc.region,
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

// SetModelPerm sets the permissions on a model entity.
// Only the owner (arg.EntityPath.User) can change the permissions
// on an an entity. The owner can always read an entity, even
// if it has empty ACL.
func (h *Handler) SetModelPerm(arg *params.SetModelPerm) error {
	return errgo.Mask(
		h.jem.SetModelACL(arg.EntityPath, arg.ACL),
		errgo.Is(params.ErrNotFound),
		errgo.Is(params.ErrUnauthorized),
	)
}

func (h *Handler) setPerm(coll *mgo.Collection, path params.EntityPath, acl params.ACL) error {
	// Only path.User (or members thereof) can change permissions.
	if err := h.jem.CheckIsUser(path.User); err != nil {
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
	if err := h.jem.CheckIsUser(path.User); err != nil {
		return params.ACL{}, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	acl, err := h.jem.GetACL(coll, path)
	if err != nil {
		return params.ACL{}, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	sort.Strings(acl.Read)
	return acl, nil
}

// UpdateCredential stores the provided credential under the provided,
// user, cloud and name. If there is already a credential with that name
// it is overwritten.
func (h *Handler) UpdateCredential(arg *params.UpdateCredential) error {
	// TODO(mhilton) validate the credentials.
	err := h.jem.UpdateCredential(&mongodoc.Credential{
		User:       arg.EntityPath.User,
		Cloud:      arg.Cloud,
		Name:       arg.EntityPath.Name,
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
