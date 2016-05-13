// Copyright 2016 Canonical Ltd.

package v2

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/juju/httprequest"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/api/usermanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	controllermodelmanager "github.com/juju/juju/controller/modelmanager"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jujuschema "github.com/juju/schema"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jem/internal/apiconn"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/jemerror"
	"github.com/CanonicalLtd/jem/internal/jemserver"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

var logger = loggo.GetLogger("jem.internal.v1")

type Handler struct {
	jem    *jem.JEM
	config jemserver.Params
}

// randIntn is declared as a variable so that it can be overridden in tests.
var randIntn = rand.Intn

func NewAPIHandler(jp *jem.Pool, sp jemserver.Params) ([]httprequest.Handler, error) {
	return jemerror.Mapper.Handlers(func(p httprequest.Params) (*Handler, error) {
		// All requests require an authenticated client.
		h := &Handler{
			jem:    jp.JEM(),
			config: sp,
		}
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
	if err := h.jem.CheckIsUser(arg.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	if arg.Info.Public {
		admin := h.jem.ControllerAdmin()
		if admin == "" {
			return errgo.Newf("no controller admin configured")
		}
		if err := h.jem.CheckIsUser(admin); err != nil {
			if errgo.Cause(err) == params.ErrUnauthorized {
				return errgo.WithCausef(nil, params.ErrUnauthorized, "admin access required to add public controllers")
			}
			return errgo.Mask(err)
		}
		if len(arg.Info.Location) == 0 {
			return badRequestf(nil, "cannot add public controller with no location")
		}
	}
	if len(arg.Info.HostPorts) == 0 {
		return badRequestf(nil, "no host-ports in request")
	}
	if arg.Info.CACert == "" {
		return badRequestf(nil, "no ca-cert in request")
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
		HostPorts:     arg.Info.HostPorts,
		AdminUser:     arg.Info.User,
		AdminPassword: arg.Info.Password,
		UUID:          arg.Info.ControllerUUID,
		Location:      arg.Info.Location,
		Public:        arg.Info.Public,
	}
	m := &mongodoc.Model{
		UUID: arg.Info.ControllerUUID,
	}
	logger.Infof("dialling model")
	// Attempt to connect to the model before accepting it.
	conn, err := h.jem.OpenAPIFromDocs(m, ctl)
	if err != nil {
		logger.Infof("cannot open API: %v", err)
		return badRequestf(err, "cannot connect to model")
	}
	defer conn.Close()

	// Use the controller's UUID even if we've been given the UUID
	// of some model within it.
	info, err := conn.Client().ModelInfo()
	if err != nil {
		return errgo.Notef(err, "cannot get model information")
	}
	m.UUID = info.ControllerUUID
	ctl.UUID = info.ControllerUUID

	// Find out the provider type.
	// TODO cache all the schemaForNewModel information here
	// and store it with the controller?
	skeleton, err := modelmanager.NewClient(conn).ConfigSkeleton("", "")
	if err != nil {
		return errgo.Notef(err, "cannot get base configuration")
	}
	ctl.ProviderType = skeleton["type"].(string)

	// Update addresses from latest known in controller.
	// Note that state.APIHostPorts is always guaranteed
	// to include the actual address we succeeded in
	// connecting to.
	ctl.HostPorts = collapseHostPorts(conn.APIHostPorts())

	err = h.jem.AddController(ctl, m)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrAlreadyExists))
	}
	return nil
}

