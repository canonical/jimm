// Copyright 2015 Canonical Ltd.

package jem_test

import (
	"context"

	cloudapi "github.com/juju/juju/api/cloud"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/network"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/pubsub"
	"github.com/CanonicalLtd/jimm/params"
)

type controllerSuite struct {
	jemtest.JujuConnSuite
	pool                           *jem.Pool
	sessionPool                    *mgosession.Pool
	jem                            *jem.JEM
	usageSenderAuthorizationClient *testUsageSenderAuthorizationClient
}

var _ = gc.Suite(&controllerSuite{})

func (s *controllerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.sessionPool = mgosession.NewPool(context.TODO(), s.Session, 5)
	publicCloudMetadata, _, err := cloud.PublicCloudMetadata()
	c.Assert(err, gc.Equals, nil)
	s.usageSenderAuthorizationClient = &testUsageSenderAuthorizationClient{}
	s.PatchValue(&jem.NewUsageSenderAuthorizationClient, func(_ string, _ *httpbakery.Client) (jem.UsageSenderAuthorizationClient, error) {
		return s.usageSenderAuthorizationClient, nil
	})
	pool, err := jem.NewPool(context.TODO(), jem.Params{
		DB:                  s.Session.DB("jem"),
		ControllerAdmin:     "controller-admin",
		SessionPool:         s.sessionPool,
		PublicCloudMetadata: publicCloudMetadata,
		UsageSenderURL:      "test-usage-sender-url",
		Pubsub: &pubsub.Hub{
			MaxConcurrency: 10,
		},
	})
	c.Assert(err, gc.Equals, nil)
	s.pool = pool
	s.jem = s.pool.JEM(context.TODO())
	s.PatchValue(&utils.OutgoingAccessAllowed, true)
}

func (s *controllerSuite) TearDownTest(c *gc.C) {
	s.jem.Close()
	s.pool.Close()
	s.sessionPool.Close()
	s.JujuConnSuite.TearDownTest(c)
}

var mongodocAPIHostPortsTests = []struct {
	about  string
	hps    []network.MachineHostPorts
	expect [][]mongodoc.HostPort
}{{
	about:  "one address",
	hps:    []network.MachineHostPorts{{{MachineAddress: network.MachineAddress{Value: "0.1.2.3", Scope: network.ScopePublic}, NetPort: 1234}}},
	expect: [][]mongodoc.HostPort{{{Host: "0.1.2.3", Port: 1234, Scope: "public"}}},
}, {
	about:  "unknown scope changed to public",
	hps:    []network.MachineHostPorts{{{MachineAddress: network.MachineAddress{Value: "0.1.2.3", Scope: network.ScopeUnknown}, NetPort: 1234}}},
	expect: [][]mongodoc.HostPort{{{Host: "0.1.2.3", Port: 1234, Scope: "public"}}},
}, {
	about: "unusable addresses removed",
	hps: []network.MachineHostPorts{{
		{MachineAddress: network.MachineAddress{Value: "0.1.2.3", Scope: network.ScopeMachineLocal}, NetPort: 1234},
	}, {
		{MachineAddress: network.MachineAddress{Value: "0.1.2.4", Scope: network.ScopeLinkLocal}, NetPort: 1234},
		{MachineAddress: network.MachineAddress{Value: "0.1.2.5", Scope: network.ScopePublic}, NetPort: 1234},
	}},
	expect: [][]mongodoc.HostPort{{{Host: "0.1.2.5", Port: 1234, Scope: "public"}}},
}}

func (s *controllerSuite) TestMongodocAPIHostPorts(c *gc.C) {
	for i, test := range mongodocAPIHostPortsTests {
		c.Logf("test %d: %v", i, test.about)
		got := jem.MongodocAPIHostPorts(test.hps)
		c.Assert(got, jc.DeepEquals, test.expect)
	}
}

