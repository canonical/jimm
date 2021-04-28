package debugapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/juju/utils/v2/debugstatus"
	"gopkg.in/httprequest.v1"

	"github.com/CanonicalLtd/jimm/internal/jemserver"
	"github.com/CanonicalLtd/jimm/version"
)

// NewAPIHandler returns a new API handler that serves the /debug
// endpoints.
func NewAPIHandler(ctx context.Context, params jemserver.HandlerParams) ([]httprequest.Handler, error) {
	srv := &httprequest.Server{}

	return srv.Handlers(func(p httprequest.Params) (*handler, context.Context, error) {
		h := &handler{
			params: params.Params,
		}
		h.Handler = debugstatus.Handler{
			Version:           debugstatus.Version(version.VersionInfo),
			CheckPprofAllowed: func(req *http.Request) error { return h.checkIsAdmin(req.Context(), req) },
			Check:             h.check,
		}
		return h, p.Context, nil
	}), nil
}

type handler struct {
	debugstatus.Handler
	params jemserver.Params
}

func (h *handler) checkIsAdmin(ctx context.Context, req *http.Request) error {
	// TODO(mhilton) decide if this should be available.
	return errors.New("pprof disabled")
}

func (h *handler) check(ctx context.Context) map[string]debugstatus.CheckResult {
	return debugstatus.Check(
		ctx,
		debugstatus.ServerStartTime,
	)
}

// Close implements io.Closer and is called by httprequest
// when the request is complete.
func (h *handler) Close() error {
	return nil
}
