// Copyright 2020 Canonical Ltd.

package dashboard_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	"github.com/julienschmidt/httprouter"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/dashboard"
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

	tmpFile := filepath.Join(dir, "config.js")
	err = ioutil.WriteFile(tmpFile, []byte(configFile), 0666)
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

func (s *dashboardSuite) TestDashboardConfigFile(c *gc.C) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/dashboard/config.js", s.server.URL), nil)
	c.Assert(err, jc.ErrorIsNil)

	response, err := http.DefaultClient.Do(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.StatusCode, gc.Equals, http.StatusOK)

	data, err := ioutil.ReadAll(response.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, configFile)
}

func (s *dashboardSuite) TestDashboardFileNotFound(c *gc.C) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/dashboard/not_found.js", s.server.URL), nil)
	c.Assert(err, jc.ErrorIsNil)

	response, err := http.DefaultClient.Do(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.StatusCode, gc.Equals, http.StatusNotFound)
}
