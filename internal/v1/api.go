package v1

import (
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
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jem/internal/apiconn"
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
		Path:      arg.EntityPath,
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
	conn, err := h.jem.OpenAPIFromDocs(env, srv)
	if err != nil {
		logger.Infof("cannot open API: %v", err)
		return badRequestf(err, "cannot connect to environment")
	}
	defer conn.Close()

	// Update addresses from latest known in state server.
	// Note that state.APIHostPorts is always guaranteed
	// to include the actual address we succeeded in
	// connecting to.
	srv.HostPorts = collapseHostPorts(conn.APIHostPorts())

	err = h.jem.AddStateServer(srv, env)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrAlreadyExists))
	}
	return nil
}

// GetJES returns information on a state server.
func (h *Handler) GetJES(arg *params.GetJES) (*params.JESResponse, error) {
	neSchema, err := h.schemaForNewEnv(arg.EntityPath)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return &params.JESResponse{
		Path:         arg.EntityPath,
		ProviderType: neSchema.providerType,
		Template:     neSchema.schema,
	}, nil
}

// GetEnvironment returns information on a given environment.
func (h *Handler) GetEnvironment(arg *params.GetEnvironment) (*params.EnvironmentResponse, error) {
	env, err := h.jem.Environment(arg.EntityPath)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	srv, err := h.jem.StateServer(env.StateServer)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return &params.EnvironmentResponse{
		Path:      arg.EntityPath,
		User:      env.AdminUser,
		UUID:      env.UUID,
		CACert:    srv.CACert,
		HostPorts: srv.HostPorts,
	}, nil
}

// ListEnvironments returns all the environments stored in JEM.
func (h *Handler) ListEnvironments(arg *params.ListEnvironments) (*params.ListEnvironmentsResponse, error) {
	// TODO provide a way of restricting the results.

	// We get all state servers first, because many environments
	// will be sharing the same state servers.
	// TODO we could do better than this and avoid
	// gathering all the state servers into memory.
	// Possiblities include caching state servers, and
	// gathering results to do only a few
	// concurrent queries.
	servers := make(map[params.EntityPath]mongodoc.StateServer)
	iter := h.jem.DB.StateServers().Find(nil).Sort("_id").Iter()
	var srv mongodoc.StateServer
	for iter.Next(&srv) {
		servers[srv.Path] = srv
	}
	if err := iter.Err(); err != nil {
		return nil, errgo.Notef(err, "cannot get state servers")
	}
	envs := make([]params.EnvironmentResponse, 0, len(servers))
	iter = h.jem.DB.Environments().Find(nil).Iter()
	var env mongodoc.Environment
	for iter.Next(&env) {
		srv, ok := servers[env.StateServer]
		if !ok {
			logger.Errorf("environment %s has invalid state server value %s; omitting from result", env.Path, env.StateServer)
			continue
		}
		envs = append(envs, params.EnvironmentResponse{
			Path:      env.Path,
			User:      env.AdminUser,
			UUID:      env.UUID,
			CACert:    srv.CACert,
			HostPorts: srv.HostPorts,
		})
	}
	if err := iter.Err(); err != nil {
		return nil, errgo.Notef(err, "cannot get environments")
	}
	return &params.ListEnvironmentsResponse{
		Environments: envs,
	}, nil
}

// ListJES returns all the state servers stored in JEM.
// Currently the Template  and ProviderType field in each JESResponse is not
// populated.
func (h *Handler) ListJES(arg *params.ListJES) (*params.ListJESResponse, error) {
	var srvs []params.JESResponse

	// TODO populate ProviderType and Template fields when we have a cache
	// for the schemaForNewEnv results.
	iter := h.jem.DB.StateServers().Find(nil).Iter()
	var srv mongodoc.StateServer
	for iter.Next(&srv) {
		srvs = append(srvs, params.JESResponse{
			Path: srv.Path,
		})
	}
	if err := iter.Err(); err != nil {
		return nil, errgo.Notef(err, "cannot get environments")
	}
	return &params.ListJESResponse{
		StateServers: srvs,
	}, nil
}

