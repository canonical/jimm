// Copyright 2015 Canonical Ltd.

package jem_test

import (
	"encoding/json"
	"net/http"

	"github.com/juju/httprequest"
	jujutesting "github.com/juju/testing"
	"github.com/juju/testing/httptesting"
	"github.com/julienschmidt/httprouter"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jem/internal/jem"
)

type serverSuite struct {
	jujutesting.IsolatedMgoSuite
}

var _ = gc.Suite(&serverSuite{})

func (s *serverSuite) TestNewServerWithNoVersions(c *gc.C) {
	params := jem.ServerParams{
		DB: s.Session.DB("foo"),
	}
	h, err := jem.NewServer(params, nil)
	c.Assert(err, gc.ErrorMatches, `JEM server must serve at least one version of the API`)
	c.Assert(h, gc.IsNil)
}

type versionResponse struct {
	Version string
	Path    string
}

func (s *serverSuite) TestNewServerWithVersions(c *gc.C) {
	serverParams := jem.ServerParams{
		DB: s.Session.DB("foo"),
	}
	serveVersion := func(vers string) jem.NewAPIHandlerFunc {
		return func(p *jem.Pool, config jem.ServerParams) ([]httprequest.Handler, error) {
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

	h, err := jem.NewServer(serverParams, map[string]jem.NewAPIHandlerFunc{
		"version1": serveVersion("version1"),
	})
	c.Assert(err, gc.IsNil)
	assertServesVersion(c, h, "version1")
	assertDoesNotServeVersion(c, h, "version2")
	assertDoesNotServeVersion(c, h, "version3")

	h, err = jem.NewServer(serverParams, map[string]jem.NewAPIHandlerFunc{
		"version1": serveVersion("version1"),
		"version2": serveVersion("version2"),
	})
	c.Assert(err, gc.IsNil)
	assertServesVersion(c, h, "version1")
	assertServesVersion(c, h, "version2")
	assertDoesNotServeVersion(c, h, "version3")

	h, err = jem.NewServer(serverParams, map[string]jem.NewAPIHandlerFunc{
		"version1": serveVersion("version1"),
		"version2": serveVersion("version2"),
		"version3": serveVersion("version3"),
	})
	c.Assert(err, gc.IsNil)
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
