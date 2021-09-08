package debugapi

import (
	"context"
	"net/http"

	"github.com/juju/mgo/v2/bson"
	"github.com/juju/utils/v2/debugstatus"
	"gopkg.in/errgo.v1"
	"gopkg.in/httprequest.v1"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jemerror"
	"github.com/CanonicalLtd/jimm/internal/jemserver"
	"github.com/CanonicalLtd/jimm/version"
)

// NewAPIHandler returns a new API handler that serves the /debug
// endpoints.
func NewAPIHandler(ctx context.Context, params jemserver.HandlerParams) ([]httprequest.Handler, error) {
	srv := &httprequest.Server{
		ErrorMapper: func(ctx context.Context, err error) (int, interface{}) {
			return jemerror.Mapper(ctx, err)
		},
	}

	return srv.Handlers(func(p httprequest.Params) (*handler, context.Context, error) {
		h := &handler{
			params: params.Params,
			jem:    params.JEMPool.JEM(ctx),
			auth:   params.Authenticator,
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
	jem    *jem.JEM
	auth   *auth.Authenticator
}

func (h *handler) checkIsAdmin(ctx context.Context, req *http.Request) error {
	id, err := h.auth.AuthenticateRequest(ctx, req)
	if err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	admins := make([]string, len(h.params.ControllerAdmins))
	for i, v := range h.params.ControllerAdmins {
		admins[i] = string(v)
	}
	return auth.CheckACL(ctx, id, admins)
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