// NewEnvironment creates a new environment inside an existing JES.
func (h *Handler) NewEnvironment(args *params.NewEnvironment) (*params.EnvironmentResponse, error) {
	if !h.isUser(string(args.User)) {
		return nil, params.ErrUnauthorized
	}
	conn, err := h.jem.OpenAPI(args.Info.StateServer)
	if err != nil {
		return nil, errgo.NoteMask(err, "cannot connect to state server", errgo.Is(params.ErrNotFound))
	}
	defer conn.Close()

	neSchema, err := h.schemaForNewEnv(args.Info.StateServer)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	// Ensure that the attributes look reasonably OK before bothering
	// the state server with them.
	attrs, err := neSchema.checker.Coerce(args.Info.Config, nil)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "cannot validate attributes")
	}
	// We create a user for the environment so that the caller knows
	// what password to use when accessing their new environment.
	// When juju supports macaroon authorization, we won't need
	// to do this - we could just inform the state server of the required
	// user/group (args.User) instead.
	envPath := params.EntityPath{args.User, args.Info.Name}
	jujuUser := userForEnvironment(envPath)
	if err := h.createUser(conn, jujuUser, args.Info.Password); err != nil {
		return nil, errgo.NoteMask(err, "cannot create user", errgo.Is(params.ErrBadRequest))
	}
	logger.Infof("created user %q", args.User)

	// Create the environment record in the database before actually
	// creating the environment on the state server. It will have an invalid
	// UUID because it doesn't exist but that's better than creating
	// an environment that we can't add locally because the name
	// already exists.
	envDoc := &mongodoc.Environment{
		Path:          envPath,
		AdminUser:     jujuUser,
		AdminPassword: args.Info.Password,
		StateServer:   args.Info.StateServer,
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

	emclient := environmentmanager.NewClient(conn.State)
	env, err := emclient.CreateEnvironment(jujuUser, nil, fields)
	if err != nil {
		// Remove the environment that was created, because it's no longer valid.
		if err := h.jem.DB.Environments().RemoveId(envDoc.Id); err != nil {
			logger.Errorf("cannot remove environment from database after env creation error: %v", err)
		}
		return nil, errgo.Notef(err, "cannot create environment")
	}
	// Now set the UUID to that of the actually created environment.
	if err := h.jem.DB.Environments().UpdateId(envDoc.Id, bson.D{{"$set", bson.D{{"uuid", env.UUID}}}}); err != nil {
		return nil, errgo.Notef(err, "cannot update environment UUID in database, leaked environment %s", env.UUID)
	}
	logger.Infof("returning server uuid %q", conn.Info.EnvironTag.Id())
	return &params.EnvironmentResponse{
		Path:       envPath,
		User:       jujuUser,
		ServerUUID: conn.Info.EnvironTag.Id(),
		UUID:       env.UUID,
		CACert:     conn.Info.CACert,
		HostPorts:  conn.Info.Addrs,
	}, nil
}

// userForEnvironment returns the juju user to use for
// the environment with the given path.
// This should go when juju supports macaroon
// authorization.
func userForEnvironment(p params.EntityPath) string {
	return "jem-" + string(p.User) + "--" + string(p.Name)
}

func idToEnvName(id string) string {
	return strings.Replace(id, "/", "_", -1)
}

// createUser creates the given user account with the
// given password. If the account already exists then it changes
// the password accordingly.
func (h *Handler) createUser(conn *apiconn.Conn, user, password string) error {
	if password == "" {
		return badRequestf(nil, "no password specified")
	}
	uclient := usermanager.NewClient(conn.State)
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

type schemaForNewEnv struct {
	providerType string
	schema       environschema.Fields
	checker      schema.Checker
	skeleton     map[string]interface{}
}

// schemaForNewEnv returns the schema for the configuration options
// for creating new environments on the state server with the given id.
func (h *Handler) schemaForNewEnv(srvPath params.EntityPath) (*schemaForNewEnv, error) {
	st, err := h.jem.OpenAPI(srvPath)
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
	// Also remove any attributes ending in "-path" because
	// they're only applicable locally.
	for name := range neSchema.schema {
		if strings.HasSuffix(name, "-path") {
			delete(neSchema.schema, name)
		}
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
