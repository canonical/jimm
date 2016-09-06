// Copyright 2015 Canonical Ltd.

package jem_test

import (
	"fmt"
	"time"

	"github.com/juju/idmclient"
	"github.com/juju/idmclient/idmtest"
	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/api/controller"
	modelmanagerapi "github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	corejujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/multiwatcher"
	jujutesting "github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/bakery"

	"github.com/CanonicalLtd/jem/internal/apiconn"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type jemSuite struct {
	corejujutesting.JujuConnSuite
	idmSrv *idmtest.Server
	pool   *jem.Pool
	jem    *jem.JEM
}

var _ = gc.Suite(&jemSuite{})

func (s *jemSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.idmSrv = idmtest.NewServer()
	pool, err := jem.NewPool(jem.Params{
		DB: s.Session.DB("jem"),
		BakeryParams: bakery.NewServiceParams{
			Location: "here",
		},
		IDMClient: idmclient.New(idmclient.NewParams{
			BaseURL: s.idmSrv.URL.String(),
			Client:  s.idmSrv.Client("agent"),
		}),
		ControllerAdmin: "controller-admin",
	})
	c.Assert(err, gc.IsNil)
	s.pool = pool
	s.jem = s.pool.JEM()
}

func (s *jemSuite) TearDownTest(c *gc.C) {
	s.jem.Close()
	s.pool.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *jemSuite) TestPoolRequiresControllerAdmin(c *gc.C) {
	pool, err := jem.NewPool(jem.Params{
		DB: s.Session.DB("jem"),
		BakeryParams: bakery.NewServiceParams{
			Location: "here",
		},
		IDMClient: idmclient.New(idmclient.NewParams{
			BaseURL: s.idmSrv.URL.String(),
			Client:  s.idmSrv.Client("agent"),
		}),
	})
	c.Assert(err, gc.ErrorMatches, "no controller admin group specified")
	c.Assert(pool, gc.IsNil)
}

func (s *jemSuite) TestAddController(c *gc.C) {
	info := s.APIInfo(c)
	var addControllerTests = []struct {
		about            string
		authUser         params.User
		ctl              mongodoc.Controller
		expectError      string
		expectErrorCause error
	}{{
		about: "add controller",
		ctl: mongodoc.Controller{
			HostPorts:     info.Addrs,
			CACert:        info.CACert,
			AdminUser:     info.Tag.Id(),
			AdminPassword: info.Password,
			UUID:          info.ModelTag.Id(),
		},
	}, {
		about:    "add controller as part of group",
		authUser: "alice",
		ctl: mongodoc.Controller{
			Path: params.EntityPath{
				User: "beatles",
			},
			HostPorts:     info.Addrs,
			CACert:        info.CACert,
			AdminUser:     info.Tag.Id(),
			AdminPassword: info.Password,
			UUID:          info.ModelTag.Id(),
		},
	}, {
		about:    "add public controller",
		authUser: "controller-admin",
		ctl: mongodoc.Controller{
			HostPorts:     info.Addrs,
			CACert:        info.CACert,
			AdminUser:     info.Tag.Id(),
			AdminPassword: info.Password,
			UUID:          info.ModelTag.Id(),
			Public:        true,
		},
	}, {
		about:    "incorrect user",
		authUser: "alice",
		ctl: mongodoc.Controller{
			Path: params.EntityPath{
				User: "bob",
			},
			HostPorts:     info.Addrs,
			CACert:        info.CACert,
			AdminUser:     info.Tag.Id(),
			AdminPassword: info.Password,
			UUID:          info.ModelTag.Id(),
		},
		expectError:      "unauthorized",
		expectErrorCause: params.ErrUnauthorized,
	}, {
		about: "no hosts",
		ctl: mongodoc.Controller{
			CACert:        info.CACert,
			AdminUser:     info.Tag.Id(),
			AdminPassword: info.Password,
			UUID:          info.ModelTag.Id(),
		},
		expectError:      `no host-ports in request`,
		expectErrorCause: params.ErrBadRequest,
	}, {
		about: "no ca-cert",
		ctl: mongodoc.Controller{
			HostPorts:     info.Addrs,
			AdminUser:     info.Tag.Id(),
			AdminPassword: info.Password,
			UUID:          info.ModelTag.Id(),
		},
		expectError:      `no ca-cert in request`,
		expectErrorCause: params.ErrBadRequest,
	}, {
		about: "no user",
		ctl: mongodoc.Controller{
			HostPorts:     info.Addrs,
			CACert:        info.CACert,
			AdminPassword: info.Password,
			UUID:          info.ModelTag.Id(),
		},
		expectError:      `no user in request`,
		expectErrorCause: params.ErrBadRequest,
	}, {
		about: "no model uuid",
		ctl: mongodoc.Controller{
			HostPorts:     info.Addrs,
			CACert:        info.CACert,
			AdminUser:     info.Tag.Id(),
			AdminPassword: info.Password,
		},
		expectError:      `bad model UUID in request`,
		expectErrorCause: params.ErrBadRequest,
	}, {
		about: "public but no controller-admin access",
		ctl: mongodoc.Controller{
			HostPorts:     info.Addrs,
			CACert:        info.CACert,
			AdminUser:     info.Tag.Id(),
			AdminPassword: info.Password,
			UUID:          info.ModelTag.Id(),
			Public:        true,
		},
		expectError:      `admin access required to add public controllers`,
		expectErrorCause: params.ErrUnauthorized,
	}, {
		about: "cannot connect to environment",
		ctl: mongodoc.Controller{
			HostPorts:     []string{"0.1.2.3:1234"},
			CACert:        info.CACert,
			AdminUser:     info.Tag.Id(),
			AdminPassword: info.Password,
			UUID:          info.ModelTag.Id(),
		},
		expectError:      `cannot connect to controller: cannot connect to API: unable to connect to API: websocket.Dial wss://0.1.2.3:1234/api: dial tcp 0.1.2.3:1234: connect: invalid argument`,
		expectErrorCause: params.ErrBadRequest,
	}}
	s.idmSrv.AddUser("alice", "beatles")
	s.idmSrv.AddUser("bob", "beatles")
	for i, test := range addControllerTests {
		c.Logf("test %d: %s", i, test.about)
		if test.authUser == "" {
			test.authUser = "testuser"
		}
		if test.ctl.Path.User == "" {
			test.ctl.Path.User = test.authUser
		}
		if test.ctl.Path.Name == "" {
			test.ctl.Path.Name = params.Name(fmt.Sprintf("controller%d", i))
		}
		s.authenticate(string(test.authUser))
		err := s.jem.AddController(&test.ctl)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			if test.expectErrorCause != nil {
				c.Assert(errgo.Cause(err), gc.Equals, test.expectErrorCause)
			}
			continue
		}
		c.Assert(err, gc.IsNil)
		// The controller was added successfully. Check that we
		// can connect to it.
		conn, err := s.jem.OpenAPI(&test.ctl)
		c.Assert(err, gc.IsNil)
		conn.Close()
		// Clear the connection pool for the next test.
		s.pool.ClearAPIConnCache()
	}
}

