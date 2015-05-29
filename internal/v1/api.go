package v1

import (
	"github.com/juju/httprequest"
	"github.com/juju/juju/network"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type Handler struct {
	jem    *jem.JEM
	config jem.ServerParams
	auth   authorization
}

var logger = loggo.GetLogger("jem.internal.v1")

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

func (h *Handler) AddJES(arg *params.AddJES) error {
	if !h.isAdmin() || string(arg.User) != h.auth.username {
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

func entityPathToId(u params.EntityPath) string {
	return string(u.User) + "/" + string(u.Name)
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
