// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"bytes"
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery/agent"
	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"github.com/julienschmidt/httprouter"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jemserver"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	"github.com/CanonicalLtd/jimm/internal/jujuapi"
)

type websocketSuite struct {
	jimmtest.CandidSuite
	jimmtest.BootstrapSuite

	Params     jemserver.HandlerParams
	APIHandler http.Handler
	HTTP       *httptest.Server

	Credential2 *dbmodel.CloudCredential
	Model2      *dbmodel.Model
	Model3      *dbmodel.Model
}

func (s *websocketSuite) SetUpTest(c *gc.C) {
	ctx := context.Background()

	s.ControllerAdmins = []string{"controller-admin"}

	s.CandidSuite.SetUpTest(c)
	s.BootstrapSuite.SetUpTest(c)

	s.JIMM.Authenticator = s.Authenticator

	s.Params.WebsocketRequestTimeout = time.Second
	s.Params.ControllerUUID = "914487b5-60e7-42bb-bd63-1adc3fd3a388"
	s.Params.CharmstoreLocation = "https://api.jujucharms.com/charmstore"
	s.Params.MeteringLocation = "https://api.jujucharms.com/omnibus"
	s.Params.IdentityLocation = s.Candid.URL.String()
	handlers, err := jujuapi.NewAPIHandler(ctx, s.JIMM, s.Params)
	c.Assert(err, gc.Equals, nil)
	var r httprouter.Router
	for _, h := range handlers {
		r.Handle(h.Method, h.Path, h.Handle)
	}
	s.APIHandler = &r
	s.HTTP = httptest.NewTLSServer(s.APIHandler)

	s.Candid.AddUser("alice")

	cct := names.NewCloudCredentialTag("dummy/charlie@external/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	s.Credential2 = new(dbmodel.CloudCredential)
	s.Credential2.SetTag(cct)
	err = s.JIMM.Database.GetCloudCredential(ctx, s.Credential2)
	c.Assert(err, gc.Equals, nil)

	mt := s.AddModel(c, names.NewUserTag("charlie@external"), "model-2", names.NewCloudTag("dummy"), "dummy-region", cct)
	s.Model2 = new(dbmodel.Model)
	s.Model2.SetTag(mt)
	err = s.JIMM.Database.GetModel(ctx, s.Model2)
	c.Assert(err, gc.Equals, nil)

	mt = s.AddModel(c, names.NewUserTag("charlie@external"), "model-3", names.NewCloudTag("dummy"), "dummy-region", cct)
	s.Model3 = new(dbmodel.Model)
	s.Model3.SetTag(mt)
	err = s.JIMM.Database.GetModel(ctx, s.Model3)
	c.Assert(err, gc.Equals, nil)
	s.JIMM.Database.UpdateUserModelAccess(ctx, &dbmodel.UserModelAccess{
		ModelID:  s.Model3.ID,
		Username: "bob@external",
		Access:   "read",
	})
}

func (s *websocketSuite) TearDownTest(c *gc.C) {
	s.BootstrapSuite.TearDownTest(c)
	s.CandidSuite.TearDownTest(c)
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

	s.Candid.AddUser(username)
	key := s.Candid.UserPublicKey(username)
	bClient := httpbakery.NewClient()
	bClient.Key = &bakery.KeyPair{
		Public:  bakery.PublicKey{Key: bakery.Key(key.Public.Key)},
		Private: bakery.PrivateKey{Key: bakery.Key(key.Private.Key)},
	}
	agent.SetUpAuth(bClient, &agent.AuthInfo{
		Key: bClient.Key,
		Agents: []agent.Agent{{
			URL:      s.Candid.URL.String(),
			Username: username,
		}},
	})

	conn, err := api.Open(&inf, api.DialOpts{
		InsecureSkipVerify: true,
		BakeryClient:       bClient,
	})
	c.Assert(err, gc.Equals, nil)
	return conn
}
