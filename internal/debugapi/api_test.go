package debugapi_test

import (
	"context"
	"net/http"

	"github.com/juju/testing/httptesting"
	"github.com/juju/utils/debugstatus"
	gc "gopkg.in/check.v1"
	"gopkg.in/httprequest.v1"

	"github.com/CanonicalLtd/jimm/internal/debugapi"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/jemtest/apitest"
	"github.com/CanonicalLtd/jimm/params"
	"github.com/CanonicalLtd/jimm/version"
)

type APISuite struct {
	jemtest.JEMSuite
	apitest.APISuite
}

var _ = gc.Suite(&APISuite{})

func (s *APISuite) SetUpSuite(c *gc.C) {
	s.JEMSuite.SetUpSuite(c)
	s.APISuite.SetUpSuite(c)
}

func (s *APISuite) TearDownSuite(c *gc.C) {
	s.APISuite.TearDownSuite(c)
	s.JEMSuite.TearDownSuite(c)
}

func (s *APISuite) SetUpTest(c *gc.C) {
	s.JEMSuite.SetUpTest(c)
	s.APISuite.Params.SessionPool = s.SessionPool
	s.APISuite.Params.JEMPool = s.Pool
	s.APISuite.NewAPIHandler = debugapi.NewAPIHandler
	s.APISuite.SetUpTest(c)
}

func (s *APISuite) TearDownTest(c *gc.C) {
	s.APISuite.TearDownTest(c)
	s.JEMSuite.TearDownTest(c)
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

func (s *APISuite) TestPprofDeniedWithBadAuth(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Handler:      s.APIHandler,
		URL:          "/debug/pprof/",
		ExpectStatus: http.StatusProxyAuthRequired,
		ExpectBody:   apitest.AnyBody,
	})

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Handler:      s.APIHandler,
		URL:          "/debug/pprof/",
		ExpectStatus: http.StatusUnauthorized,
		ExpectBody: params.Error{
			Message: "unauthorized",
			Code:    params.ErrUnauthorized,
		},
		Do: apitest.Do(s.Client("someone")),
	})
}

func (s *APISuite) TestPprofOKWithAdmin(c *gc.C) {
	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.APIHandler,
		URL:     "/debug/pprof/",
		Do:      apitest.Do(s.Client(jemtest.ControllerAdmin)),
	})
	c.Assert(resp.Code, gc.Equals, http.StatusOK)
	c.Assert(resp.HeaderMap.Get("Content-Type"), gc.Matches, "text/html.*")
}

func (s *APISuite) TestDBStats(c *gc.C) {
	client := &httprequest.Client{
		BaseURL: s.HTTP.URL,
	}
	var resp debugapi.DebugDBStatsResponse
	err := client.Call(context.Background(), &debugapi.DebugDBStatsRequest{}, &resp)
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp.Stats["ok"], gc.Equals, float64(1))
	c.Assert(len(resp.Collections), gc.Not(gc.Equals), 0)
	for _, coll := range resp.Collections {
		c.Assert(coll["ok"], gc.Equals, float64(1))
	}
}
