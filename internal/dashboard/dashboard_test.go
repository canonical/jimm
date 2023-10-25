// Copyright 2020 Canonical Ltd.

package dashboard_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	jujuparams "github.com/juju/juju/apiserver/params"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"github.com/julienschmidt/httprouter"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/internal/dashboard"
)

const (
	configFile = `// eslint-disable-next-line no-unused-vars
var jaasDashboardConfig = {
  // API host to allow app to connect and retrieve models
  baseControllerURL: "jimm.jujucharms.com",
  // Configurable base url to allow deploying to different paths.
  baseAppURL: "/dashboard",
  // If true then identity will be provided by a third party provider.
  identityProviderAvailable: true
};
`
	indexFile   = `Index File`
	versionFile = `{"version": "0.8.1", "git-sha": "34388e4b0b3e68e2c2ba342cb45f0f21d248fd3c"}`
)

var _ = gc.Suite(&dashboardSuite{})

type dashboardSuite struct {
	dataPath string
	server   *httptest.Server
}

func (s *dashboardSuite) SetUpTest(c *gc.C) {
	dir, err := ioutil.TempDir("", "dashboard_test")
	if err != nil {
		log.Fatal(err)
	}
	s.dataPath = dir

	tmpFile := filepath.Join(dir, "config.js.go")
	err = ioutil.WriteFile(tmpFile, []byte(configFile), 0666)
	c.Assert(err, jc.ErrorIsNil)

	tmpFile = filepath.Join(dir, "index.html")
	err = ioutil.WriteFile(tmpFile, []byte(indexFile), 0666)
	c.Assert(err, jc.ErrorIsNil)

	tmpFile = filepath.Join(dir, "version.json")
	err = ioutil.WriteFile(tmpFile, []byte(versionFile), 0666)
	c.Assert(err, jc.ErrorIsNil)

	ctx := context.Background()
	router := httprouter.New()
	err = dashboard.Register(ctx, router, dir)
	c.Assert(err, jc.ErrorIsNil)

	s.server = httptest.NewServer(router)
}

func (s *dashboardSuite) TearDownTest(c *gc.C) {
	if s.dataPath != "" {
		os.RemoveAll(s.dataPath)
	}
	if s.server != nil {
		s.server.Close()
	}
}

func (s *dashboardSuite) TestDashboardRedirect(c *gc.C) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/dashboard", s.server.URL), nil)
	c.Assert(err, jc.ErrorIsNil)

	response, err := http.DefaultClient.Do(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.StatusCode, gc.Equals, http.StatusOK)

	data, err := ioutil.ReadAll(response.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, indexFile)
}

func (s *dashboardSuite) TestDashboardConfigFile(c *gc.C) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/config.js", s.server.URL), nil)
	c.Assert(err, jc.ErrorIsNil)

	response, err := http.DefaultClient.Do(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.StatusCode, gc.Equals, http.StatusOK)

	data, err := ioutil.ReadAll(response.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, configFile)
}

func (s *dashboardSuite) TestDashboardDefaultToIndex(c *gc.C) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/dashboard/not_found.js", s.server.URL), nil)
	c.Assert(err, jc.ErrorIsNil)

	response, err := http.DefaultClient.Do(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.StatusCode, gc.Equals, http.StatusOK)

	data, err := ioutil.ReadAll(response.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, indexFile)
}

func (s *dashboardSuite) TestGUIArchiveEndpoint(c *gc.C) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/gui-archive", s.server.URL), nil)
	c.Assert(err, jc.ErrorIsNil)

	response, err := http.DefaultClient.Do(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.StatusCode, gc.Equals, http.StatusOK)

	data, err := ioutil.ReadAll(response.Body)
	c.Assert(err, jc.ErrorIsNil)
	var gar jujuparams.GUIArchiveResponse
	err = json.Unmarshal(data, &gar)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gar, jc.DeepEquals, jujuparams.GUIArchiveResponse{
		Versions: []jujuparams.GUIArchiveVersion{{
			Version: version.MustParse("0.8.1"),
			SHA256:  "34388e4b0b3e68e2c2ba342cb45f0f21d248fd3c",
			Current: true,
		}},
	})
}

func (s *dashboardSuite) TestRedirectGUIArchiveEndpoint(c *gc.C) {
	ctx := context.Background()
	router := httprouter.New()
	err := dashboard.Register(ctx, router, "https://test.example.com")
	c.Assert(err, jc.ErrorIsNil)

	server := httptest.NewServer(router)
	defer server.Close()

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/gui-archive", server.URL), nil)
	c.Assert(err, jc.ErrorIsNil)

	response, err := http.DefaultClient.Do(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.StatusCode, gc.Equals, http.StatusOK)

	data, err := ioutil.ReadAll(response.Body)
	c.Assert(err, jc.ErrorIsNil)
	var gar jujuparams.GUIArchiveResponse
	err = json.Unmarshal(data, &gar)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gar, jc.DeepEquals, jujuparams.GUIArchiveResponse{
		Versions: []jujuparams.GUIArchiveVersion{{
			Version: version.Number{},
			SHA256:  "",
			Current: true,
		}},
	})
}
