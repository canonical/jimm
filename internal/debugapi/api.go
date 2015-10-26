package debugapi

import (
	"net/http"

	"github.com/juju/httprequest"
	"github.com/juju/utils/debugstatus"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/jemerror"
	"github.com/CanonicalLtd/jem/version"
)

// NewAPIHandler returns a new API handler that serves the /debug
// endpoints.
func NewAPIHandler(jp *jem.Pool, sp jem.ServerParams) ([]httprequest.Handler, error) {
	return jemerror.Mapper.Handlers(func(p httprequest.Params) (*handler, error) {
		h := &handler{
			jem: jp.JEM(),
		}
		h.Handler = debugstatus.Handler{
			Version:           debugstatus.Version(version.VersionInfo),
			CheckPprofAllowed: h.checkIsAdmin,
			Check:             h.check,
		}
		return h, nil
	}), nil
}

type handler struct {
	jem *jem.JEM
	debugstatus.Handler
}

func (h *handler) checkIsAdmin(req *http.Request) error {
	if err := h.jem.Authenticate(req); err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	return h.jem.CheckIsAdmin()
}

func (h *handler) check() map[string]debugstatus.CheckResult {
	return debugstatus.Check(
		debugstatus.ServerStartTime,
		debugstatus.Connection(h.jem.DB.Session),
		debugstatus.MongoCollections(h.jem.DB),
	)
}

// Close implements io.Closer and is called by httprequest
// when the request is complete.
func (h *handler) Close() error {
	h.jem.Close()
	h.jem = nil
	return nil
}
