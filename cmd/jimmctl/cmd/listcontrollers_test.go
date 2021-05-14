// Copyright 2021 Canonical Ltd.

package cmd_test

import (
	"bytes"
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery/agent"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/juju/jujuclient"
	jujutesting "github.com/juju/juju/testing"
	cookiejar "github.com/juju/persistent-cookiejar"
	"github.com/julienschmidt/httprouter"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/cmd/jimmctl/cmd"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jemserver"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	"github.com/CanonicalLtd/jimm/internal/jujuapi"
)

func TestPackage(t *testing.T) {
	jujutesting.MgoTestPackage(t)
}

var (
	expectedSuperuserOutput = `- name: controller-1
  uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
  publicaddress: ""
  apiaddresses:
  - localhost:.*
  cacertificate: |
    -----BEGIN CERTIFICATE-----
    .*
    -----END CERTIFICATE-----
  cloudtag: cloud-dummy
  cloudregion: dummy-region
  username: admin
  agentversion: .*
  status:
    status: available
    info: ""
    data: {}
    since: null
- name: dummy-1
  uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
  publicaddress: ""
  apiaddresses:
  - localhost:46539
  cacertificate: |
    -----BEGIN CERTIFICATE-----
    .*
    -----END CERTIFICATE-----
  cloudtag: cloud-dummy
  cloudregion: dummy-region
  username: admin
  agentversion: .*
  status:
    status: available
    info: ""
    data: {}
    since: null
`

	expectedOutput = `- name: jaas
  uuid: 914487b5-60e7-42bb-bd63-1adc3fd3a388
  publicaddress: ""
  apiaddresses: \[\]
  cacertificate: ""
  cloudtag: ""
  cloudregion: ""
  username: ""
  agentversion: .*
  status:
    status: available
    info: ""
    data: {}
    since: null
`
)

type listControllersSuite struct {
	jimmSuite
}

var _ = gc.Suite(&listControllersSuite{})

func (s *listControllersSuite) TestListControllersSuperuser(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	// alice is superuser
	bClient := s.userBakeryClient("alice")
	context, err := cmdtesting.RunCommand(c, cmd.NewListControllersCommandForTesting(s.store, bClient))
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Matches, expectedSuperuserOutput)
}

func (s *listControllersSuite) TestListControllers(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	// bob is not superuser
	bClient := s.userBakeryClient("bob")
	context, err := cmdtesting.RunCommand(c, cmd.NewListControllersCommandForTesting(s.store, bClient))
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Matches, expectedOutput)
}

type jimmSuite struct {
	jimmtest.CandidSuite
	jimmtest.BootstrapSuite

	store      *jujuclient.MemStore
	Params     jemserver.HandlerParams
	APIHandler http.Handler
	HTTP       *httptest.Server

	Credential2 *dbmodel.CloudCredential
	Model2      *dbmodel.Model
}

func (s *jimmSuite) TearDownTest(c *gc.C) {
	s.CandidSuite.TearDownTest(c)
	s.BootstrapSuite.TearDownTest(c)
}

func (s *jimmSuite) SetUpTest(c *gc.C) {
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

	u, err := url.Parse(s.HTTP.URL)
	c.Assert(err, gc.IsNil)

	w := new(bytes.Buffer)
	err = pem.Encode(w, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: s.HTTP.TLS.Certificates[0].Certificate[0],
	})
	c.Assert(err, gc.Equals, nil)

	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "JIMM"
	s.store.Controllers["JIMM"] = jujuclient.ControllerDetails{
		ControllerUUID: "914487b5-60e7-42bb-bd63-1adc3fd3a388",
		APIEndpoints:   []string{u.Host},
		PublicDNSName:  s.HTTP.URL,
		CACert:         w.String(),
	}
	s.store.CookieJars["JIMM"] = &cookiejar.Jar{}
}

func (s *jimmSuite) userBakeryClient(username string) *httpbakery.Client {
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
	return bClient
}
