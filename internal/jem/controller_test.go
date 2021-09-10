// Copyright 2015 Canonical Ltd.

package jem_test

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	cloudapi "github.com/juju/juju/api/cloud"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jem/jimmdb"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

type addControllerSuite struct {
	jemtest.JEMSuite
}

var _ = gc.Suite(&addControllerSuite{})

var addControllerTests = []struct {
	about            string
	id               identchecker.ACLIdentity
	ctl              mongodoc.Controller
	expectError      string
	expectErrorCause error
}{{
	about: "success",
	id:    jemtest.Alice,
	ctl: mongodoc.Controller{
		Path:   params.EntityPath{"alice", "controller-1"},
		Public: true,
	},
}, {
	// This test is dependent on the previous one succeeding.
	about: "duplicate",
	id:    jemtest.Alice,
	ctl: mongodoc.Controller{
		Path:   params.EntityPath{"alice", "controller-1"},
		Public: true,
	},
	expectError:      `already exists`,
	expectErrorCause: params.ErrAlreadyExists,
}, {
	about: "unauthorized",
	id:    jemtest.Bob,
	ctl: mongodoc.Controller{
		Path:   params.EntityPath{"bob", "controller-1"},
		Public: true,
	},
	expectError:      `unauthorized`,
	expectErrorCause: params.ErrUnauthorized,
}, {
	about: "not public",
	id:    jemtest.Alice,
	ctl: mongodoc.Controller{
		Path:   params.EntityPath{"alice", "controller-2"},
		Public: false,
	},
	expectError:      `cannot add private controller`,
	expectErrorCause: params.ErrForbidden,
}, {
	about: "wrong namespace",
	id:    jemtest.Alice,
	ctl: mongodoc.Controller{
		Path:   params.EntityPath{"bob", "controller-1"},
		Public: true,
	},
	expectError:      `unauthorized`,
	expectErrorCause: params.ErrUnauthorized,
}, {
	about: "connect failure",
	id:    jemtest.Alice,
	ctl: mongodoc.Controller{
		Path:          params.EntityPath{"alice", "controller-2"},
		AdminPassword: "not-the-password",
		Public:        true,
	},
	expectError:      `invalid entity name or password \(unauthorized access\)`,
	expectErrorCause: jem.ErrAPIConnection,
}}

func (s *addControllerSuite) TestAddController(c *gc.C) {
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

		err := s.JEM.AddController(ctx, test.id, &test.ctl)
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

type controllerSuite struct {
	jemtest.BootstrapSuite
}

var _ = gc.Suite(&controllerSuite{})

var getControllerTests = []struct {
	path             params.EntityPath
	expectErrorCause error
}{{
	path: params.EntityPath{"bob", "controller"},
}, {
	path: params.EntityPath{"bob-group", "controller"},
}, {
	path:             params.EntityPath{"alice", "controller"},
	expectErrorCause: params.ErrUnauthorized,
}, {
	path:             params.EntityPath{"bob", "controller2"},
	expectErrorCause: params.ErrNotFound,
}, {
	path:             params.EntityPath{"bob-group", "controller2"},
	expectErrorCause: params.ErrNotFound,
}, {
	path:             params.EntityPath{"alice", "controller2"},
	expectErrorCause: params.ErrNotFound,
}}

func (s *controllerSuite) TestGetController(c *gc.C) {
	s.AddController(c, &mongodoc.Controller{
		Path: params.EntityPath{"alice", "controller"},
	})
	s.AddController(c, &mongodoc.Controller{
		Path: params.EntityPath{"bob", "controller"},
	})
	s.AddController(c, &mongodoc.Controller{
		Path: params.EntityPath{"bob-group", "controller"},
	})

	for i, test := range getControllerTests {
		c.Logf("test %d. %s", i, test.path)
		ctl := mongodoc.Controller{Path: test.path}
		err := s.JEM.GetController(testContext, jemtest.Bob, &ctl)
		if test.expectErrorCause != nil {
			c.Assert(errgo.Cause(err), gc.Equals, test.expectErrorCause)
			continue
		}
		c.Assert(err, gc.Equals, nil)
		c.Assert(ctl.Id, gc.Equals, test.path.String())
	}
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

func (s *controllerSuite) TestSetControllerDeprecated(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}

	err := s.JEM.SetControllerDeprecated(testContext, jemtest.Alice, ctlPath, true)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	err = s.JEM.SetControllerDeprecated(testContext, jemtest.Alice, ctlPath, false)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// Set the controller to deprecated and check that the field
	// is set to true.
	err = s.JEM.SetControllerDeprecated(testContext, jemtest.Alice, s.Controller.Path, true)
	c.Assert(err, gc.Equals, nil)

	ctl := &mongodoc.Controller{
		Path: s.Controller.Path,
	}
	err = s.JEM.DB.GetController(testContext, ctl)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ctl.Deprecated, gc.Equals, true)

	// Set it back to non-deprecated and check that the field is removed.
	err = s.JEM.SetControllerDeprecated(testContext, jemtest.Alice, s.Controller.Path, false)
	c.Assert(err, gc.Equals, nil)

	ctl2 := &mongodoc.Controller{
		Path: s.Controller.Path,
	}
	err = s.JEM.DB.GetController(testContext, ctl2)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ctl2.Deprecated, gc.Equals, false)
}