var addControllerTests = []struct {
	about            string
	id               identchecker.ACLIdentity
	ctl              mongodoc.Controller
	expectError      string
	expectErrorCause error
}{{
	about: "success",
	id:    jemtest.NewIdentity("bob", "controller-admin"),
	ctl: mongodoc.Controller{
		Path:   params.EntityPath{"bob", "controller"},
		Public: true,
	},
}, {
	// This test is dependent on the previous one succeeding.
	about: "duplicate",
	id:    jemtest.NewIdentity("bob", "controller-admin"),
	ctl: mongodoc.Controller{
		Path:   params.EntityPath{"bob", "controller"},
		Public: true,
	},
	expectError:      `already exists`,
	expectErrorCause: params.ErrAlreadyExists,
}, {
	about: "unauthorized",
	id:    jemtest.NewIdentity("bob"),
	ctl: mongodoc.Controller{
		Path:   params.EntityPath{"bob", "controller"},
		Public: true,
	},
	expectError:      `unauthorized`,
	expectErrorCause: params.ErrUnauthorized,
}, {
	about: "not public",
	id:    jemtest.NewIdentity("bob", "controller-admin"),
	ctl: mongodoc.Controller{
		Path:   params.EntityPath{"bob", "controller"},
		Public: false,
	},
	expectError:      `cannot add private controller`,
	expectErrorCause: params.ErrForbidden,
}, {
	about: "wrong namespace",
	id:    jemtest.NewIdentity("alice", "controller-admin"),
	ctl: mongodoc.Controller{
		Path:   params.EntityPath{"bob", "controller"},
		Public: true,
	},
	expectError:      `unauthorized`,
	expectErrorCause: params.ErrUnauthorized,
}, {
	about: "connect failure",
	id:    jemtest.NewIdentity("bob", "controller-admin"),
	ctl: mongodoc.Controller{
		Path:          params.EntityPath{"bob", "controller"},
		AdminPassword: "not-the-password",
		Public:        true,
	},
	expectError:      `invalid entity name or password \(unauthorized access\)`,
	expectErrorCause: jem.ErrAPIConnection,
}}

func (s *controllerSuite) TestAddController(c *gc.C) {
	ctx := context.Background()
	info := s.APIInfo(c)
	hps, err := mongodoc.ParseAddresses(info.Addrs)
	c.Assert(err, gc.Equals, nil)

	for i, test := range addControllerTests {
		c.Logf("%d. %s", i, test.about)
		if test.ctl.HostPorts == nil {
			test.ctl.HostPorts = [][]mongodoc.HostPort{hps}
		}
		if test.ctl.CACert == "" {
			test.ctl.CACert = info.CACert
		}
		if test.ctl.AdminUser == "" {
			test.ctl.AdminUser = info.Tag.Id()
		}
		if test.ctl.AdminPassword == "" {
			test.ctl.AdminPassword = info.Password
		}

		err := s.jem.AddController(ctx, test.id, &test.ctl)
		if test.expectError != "" {
			c.Check(err, gc.ErrorMatches, test.expectError)
			if test.expectErrorCause != nil {
				c.Check(errgo.Cause(err), gc.Equals, test.expectErrorCause)
			}
		} else {
			c.Check(err, gc.Equals, nil)
		}
	}
}

func (s *controllerSuite) TestControllerUpdateCredentials(c *gc.C) {
	ctlPath := addController(c, params.EntityPath{User: "bob", Name: "controller"}, s.APIInfo(c), s.jem)
	credPath := credentialPath("dummy", "bob", "cred")
	mCredPath := mgoCredentialPath("dummy", "bob", "cred")
	credTag := names.NewCloudCredentialTag("dummy/bob@external/cred")
	cred := &mongodoc.Credential{
		Path: mCredPath,
		Type: "empty",
	}
	err := jem.UpdateCredential(s.jem.DB, testContext, cred)
	c.Assert(err, gc.Equals, nil)

	err = jem.SetCredentialUpdates(s.jem.DB, testContext, []params.EntityPath{ctlPath}, mongodoc.CredentialPathFromParams(credPath))
	c.Assert(err, gc.Equals, nil)

	ctl, err := s.jem.DB.Controller(testContext, ctlPath)
	c.Assert(err, gc.Equals, nil)

	conn, err := s.jem.OpenAPIFromDoc(testContext, ctl)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()

	err = jem.ControllerUpdateCredentials(s.jem, testContext, conn, ctl)
	c.Assert(err, gc.Equals, nil)

	// check it was updated on the controller.
	client := cloudapi.NewClient(conn)
	creds, err := client.Credentials(credTag)
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType:   "empty",
			Attributes: nil,
			Redacted:   nil,
		},
	}})
}

