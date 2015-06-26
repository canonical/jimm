package v1

import (
	"fmt"
	"strings"

	"github.com/juju/httprequest"
	"github.com/juju/juju/api"
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
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

var logger = loggo.GetLogger("jem.internal.v1")

type Handler struct {
	jem    *jem.JEM
	config jem.ServerParams
	auth   authorization
}

func NewAPIHandler(jp *jem.Pool, sp jem.ServerParams) ([]httprequest.Handler, error) {
	return errorMapper.Handlers(func(p httprequest.Params) (*Handler, error) {
		// All requests require an authenticated client.
		h := &Handler{
			jem:    jp.JEM(),
			config: sp,
		}
		auth, err := h.checkRequest(p.Request)
		if err != nil {
			h.Close()
			return nil, errgo.Mask(err, errgo.Any)
		}
		h.auth = auth
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

// AddJES adds a new state server.
func (h *Handler) AddJES(arg *params.AddJES) error {
	if string(arg.User) != h.auth.username {
		logger.Warningf("authorization denied for user %q to modify environment %s/env/%s", h.auth.username, arg.User, arg.Name)
		return params.ErrUnauthorized
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
	if !names.IsValidEnvironment(arg.Info.EnvironUUID) {
		return badRequestf(nil, "bad environment UUID in request")
	}
	srv := &mongodoc.StateServer{
		User:      arg.User,
		Name:      arg.Name,
		CACert:    arg.Info.CACert,
		HostPorts: arg.Info.HostPorts,
	}
	env := &mongodoc.Environment{
		AdminUser:     arg.Info.User,
		AdminPassword: arg.Info.Password,
		UUID:          arg.Info.EnvironUUID,
	}
	logger.Infof("dialling environment")
	// Attempt to connect to the environment before accepting it.
	state, err := h.jem.OpenAPIFromDocs(env, srv)
	if err != nil {
		logger.Infof("cannot open API: %v", err)
		return badRequestf(err, "cannot connect to environment")
	}
	state.Close()

	// Update addresses from latest known in state server.
	// Note that state.APIHostPorts is always guaranteed
	// to include the actual address we succeeded in
	// connecting to.
	srv.HostPorts = collapseHostPorts(state.APIHostPorts())

	err = h.jem.AddStateServer(srv, env)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrAlreadyExists))
	}
	return nil
}

// GetJES returns information on a state server.
func (h *Handler) GetJES(arg *params.GetJES) (*params.JESInfo, error) {
	neSchema, err := h.schemaForNewEnv(entityId(string(arg.User), string(arg.Name)))
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return &params.JESInfo{
		ProviderType: neSchema.providerType,
		Template:     neSchema.schema,
	}, nil
}

// GetEnvironment returns information on a given environment.
func (h *Handler) GetEnvironment(arg *params.GetEnvironment) (*params.EnvironmentResponse, error) {
	env, err := h.jem.Environment(entityPathToId(arg.EntityPath))
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	srv, err := h.jem.StateServer(env.StateServer)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return &params.EnvironmentResponse{
		UUID:      env.UUID,
		CACert:    srv.CACert,
		HostPorts: srv.HostPorts,
	}, nil
}

// NewEnvironment creates a new environment inside an existing JES.
func (h *Handler) NewEnvironment(args *params.NewEnvironment) (*params.EnvironmentResponse, error) {
	if !h.isUser(string(args.User)) {
		return nil, params.ErrUnauthorized
	}
	id, err := parseStateServerPath(args.Info.StateServer)
	if err != nil {
		return nil, errgo.NoteMask(err, fmt.Sprintf("cannot parse state server path %q", args.Info.StateServer), errgo.Is(params.ErrBadRequest))
	}
	st, err := h.jem.OpenAPI(id)
	if err != nil {
		return nil, errgo.NoteMask(err, "cannot connect to state server", errgo.Is(params.ErrNotFound))
	}
	defer st.Close()

	neSchema, err := h.schemaForNewEnv(id)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	// Ensure that the attributes look reasonably OK before bothering
	// the state server with them.
	attrs, err := neSchema.checker.Coerce(args.Info.Config, nil)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "cannot validate attributes")
	}
	if err := h.maybeCreateUser(st.State, h.auth.username, args.Info.Password); err != nil {
		return nil, errgo.NoteMask(err, "cannot create user", errgo.Is(params.ErrBadRequest))
	}

	// Create the environment record in the database before actually
	// creating the environment on the state server. It will have an invalid
	// UUID because it doesn't exist but that's better than creating
	// an environment that we can't add locally because the name
	// already exists.
	envDoc := &mongodoc.Environment{
		User:          args.User,
		Name:          args.Info.Name,
		AdminUser:     h.auth.username,
		AdminPassword: args.Info.Password,
		StateServer:   id,
	}
	if err := h.jem.AddEnvironment(envDoc); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrAlreadyExists))
	}

	fields := attrs.(map[string]interface{})
	// Add the values from the skeleton to the configuration.
	for name, field := range neSchema.skeleton {
		fields[name] = field
	}
	// Add the environment name.
	// Note that AddEnvironment has set envdoc.Id for us.
	fields["name"] = idToEnvName(envDoc.Id)

	emclient := environmentmanager.NewClient(st)
	env, err := emclient.CreateEnvironment(h.auth.username, nil, fields)
	if err != nil {
		// Remove the environment that was created, because it's no longer valid.
		if err := h.jem.DB.Environments().RemoveId(envDoc.Id); err != nil {
			logger.Errorf("cannot remove environment from database after env creation error: %v", err)
		}
		return nil, errgo.Notef(err, "cannot create environment")
	}
	// Now set the UUID to that of the actually created environment.
	if err := h.jem.DB.Environments().UpdateId(envDoc.Id, bson.D{{"uuid", env.UUID}}); err != nil {
		return nil, errgo.Notef(err, "cannot update environment UUID in database, leaked environment %s", env.UUID)
	}
	return &params.EnvironmentResponse{
		UUID:      env.UUID,
		CACert:    st.Info.CACert,
		HostPorts: st.Info.Addrs,
	}, nil
}