func (s *jemSuite) TestDeleteController(c *gc.C) {
	ctlPath := params.EntityPath{"dalek", "who"}
	ctl := &mongodoc.Controller{
		Id:            "ignored",
		Path:          ctlPath,
		CACert:        "certainly",
		HostPorts:     []string{"host1:1234", "host2:9999"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	err := s.jem.DB.AddController(ctl)
	c.Assert(err, gc.IsNil)

	s.authenticate("who")
	err = s.jem.DeleteController(ctlPath)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)

	s.authenticate("dalek")
	err = s.jem.DeleteController(ctlPath)
	c.Assert(err, gc.IsNil)

	err = s.jem.DeleteController(ctlPath)
	c.Assert(err, gc.ErrorMatches, `controller "dalek/who" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *jemSuite) TestController(c *gc.C) {
	path := s.addController(c, params.EntityPath{"bob", "controller"}, false)

	ctl, err := s.jem.Controller(path)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	c.Assert(ctl, gc.IsNil)

	s.authenticate("bob")
	ctl, err = s.jem.Controller(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctl.Path, gc.Equals, path)
}

func (s *jemSuite) TestModel(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{"bob", "model"})

	m, err := s.jem.Model(model.Path)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	c.Assert(m, gc.IsNil)

	s.authenticate("bob")
	m, err = s.jem.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, jc.DeepEquals, model)
}

func (s *jemSuite) TestModelFromUUID(c *gc.C) {
	uuid := "99999999-9999-9999-9999-999999999999"
	path := params.EntityPath{"bob", "x"}
	m := &mongodoc.Model{
		Id:   "ignored",
		Path: path,
		UUID: uuid,
	}
	err := s.jem.DB.AddModel(m)
	c.Assert(err, gc.IsNil)
	c.Assert(m, jc.DeepEquals, &mongodoc.Model{
		Id:   "bob/x",
		Path: path,
		UUID: uuid,
	})

	_, err = s.jem.ModelFromUUID(uuid)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)

	s.authenticate("bob")
	m1, err := s.jem.ModelFromUUID(uuid)
	c.Assert(err, gc.IsNil)
	c.Assert(m1, jc.DeepEquals, m)

	m2, err := s.jem.ModelFromUUID("no-such-uuid")
	c.Assert(err, gc.ErrorMatches, `model "no-such-uuid" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(m2, gc.IsNil)
}

func (s *jemSuite) TestJEMCopiesSession(c *gc.C) {
	session := s.Session.Copy()
	pool, err := jem.NewPool(jem.Params{
		DB: session.DB("jem"),
		BakeryParams: bakery.NewServiceParams{
			Location: "here",
		},
		IDMClient: idmclient.New(idmclient.NewParams{
			BaseURL: s.idmSrv.URL.String(),
			Client:  s.idmSrv.Client("agent"),
		}),
		ControllerAdmin: "controller-admin",
	})
	c.Assert(err, gc.IsNil)

	jem := pool.JEM()
	defer jem.Close()
	// Check that we get an appropriate error when getting
	// a non-existent model, indicating that database
	// access is going OK.
	_, err = jem.DB.Model(params.EntityPath{"bob", "x"})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// Close the session and check that we still get the
	// same error.
	session.Close()

	_, err = jem.DB.Model(params.EntityPath{"bob", "x"})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// Also check the macaroon storage as that also has its own session reference.
	m, err := jem.Bakery.NewMacaroon("", nil, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.NotNil)
}

func (s *jemSuite) TestClone(c *gc.C) {
	j := s.jem.Clone()
	j.Close()
	_, err := s.jem.DB.Model(params.EntityPath{"bob", "x"})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *jemSuite) TestAddAndGetCredential(c *gc.C) {
	user := params.User("test-user")
	cloud := params.Cloud("test-cloud")
	name := params.Name("test-credential")
	expectId := fmt.Sprintf("%s/%s/%s", user, cloud, name)
	cred, err := s.jem.Credential(user, cloud, name)
	c.Assert(cred, gc.IsNil)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(err, gc.ErrorMatches, `credential "test-user/test-cloud/test-credential" not found`)

	attrs := map[string]string{
		"attr1": "val1",
		"attr2": "val2",
	}
	err = jem.UpdateCredential(s.jem.DB, &mongodoc.Credential{
		User:       user,
		Cloud:      cloud,
		Name:       name,
		Type:       "credtype",
		Label:      "Test Label",
		Attributes: attrs,
	})
	c.Assert(err, gc.IsNil)

	s.authenticate(string(user))
	cred, err = s.jem.Credential(user, cloud, name)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:         expectId,
		User:       user,
		Cloud:      cloud,
		Name:       name,
		Type:       "credtype",
		Label:      "Test Label",
		Attributes: attrs,
	})

	err = jem.UpdateCredential(s.jem.DB, &mongodoc.Credential{
		User:       user,
		Cloud:      cloud,
		Name:       name,
		Type:       "credtype",
		Label:      "Test Label 2",
		Attributes: attrs,
	})
	c.Assert(err, gc.IsNil)

	cred, err = s.jem.Credential(user, cloud, name)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:         expectId,
		User:       user,
		Cloud:      cloud,
		Name:       name,
		Type:       "credtype",
		Label:      "Test Label 2",
		Attributes: attrs,
	})

	s.authenticate("")
	cred, err = s.jem.Credential(user, cloud, name)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	c.Assert(cred, gc.IsNil)
}

