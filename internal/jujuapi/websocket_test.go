// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"bytes"
	"encoding/pem"
	"net/url"
	"time"

	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/apiserver/params"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/jemtest/apitest"
	"github.com/CanonicalLtd/jimm/internal/jujuapi"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

type websocketSuite struct {
	apitest.BootstrapAPISuite

	Credential2 mongodoc.Credential
	Model2      mongodoc.Model
	Model3      mongodoc.Model
}

func (s *websocketSuite) SetUpTest(c *gc.C) {
	s.NewAPIHandler = jujuapi.NewAPIHandler
	s.UseTLS = true
	s.Params.WebsocketRequestTimeout = time.Second
	s.Params.ControllerUUID = "914487b5-60e7-42bb-bd63-1adc3fd3a388"
	s.Params.CharmstoreLocation = "https://api.jujucharms.com/charmstore"
	s.Params.MeteringLocation = "https://api.jujucharms.com/omnibus"
	s.BootstrapAPISuite.SetUpTest(c)

	s.Candid.AddUser("alice", string(s.JEM.ControllerAdmin()))

	s.Credential2 = jemtest.EmptyCredential("charlie", "cred")
	s.UpdateCredential(c, &s.Credential2)
	s.Model2.Path = params.EntityPath{User: "charlie", Name: "model-2"}
	s.Model2.Controller = s.Controller.Path
	s.Model2.Credential = s.Credential2.Path
	s.CreateModel(c, &s.Model2, nil, nil)
	s.Model3.Path = params.EntityPath{User: "charlie", Name: "model-3"}
	s.Model3.Controller = s.Controller.Path
	s.Model3.Credential = s.Credential2.Path
	s.CreateModel(c, &s.Model3, nil, map[params.User]jujuparams.UserAccessPermission{"bob": jujuparams.ModelReadAccess})
}

// open creates a new websockec connection to the test server, using the
// connection info specified in info, authenticating as the given user.
// If info is nil then default values will be used.
func (s *websocketSuite) open(c *gc.C, info *api.Info, username string) api.Connection {
	var inf api.Info
	if info != nil {
		inf = *info
	}
	u, err := url.Parse(s.HTTP.URL)
	c.Assert(err, gc.Equals, nil)
	inf.Addrs = []string{
		u.Host,
	}
	w := new(bytes.Buffer)
	err = pem.Encode(w, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: s.HTTP.TLS.Certificates[0].Certificate[0],
	})
	c.Assert(err, gc.Equals, nil)
	inf.CACert = w.String()
	conn, err := api.Open(&inf, api.DialOpts{
		InsecureSkipVerify: true,
		BakeryClient:       s.Candid.Client(username),
	})
	c.Assert(err, gc.Equals, nil)
	return conn
}
