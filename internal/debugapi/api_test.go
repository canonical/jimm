package debugapi_test

import (
	"net/http"

	"github.com/juju/testing/httptesting"
	"github.com/juju/utils/debugstatus"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jem/internal/apitest"
	"github.com/CanonicalLtd/jem/params"
	"github.com/CanonicalLtd/jem/version"
)

type APISuite struct {
	apitest.Suite
}

var _ = gc.Suite(&APISuite{})

func (s *APISuite) TestDebugInfo(c *gc.C) {
	// The version endpoint is open to anyone, so use the
	// default HTTP client.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Handler:    s.JEMSrv,
		URL:        "/debug/info",
		ExpectBody: debugstatus.Version(version.VersionInfo),
	})
}

func (s *APISuite) TestPprofDeniedWithBadAuth(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Handler:      s.JEMSrv,
		URL:          "/debug/pprof/",
		ExpectStatus: http.StatusProxyAuthRequired,
		ExpectBody:   apitest.AnyBody,
	})

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Handler:      s.JEMSrv,
		URL:          "/debug/pprof/",
		ExpectStatus: http.StatusUnauthorized,
		ExpectBody: params.Error{
			Message: "unauthorized",
			Code:    params.ErrUnauthorized,
		},
		Do: apitest.Do(s.IDMSrv.Client("someone")),
	})
}

func (s *APISuite) TestPprofOKWithAdmin(c *gc.C) {
	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.JEMSrv,
		URL:     "/debug/pprof/",
		Do:      apitest.Do(s.IDMSrv.Client("controller-admin")),
	})
	c.Assert(resp.Code, gc.Equals, http.StatusOK)
	c.Assert(resp.HeaderMap.Get("Content-Type"), gc.Matches, "text/html.*")
}