func idToEnvName(id string) string {
	return strings.Replace(id, "/", "_", -1)
}

// maybeCreateUser creates a user account if one does not already exist.
func (h *Handler) maybeCreateUser(st *api.State, user, password string) error {
	if password == "" {
		return badRequestf(nil, "no password specified for new user")
	}
	uclient := usermanager.NewClient(st)
	_, err := uclient.AddUser(user, "", password)
	if err == nil {
		return nil
	}
	if err, ok := errgo.Cause(err).(*jujuparams.Error); ok && err.Code == jujuparams.CodeAlreadyExists {
		return nil
	}
	return errgo.Mask(err)
}

type schemaForNewEnv struct {
	providerType string
	schema       environschema.Fields
	checker      schema.Checker
	skeleton     map[string]interface{}
}

// schemaForNewEnv returns the schema for the configuration options
// for creating new environments on the state server with the given id.
func (h *Handler) schemaForNewEnv(id string) (*schemaForNewEnv, error) {
	st, err := h.jem.OpenAPI(id)
	if err != nil {
		return nil, errgo.NoteMask(err, "cannot open API", errgo.Is(params.ErrNotFound))
	}
	defer st.Close()

	var neSchema schemaForNewEnv

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
	// TODO get the environment schema over the juju API.
	neSchema.schema = schp.Schema()

	// Remove everything from the schema that's in the skeleton.
	for name := range neSchema.skeleton {
		delete(neSchema.schema, name)
	}
	// We're going to set the environment name from the
	// JEM environment path, so remove it from
	// the schema.
	delete(neSchema.schema, "name")
	fields, defaults, err := neSchema.schema.ValidationSchema()
	if err != nil {
		return nil, errgo.Notef(err, "cannot create validation schema for provider %s", neSchema.providerType)
	}
	neSchema.checker = schema.FieldMap(fields, defaults)
	return &neSchema, nil
}

// parseStateServerPath parses a path of the form
// $user/server/$name into an entity id as
// understood by JEM.StateServer.
func parseStateServerPath(s string) (string, error) {
	parts := strings.Split(s, "/")
	if len(parts) != 3 {
		return "", badRequestf(nil, "wrong number of parts")
	}
	if parts[1] != "server" {
		return "", badRequestf(nil, `second part of state server id must be "server"`)
	}
	if parts[0] == "" || parts[2] == "" {
		return "", badRequestf(nil, "empty user name or entity name")
	}
	return entityId(parts[0], parts[2]), nil
}

func entityPathToId(u params.EntityPath) string {
	return entityId(string(u.User), string(u.Name))
}

func entityId(user, name string) string {
	return user + "/" + name
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