// GetController returns information on a controller.
func (h *Handler) GetController(arg *params.GetController) (*params.ControllerResponse, error) {
	if err := h.jem.CheckReadACL(h.jem.DB.Controllers(), arg.EntityPath); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	ctl, err := h.jem.Controller(arg.EntityPath)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	neSchema, err := h.schemaForNewModel(arg.EntityPath)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return &params.ControllerResponse{
		Path:         arg.EntityPath,
		ProviderType: neSchema.providerType,
		Schema:       neSchema.schema,
		Location:     ctl.Location,
		Public:       ctl.Public,
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
	attrs, err := formToLocationAttrs(p.Request.Form)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	return h.schemaForLocation(attrs)
}

// schemaForLocation returns the schema for the controllers matching
// the given location as a SchemaResponse.
// If the controllers selected by the location are not compatible,
// it returns an error with a params.ErrAmbiguousLocation cause.
// If there are no controllers selected, it returns an error with a
// params.ErrNotFound cause.
func (h *Handler) schemaForLocation(location map[string]string) (*params.SchemaResponse, error) {
	// TODO This will be insufficient when we can have several servers with the
	// same provider type but different versions that could potentially have
	// different configuration schemas. In that case, we could return a schema
	// that's the intersection of all the matched schemas and check that it's
	// valid for all of them before returning it.
	providerType := ""
	err := h.doControllers(location, func(ctl *mongodoc.Controller) error {
		if providerType != "" && ctl.ProviderType != providerType {
			return errgo.WithCausef(nil, params.ErrAmbiguousLocation, "ambiguous location matches controller of more than one type")
		}
		providerType = ctl.ProviderType
		return nil
	})
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrAmbiguousLocation))
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
	if err := h.jem.CheckIsUser(arg.EntityPath.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
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
		strings.HasSuffix(s, " access")
}

// GetModel returns information on a given model.
func (h *Handler) GetModel(arg *params.GetModel) (*params.ModelResponse, error) {
	if err := h.jem.CheckReadACL(h.jem.DB.Models(), arg.EntityPath); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}

	m, err := h.jem.Model(arg.EntityPath)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	ctl, err := h.jem.Controller(m.Controller)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	conn, err := h.jem.OpenAPI(m.Controller)
	if err != nil {
		return nil, errgo.NoteMask(err, "cannot connect to controller", errgo.Is(params.ErrNotFound))
	}
	defer conn.Close()

	jujuUser := "jem-" + h.jem.Auth.Username
	password, err := h.ensureUser(conn, jujuUser, m.Controller)
	if err != nil {
		return nil, errgo.Notef(err, "cannot create user")
	}
	if !m.Users[mongodoc.Sanitize(jujuUser)].Granted {
		// Either we've explicitly removed access from the user
		// or the user didn't previously exist. Ether way,
		// make grant the account access to the model.
		//
		// Note that we *don't* grant access if we have recorded
		// that we already granted access. This has an important
		// security ramification: if someone goes directly to
		// the controller and revokes access to a particular user,
		// we won't immediately add access back again.
		mmClient := modelmanager.NewClient(conn)
		if err := mmClient.GrantModel(jujuUser, string(jujuparams.ModelWriteAccess), m.UUID); err != nil && !isAlreadyGrantedError(err) {
			return nil, errgo.Notef(err, "cannot grant access rights to %q", jujuUser)
		}

		// Add the user to the set of users managed by the model.
		if err := h.jem.SetModelManagedUser(m.Path, jujuUser, mongodoc.ModelUserInfo{
			Granted: true,
		}); err != nil {
			// We've failed to update the database but the user
			// has already been granted permission.
			//
			// This means we'll need to be careful about removing
			// user permissions - just because a user is not in the
			// model users doesn't necessarily mean we don't manage
			// that user. Being conservative about adding permissions
			// and less so about removing permissions should
			// work OK.
			return nil, errgo.Notef(err, "cannot update model users")
		}
	}

	r := &params.ModelResponse{
		Path:           arg.EntityPath,
		User:           jujuUser,
		Password:       password,
		UUID:           m.UUID,
		ControllerUUID: conn.Info.ModelTag.Id(),
		CACert:         ctl.CACert,
		HostPorts:      ctl.HostPorts,
		ControllerPath: m.Controller,
		Life:           m.Life,
	}
	if !ctl.UnavailableSince.IsZero() {
		ctl.UnavailableSince = ctl.UnavailableSince.UTC()
		r.UnavailableSince = &ctl.UnavailableSince
	}
	return r, nil
}

