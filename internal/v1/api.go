package v1

import (
	"reflect"
	"strings"

	"github.com/juju/httprequest"
	"github.com/juju/juju/api/environmentmanager"
	"github.com/juju/juju/api/usermanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/schema"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jem/internal/apiconn"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/jemerror"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

var logger = loggo.GetLogger("jem.internal.v1")

type Handler struct {
	jem    *jem.JEM
	config jem.ServerParams
}

func NewAPIHandler(jp *jem.Pool, sp jem.ServerParams) ([]httprequest.Handler, error) {
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
	if len(arg.Info.HostPorts) == 0 {
		return badRequestf(nil, "no host-ports in request")
	}
	if arg.Info.CACert == "" {
		return badRequestf(nil, "no ca-cert in request")
	}
	if arg.Info.User == "" {
		return badRequestf(nil, "no user in request")
	}
	if !names.IsValidEnvironment(arg.Info.ModelUUID) {
		return badRequestf(nil, "bad model UUID in request")
	}
	ctl := &mongodoc.Controller{
		Path:      arg.EntityPath,
		CACert:    arg.Info.CACert,
		HostPorts: arg.Info.HostPorts,
	}
	m := &mongodoc.Model{
		AdminUser:     arg.Info.User,
		AdminPassword: arg.Info.Password,
		UUID:          arg.Info.ModelUUID,
	}
	logger.Infof("dialling model")
	// Attempt to connect to the model before accepting it.
	conn, err := h.jem.OpenAPIFromDocs(m, ctl)
	if err != nil {
		logger.Infof("cannot open API: %v", err)
		return badRequestf(err, "cannot connect to model")
	}
	defer conn.Close()

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
	neSchema, err := h.schemaForNewModel(arg.EntityPath)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return &params.ControllerResponse{
		Path:         arg.EntityPath,
		ProviderType: neSchema.providerType,
		Schema:       neSchema.schema,
	}, nil
}

// DeleteController removes an existing controller.
func (h *Handler) DeleteController(arg *params.DeleteController) error {
	// Check if user has permissions.
	if err := h.jem.CheckIsUser(arg.EntityPath.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	if err := h.jem.DeleteController(arg.EntityPath); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return nil
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
	return &params.ModelResponse{
		Path:      arg.EntityPath,
		User:      m.AdminUser,
		Password:  m.AdminPassword,
		UUID:      m.UUID,
		CACert:    ctl.CACert,
		HostPorts: ctl.HostPorts,
	}, nil
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
		models = append(models, params.ModelResponse{
			Path:      m.Path,
			User:      m.AdminUser,
			Password:  m.AdminPassword,
			UUID:      m.UUID,
			CACert:    ctl.CACert,
			HostPorts: ctl.HostPorts,
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
		controllers = append(controllers, params.ControllerResponse{
			Path: ctl.Path,
		})
	}
	if err := iter.Err(); err != nil {
		return nil, errgo.Notef(err, "cannot get models")
	}
	return &params.ListControllerResponse{
		Controllers: controllers,
	}, nil
}

// configWithTemplates returns the given configuration applied
// along with the given templates.
// Each template is applied in turn, then the configuration
// is added on top of that.
func (h *Handler) configWithTemplates(config map[string]interface{}, paths []params.EntityPath) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	for _, path := range paths {
		tmpl, err := h.jem.Template(path)
		if err != nil {
			return nil, errgo.Notef(err, "cannot get template %q", path)
		}
		if err := h.jem.CheckCanRead(tmpl); err != nil {
			return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
		}
		for name, val := range tmpl.Config {
			result[name] = val
		}
	}
	for name, val := range config {
		result[name] = val
	}
	return result, nil
}

// NewModel creates a new model inside an existing Controller.
func (h *Handler) NewModel(args *params.NewModel) (*params.ModelResponse, error) {
	if err := h.jem.CheckIsUser(args.User); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	if err := h.jem.CheckReadACL(h.jem.DB.Controllers(), args.Info.Controller); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	conn, err := h.jem.OpenAPI(args.Info.Controller)
	if err != nil {
		return nil, errgo.NoteMask(err, "cannot connect to controller", errgo.Is(params.ErrNotFound))
	}
	defer conn.Close()

	neSchema, err := h.schemaForNewModel(args.Info.Controller)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	config, err := h.configWithTemplates(args.Info.Config, args.Info.TemplatePaths)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	// Ensure that the attributes look reasonably OK before bothering
	// the controller with them.
	attrs, err := neSchema.checker.Coerce(config, nil)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "cannot validate attributes")
	}
	// We create a user for the model so that the caller knows
	// what password to use when accessing their new model.
	// When juju supports macaroon authorization, we won't need
	// to do this - we could just inform the controller of the required
	// user/group (args.User) instead.
	modelPath := params.EntityPath{args.User, args.Info.Name}
	jujuUser := userForModel(modelPath)
	if err := h.createUser(conn, jujuUser, args.Info.Password); err != nil {
		return nil, errgo.NoteMask(err, "cannot create user", errgo.Is(params.ErrBadRequest))
	}
	logger.Infof("created user %q", args.User)

	// Create the model record in the database before actually
	// creating the model on the controller. It will have an invalid
	// UUID because it doesn't exist but that's better than creating
	// an model that we can't add locally because the name
	// already exists.
	modelDoc := &mongodoc.Model{
		Path:          modelPath,
		AdminUser:     jujuUser,
		AdminPassword: args.Info.Password,
		Controller:    args.Info.Controller,
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

	emclient := environmentmanager.NewClient(conn.Connection)
	m, err := emclient.CreateEnvironment(jujuUser, nil, fields)
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
	return &params.ModelResponse{
		Path:           modelPath,
		User:           jujuUser,
		Password:       args.Info.Password,
		ControllerUUID: conn.Info.EnvironTag.Id(),
		UUID:           m.UUID,
		CACert:         conn.Info.CACert,
		HostPorts:      conn.Info.Addrs,
	}, nil
}

// AddTemplate adds a new template.
func (h *Handler) AddTemplate(arg *params.AddTemplate) error {
	if err := h.jem.CheckIsUser(arg.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	neSchema, err := h.schemaForNewModel(arg.Info.Controller)
	if err != nil {
		return errgo.Notef(err, "cannot get schema for controller")
	}

	fields, defaults, err := neSchema.schema.ValidationSchema()
	if err != nil {
		return errgo.Notef(err, "cannot create validation schema for provider %s", neSchema.providerType)
	}
	// Delete all fields and defaults that are not in the
	// provided configuration attributes, so that we can
	// check only the provided fields for compatibility
	// without worrying about other mandatory fields.
	for name := range fields {
		if _, ok := arg.Info.Config[name]; !ok {
			delete(fields, name)
		}
	}
	for name := range defaults {
		if _, ok := arg.Info.Config[name]; !ok {
			delete(defaults, name)
		}
	}
	result, err := schema.StrictFieldMap(fields, defaults).Coerce(arg.Info.Config, nil)
	if err != nil {
		return badRequestf(err, "configuration not compatible with schema")
	}
	if err := h.jem.AddTemplate(&mongodoc.Template{
		Path:   arg.EntityPath,
		Schema: neSchema.schema,
		Config: result.(map[string]interface{}),
	}); err != nil {
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
		Path:   arg.EntityPath,
		Schema: tmpl.Schema,
		Config: tmpl.Config,
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
			Path:   tmpl.Path,
			Schema: tmpl.Schema,
			Config: tmpl.Config,
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

// userForModel returns the juju user to use for
// the model with the given path.
// This should go when juju supports macaroon
// authorization.
func userForModel(p params.EntityPath) string {
	return "jem-" + string(p.User) + "--" + string(p.Name)
}

func idToModelName(id string) string {
	return strings.Replace(id, "/", "_", -1)
}

// createUser creates the given user account with the
// given password. If the account already exists then it changes
// the password accordingly.
func (h *Handler) createUser(conn *apiconn.Conn, user, password string) error {
	if password == "" {
		return badRequestf(nil, "no password specified")
	}
	uclient := usermanager.NewClient(conn.Connection)
	_, err := uclient.AddUser(user, "", password)
	if err == nil {
		return nil
	}
	if err, ok := errgo.Cause(err).(*jujuparams.Error); ok && err.Code != jujuparams.CodeAlreadyExists {
		return errgo.Notef(err, "cannot add user")
	}
	err = uclient.SetPassword(user, password)
	if err != nil {
		return errgo.Notef(err, "cannot change password")
	}
	return nil
}

type schemaForNewModel struct {
	providerType string
	schema       environschema.Fields
	checker      schema.Checker
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

	client := environmentmanager.NewClient(st)
	neSchema.skeleton, err = client.ConfigSkeleton("", "")
	if err != nil {
		return nil, errgo.Notef(err, "cannot get base configuration")
	}
	neSchema.providerType = neSchema.skeleton["type"].(string)
	provider, err := environs.Provider(neSchema.providerType)
	if err != nil {
		return nil, errgo.Notef(err, "cannot get provider type %q", neSchema.providerType)
	}
	schp, ok := provider.(interface {
		Schema() environschema.Fields
	})
	if !ok {
		return nil, errgo.Notef(err, "provider %q does not provide schema", neSchema.providerType)
	}
	// TODO get the model schema over the juju API.
	neSchema.schema = schp.Schema()

	// Remove everything from the schema that's in the skeleton.
	for name := range neSchema.skeleton {
		delete(neSchema.schema, name)
	}
	// Also remove any attributes ending in "-path" because
	// they're only applicable locally.
	for name := range neSchema.schema {
		if strings.HasSuffix(name, "-path") {
			delete(neSchema.schema, name)
		}
	}
	// We're going to set the model name from the
	// JEM model path, so remove it from
	// the schema.
	delete(neSchema.schema, "name")
	// TODO Delete admin-secret too, because it's never a valid
	// attribute for the client to provide. We can't do that right
	// now because it's the only secret attribute in the dummy
	// provider and we need it to test secret template attributes.
	// When Juju provides the schema over its API, that API call
	// should delete it before returning.
	fields, defaults, err := neSchema.schema.ValidationSchema()
	if err != nil {
		return nil, errgo.Notef(err, "cannot create validation schema for provider %s", neSchema.providerType)
	}
	neSchema.checker = schema.FieldMap(fields, defaults)
	return &neSchema, nil
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
