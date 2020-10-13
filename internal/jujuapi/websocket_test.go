// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"bytes"
	"context"
	"encoding/pem"
	"fmt"
	"net/http/httptest"
	"net/url"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/apitest"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

type websocketSuite struct {
	apitest.Suite
	Server *httptest.Server
}

func (s *websocketSuite) SetUpTest(c *gc.C) {
	s.Suite.SetUpTest(c)
	s.Server = httptest.NewTLSServer(s.JEMSrv)
}

func (s *websocketSuite) TearDownTest(c *gc.C) {
	s.Server.Close()
	s.Suite.TearDownTest(c)
}

// open creates a new websockec connection to the test server, using the
// connection info specified in info, authenticating as the given user.
// If info is nil then default values will be used.
func (s *websocketSuite) open(c *gc.C, info *api.Info, username string) api.Connection {
	var inf api.Info
	if info != nil {
		inf = *info
	}
	u, err := url.Parse(s.Server.URL)
	c.Assert(err, gc.Equals, nil)
	inf.Addrs = []string{
		u.Host,
	}
	w := new(bytes.Buffer)
	err = pem.Encode(w, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: s.Server.TLS.Certificates[0].Certificate[0],
	})
	c.Assert(err, gc.Equals, nil)
	inf.CACert = w.String()
	conn, err := api.Open(&inf, api.DialOpts{
		InsecureSkipVerify: true,
		BakeryClient:       s.IDMSrv.Client(username),
	})
	c.Assert(err, gc.Equals, nil)
	return conn
}

type createModelParams struct {
	name     string
	username string
	cloud    string
	region   string
	cred     params.CredentialName
	config   map[string]interface{}
}

// assertCreateModel creates a model for use in tests, using a
// connection authenticated as the given user. The model info for the
// newly created model is returned.
func (s *websocketSuite) assertCreateModel(c *gc.C, p createModelParams) base.ModelInfo {
	conn := s.open(c, nil, p.username)
	defer conn.Close()
	client := modelmanager.NewClient(conn)
	if p.cloud == "" {
		p.cloud = "dummy"
	}
	credentialTag := names.NewCloudCredentialTag(fmt.Sprintf("dummy/%s@external/%s", p.username, p.cred))
	mi, err := client.CreateModel(p.name, p.username+"@external", p.cloud, p.region, credentialTag, p.config)
	c.Assert(err, gc.Equals, nil)
	return mi
}

func (s *websocketSuite) grant(c *gc.C, path params.EntityPath, user params.User, access jujuparams.UserAccessPermission) {
	ctx := context.Background()
	err := s.JEM.GrantModel(ctx, jemtest.NewIdentity(string(path.User)), &mongodoc.Model{Path: path}, user, access)
	c.Assert(err, gc.Equals, nil)
}