// DeleteModel deletes an model from JEM.
func (h *Handler) DeleteModel(arg *params.DeleteModel) error {
	if err := h.jem.CheckIsUser(arg.EntityPath.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	if err := h.jem.DeleteModel(arg.EntityPath); err != nil {
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

	// We get all controllers first, because many models
	// will be sharing the same controllers.
	// TODO we could do better than this and avoid
	// gathering all the controllers into memory.
	// Possiblities include caching controllers, and
	// gathering results to do only a few
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
		var uvt *time.Time
		if !ctl.UnavailableSince.IsZero() {
			ctl.UnavailableSince = ctl.UnavailableSince.UTC()
			uvt = &ctl.UnavailableSince
		}
		models = append(models, params.ModelResponse{
			Path:             m.Path,
			UUID:             m.UUID,
			ControllerUUID:   ctl.UUID,
			CACert:           ctl.CACert,
			HostPorts:        ctl.HostPorts,
			ControllerPath:   m.Controller,
			Life:             m.Life,
			UnavailableSince: uvt,
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
// Currently the Template  and ProviderType field in each ControllerResponse is not
// populated.
func (h *Handler) ListController(arg *params.ListController) (*params.ListControllerResponse, error) {
	var controllers []params.ControllerResponse

	// TODO populate ProviderType and Schema fields when we have a cache
	// for the schemaForNewModel results.
	iter := h.jem.CanReadIter(h.jem.DB.Controllers().Find(nil).Sort("_id").Iter())
	var ctl mongodoc.Controller
	for iter.Next(&ctl) {
		var uvt *time.Time
		if !ctl.UnavailableSince.IsZero() {
			ctl.UnavailableSince = ctl.UnavailableSince.UTC()
			uvt = &ctl.UnavailableSince
		}
		controllers = append(controllers, params.ControllerResponse{
			Path:             ctl.Path,
			Location:         ctl.Location,
			Public:           ctl.Public,
			UnavailableSince: uvt,
		})
	}
	if err := iter.Err(); err != nil {
		return nil, errgo.Notef(err, "cannot get models")
	}
	return &params.ListControllerResponse{
		Controllers: controllers,
	}, nil
}

// GetControllerLocations returns all the available values for a given controller
// location attribute. The set of controllers is constrained by the URL query
// parameters.
func (h *Handler) GetControllerLocations(p httprequest.Params, arg *params.GetControllerLocations) (*params.ControllerLocationsResponse, error) {
	attr := arg.Attr
	if !params.IsValidLocationAttr(attr) {
		return nil, badRequestf(nil, "invalid location %q", attr)
	}
	attrs, err := formToLocationAttrs(p.Request.Form)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	found := make(map[string]bool)
	err = h.doControllers(attrs, func(ctl *mongodoc.Controller) error {
		if val, ok := ctl.Location[attr]; ok {
			found[val] = true
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
	attrs, err := formToLocationAttrs(p.Request.Form)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	locSet := make(map[string]map[string]string)
	err = h.doControllers(attrs, func(ctl *mongodoc.Controller) error {
		if len(ctl.Location) == 0 {
			// Ignore controllers with no location set.
			return nil
		}
		data, err := json.Marshal(ctl.Location)
		if err != nil {
			panic(errgo.Notef(err, "can't marshal map for some weird reason"))
		}
		locSet[string(data)] = ctl.Location
		return nil
	})
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	ordered := make([]string, 0, len(locSet))
	for k := range locSet {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)
	result := make([]map[string]string, len(ordered))
	for i := range ordered {
		result[i] = locSet[ordered[i]]
	}
	return &params.AllControllerLocationsResponse{
		Locations: result,
	}, nil
}

// GetControllerLocation returns a map of location attributes for a given controller.
func (h *Handler) GetControllerLocation(arg *params.GetControllerLocation) (params.ControllerLocation, error) {
	if err := h.jem.CheckReadACL(h.jem.DB.Controllers(), arg.EntityPath); err != nil {
		return params.ControllerLocation{}, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	ctl, err := h.jem.Controller(arg.EntityPath)
	if err != nil {
		return params.ControllerLocation{}, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return params.ControllerLocation{
		Location: ctl.Location,
	}, nil
}

// SetControllerLocation updates the attributes associated with the controller's location.
// Only the owner (arg.EntityPath.User) can change the location attributes
// on an an entity.
func (h *Handler) SetControllerLocation(arg *params.SetControllerLocation) error {
	if err := h.jem.CheckIsUser(arg.EntityPath.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	return h.jem.SetControllerLocation(arg.EntityPath, arg.Location.Location)
}

// configWithTemplates returns the given configuration applied
// along with the given templates.
// Each template is applied in turn, then the configuration
// is added on top of that.
func (h *Handler) configWithTemplates(config map[string]interface{}, tmpls []*mongodoc.Template) map[string]interface{} {
	result := make(map[string]interface{})
	for _, tmpl := range tmpls {
		for name, val := range tmpl.Config {
			result[name] = val
		}
	}
	for name, val := range config {
		result[name] = val
	}
	return result
}

// NewModel creates a new model inside an existing Controller.
func (h *Handler) NewModel(args *params.NewModel) (*params.ModelResponse, error) {
	if err := h.jem.CheckIsUser(args.User); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	tmpls, err := h.getTemplates(args.Info.TemplatePaths)
	if err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			err = errgo.WithCausef(err, params.ErrBadRequest, "%s", "")
		}
		return nil, errgo.NoteMask(err, "invalid template provided", errgo.Is(params.ErrBadRequest), errgo.Is(params.ErrUnauthorized))
	}
	tmplLocation, err := h.locationForTemplates(tmpls)
	if err != nil {
		return nil, errgo.NoteMask(err, "invalid templates", errgo.Is(params.ErrIncompatibleTemplateLocations))
	}
	ctlPath, err := h.selectController(&args.Info, tmplLocation)
	if err != nil {
		return nil, errgo.NoteMask(err, "cannot select controller", errgo.Is(params.ErrBadRequest), errgo.Is(params.ErrNotFound), errgo.Is(params.ErrIncompatibleTemplateLocations))
	}
	if err := h.jem.CheckReadACL(h.jem.DB.Controllers(), ctlPath); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	conn, err := h.jem.OpenAPI(ctlPath)
	if err != nil {
		return nil, errgo.NoteMask(err, "cannot connect to controller", errgo.Is(params.ErrNotFound))
	}
	defer conn.Close()

	neSchema, err := h.schemaForNewModel(ctlPath)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	config := h.configWithTemplates(args.Info.Config, tmpls)
	// Ensure that the attributes look reasonably OK before bothering
	// the controller with them.
	attrs, err := neSchema.checker.Coerce(config, nil)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "cannot validate attributes")
	}

	modelPath := params.EntityPath{args.User, args.Info.Name}
	// Create the model record in the database before actually
	// creating the model on the controller. It will have an invalid
	// UUID because it doesn't exist but that's better than creating
	// an model that we can't add locally because the name
	// already exists.
	modelDoc := &mongodoc.Model{
		Path:       modelPath,
		Controller: ctlPath,
	}
	if err := h.jem.AddModel(modelDoc); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrAlreadyExists))
	}

	fields := attrs.(map[string]interface{})
	// Add the values from the skeleton to the configuration.
	for name, field := range neSchema.skeleton {
		fields[name] = field
	}
	// Add the model name.
	// Note that AddModel has set modelDoc.Id for us.
	fields["name"] = idToModelName(modelDoc.Id)

	mmClient := modelmanager.NewClient(conn.Connection)
	// Always grant access to the user that we use to connect
	// to the controller.
	adminUser := conn.Info.Tag.(names.UserTag).Id()
	m, err := mmClient.CreateModel(adminUser, nil, fields)
	if err != nil {
		// Remove the model that was created, because it's no longer valid.
		if err := h.jem.DB.Models().RemoveId(modelDoc.Id); err != nil {
			logger.Errorf("cannot remove model from database after model creation error: %v", err)
		}
		return nil, errgo.Notef(err, "cannot create model")
	}
	// Now set the UUID to that of the actually created model.
	if err := h.jem.DB.Models().UpdateId(modelDoc.Id, bson.D{{"$set", bson.D{{"uuid", m.UUID}}}}); err != nil {
		return nil, errgo.Notef(err, "cannot update model UUID in database, leaked model %s", m.UUID)
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

// selectController selects a controller suitable for starting a new model in,
// based on the criteria specified in info, and returns its path.
func (h *Handler) selectController(info *params.NewModelInfo, tmplLocation map[string]string) (params.EntityPath, error) {
	if info.Controller != nil {
		return *info.Controller, nil
	}
	location := make(map[string]string)
	mergeLocations(location, tmplLocation)
	if !mergeLocations(location, info.Location) {
		return params.EntityPath{}, errgo.WithCausef(nil, params.ErrIncompatibleTemplateLocations, "specified location is incompatible with templates")
	}
	q, err := h.jem.ControllerLocationQuery(location)
	if err != nil {
		return params.EntityPath{}, errgo.WithCausef(err, params.ErrBadRequest, "%s", "")
	}
	// Sort by _id so that we can make easily reproducible tests.
	iter := h.jem.CanReadIter(q.Sort("_id").Iter())

	var controllers []mongodoc.Controller
	var ctl mongodoc.Controller
	for iter.Next(&ctl) {
		controllers = append(controllers, ctl)
	}
	if err := iter.Err(); err != nil {
		return params.EntityPath{}, errgo.Newf("cannot get controllers")
	}
	if len(controllers) == 0 {
		return params.EntityPath{}, errgo.WithCausef(nil, params.ErrNotFound, "no matching controllers found")
	}
	// Choose a random controller.
	// TODO select a controller more intelligently, for example
	// by choosing the most lightly loaded controller
	n := randIntn(len(controllers))
	return controllers[n].Path, nil
}

// AddTemplate adds or updates a new template.
func (h *Handler) AddTemplate(arg *params.AddTemplate) error {
	return h.addTemplate(arg.EntityPath, arg.Info, true)
}

// AddNewTemplate adds a new template. It fails if a template with the new
// name already exists.
func (h *Handler) AddNewTemplate(arg *params.AddNewTemplate) error {
	return h.addTemplate(arg.EntityPath, arg.Info, false)
}

// addTemplate adds or update a new template.
func (h *Handler) addTemplate(path params.EntityPath, info params.AddTemplateInfo, overwrite bool) error {
	if err := h.jem.CheckIsUser(path.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	var schema environschema.Fields
	var providerType string
	if info.Controller != nil {
		if len(info.Location) > 0 {
			return badRequestf(nil, "cannot specify location with explicit controller")
		}
		neSchema, err := h.schemaForNewModel(*info.Controller)
		if err != nil {
			return errgo.Notef(err, "cannot get schema for controller")
		}
		schema = neSchema.schema
		providerType = neSchema.providerType
	} else {
		r, err := h.schemaForLocation(info.Location)
		if err != nil {
			return errgo.NoteMask(err, "cannot get schema for controller", errgo.Is(params.ErrNotFound), errgo.Is(params.ErrAmbiguousLocation))
		}
		schema = r.Schema
		providerType = r.ProviderType
	}

	fields, defaults, err := schema.ValidationSchema()
	if err != nil {
		return errgo.Notef(err, "cannot create validation schema for provider %s", providerType)
	}
	// Delete all fields and defaults that are not in the
	// provided configuration attributes, so that we can
	// check only the provided fields for compatibility
	// without worrying about other mandatory fields.
	for name := range fields {
		if _, ok := info.Config[name]; !ok {
			delete(fields, name)
		}
	}
	for name := range defaults {
		if _, ok := info.Config[name]; !ok {
			delete(defaults, name)
		}
	}
	result, err := jujuschema.StrictFieldMap(fields, defaults).Coerce(info.Config, nil)
	if err != nil {
		return badRequestf(err, "configuration not compatible with schema")
	}
	if err := h.jem.AddTemplate(&mongodoc.Template{
		Path:     path,
		Schema:   schema,
		Config:   result.(map[string]interface{}),
		Location: info.Location,
	}, overwrite); err != nil {
		if errgo.Cause(err) == params.ErrAlreadyExists {
			return badRequestf(err, "%s", "")
		}
		return errgo.Notef(err, "cannot add template")
	}
	return nil
}

// GetTemplate returns information on a single template.
func (h *Handler) GetTemplate(arg *params.GetTemplate) (*params.TemplateResponse, error) {
	tmpl, err := h.jem.Template(arg.EntityPath)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if err := h.jem.CheckCanRead(tmpl); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	hideTemplateSecrets(tmpl)
	return &params.TemplateResponse{
		Path:     arg.EntityPath,
		Schema:   tmpl.Schema,
		Config:   tmpl.Config,
		Location: tmpl.Location,
	}, nil
}

// DeleteTemplate deletes a template.
func (h *Handler) DeleteTemplate(arg *params.DeleteTemplate) error {
	if err := h.jem.CheckIsUser(arg.EntityPath.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	if err := h.jem.DeleteTemplate(arg.EntityPath); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return nil
}

// hideTemplateSecrets zeros all secret fields in the
// given template.
func hideTemplateSecrets(tmpl *mongodoc.Template) {
	for name, attr := range tmpl.Config {
		if tmpl.Schema[name].Secret {
			tmpl.Config[name] = zeroValueOf(attr)
		}
	}
}

func zeroValueOf(x interface{}) interface{} {
	return reflect.Zero(reflect.TypeOf(x)).Interface()
}

// ListTemplates returns information on all accessible templates.
func (h *Handler) ListTemplates(arg *params.ListTemplates) (*params.ListTemplatesResponse, error) {
	// TODO provide a way of restricting the results.
	iter := h.jem.CanReadIter(h.jem.DB.Templates().Find(nil).Sort("_id").Iter())
	var tmpls []params.TemplateResponse
	var tmpl mongodoc.Template
	for iter.Next(&tmpl) {
		hideTemplateSecrets(&tmpl)
		tmpls = append(tmpls, params.TemplateResponse{
			Path:     tmpl.Path,
			Schema:   tmpl.Schema,
			Config:   tmpl.Config,
			Location: tmpl.Location,
		})
	}
	if err := iter.Err(); err != nil {
		return nil, errgo.Notef(err, "cannot get templates")
	}
	return &params.ListTemplatesResponse{
		Templates: tmpls,
	}, nil
}

// GetTemplatePerm returns the ACL for a given template.
// Only the owner (arg.EntityPath.User) can read the ACL.
func (h *Handler) GetTemplatePerm(arg *params.GetTemplatePerm) (params.ACL, error) {
	return h.getPerm(h.jem.DB.Templates(), arg.EntityPath)
}

// SetTemplatePerm sets the permissions on a template entity.
// Only the owner (arg.EntityPath.User) can change the permissions
// on an entity. The owner can always read an entity, even
// if it has an empty ACL.
func (h *Handler) SetTemplatePerm(arg *params.SetTemplatePerm) error {
	return h.setPerm(h.jem.DB.Templates(), arg.EntityPath, arg.ACL)
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
	return acl, nil
}

func idToModelName(id string) string {
	return strings.Replace(id, "/", "_", -1)
}

// ensureUser ensures that the given user account exists.
// If the account does not already exist, then it is created.
// It returns the password for the account.
func (h *Handler) ensureUser(conn *apiconn.Conn, user string, ctlName params.EntityPath) (string, error) {
	password, err := h.jem.EnsureUser(ctlName, user)
	if err != nil {
		return "", errgo.Mask(err)
	}
	// We have a record of the user account, but it might not
	// have been created in the controller yet, so check that it really exists.
	// and if so, set the password appropriately. This should
	// converge even if we have the same user concurrently creating
	// models.
	uclient := usermanager.NewClient(conn.Connection)
	uinfo, err := uclient.UserInfo([]string{user}, usermanager.AllUsers)
	if err == nil {
		if uinfo[0].Disabled {
			// The user has been explicitly disabled.
			// Don't override that.
			return "", errgo.Newf("user %q has been disabled explicitly", user)
		}
		// The user exists, but set the password appropriately
		// anyway in case it aleady existed with the wrong password
		// (perhaps because JEM lost its database state).
		if err := uclient.SetPassword(user, password); err != nil {
			return "", errgo.Notef(err, "cannot set user password")
		}
		return password, nil
	}
	// Unfortunately there's no way to find out if it's a "not found" error
	// (see https://bugs.launchpad.net/juju-core/+bug/1561526)
	// so we assume that any error means the user account doesn't
	// exist and go ahead with creating the account anyway.
	_, _, err = uclient.AddUser(user, "", password, "")
	if err == nil {
		return password, nil
	}
	if err, ok := errgo.Cause(err).(*jujuparams.Error); ok && err.Code == jujuparams.CodeAlreadyExists {
		// The user's been created a moment ago. Assume that it's
		// by an instance of the same server and therefore will have the
		// same password.
		return password, nil
	}
	return "", errgo.Notef(err, "cannot add user")
}

type schemaForNewModel struct {
	providerType string
	schema       environschema.Fields
	checker      jujuschema.Checker
	skeleton     map[string]interface{}
}

// schemaForNewModel returns the schema for the configuration options
// for creating new models on the controller with the given id.
func (h *Handler) schemaForNewModel(ctlPath params.EntityPath) (*schemaForNewModel, error) {
	st, err := h.jem.OpenAPI(ctlPath)
	if err != nil {
		return nil, errgo.NoteMask(err, "cannot open API", errgo.Is(params.ErrNotFound))
	}
	defer st.Close()

	var neSchema schemaForNewModel

	client := modelmanager.NewClient(st)
	neSchema.skeleton, err = client.ConfigSkeleton("", "")
	if err != nil {
		return nil, errgo.Notef(err, "cannot get base configuration")
	}
	neSchema.providerType = neSchema.skeleton["type"].(string)
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
// TODO get the model schema over the juju API. We'll
// need make GetSchema be cleverer about mapping
// from provider type to schema in that case.
func schemaForProviderType(providerType string) (environschema.Fields, error) {
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

	restrictedFields, err := controllermodelmanager.RestrictedProviderFields(providerType)
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

// doControllers calls the given function for each controller that
// can be read by the current user that matches the given attributes.
// If the function returns an error, the iteration stops and
// doControllers returns the error with the same cause.
func (h *Handler) doControllers(attrs map[string]string, do func(c *mongodoc.Controller) error) error {
	// Query all the controllers that match the attributes, building
	// up all the possible values.
	q, err := h.jem.ControllerLocationQuery(attrs)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "%s", "")
	}
	iter := h.jem.CanReadIter(q.Iter())
	var ctl mongodoc.Controller
	for iter.Next(&ctl) {
		if err := do(&ctl); err != nil {
			iter.Close()
			return errgo.Mask(err, errgo.Any)
		}
	}
	if err := iter.Err(); err != nil {
		return errgo.Notef(err, "cannot query")
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

// getTemplates gets the templates at all the given paths.
// It returns a params.ErrNotFound error if any of them
// aren't found, or params.ErrUnauthorized if the user
// doesn't have permission to access them.
func (h *Handler) getTemplates(paths []params.EntityPath) ([]*mongodoc.Template, error) {
	tmpls := make([]*mongodoc.Template, len(paths))
	for i, path := range paths {
		tmpl, err := h.jem.Template(path)
		if err != nil {
			return nil, errgo.NoteMask(err, fmt.Sprintf("cannot get template %q", path), errgo.Is(params.ErrNotFound))
		}
		if err := h.jem.CheckCanRead(tmpl); err != nil {
			return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
		}
		tmpls[i] = tmpl
	}
	return tmpls, nil
}

// locationForTemplates returns the location implied by the given
// templates. If the locations are not compatible, it returns a
// params.ErrIncompatibleTemplateLocations error.
func (h *Handler) locationForTemplates(tmpls []*mongodoc.Template) (map[string]string, error) {
	loc := make(map[string]string)
	for _, tmpl := range tmpls {
		if !mergeLocations(loc, tmpl.Location) {
			return nil, errgo.WithCausef(nil, params.ErrIncompatibleTemplateLocations, "template %v is incompatible with earlier templates", tmpl.Path)
		}
	}
	return loc, nil
}

// mergeLocations adds all the attributes in loc1 to loc0;
// it reports whether the merge succeeded - the merge
// will fail if the two maps have attributes with the same
// name but different values.
// TODO refactor things so that we can produce a nicer
// error message that says what the incompatiblity was.
func mergeLocations(loc0, loc1 map[string]string) bool {
	for attr, val := range loc1 {
		if v, ok := loc0[attr]; ok && v != val {
			return false
		}
		loc0[attr] = val
	}
	return true
}
