// Copyright 2020 Canonical Ltd.

package jimmtest

import (
	"context"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/canonical/candid/candidtest"
	cofga "github.com/canonical/ofga"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/juju/api"
	"github.com/juju/juju/core/network"
	corejujutesting "github.com/juju/juju/juju/testing"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmhttp"
	"github.com/canonical/jimm/internal/jimmjwx"
	"github.com/canonical/jimm/internal/jujuclient"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	"github.com/canonical/jimm/internal/pubsub"
	"github.com/canonical/jimm/internal/wellknownapi"
)

// ControllerUUID is the UUID of the JIMM controller used in tests.
const ControllerUUID = "c1991ce8-96c2-497d-8e2a-e0cc42ca3aca"

// A GocheckTester adapts a gc.C to the Tester interface.
type GocheckTester struct {
	*gc.C
}

// Name implements Tester.Name.
func (t GocheckTester) Name() string {
	return t.C.TestName()
}

func (t GocheckTester) Cleanup(f func()) {
	t.C.Logf("warning: gocheck does not support Cleanup functions; make sure you're using suite's tear-down method")
}

// A JIMMSuite is a suite that initialises a JIMM.
type JIMMSuite struct {
	// JIMM is a JIMM that can be used in tests. JIMM is initialised in
	// SetUpTest. The JIMM configured in this suite does not have an
	// Authenticator configured.
	JIMM *jimm.JIMM

	AdminUser   *dbmodel.Identity
	OFGAClient  *openfga.OFGAClient
	COFGAClient *cofga.Client
	COFGAParams *cofga.OpenFGAParams

	Server *httptest.Server
	cancel context.CancelFunc
}

