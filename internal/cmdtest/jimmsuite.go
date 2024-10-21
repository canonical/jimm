// Copyright 2024 Canonical.

// Package cmdtest provides the test suite used for CLI tests
// as well as helper functions used for integration based CLI tests.
package cmdtest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"time"

	cofga "github.com/canonical/ofga"
	"github.com/juju/juju/api"
	"github.com/juju/juju/core/network"
	corejujutesting "github.com/juju/juju/juju/testing"
	jjclient "github.com/juju/juju/jujuclient"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	service "github.com/canonical/jimm/v3/cmd/jimmsrv/service"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type JimmCmdSuite struct {
	corejujutesting.JujuConnSuite

	Params      service.Params
	HTTP        *httptest.Server
	Service     *service.Service
	AdminUser   *dbmodel.Identity
	ClientStore func() *jjclient.MemStore
	JIMM        *jimm.JIMM
	cancel      context.CancelFunc
	testUser    string

	OFGAClient  *openfga.OFGAClient
	COFGAClient *cofga.Client
	COFGAParams *cofga.OpenFGAParams

	databaseName string
}

func (s *JimmCmdSuite) SetUpTest(c *gc.C) {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	s.HTTP = httptest.NewUnstartedServer(nil)
	u, err := url.Parse("http://" + s.HTTP.Listener.Addr().String())
	c.Assert(err, gc.Equals, nil)

	ofgaClient, cofgaClient, cofgaParams, err := jimmtest.SetupTestOFGAClient(c.TestName())
	c.Assert(err, gc.Equals, nil)
	s.OFGAClient = ofgaClient
	s.COFGAClient = cofgaClient
	s.COFGAParams = cofgaParams

	s.Params = jimmtest.NewTestJimmParams(&jimmtest.GocheckTester{C: c})
	dsn, err := url.Parse(s.Params.DSN)
	c.Assert(err, gc.Equals, nil)
	s.databaseName = strings.ReplaceAll(dsn.Path, "/", "")
	s.Params.PublicDNSName = u.Host
	s.Params.ControllerAdmins = []string{"admin"}
	s.Params.OpenFGAParams = service.OpenFGAParams{
		Scheme:    cofgaParams.Scheme,
		Host:      cofgaParams.Host,
		Port:      cofgaParams.Port,
		Store:     cofgaParams.StoreID,
		Token:     cofgaParams.Token,
		AuthModel: cofgaParams.AuthModelID,
	}
	s.Params.JWTExpiryDuration = time.Minute
	s.Params.InsecureSecretStorage = true
	s.Params.CookieSessionKey = []byte("test-secret")

	srv, err := service.NewService(ctx, s.Params)
	c.Assert(err, gc.Equals, nil)
	s.Service = srv
	s.JIMM = srv.JIMM()
	s.HTTP.Config = &http.Server{Handler: srv, ReadHeaderTimeout: time.Second * 5}

	err = s.Service.StartJWKSRotator(ctx, time.NewTicker(time.Hour).C, time.Now().UTC().AddDate(0, 3, 0))
	c.Assert(err, gc.Equals, nil)

	s.HTTP.Start()

	// Now we can set up the juju conn suites
	s.ControllerConfigAttrs = map[string]interface{}{
		"login-token-refresh-url": u.String() + "/.well-known/jwks.json",
	}
	s.JujuConnSuite.SetUpTest(c)

	i, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, gc.IsNil)
	s.AdminUser = i
	s.AdminUser.LastLogin = db.Now()

	s.AddAdminUser(c, "alice@canonical.com")

	// Return the same in-memory store on every invocation because
	// the store details will be updated on every controller connection.
	s.ClientStore = func() *jjclient.MemStore {
		store := jjclient.NewMemStore()
		store.CurrentControllerName = "JIMM"
		store.Controllers["JIMM"] = jjclient.ControllerDetails{
			ControllerUUID: jimmtest.ControllerUUID,
			APIEndpoints:   []string{u.Host},
			PublicDNSName:  s.HTTP.URL,
		}
		store.Accounts["JIMM"] = jjclient.AccountDetails{User: s.testUser}
		return store
	}
}

func (s *JimmCmdSuite) TearDownTest(c *gc.C) {
	if s.cancel != nil {
		s.cancel()
	}
	if s.HTTP != nil {
		s.HTTP.Close()
	}
	if s.JIMM != nil && s.JIMM.Database.DB != nil {
		if err := s.JIMM.Database.Close(); err != nil {
			c.Logf("failed to close database connections at tear down: %s", err)
		}
	}
	// Only delete the DB after closing connections to it.
	_, skipCleanup := os.LookupEnv("NO_DB_CLEANUP")
	if !skipCleanup {
		err := jimmtest.DeleteDatabase(s.databaseName)
		if err != nil {
			c.Logf("failed to delete database (%s): %s", s.databaseName, err)
		}
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *JimmCmdSuite) AddAdminUser(c *gc.C, email string) {
	identity, err := dbmodel.NewIdentity(email)
	c.Assert(err, gc.IsNil)
	err = s.JIMM.Database.GetIdentity(context.Background(), identity)
	c.Assert(err, gc.IsNil)
	ofgaUser := openfga.NewUser(identity, s.OFGAClient)
	err = ofgaUser.SetControllerAccess(context.Background(), s.JIMM.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, gc.IsNil)
}

// SetupCLIAccess should be run at the start of all CLI tests that want to communicate with JIMM.
// It will ensure a user is created in JIMM's in-memory accounts store that matches the username passed in.
// This is necessary as Juju's CLI library validates the logged-in user matches the expected user from the accounts store.
// This function will return a login provider that can be used to handle authentication.
func (s *JimmCmdSuite) SetupCLIAccess(c *gc.C, username string) api.LoginProvider {
	email := jimmtest.ConvertUsernameToEmail(username)
	lp := jimmtest.NewUserSessionLogin(c, email)
	s.testUser = email
	return lp
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
	u, err := dbmodel.NewIdentity(tag.Owner().Id())
	c.Assert(err, gc.IsNil)
	user := openfga.NewUser(u, s.JIMM.OpenFGAClient)
	err = s.JIMM.Database.GetIdentity(ctx, u)
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
	i, err := dbmodel.NewIdentity(owner.Id())
	c.Assert(err, gc.IsNil)
	u := openfga.NewUser(
		i,
		s.OFGAClient,
	)
	err = s.JIMM.Database.GetIdentity(ctx, u.Identity)
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
