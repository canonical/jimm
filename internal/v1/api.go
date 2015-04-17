package v1

import (
	"github.com/juju/httprequest"
	"github.com/juju/names"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

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
	err := h.jem.DB.StateServers().Insert(mongodoc.StateServer{
		Id:   string(arg.User) + "/" + string(arg.Name),
		User: string(arg.User),
		Name: string(arg.Name),
		Info: arg.Info,
	})
	if mgo.IsDup(err) {
		return errgo.WithCausef(nil, params.ErrAlreadyExists, "")
	}
	if err != nil {
		return errgo.Notef(err, "cannot insert state server")
	}
	return nil
}

func badRequestf(underlying error, f string, a ...interface{}) error {
	err := errgo.WithCausef(underlying, params.ErrBadRequest, f, a...)
	err.(*errgo.Err).SetLocation(1)
	return err
}
