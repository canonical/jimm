// Copyright 2021 Canonical Ltd.

// Package cmdtest provides the test suite used for CLI tests
// as well as helper functions used for integration based CLI tests.
package cmdtest

import (
	"bytes"
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	cofga "github.com/canonical/ofga"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/core/network"
	corejujutesting "github.com/juju/juju/juju/testing"
	jjclient "github.com/juju/juju/jujuclient"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"

	service "github.com/canonical/jimm"
	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
)

type JimmCmdSuite struct {
	jimmtest.CandidSuite
	corejujutesting.JujuConnSuite

	Params      service.Params
	HTTP        *httptest.Server
	Service     *service.Service
	AdminUser   *dbmodel.Identity
	ClientStore func() *jjclient.MemStore
	JIMM        *jimm.JIMM
	cancel      context.CancelFunc

	OFGAClient  *openfga.OFGAClient
	COFGAClient *cofga.Client
	COFGAParams *cofga.OpenFGAParams
}

func (s *JimmCmdSuite) SetUpTest(c *gc.C) {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	s.CandidSuite.SetUpTest(c)

	s.HTTP = httptest.NewUnstartedServer(nil)
	u, err := url.Parse("https://" + s.HTTP.Listener.Addr().String())
	c.Assert(err, gc.Equals, nil)

	ofgaClient, cofgaClient, cofgaParams, err := jimmtest.SetupTestOFGAClient(c.TestName())
	c.Assert(err, gc.Equals, nil)
	s.OFGAClient = ofgaClient
	s.COFGAClient = cofgaClient
	s.COFGAParams = cofgaParams

	s.Params = service.Params{
		PublicDNSName:    u.Host,
		ControllerUUID:   "914487b5-60e7-42bb-bd63-1adc3fd3a388",
		CandidURL:        s.Candid.URL.String(),
		CandidPublicKey:  s.CandidPublicKey,
		ControllerAdmins: []string{"admin"},
		DSN:              jimmtest.CreateEmptyDatabase(&jimmtest.GocheckTester{c}),
		OpenFGAParams: service.OpenFGAParams{
			Scheme:    cofgaParams.Scheme,
			Host:      cofgaParams.Host,
			Port:      cofgaParams.Port,
			Store:     cofgaParams.StoreID,
			Token:     cofgaParams.Token,
			AuthModel: cofgaParams.AuthModelID,
		},
		JWTExpiryDuration:     time.Minute,
		InsecureSecretStorage: true,
		OAuthAuthenticatorParams: service.OAuthAuthenticatorParams{
			IssuerURL:          "http://localhost:8082/realms/jimm",
			ClientID:           "jimm-device",
			Scopes:             []string{oidc.ScopeOpenID, "profile", "email"},
			SessionTokenExpiry: time.Duration(time.Hour),
		},
		DashboardFinalRedirectURL: "<no dashboard needed for this test>",
	}

	srv, err := service.NewService(ctx, s.Params)
	c.Assert(err, gc.Equals, nil)
	s.Service = srv
	s.JIMM = srv.JIMM()
	s.HTTP.Config = &http.Server{Handler: srv}

	err = s.Service.StartJWKSRotator(ctx, time.NewTicker(time.Hour).C, time.Now().UTC().AddDate(0, 3, 0))
	c.Assert(err, gc.Equals, nil)

	s.HTTP.StartTLS()

	// NOW we can set up the  juju conn suites
	s.ControllerConfigAttrs = map[string]interface{}{
		"login-token-refresh-url": u.String() + "/.well-known/jwks.json",
	}
	s.ControllerAdmins = []string{"controller-admin"}
	s.JujuConnSuite.SetUpTest(c)

	s.AdminUser = &dbmodel.Identity{
		Name:      "alice@external",
		LastLogin: db.Now(),
	}
	err = s.JIMM.Database.GetIdentity(ctx, s.AdminUser)
	c.Assert(err, gc.Equals, nil)

	alice := openfga.NewUser(s.AdminUser, ofgaClient)
	err = alice.SetControllerAccess(context.Background(), s.JIMM.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, gc.Equals, nil)

	s.Candid.AddUser("alice")

	w := new(bytes.Buffer)
	err = pem.Encode(w, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: s.HTTP.TLS.Certificates[0].Certificate[0],
	})
	c.Assert(err, gc.Equals, nil)

	s.ClientStore = func() *jjclient.MemStore {
		store := jjclient.NewMemStore()
		store.CurrentControllerName = "JIMM"
		store.Controllers["JIMM"] = jjclient.ControllerDetails{
			ControllerUUID: "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			APIEndpoints:   []string{u.Host},
			PublicDNSName:  s.HTTP.URL,
			CACert:         w.String(),
		}
		return store
	}
}

