package debugapi

import (
	"net/http"

	"github.com/juju/httprequest"
	"github.com/juju/utils/debugstatus"
	"github.com/uber-go/zap"
	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/auth"
	"github.com/CanonicalLtd/jem/internal/ctxutil"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/jemerror"
	"github.com/CanonicalLtd/jem/internal/jemserver"
	"github.com/CanonicalLtd/jem/internal/zapctx"
	"github.com/CanonicalLtd/jem/version"
)

// NewAPIHandler returns a new API handler that serves the /debug
// endpoints.
func NewAPIHandler(ctx context.Context, jp *jem.Pool, ap *auth.Pool, sp jemserver.Params) ([]httprequest.Handler, error) {
	return jemerror.Mapper.Handlers(func(p httprequest.Params) (*handler, error) {
		ctx := ctxutil.Join(ctx, p.Context)
		ctx = zapctx.WithFields(ctx, zap.String("req-id", httprequest.RequestUUID(ctx)))
		h := &handler{
			params:   sp,
			jem:      jp.JEM(ctx),
			authPool: ap,
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
	debugstatus.Handler
	params   jemserver.Params
	jem      *jem.JEM
	authPool *auth.Pool
	ctx      context.Context
}

func (h *handler) checkIsAdmin(req *http.Request) error {
	if h.ctx == nil {
		a := h.authPool.Authenticator(context.TODO())
		defer a.Close()
		ctx, err := a.AuthenticateRequest(context.TODO(), req)
		if err != nil {
			return errgo.Mask(err, errgo.Any)
		}
		h.ctx = ctx
	}
	return auth.CheckIsUser(h.ctx, h.params.ControllerAdmin)
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
