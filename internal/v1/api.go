package v1

import (
	"github.com/juju/httprequest"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/params"
)

type Handler struct {
	db *mgo.Database
}

func NewAPIHandler(j *jem.JEM, p jem.ServerParams) ([]httprequest.Handler, error) {
	return errorMapper.Handlers(func(p httprequest.Params) (*Handler, error) {
		session := j.DB.Session.Copy()
		return &Handler{
			db: session.DB(j.DB.Name),
		}, nil
	}), nil
}

func (h *Handler) Close() error {
	h.db.Session.Close()
	h.db = nil
	return nil
}

func (h *Handler) Test(arg *struct {
	httprequest.Route `httprequest:"GET /v1/test"`
}) error {
	return errgo.WithCausef(errgo.Newf("testing"), params.ErrUnauthorized, "go away")
}