func (s *jemSuite) TestDoControllers(c *gc.C) {
	testControllers := []mongodoc.Controller{{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-eu-west-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-us-east-1",
		},
		ACL: params.ACL{
			Read: []string{"bob-group"},
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-eu-west-1",
		},
		ACL: params.ACL{
			Read: []string{"bob"},
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-us-east-2",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "gce-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "gce-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}}
	for i := range testControllers {
		err := s.jem.DB.AddController(&testControllers[i])
		c.Assert(err, gc.IsNil)
	}
	s.authenticate("bob", "bob-group")
	for i, test := range doContollerTests {
		c.Logf("test %d. %s", i, test.about)
		var obtainedControllers []params.EntityPath
		err := s.jem.DoControllers(test.cloud, test.region, func(ctl *mongodoc.Controller) error {
			obtainedControllers = append(obtainedControllers, ctl.Path)
			return nil
		})
		c.Assert(err, gc.IsNil)
		c.Assert(obtainedControllers, jc.DeepEquals, test.expectControllers)
	}
}

func (s *jemSuite) TestDoControllersErrorResponse(c *gc.C) {
	testControllers := []mongodoc.Controller{{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-eu-west-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-us-east-1",
		},
		ACL: params.ACL{
			Read: []string{"bob-group"},
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-eu-west-1",
		},
		ACL: params.ACL{
			Read: []string{"bob"},
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-us-east-2",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "gce-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "gce-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}}
	for i := range testControllers {
		err := s.jem.DB.AddController(&testControllers[i])
		c.Assert(err, gc.IsNil)
	}
	s.authenticate("bob", "bob-group")
	testCause := errgo.New("test-cause")
	err := s.jem.DoControllers("", "", func(ctl *mongodoc.Controller) error {
		return errgo.WithCausef(nil, testCause, "test error")
	})
	c.Assert(errgo.Cause(err), gc.Equals, testCause)
}

var selectContollerTests = []struct {
	about            string
	cloud            params.Cloud
	region           string
	randIntn         func(int) int
	expectController params.EntityPath
	expectError      string
	expectErrorCause error
}{{
	about: "no parameters",
	randIntn: func(n int) int {
		return 4
	},
	expectController: params.EntityPath{
		User: "bob",
		Name: "gce-us-east-1",
	},
}, {
	about: "aws",
	cloud: "aws",
	randIntn: func(n int) int {
		return 1
	},
	expectController: params.EntityPath{
		User: "alice",
		Name: "aws-us-east-1",
	},
}, {
	about:  "aws-us-east-1",
	cloud:  "aws",
	region: "us-east-1",
	randIntn: func(n int) int {
		return 1
	},
	expectController: params.EntityPath{
		User: "bob",
		Name: "aws-us-east-1",
	},
}, {
	about:  "no match",
	cloud:  "aws",
	region: "us-east-2",
	randIntn: func(n int) int {
		return 1
	},
	expectError:      `no matching controllers found`,
	expectErrorCause: params.ErrNotFound,
}}

func (s *jemSuite) TestSelectController(c *gc.C) {
	var randIntn *func(int) int
	s.PatchValue(jem.RandIntn, func(n int) int {
		return (*randIntn)(n)
	})
	testControllers := []mongodoc.Controller{{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-eu-west-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-us-east-1",
		},
		ACL: params.ACL{
			Read: []string{"bob-group"},
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-eu-west-1",
		},
		ACL: params.ACL{
			Read: []string{"bob"},
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-us-east-2",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "gce-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "gce-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}}
	for i := range testControllers {
		err := s.jem.DB.AddController(&testControllers[i])
		c.Assert(err, gc.IsNil)
	}
	s.authenticate("bob", "bob-group")
	for i, test := range selectContollerTests {
		c.Logf("test %d. %s", i, test.about)
		randIntn = &test.randIntn
		ctl, err := jem.SelectController(s.jem, test.cloud, test.region)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			if test.expectErrorCause != nil {
				c.Assert(errgo.Cause(err), gc.Equals, test.expectErrorCause)
			}
			continue
		}
		c.Assert(err, gc.IsNil)
		c.Assert(ctl.Path, jc.DeepEquals, test.expectController)
	}
}

var createModelTests = []struct {
	about            string
	params           jem.CreateModelParams
	expectError      string
	expectErrorCause error
}{{
	about: "success",
	params: jem.CreateModelParams{
		Path:           params.EntityPath{"bob", ""},
		ControllerPath: params.EntityPath{"bob", "controller"},
		Credential:     "cred1",
		Cloud:          "dummy",
	},
}, {
	about: "unknown credential",
	params: jem.CreateModelParams{
		Path:           params.EntityPath{"bob", ""},
		ControllerPath: params.EntityPath{"bob", "controller"},
		Credential:     "cred2",
		Cloud:          "dummy",
	},
	expectError:      `credential "bob/dummy/cred2" not found`,
	expectErrorCause: params.ErrNotFound,
}, {
	about: "model exists",
	params: jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "existing"},
		ControllerPath: params.EntityPath{"bob", "controller"},
		Credential:     "cred1",
		Cloud:          "dummy",
	},
	expectError:      `already exists`,
	expectErrorCause: params.ErrAlreadyExists,
}, {
	about: "unrecognised region",
	params: jem.CreateModelParams{
		Path:           params.EntityPath{"bob", ""},
		ControllerPath: params.EntityPath{"bob", "controller"},
		Credential:     "cred1",
		Cloud:          "dummy",
		Region:         "not-a-region",
	},
	expectError: `cannot create model: getting cloud region definition: region "not-a-region" not found \(expected one of \["dummy-region"\]\) \(not found\)`,
}, {
	about: "with region",
	params: jem.CreateModelParams{
		Path:           params.EntityPath{"bob", ""},
		ControllerPath: params.EntityPath{"bob", "controller"},
		Credential:     "cred1",
		Cloud:          "dummy",
		Region:         "dummy-region",
	},
}, {
	about: "unauthorized",
	params: jem.CreateModelParams{
		Path:           params.EntityPath{"alice", ""},
		ControllerPath: params.EntityPath{"bob", "controller"},
		Credential:     "cred1",
		Cloud:          "dummy",
		Region:         "dummy-region",
	},
	expectError:      `unauthorized`,
	expectErrorCause: params.ErrUnauthorized,
}, {
	about: "choose controller",
	params: jem.CreateModelParams{
		Path:       params.EntityPath{"bob", ""},
		Credential: "cred1",
		Cloud:      "dummy",
	},
}, {
	about: "choose controller - not found",
	params: jem.CreateModelParams{
		Path:       params.EntityPath{"bob", ""},
		Credential: "cred1",
		Cloud:      "dummy",
		Region:     "dummy-region-2",
	},
	expectError:      `no matching controllers found`,
	expectErrorCause: params.ErrNotFound,
}, {
	about: "unknown controller",
	params: jem.CreateModelParams{
		Path:           params.EntityPath{"bob", ""},
		ControllerPath: params.EntityPath{"bob", "no-such-controller"},
		Credential:     "cred1",
		Cloud:          "dummy",
	},
	expectError:      `controller "bob/no-such-controller" not found`,
	expectErrorCause: params.ErrNotFound,
}}

func (s *jemSuite) TestCreateModel(c *gc.C) {
	s.addController(c, params.EntityPath{"bob", "controller"}, true)
	err := jem.UpdateCredential(s.jem.DB, &mongodoc.Credential{
		User:  "bob",
		Cloud: "dummy",
		Name:  "cred1",
		Type:  "empty",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.authenticate("bob")
	_, _, err = s.jem.CreateModel(jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "existing"},
		ControllerPath: params.EntityPath{"bob", "controller"},
		Credential:     "cred1",
		Cloud:          "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)

	for i, test := range createModelTests {
		c.Logf("test %d. %s", i, test.about)
		if test.params.Path.Name == "" {
			test.params.Path.Name = params.Name(fmt.Sprintf("test-%d", i))
		}
		m, _, err := s.jem.CreateModel(test.params)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			if test.expectErrorCause != nil {
				c.Assert(errgo.Cause(err), gc.Equals, test.expectErrorCause)
			}
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.Path, jc.DeepEquals, test.params.Path)
		c.Assert(m.UUID, gc.Not(gc.Equals), "")
	}
}

func (s *jemSuite) TestGrantModel(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	s.authenticate("bob")
	err := s.jem.GrantModel(model.Path, "alice", "write")
	c.Assert(err, jc.ErrorIsNil)
	model1, err := s.jem.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{"alice"}})
}

func (s *jemSuite) TestGrantModelNotOwner(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	s.authenticate("alice")
	err := s.jem.GrantModel(model.Path, "alice", "write")
	c.Assert(err, gc.ErrorMatches, "unauthorized")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	model1, err := s.jem.DB.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{}})
}