func (s *controllerSuite) TestConnectMonitor(c *gc.C) {
	ctlPath := addController(c, params.EntityPath{User: "bob", Name: "controller"}, s.APIInfo(c), s.jem)

	// create a credential.
	credPath := credentialPath("dummy", "bob", "cred")
	mCredPath := mgoCredentialPath("dummy", "bob", "cred")
	credTag := names.NewCloudCredentialTag("dummy/bob@external/cred")
	cred := &mongodoc.Credential{
		Path: mCredPath,
		Type: "empty",
	}
	err := jem.UpdateCredential(s.jem.DB, testContext, cred)
	c.Assert(err, gc.Equals, nil)
	err = jem.SetCredentialUpdates(s.jem.DB, testContext, []params.EntityPath{ctlPath}, mongodoc.CredentialPathFromParams(credPath))
	c.Assert(err, gc.Equals, nil)

	// Remove the controller from known clouds.
	_, err = s.jem.DB.CloudRegions().UpdateAll(
		bson.D{{"cloud", "dummy"}},
		bson.D{
			{"$pull", bson.D{{"primarycontrollers", ctlPath}}},
			{"$pull", bson.D{{"secondarycontrollers", ctlPath}}},
		},
	)
	c.Assert(err, gc.Equals, nil)
	cr, err := s.jem.DB.CloudRegion(testContext, "dummy", "")
	c.Assert(err, gc.Equals, nil)
	c.Assert(cr.PrimaryControllers, gc.HasLen, 0)

	// Set the version obviously wrong.
	err = s.jem.DB.SetControllerVersion(testContext, ctlPath, version.Zero)
	c.Assert(err, gc.Equals, nil)

	conn, err := s.jem.ConnectMonitor(testContext, ctlPath)
	c.Assert(err, gc.Equals, nil)

	v, ok := conn.ServerVersion()
	c.Assert(ok, gc.Equals, true)

	// check the credential has been updated.
	client := cloudapi.NewClient(conn)
	creds, err := client.Credentials(credTag)
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType:   "empty",
			Attributes: nil,
			Redacted:   nil,
		},
	}})

	err = conn.Close()
	c.Assert(err, gc.Equals, nil)

	// Check the cloud has been updated.
	cr, err = s.jem.DB.CloudRegion(testContext, "dummy", "")
	c.Assert(err, gc.Equals, nil)
	c.Assert(cr.PrimaryControllers, jc.DeepEquals, []params.EntityPath{ctlPath})

	// Check the version has been updated.
	ctl, err := s.jem.DB.Controller(testContext, ctlPath)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ctl.Version, jc.DeepEquals, &v)
}

func (s *controllerSuite) TestConnectMonitorNotFound(c *gc.C) {
	_, err := s.jem.ConnectMonitor(testContext, params.EntityPath{"not", "there"})
	c.Check(err, gc.ErrorMatches, `controller "not/there" not found`)
	c.Check(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *controllerSuite) TestConnectMonitorConnectionFailure(c *gc.C) {
	ctlPath := addController(c, params.EntityPath{User: "bob", Name: "controller"}, s.APIInfo(c), s.jem)

	// Set the password wrong
	err := s.jem.DB.Controllers().Update(
		bson.D{{"path", ctlPath}},
		bson.D{{"$set", bson.D{{"adminpassword", "bad-password"}}}},
	)

	_, err = s.jem.ConnectMonitor(testContext, ctlPath)
	c.Check(err, gc.ErrorMatches, `invalid entity name or password \(unauthorized access\)`)
	c.Check(errgo.Cause(err), gc.Equals, jem.ErrAPIConnection)
}