func (s *JIMMSuite) SetUpTest(c *gc.C) {
	var err error
	s.OFGAClient, s.COFGAClient, s.COFGAParams, err = SetupTestOFGAClient(c.TestName())
	c.Assert(err, gc.IsNil)

	// Setup OpenFGA.
	s.JIMM = &jimm.JIMM{
		Database: db.Database{
			DB: PostgresDB(GocheckTester{c}, nil),
		},
		CredentialStore: &InMemoryCredentialStore{},
		Pubsub:          &pubsub.Hub{MaxConcurrency: 10},
		UUID:            ControllerUUID,
		OpenFGAClient:   s.OFGAClient,
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	// Connects to a pre-configured keycloak realm
	authSvc, err := auth.NewAuthenticationService(ctx, auth.AuthenticationServiceParams{
		IssuerURL:      "http://localhost:8082/realms/jimm",
		DeviceClientID: "jimm-device",
		DeviceScopes:   []string{oidc.ScopeOpenID, "profile", "email"},
	})
	c.Assert(err, gc.Equals, nil)
	s.JIMM.OAuthAuthenticator = authSvc

	err = s.JIMM.Database.Migrate(ctx, false)
	c.Assert(err, gc.Equals, nil)
	s.AdminUser = &dbmodel.Identity{
		Name:      "alice@external",
		LastLogin: db.Now(),
	}
	err = s.JIMM.Database.GetIdentity(ctx, s.AdminUser)
	c.Assert(err, gc.Equals, nil)

	adminUser := openfga.NewUser(s.AdminUser, s.OFGAClient)
	err = adminUser.SetControllerAccess(ctx, s.JIMM.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, gc.Equals, nil)

	// add jimmtest.DefaultControllerUUID as a controller to JIMM
	err = s.OFGAClient.AddController(ctx, s.JIMM.ResourceTag(), names.NewControllerTag("982b16d9-a945-4762-b684-fd4fd885aa10"))
	c.Assert(err, gc.Equals, nil)

	mux := chi.NewRouter()
	mountHandler := func(path string, h jimmhttp.JIMMHttpHandler) {
		mux.Mount(path, h.Routes())
	}

	mountHandler(
		"/.well-known",
		wellknownapi.NewWellKnownHandler(s.JIMM.CredentialStore),
	)

	s.Server = httptest.NewServer(mux)

	s.JIMM.JWKService = jimmjwx.NewJWKSService(s.JIMM.CredentialStore)
	err = s.JIMM.JWKService.StartJWKSRotator(ctx, time.NewTicker(time.Hour).C, time.Now().UTC().AddDate(0, 3, 0))
	c.Assert(err, gc.Equals, nil)

	u, _ := url.Parse(s.Server.URL)

	s.JIMM.JWTService = jimmjwx.NewJWTService(jimmjwx.JWTServiceParams{
		Host:   u.Host,
		Store:  s.JIMM.CredentialStore,
		Secure: false,
		Expiry: time.Minute,
	})
	s.JIMM.JWTService.RegisterJWKSCache(ctx, s.Server.Client())
	s.JIMM.Dialer = &jujuclient.Dialer{
		JWTService: s.JIMM.JWTService,
	}
}

func (s *JIMMSuite) TearDownTest(c *gc.C) {
	if s.cancel != nil {
		s.cancel()
	}
	if s.Server != nil {
		s.Server.Close()
	}
	if err := s.JIMM.Database.Close(); err != nil {
		c.Logf("failed to close database connections at tear down: %s", err)
	}
}

func (s *JIMMSuite) NewUser(u *dbmodel.Identity) *openfga.User {
	return openfga.NewUser(u, s.OFGAClient)
}

func (s *JIMMSuite) AddController(c *gc.C, name string, info *api.Info) {
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
	adminUser := s.NewUser(s.AdminUser)
	adminUser.JimmAdmin = true
	err := s.JIMM.AddController(context.Background(), adminUser, ctl)
	c.Assert(err, gc.Equals, nil)
}

func (s *JIMMSuite) UpdateCloudCredential(c *gc.C, tag names.CloudCredentialTag, cred jujuparams.CloudCredential) {
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

func (s *JIMMSuite) AddModel(c *gc.C, owner names.UserTag, name string, cloud names.CloudTag, region string, cred names.CloudCredentialTag) names.ModelTag {
	ctx := context.Background()
	u := dbmodel.Identity{
		Name: owner.Id(),
	}
	err := s.JIMM.Database.GetIdentity(ctx, &u)
	c.Assert(err, gc.Equals, nil)
	mi, err := s.JIMM.AddModel(ctx, s.NewUser(&u), &jimm.ModelCreateArgs{
		Name:            name,
		Owner:           owner,
		Cloud:           cloud,
		CloudRegion:     region,
		CloudCredential: cred,
	})
	c.Assert(err, gc.Equals, nil)

	user := s.NewUser(&u)
	err = user.SetModelAccess(context.Background(), names.NewModelTag(mi.UUID), ofganames.AdministratorRelation)
	c.Assert(err, gc.Equals, nil)

	return names.NewModelTag(mi.UUID)
}

// A CandidSuite is a suite that initialises a candid test system to use a
// jimm Authenticator.
type CandidSuite struct {
	// ControllerAdmins is the list of users and groups that are
	// controller adminstrators.
	ControllerAdmins []string

	// The following are created in SetUpTest
	Candid          *candidtest.Server
	CandidPublicKey string
	Authenticator   jimm.Authenticator
}

func (s *CandidSuite) SetUpTest(c *gc.C) {
	s.Candid = candidtest.NewServer()
	s.Candid.AddUser("agent-user", candidtest.GroupListGroup)
	ofgaClient, _, _, err := SetupTestOFGAClient(c.TestName())
	c.Assert(err, gc.IsNil)
	s.Authenticator = auth.JujuAuthenticator{
		Client: ofgaClient,
		Bakery: identchecker.NewBakery(identchecker.BakeryParams{
			Locator:        s.Candid,
			Key:            bakery.MustGenerateKey(),
			IdentityClient: s.Candid.CandidClient("agent-user"),
			Location:       "jimmtest",
		}),
		ControllerAdmins: s.ControllerAdmins,
	}
	tpi, err := httpbakery.ThirdPartyInfoForLocation(context.Background(), nil, s.Candid.URL.String())
	c.Assert(err, gc.Equals, nil)
	pk, err := tpi.PublicKey.MarshalText()
	c.Assert(err, gc.Equals, nil)
	s.CandidPublicKey = string(pk)

}

func (s *CandidSuite) TearDownTest(c *gc.C) {
	s.Authenticator = nil
	if s.Candid != nil {
		s.Candid.Close()
		s.Candid = nil
	}
}

// A JujuSuite is a suite that intialises a JIMM and adds the testing juju
// controller.
type JujuSuite struct {
	JIMMSuite
	corejujutesting.JujuConnSuite
	LoggingSuite
}

func (s *JujuSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.LoggingSuite.SetUpSuite(c)
}

func (s *JujuSuite) TearDownSuite(c *gc.C) {
	s.LoggingSuite.TearDownSuite(c)
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *JujuSuite) SetUpTest(c *gc.C) {
	s.JIMMSuite.SetUpTest(c)
	s.ControllerConfigAttrs = map[string]interface{}{
		"login-token-refresh-url": s.Server.URL + "/.well-known/jwks.json",
	}
	s.JujuConnSuite.SetUpTest(c)
	s.LoggingSuite.SetUpTest(c)

	s.AddController(c, "controller-1", s.APIInfo(c))
}

func (s *JujuSuite) TearDownTest(c *gc.C) {
	s.LoggingSuite.TearDownTest(c)
	s.JujuConnSuite.TearDownTest(c)
	s.JIMMSuite.TearDownTest(c)
}

type BootstrapSuite struct {
	JujuSuite

	CloudCredential *dbmodel.CloudCredential
	Model           *dbmodel.Model
}

func (s *BootstrapSuite) SetUpTest(c *gc.C) {
	s.JujuSuite.SetUpTest(c)

	cct := names.NewCloudCredentialTag(TestCloudName + "/bob@external/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{
		AuthType: "empty",
	})
	ctx := context.Background()
	s.CloudCredential = new(dbmodel.CloudCredential)
	s.CloudCredential.SetTag(cct)
	err := s.JIMM.Database.GetCloudCredential(ctx, s.CloudCredential)
	c.Assert(err, gc.Equals, nil)

	mt := s.AddModel(c, names.NewUserTag("bob@external"), "model-1", names.NewCloudTag(TestCloudName), TestCloudRegionName, cct)
	s.Model = new(dbmodel.Model)
	s.Model.SetTag(mt)
	err = s.JIMM.Database.GetModel(ctx, s.Model)
	c.Assert(err, gc.Equals, nil)
}
