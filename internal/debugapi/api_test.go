package debugapi_test

import (
	"context"
	"net/http"

	"github.com/juju/testing/httptesting"
	"github.com/juju/utils/debugstatus"
	"github.com/julienschmidt/httprouter"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/debugapi"
	"github.com/CanonicalLtd/jimm/internal/jemserver"
	"github.com/CanonicalLtd/jimm/version"
)

type APISuite struct {
	APIHandler http.Handler
}

var _ = gc.Suite(&APISuite{})

func (s *APISuite) SetUpTest(c *gc.C) {
	var r httprouter.Router
	handlers, err := debugapi.NewAPIHandler(context.Background(), jemserver.HandlerParams{})
	c.Assert(err, gc.Equals, nil)
	for _, hnd := range handlers {
		r.Handle(hnd.Method, hnd.Path, hnd.Handle)
	}
	s.APIHandler = &r
}

func (s *APISuite) TestDebugInfo(c *gc.C) {
	// The version endpoint is open to anyone, so use the
	// default HTTP client.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Handler:    s.APIHandler,
		URL:        "/debug/info",
		ExpectBody: debugstatus.Version(version.VersionInfo),
	})
}