func (s *jemSuite) TestGrantModelControllerFailure(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	s.authenticate("bob")
	err := s.jem.GrantModel(model.Path, "alice", "superpowers")
	c.Assert(err, gc.ErrorMatches, `invalid model access permission "superpowers"`)
	model1, err := s.jem.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{}})
}

func (s *jemSuite) TestRevokeModel(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	s.authenticate("bob")
	err := s.jem.GrantModel(model.Path, "alice", "write")
	c.Assert(err, jc.ErrorIsNil)
	model1, err := s.jem.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{"alice"}})
	err = s.jem.RevokeModel(model.Path, "alice", "write")
	c.Assert(err, jc.ErrorIsNil)
	model1, err = s.jem.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{}})
}

func (s *jemSuite) TestRevokeModelNotOwner(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	s.authenticate("bob")
	err := s.jem.GrantModel(model.Path, "alice", "write")
	c.Assert(err, jc.ErrorIsNil)
	model1, err := s.jem.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{"alice"}})
	s.authenticate("alice")
	err = s.jem.RevokeModel(model.Path, "alice", "write")
	c.Assert(err, gc.ErrorMatches, "unauthorized")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	model1, err = s.jem.DB.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{"alice"}})
}

func (s *jemSuite) TestRevokeModelControllerFailure(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	s.authenticate("bob")
	err := s.jem.GrantModel(model.Path, "alice", "write")
	c.Assert(err, jc.ErrorIsNil)
	model1, err := s.jem.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{"alice"}})
	err = s.jem.RevokeModel(model.Path, "alice", "superpowers")
	c.Assert(err, gc.ErrorMatches, `invalid model access permission "superpowers"`)
	model1, err = s.jem.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{}})
}

