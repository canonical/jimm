package debugapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"

	"github.com/juju/testing"
	"github.com/juju/testing/httptesting"
	"github.com/juju/utils/debugstatus"
	gc "gopkg.in/check.v1"
	"gopkg.in/httprequest.v1"

	"github.com/CanonicalLtd/jimm/internal/apitest"
	"github.com/CanonicalLtd/jimm/internal/debugapi"
	"github.com/CanonicalLtd/jimm/params"
	"github.com/CanonicalLtd/jimm/version"
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

func (s *APISuite) TestDebugUsageSenderCheck(c *gc.C) {
	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.JEMSrv,
		URL:     "/debug/usage/test-user",
		Do:      apitest.Do(s.IDMSrv.Client("controller-admin")),
	})
	c.Assert(resp.Code, gc.Equals, http.StatusOK)

	s.Suite.MetricsRegistrationClient.CheckCalls(c, []testing.StubCall{{
		FuncName: "AuthorizeReseller",
		Args: []interface{}{
			"canonical/jimm",
			"cs:~canonical/jimm-0",
			"jimm",
			"canonical",
			"test-user",
		},
	}})
}

func (s *APISuite) TestDebugUsageSenderCheckError(c *gc.C) {
	s.MetricsRegistrationClient.SetErrors(errors.New("an embarassing error"))
	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.JEMSrv,
		URL:     "/debug/usage/test-user",
		Do:      apitest.Do(s.IDMSrv.Client("controller-admin")),
	})
	c.Assert(resp.Code, gc.Equals, http.StatusInternalServerError)

	var errorMessage struct {
		Message string `json:"message"`
	}
	decoder := json.NewDecoder(resp.Body)
	err := decoder.Decode(&errorMessage)
	c.Assert(err, gc.IsNil)
	c.Assert(errorMessage.Message, gc.Equals, "check failed: an embarassing error")

	s.Suite.MetricsRegistrationClient.CheckCalls(c, []testing.StubCall{{
		FuncName: "AuthorizeReseller",
		Args: []interface{}{
			"canonical/jimm",
			"cs:~canonical/jimm-0",
			"jimm",
			"canonical",
			"test-user",
		},
	}})
}

func (s *APISuite) TestDBStats(c *gc.C) {
	srv := httptest.NewServer(s.JEMSrv)
	defer srv.Close()
	client := &httprequest.Client{
		BaseURL: srv.URL,
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
