package v1

import (
	"time"

	"github.com/juju/httprequest"
	"github.com/juju/juju/api"
	"github.com/juju/names"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/network"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type Handler struct {
	jem    *jem.JEM
	config jem.ServerParams
	auth   authorization
}

var dialTimeout = 15 * time.Second

func NewAPIHandler(jp *jem.Pool, sp jem.ServerParams) ([]httprequest.Handler, error) {
	return errorMapper.Handlers(func(p httprequest.Params) (*Handler, error) {
		// All requests require an authenticated client.
		h := &Handler{
			jem:    jp.JEM(),
			config: sp,
		}
		auth, err := h.checkRequest(p.Request)
		if err != nil {
			h.jem.Close()
			return nil, errgo.Mask(err, errgo.Any)
		}
		h.auth = auth
		return h, nil
	}), nil
}

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
	id := string(arg.User) + "/" + string(arg.Name)
	env := &mongodoc.Environment{
		Id:            id,
		User:          arg.User,
		Name:          arg.Name,
		AdminUser:     arg.Info.User,
		AdminPassword: arg.Info.Password,
		StateServer:   id,
	}
	srv := &mongodoc.StateServer{
		Id:        id,
		User:      arg.User,
		Name:      arg.Name,
		CACert:    arg.Info.CACert,
		HostPorts: arg.Info.HostPorts,
	}
	// Attempt to connect to the environment before accepting it.
	state, err := dialEnvironment(env, srv)
	if err != nil {
		return badRequestf(err, "cannot connect to environment")
	}
	state.Close()

	// Update addresses from latest known in state server.
	// Note that state.APIHostPorts is always guaranteed
	// to include the actual address we succeeded in
	// connecting to.
	srv.HostPorts = collapseHostPorts(state.APIHostPorts())

	// Insert the environment before inserting the state server
	// to avoid races with other clients creating non-state-server
	// environments.
	err = h.jem.DB.Environments().Insert(env)
	if mgo.IsDup(err) {
		return errgo.WithCausef(nil, params.ErrAlreadyExists, "")
	}
	if err != nil {
		return errgo.Notef(err, "cannot insert state server environment")
	}
	err = h.jem.DB.StateServers().Insert(srv)
	if err != nil {
		return errgo.Notef(err, "cannot insert state server")
	}
	return nil
}

func dialEnvironment(
	env *mongodoc.Environment,
	srv *mongodoc.StateServer,
) (*api.State, error) {
	return api.Open(&api.Info{
		Addrs:      srv.HostPorts,
		CACert:     srv.CACert,
		Tag:        names.NewUserTag(env.AdminUser),
		Password:   env.AdminPassword,
		EnvironTag: names.NewEnvironTag(env.UUID),
	}, api.DialOpts{
		Timeout:    dialTimeout,
		RetryDelay: 500 * time.Millisecond,
	})
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