func (s *jemSuite) TestDestroyModel(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	s.authenticate("bob")
	conn, err := jem.OpenAPIPath(s.jem, model.Controller)
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()

	// Sanity check the model exists
	client := modelmanagerapi.NewClient(conn)
	models, err := client.ListModels("bob@external")
	c.Assert(err, jc.ErrorIsNil)
	var found bool
	for _, m := range models {
		if m.UUID == model.UUID {
			c.Logf("found %#v", m)
			found = true
			break
		}
	}
	c.Assert(found, gc.Equals, true)

	ch := waitForDestruction(conn, c, model.UUID)

	s.authenticate("alice")
	err = s.jem.DestroyModel(model.Path)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)

	s.authenticate("bob")
	err = s.jem.DestroyModel(model.Path)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-ch:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("model not destroyed")
	}

	// Check the model is removed.
	_, err = s.jem.Model(model.Path)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// Check that it cannot be destroyed twice
	err = s.jem.DestroyModel(model.Path)
	c.Assert(err, gc.ErrorMatches, `model "bob/model" not found`)

	// Put the model back in the database
	err = s.jem.DB.AddModel(model)
	c.Assert(err, jc.ErrorIsNil)

	// Check that it can still be removed even if the contoller has no model.
	err = s.jem.DestroyModel(model.Path)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the model is removed.
	_, err = s.jem.Model(model.Path)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func waitForDestruction(conn *apiconn.Conn, c *gc.C, uuid string) <-chan struct{} {
	ch := make(chan struct{})
	watcher, err := controller.NewClient(conn).WatchAllModels()
	go func() {
		defer close(ch)
		if !c.Check(err, jc.ErrorIsNil) {
			return
		}
		for {
			deltas, err := watcher.Next()
			if !c.Check(err, jc.ErrorIsNil) {
				return
			}
			for _, d := range deltas {
				d, ok := d.Entity.(*multiwatcher.ModelInfo)
				if ok && d.ModelUUID == uuid && d.Life == "dead" {
					return
				}
			}
		}
	}()
	return ch
}