// RefreshControllerAddress is a useful helper function when writing table tests for JIMM CLI
// commands that use NewAPIRootWithDialOpts. Each invocation of the NewAPIRootWithDialOpts function
// updates the ClientStore and removes local IPs thus removing JIMM's IP.
// Call this function in your table tests after each test run.
func (s *JimmCmdSuite) RefreshControllerAddress(c *gc.C) {
	jimm, ok := s.ClientStore().Controllers["JIMM"]
	c.Assert(ok, gc.Equals, true)
	u, err := url.Parse(s.HTTP.URL)
	c.Assert(err, gc.IsNil)
	jimm.APIEndpoints = []string{u.Host}
	s.ClientStore().Controllers["JIMM"] = jimm
}

func (s *JimmCmdSuite) TearDownTest(c *gc.C) {
	if s.cancel != nil {
		s.cancel()
	}
	if s.HTTP != nil {
		s.HTTP.Close()
	}
	if err := s.JIMM.Database.Close(); err != nil {
		c.Logf("failed to close database connections at tear down: %s", err)
	}
	s.CandidSuite.TearDownTest(c)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *JimmCmdSuite) UserBakeryClient(username string) *httpbakery.Client {
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

func (s *JimmCmdSuite) AddController(c *gc.C, name string, info *api.Info) {
	ctl := &dbmodel.Controller{
		UUID:              info.ControllerUUID,
		Name:              name,
		AdminIdentityName: info.Tag.Id(),
		AdminPassword:     info.Password,
		CACertificate:     info.CACert,
		Addresses:         nil,
	}
	ctl.Addresses = make(dbmodel.HostPorts, 0, len(info.Addrs))
	for _, addr := range info.Addrs {
		hp, err := network.ParseMachineHostPort(addr)
		c.Assert(err, gc.Equals, nil)
		ctl.Addresses = append(ctl.Addresses, []jujuparams.HostPort{{
			Address: jujuparams.FromMachineAddress(hp.MachineAddress),
			Port:    hp.Port(),
		}})
	}
	adminUser := openfga.NewUser(s.AdminUser, s.OFGAClient)
	adminUser.JimmAdmin = true
	err := s.JIMM.AddController(context.Background(), adminUser, ctl)
	c.Assert(err, gc.Equals, nil)
}

func (s *JimmCmdSuite) UpdateCloudCredential(c *gc.C, tag names.CloudCredentialTag, cred jujuparams.CloudCredential) {
	ctx := context.Background()
	u := dbmodel.Identity{
		Name: tag.Owner().Id(),
	}
	user := openfga.NewUser(&u, s.JIMM.OpenFGAClient)
	err := s.JIMM.Database.GetIdentity(ctx, &u)
	c.Assert(err, gc.Equals, nil)
	_, err = s.JIMM.UpdateCloudCredential(ctx, user, jimm.UpdateCloudCredentialArgs{
		CredentialTag: tag,
		Credential:    cred,
		SkipCheck:     true,
	})
	c.Assert(err, gc.Equals, nil)
}

func (s *JimmCmdSuite) AddModel(c *gc.C, owner names.UserTag, name string, cloud names.CloudTag, region string, cred names.CloudCredentialTag) names.ModelTag {
	ctx := context.Background()
	u := openfga.NewUser(
		&dbmodel.Identity{
			Name: owner.Id(),
		},
		s.OFGAClient,
	)
	err := s.JIMM.Database.GetIdentity(ctx, u.Identity)
	c.Assert(err, gc.Equals, nil)
	mi, err := s.JIMM.AddModel(ctx, u, &jimm.ModelCreateArgs{
		Name:            name,
		Owner:           owner,
		Cloud:           cloud,
		CloudRegion:     region,
		CloudCredential: cred,
	})
	c.Assert(err, gc.Equals, nil)
	return names.NewModelTag(mi.UUID)
}
