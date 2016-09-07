// Copyright 2015 Canonical Ltd.

package jemserver_test

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/juju/httprequest"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/testing"
	"github.com/juju/testing/httptesting"
	"github.com/julienschmidt/httprouter"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/jemserver"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type serverSuite struct {
	testing.IsolatedMgoSuite
}

var _ = gc.Suite(&serverSuite{})

func (s *serverSuite) TestNewServerWithNoVersions(c *gc.C) {
	params := jemserver.Params{
		DB:              s.Session.DB("foo"),
		ControllerAdmin: "controller-admin",
	}
	h, err := jemserver.New(params, nil)
	c.Assert(err, gc.ErrorMatches, `JEM server must serve at least one version of the API`)
	c.Assert(h, gc.IsNil)
}

type versionResponse struct {
	Version string
	Path    string
}

func (s *serverSuite) TestNewServerWithVersions(c *gc.C) {
	serverParams := jemserver.Params{
		DB:              s.Session.DB("foo"),
		ControllerAdmin: "controller-admin",
	}
	serveVersion := func(vers string) jemserver.NewAPIHandlerFunc {
		return func(p *jem.Pool, config jemserver.Params) ([]httprequest.Handler, error) {
			versPrefix := ""
			if vers != "" {
				versPrefix = "/" + vers
			}
			return []httprequest.Handler{{
				Method: "GET",
				Path:   versPrefix + "/*x",
				Handle: func(w http.ResponseWriter, req *http.Request, p httprouter.Params) {
					w.Header().Set("Content-Type", "application/json")
					data, err := json.Marshal(versionResponse{
						Version: vers,
						Path:    req.URL.Path,
					})
					c.Check(err, gc.IsNil)
					w.Write(data)
				},
			}}, nil
		}
	}

	h, err := jemserver.New(serverParams, map[string]jemserver.NewAPIHandlerFunc{
		"version1": serveVersion("version1"),
	})
	c.Assert(err, gc.IsNil)
	defer h.Close()
	assertServesVersion(c, h, "version1")
	assertDoesNotServeVersion(c, h, "version2")
	assertDoesNotServeVersion(c, h, "version3")

	h, err = jemserver.New(serverParams, map[string]jemserver.NewAPIHandlerFunc{
		"version1": serveVersion("version1"),
		"version2": serveVersion("version2"),
	})
	c.Assert(err, gc.IsNil)
	defer h.Close()
	assertServesVersion(c, h, "version1")
	assertServesVersion(c, h, "version2")
	assertDoesNotServeVersion(c, h, "version3")

	h, err = jemserver.New(serverParams, map[string]jemserver.NewAPIHandlerFunc{
		"version1": serveVersion("version1"),
		"version2": serveVersion("version2"),
		"version3": serveVersion("version3"),
	})
	c.Assert(err, gc.IsNil)
	defer h.Close()
	assertServesVersion(c, h, "version1")
	assertServesVersion(c, h, "version2")
	assertServesVersion(c, h, "version3")
}

func assertServesVersion(c *gc.C, h http.Handler, vers string) {
	path := vers
	if path != "" {
		path = "/" + path
	}
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Handler: h,
		URL:     path + "/some/path",
		ExpectBody: versionResponse{
			Version: vers,
			Path:    "/" + vers + "/some/path",
		},
	})
}

func assertDoesNotServeVersion(c *gc.C, h http.Handler, vers string) {
	rec := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: h,
		URL:     "/" + vers + "/some/path",
	})
	c.Assert(rec.Code, gc.Equals, http.StatusNotFound, gc.Commentf("body: %s", rec.Body.Bytes()))
}

