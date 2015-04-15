package v1

import (
	"github.com/juju/httprequest"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/params"
)

type Handler struct {
	db     *jem.JEM
	config jem.ServerParams
	auth   authorization
}

func NewAPIHandler(jp *jem.Pool, sp jem.ServerParams) ([]httprequest.Handler, error) {
	return errorMapper.Handlers(func(p httprequest.Params) (*Handler, error) {
		// All requests require an authenticated client.
		h := &Handler{
			db:     jp.JEM(),
			config: sp,
		}
		auth, err := h.checkRequest(p.Request)
		if err != nil {
			h.db.Close()
			return nil, errgo.Mask(err, errgo.Any)
		}
		h.auth = auth
		return h, nil
	}), nil
}

func (h *Handler) Close() error {
	h.db.Close()
	h.db = nil
	return nil
}

func (h *Handler) Test(arg *struct {
	httprequest.Route `httprequest:"GET /v1/test"`
}) error {
	return errgo.WithCausef(errgo.Newf("testing"), params.ErrUnauthorized, "go away")
}