func (s *jemSuite) TestUpdateCredential(c *gc.C) {
	ctlPath := s.addController(c, params.EntityPath{User: "bob", Name: "controller"}, false)
	cred := &mongodoc.Credential{
		User:  "bob",
		Cloud: "dummy",
		Name:  "cred",
		Type:  "empty",
	}

	err := s.jem.UpdateCredential(cred)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)

	s.authenticate("bob")
	err = s.jem.UpdateCredential(cred)
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.jem.CreateModel(jem.CreateModelParams{
		Path:           params.EntityPath{User: "bob", Name: "model"},
		ControllerPath: ctlPath,
		Cloud:          cred.Cloud,
		Credential:     cred.Name,
	})
	c.Assert(err, jc.ErrorIsNil)

	conn, err := jem.OpenAPIPath(s.jem, ctlPath)
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()

	// Check it was deployed
	client := cloudapi.NewClient(conn)
	credTag := names.NewCloudCredentialTag("dummy/bob@external/cred")
	creds, err := client.Credentials(credTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType: "empty",
		},
	}})

	err = s.jem.UpdateCredential(&mongodoc.Credential{
		User:  "bob",
		Cloud: "dummy",
		Name:  "cred",
		Type:  "userpass",
		Attributes: map[string]string{
			"username": "cloud-user",
			"password": "cloud-pass",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	// check it was updated on the controller.
	creds, err = client.Credentials(credTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType: "userpass",
			Attributes: map[string]string{
				"username": "cloud-user",
			},
			Redacted: []string{
				"password",
			},
		},
	}})
}