func (s *serverSuite) TestServerHasAccessControlAllowOrigin(c *gc.C) {
	serverParams := jemserver.Params{
		DB:              s.Session.DB("foo"),
		ControllerAdmin: "controller-admin",
	}
	impl := map[string]jemserver.NewAPIHandlerFunc{
		"/a": func(p *jem.Pool, config jemserver.Params) ([]httprequest.Handler, error) {
			return []httprequest.Handler{{
				Method: "GET",
				Path:   "/a",
				Handle: func(w http.ResponseWriter, req *http.Request, p httprouter.Params) {
				},
			}}, nil
		},
	}
	h, err := jemserver.New(serverParams, impl)
	c.Assert(err, gc.IsNil)
	defer h.Close()
	rec := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: h,
		URL:     "/a",
	})
	c.Assert(rec.Code, gc.Equals, http.StatusOK)

	c.Assert(len(rec.HeaderMap["Access-Control-Allow-Origin"]), gc.Equals, 1)
	c.Assert(rec.HeaderMap["Access-Control-Allow-Origin"][0], gc.Equals, "*")
	c.Assert(len(rec.HeaderMap["Access-Control-Allow-Headers"]), gc.Equals, 1)
	c.Assert(rec.HeaderMap["Access-Control-Allow-Headers"][0], gc.Equals, "Bakery-Protocol-Version, Macaroons, X-Requested-With, Content-Type")
	c.Assert(len(rec.HeaderMap["Access-Control-Cache-Max-Age"]), gc.Equals, 1)
	c.Assert(rec.HeaderMap["Access-Control-Cache-Max-Age"][0], gc.Equals, "600")
	c.Assert(len(rec.HeaderMap["Access-Control-Allow-Methods"]), gc.Equals, 1)
	c.Assert(rec.HeaderMap["Access-Control-Allow-Methods"][0], gc.Equals, "DELETE,GET,HEAD,PUT,POST,OPTIONS")
	c.Assert(len(rec.HeaderMap["Access-Control-Allow-Credentials"]), gc.Equals, 1)
	c.Assert(rec.HeaderMap["Access-Control-Allow-Credentials"][0], gc.Equals, "true")
	c.Assert(len(rec.HeaderMap["Access-Control-Expose-Headers"]), gc.Equals, 1)
	c.Assert(rec.HeaderMap["Access-Control-Expose-Headers"][0], gc.Equals, "WWW-Authenticate")

	rec = httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: h,
		URL:     "/a",
		Method:  "OPTIONS",
		Header:  http.Header{"Origin": []string{"MyHost"}},
	})
	c.Assert(rec.Code, gc.Equals, http.StatusOK)
	c.Assert(len(rec.HeaderMap["Access-Control-Allow-Origin"]), gc.Equals, 1)
	c.Assert(rec.HeaderMap["Access-Control-Allow-Origin"][0], gc.Equals, "MyHost")
}

func (s *serverSuite) TestServerRunsMonitor(c *gc.C) {
	db := s.Session.DB("foo")
	pool, err := jem.NewPool(jem.Params{
		DB:              db,
		ControllerAdmin: "controller-admin",
	})
	c.Assert(err, gc.IsNil)
	defer pool.Close()
	j := pool.JEM()
	defer j.Close()

	ctlPath := params.EntityPath{"bob", "foo"}
	err = j.AddController(&mongodoc.Controller{
		Path:      ctlPath,
		UUID:      "some-uuid",
		CACert:    jujutesting.CACert,
		AdminUser: "bob",
		HostPorts: []string{"0.1.2.3:4567"},
	})
	c.Assert(err, gc.IsNil)

	params := jemserver.Params{
		DB:              db,
		AgentUsername:   "foo",
		RunMonitor:      true,
		ControllerAdmin: "controller-admin",
	}
	// Patch the API opening timeout so that it doesn't take the
	// usual 15 seconds to fail - we don't, it holds on to the
	// JEM session for that long after the end of the test because
	// API dialling isn't stopped when the monitor is.
	s.PatchValue(&jem.APIOpenTimeout, time.Millisecond)
	h, err := jemserver.New(params, map[string]jemserver.NewAPIHandlerFunc{
		"/v0": func(p *jem.Pool, config jemserver.Params) ([]httprequest.Handler, error) {
			return nil, nil
		},
	})
	c.Assert(err, gc.IsNil)
	defer h.Close()

	// Poll the database to check that the monitor lease is taken out.
	var ctl *mongodoc.Controller
	for a := jujutesting.LongAttempt.Start(); a.Next(); {
		ctl, err = j.Controller(ctlPath)
		c.Assert(err, gc.IsNil)
		if ctl.MonitorLeaseOwner != "" {
			break
		}
		if !a.HasNext() {
			c.Fatalf("lease never acquired")
		}
	}
	c.Assert(ctl.MonitorLeaseOwner, gc.Matches, "foo-[a-z0-9]+")
}
