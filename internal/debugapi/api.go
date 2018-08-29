package debugapi

import (
	"context"
	"net/http"

	jujuhttprequest "github.com/juju/httprequest"
	"github.com/juju/utils/debugstatus"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"
	"gopkg.in/httprequest.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/ctxutil"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jemerror"
	"github.com/CanonicalLtd/jimm/internal/jemserver"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/version"
)

// NewAPIHandler returns a new API handler that serves the /debug
// endpoints.
func NewAPIHandler(ctx context.Context, params jemserver.HandlerParams) ([]jujuhttprequest.Handler, error) {
	var handlers []jujuhttprequest.Handler
	srv := &httprequest.Server{
		ErrorMapper: func(_ context.Context, err error) (int, interface{}) {
			return jemerror.Mapper(err)
		},
	}

	for _, hnd := range srv.Handlers(func(p httprequest.Params) (*handler, context.Context, error) {
		ctx := ctxutil.Join(ctx, p.Context)
		ctx = zapctx.WithFields(ctx, zap.String("req-id", jujuhttprequest.RequestUUID(ctx)))
		h := &handler{
			params:                         params.Params,
			jem:                            params.JEMPool.JEM(ctx),
			authPool:                       params.AuthenticatorPool,
			usageSenderAuthorizationClient: params.JEMPool.UsageAuthorizationClient(),
		}
		h.Handler = debugstatus.Handler{
			Version:           debugstatus.Version(version.VersionInfo),
			CheckPprofAllowed: h.checkIsAdmin,
			Check:             h.check,
		}
		return h, ctx, nil
	}) {
		handlers = append(handlers, jujuhttprequest.Handler(hnd))
	}
	return handlers, nil
}

type handler struct {
	debugstatus.Handler
	params                         jemserver.Params
	jem                            *jem.JEM
	authPool                       *auth.Pool
	ctx                            context.Context
	usageSenderAuthorizationClient jem.UsageSenderAuthorizationClient
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

func (h *handler) check(ctx context.Context) map[string]debugstatus.CheckResult {
	return debugstatus.Check(
		ctx,
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

// DebugUsageSenderCheckRequest defines the request structure for the
// omnibus usage sender authorization check.
type DebugUsageSenderCheckRequest struct {
	httprequest.Route `httprequest:"GET /debug/usage/:username"`
	Username          string `httprequest:"username,path"`
}

// DebugUsageSenderCheck implements a check that verifies that JIMM is able
// to perform usage sender authorization.
func (h *handler) DebugUsageSenderCheck(p httprequest.Params, r *DebugUsageSenderCheckRequest) error {
	if err := h.checkIsAdmin(p.Request); err != nil {
		return errgo.Mask(err, errgo.Any)
	}

	if h.usageSenderAuthorizationClient == nil {
		return errgo.New("cannot perform the check")
	}

	_, err := h.usageSenderAuthorizationClient.AuthorizeReseller(
		jem.OmnibusJIMMPlan,
		jem.OmnibusJIMMCharm,
		jem.OmnibusJIMMName,
		jem.OmnibusJIMMOwner,
		r.Username,
	)
	if err != nil {
		return errgo.Notef(err, "check failed")
	}
	return nil
}

// DebugDBStatsRequest contains the request for /debug/dbstats.
type DebugDBStatsRequest struct {
	httprequest.Route `httprequest:"GET /debug/dbstats"`
}

// DebugDBStatsResponse contains the response returned from
// /debug/dbstats.
type DebugDBStatsResponse struct {
	// Stats contains the response from the mongodb server of a
	// "dbStats" command. The actual value depends on the version of
	// MongoDB in use. See
	// https://docs.mongodb.com/manual/reference/command/dbStats/ for
	// details.
	Stats map[string]interface{} `json:"stats"`

	// Collections contains a mapping from collection name to the
	// response from the mongodb server of a "collStats" command
	// performed on that collection. The actual value depends on the
	// version of MongoDB in use. See
	// https://docs.mongodb.com/manual/reference/command/collStats/
	// for details.
	Collections map[string]map[string]interface{} `json:"collections"`
}

// DebugDBStats serves the /debug/dbstats endpoint. This queries dbStats
// and collStats from mongodb and returns the result.
func (h *handler) DebugDBStats(p httprequest.Params, req *DebugDBStatsRequest) (*DebugDBStatsResponse, error) {
	var resp DebugDBStatsResponse
	if err := h.jem.DB.Run("dbStats", &resp.Stats); err != nil {
		return nil, errgo.Mask(err)
	}
	names, err := h.jem.DB.CollectionNames()
	if err != nil {
		return nil, errgo.Mask(err)
	}
	resp.Collections = make(map[string]map[string]interface{}, len(names))
	for _, name := range names {
		var stats map[string]interface{}
		if err := h.jem.DB.Run(bson.D{{"collStats", name}}, &stats); err != nil {
			return nil, errgo.Mask(err)
		}
		resp.Collections[name] = stats
	}
	return &resp, nil
}