func (s *jemSuite) addController(c *gc.C, path params.EntityPath, public bool) params.EntityPath {
	info := s.APIInfo(c)
	ctl := &mongodoc.Controller{
		Path:          path,
		HostPorts:     info.Addrs,
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
		Public:        public,
		UUID:          "fake-uuid",
	}
	// Sanity check that we're really talking to the controller.
	minfo, err := s.APIState.Client().ModelInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(minfo.UUID, gc.Equals, s.ControllerConfig.ControllerUUID())

	s.authenticate(string(path.User), "controller-admin")
	err = s.jem.AddController(ctl)
	c.Assert(err, jc.ErrorIsNil)
	s.authenticate("")
	return path
}

func (s *jemSuite) bootstrapModel(c *gc.C, path params.EntityPath) *mongodoc.Model {
	ctlPath := s.addController(c, params.EntityPath{User: path.User, Name: "controller"}, false)
	err := jem.UpdateCredential(s.jem.DB, &mongodoc.Credential{
		User:  path.User,
		Cloud: "dummy",
		Name:  "cred",
		Type:  "empty",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.authenticate(string(path.User))
	model, _, err := s.jem.CreateModel(jem.CreateModelParams{
		Path:           path,
		ControllerPath: ctlPath,
		Credential:     "cred",
		Cloud:          "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.authenticate("")
	return model
}

func (s *jemSuite) authenticate(username string, groups ...string) {
	s.idmSrv.AddUser(username, groups...)
	s.jem.Auth = jem.Authorization{Username: username}
}