func (s *controllerSuite) TestDeleteController(c *gc.C) {
	// sanity check the credential is linked to the controller
	err := s.JEM.DB.GetCredential(testContext, &s.Credential)
	c.Assert(err, gc.Equals, nil)
	c.Assert(s.Credential.Controllers, jc.DeepEquals, []params.EntityPath{s.Controller.Path})

	// Attempt to delete the controller while it is still running.
	err = s.JEM.DeleteController(testContext, jemtest.Alice, &s.Controller, false)
	c.Assert(err, gc.ErrorMatches, `cannot delete controller while it is still alive`)

	// Use force.
	err = s.JEM.DeleteController(testContext, jemtest.Alice, &s.Controller, true)
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.DB.GetController(testContext, &s.Controller)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	err = s.JEM.DB.GetModel(testContext, &s.Model)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	err = s.JEM.DB.GetCredential(testContext, &s.Credential)
	c.Assert(err, gc.Equals, nil)
	c.Assert(s.Credential.Controllers, gc.HasLen, 0)

	err = s.JEM.DeleteController(testContext, jemtest.Alice, &s.Controller, false)
	c.Assert(err, gc.ErrorMatches, "controller not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *controllerSuite) TestControllerUpdateCredentials(c *gc.C) {
	cred := jemtest.EmptyCredential("bob", "test")
	err := s.JEM.DB.UpsertCredential(testContext, &cred)
	c.Assert(err, gc.Equals, nil)

	err = jem.SetCredentialUpdates(s.JEM, testContext, []params.EntityPath{s.Controller.Path}, cred.Path)
	c.Assert(err, gc.Equals, nil)

	ctl := &mongodoc.Controller{Path: s.Controller.Path}
	err = s.JEM.DB.GetController(testContext, ctl)
	c.Assert(err, gc.Equals, nil)
	c.Logf("updatecredentials: %#v", ctl.UpdateCredentials)

	conn, err := s.JEM.OpenAPIFromDoc(testContext, &s.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()

	jem.ControllerUpdateCredentials(s.JEM, testContext, conn, ctl)

	// check it was updated on the controller.
	client := cloudapi.NewClient(conn)
	creds, err := client.Credentials(conv.ToCloudCredentialTag(cred.Path))
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
	// create a credential.
	cred := jemtest.EmptyCredential("bob", "test")
	err := s.JEM.DB.UpsertCredential(testContext, &cred)
	c.Assert(err, gc.Equals, nil)
	err = jem.SetCredentialUpdates(s.JEM, testContext, []params.EntityPath{s.Controller.Path}, cred.Path)
	c.Assert(err, gc.Equals, nil)

	// Remove the controller from known clouds.
	_, err = s.JEM.DB.CloudRegions().UpdateAll(
		jimmdb.Eq("cloud", jemtest.TestCloudName),
		new(jimmdb.Update).Pull("primarycontrollers", s.Controller.Path).Pull("secondarycontrollers", s.Controller.Path),
	)
	c.Assert(err, gc.Equals, nil)
	cr := mongodoc.CloudRegion{
		Cloud: jemtest.TestCloudName,
	}
	err = s.JEM.DB.GetCloudRegion(testContext, &cr)
	c.Assert(err, gc.Equals, nil)
	c.Assert(cr.PrimaryControllers, gc.HasLen, 0)

	// Set the version obviously wrong.
	err = s.JEM.SetControllerVersion(testContext, s.Controller.Path, version.Zero)
	c.Assert(err, gc.Equals, nil)

	conn, err := s.JEM.ConnectMonitor(testContext, s.Controller.Path)
	c.Assert(err, gc.Equals, nil)

	v, ok := conn.ServerVersion()
	c.Assert(ok, gc.Equals, true)

	// check the credential has been updated.
	client := cloudapi.NewClient(conn)
	creds, err := client.Credentials(conv.ToCloudCredentialTag(cred.Path))
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
	err = s.JEM.DB.GetCloudRegion(testContext, &cr)
	c.Assert(err, gc.Equals, nil)
	c.Assert(cr.PrimaryControllers, jc.DeepEquals, []params.EntityPath{s.Controller.Path})

	// Check the version has been updated.
	ctl := mongodoc.Controller{Path: s.Controller.Path}
	err = s.JEM.DB.GetController(testContext, &ctl)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ctl.Version, jc.DeepEquals, &v)
}

func (s *controllerSuite) TestConnectMonitorNotFound(c *gc.C) {
	_, err := s.JEM.ConnectMonitor(testContext, params.EntityPath{"not", "there"})
	c.Check(err, gc.ErrorMatches, `controller not found`)
	c.Check(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *controllerSuite) TestConnectMonitorConnectionFailure(c *gc.C) {
	// Set the password wrong
	err := s.JEM.DB.UpdateController(testContext, &s.Controller, new(jimmdb.Update).Set("adminpassword", "bad-password"), false)
	s.Pool.ClearAPIConnCache()

	_, err = s.JEM.ConnectMonitor(testContext, s.Controller.Path)
	c.Check(err, gc.ErrorMatches, `invalid entity name or password \(unauthorized access\)`)
	c.Check(errgo.Cause(err), gc.Equals, jem.ErrAPIConnection)
}

func (s *controllerSuite) TestUpdateMigratedModel(c *gc.C) {
	c2 := mongodoc.Controller{Path: params.EntityPath{User: jemtest.ControllerAdmin, Name: "controller-2"}}
	s.AddController(c, &c2)

	err := s.JEM.DB.UpdateModel(testContext, &s.Model, new(jimmdb.Update).Set("controller", c2.Path), true)
	c.Assert(err, gc.Equals, nil)
	c.Check(s.Model.Controller, jc.DeepEquals, c2.Path)

	// bob is unauthorized
	err = s.JEM.UpdateMigratedModel(testContext, jemtest.Bob, names.NewModelTag(s.Model.UUID), s.Controller.Path.Name)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)

	err = s.JEM.UpdateMigratedModel(testContext, jemtest.Alice, names.NewModelTag(s.Model.UUID), s.Controller.Path.Name)
	c.Assert(err, gc.Equals, nil)

	model := mongodoc.Model{
		Path: s.Model.Path,
	}
	err = s.JEM.DB.GetModel(testContext, &model)
	c.Assert(err, gc.Equals, nil)
	c.Check(model.Controller, jc.DeepEquals, s.Controller.Path)
}
