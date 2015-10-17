// Copyright 2015 Canonical Ltd.

package jem_test

import (
	"github.com/CanonicalLtd/blues-identity/idmclient"
	"github.com/juju/schema"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"

	"github.com/CanonicalLtd/jem/internal/idmtest"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type jemSuite struct {
	jujutesting.IsolatedMgoSuite
	idmSrv *idmtest.Server
	pool   *jem.Pool
	store  *jem.JEM
}

var _ = gc.Suite(&jemSuite{})

func (s *jemSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	s.idmSrv = idmtest.NewServer()
	pool, err := jem.NewPool(
		s.Session.DB("jem"),
		bakery.NewServiceParams{
			Location: "here",
		},
		idmclient.New(idmclient.NewParams{
			BaseURL: s.idmSrv.URL.String(),
			Client:  s.idmSrv.Client("agent"),
		}),
	)
	c.Assert(err, gc.IsNil)
	s.pool = pool
	s.store = s.pool.JEM()
}

func (s *jemSuite) TearDownTest(c *gc.C) {
	s.store.Close()
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *jemSuite) TestAddStateServer(c *gc.C) {
	srvPath := params.EntityPath{"bob", "x"}
	srv := &mongodoc.StateServer{
		Id:        "ignored",
		Path:      srvPath,
		CACert:    "certainly",
		HostPorts: []string{"host1:1234", "host2:9999"},
	}
	env := &mongodoc.Environment{
		Id:            "ignored",
		Path:          params.EntityPath{"ignored-user", "ignored-name"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	err := s.store.AddStateServer(srv, env)
	c.Assert(err, gc.IsNil)

	// Check that the fields have been mutated as expected.
	c.Assert(srv, jc.DeepEquals, &mongodoc.StateServer{
		Id:        "bob/x",
		Path:      srvPath,
		CACert:    "certainly",
		HostPorts: []string{"host1:1234", "host2:9999"},
	})
	c.Assert(env, jc.DeepEquals, &mongodoc.Environment{
		Id:            "bob/x",
		Path:          srvPath,
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
		StateServer:   srvPath,
	})

	srv1, err := s.store.StateServer(srvPath)
	c.Assert(err, gc.IsNil)
	c.Assert(srv1, jc.DeepEquals, &mongodoc.StateServer{
		Id:        "bob/x",
		Path:      srvPath,
		CACert:    "certainly",
		HostPorts: []string{"host1:1234", "host2:9999"},
	})
	env1, err := s.store.Environment(srvPath)
	c.Assert(err, gc.IsNil)
	c.Assert(env1, jc.DeepEquals, env)

	err = s.store.AddStateServer(srv, env)
	c.Assert(err, gc.ErrorMatches, "already exists")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)

	srvPath2 := params.EntityPath{"bob", "y"}
	srv2 := &mongodoc.StateServer{
		Id:        "ignored",
		Path:      srvPath2,
		CACert:    "certainly",
		HostPorts: []string{"host1:1234", "host2:9999"},
	}
	env2 := &mongodoc.Environment{
		Id:            "bob/noty",
		Path:          params.EntityPath{"ignored-user", "ignored-name"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	err = s.store.AddStateServer(srv2, env2)
	c.Assert(err, gc.IsNil)
	env3, err := s.store.Environment(srvPath2)
	c.Assert(err, gc.IsNil)
	c.Assert(env3, jc.DeepEquals, env2)
}

func (s *jemSuite) TestDeleteStateServer(c *gc.C) {
	srvPath := params.EntityPath{"dalek", "who"}
	srv := &mongodoc.StateServer{
		Id:        "ignored",
		Path:      srvPath,
		CACert:    "certainly",
		HostPorts: []string{"host1:1234", "host2:9999"},
	}
	env := &mongodoc.Environment{
		Id:            "dalek/who",
		Path:          params.EntityPath{"ignored", "ignored"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	err := s.store.AddStateServer(srv, env)
	c.Assert(err, gc.IsNil)
	err = s.store.DeleteStateServer(srv)
	c.Assert(err, gc.IsNil)

	srv1, err := s.store.StateServer(srvPath)
	c.Assert(srv1, gc.IsNil)
	env1, err := s.store.Environment(srvPath)
	c.Assert(env1, gc.IsNil)

	err = s.store.DeleteStateServer(srv)
	c.Assert(err, gc.ErrorMatches, "state server \"dalek/who\" not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// Test with non-existing environment.
	srv2 := &mongodoc.StateServer{
		Id:        "dalek/who",
		Path:      srvPath,
		CACert:    "certainly",
		HostPorts: []string{"host1:1234", "host2:9999"},
	}
	env2 := &mongodoc.Environment{
		Id:            "dalek/exterminated",
		Path:          params.EntityPath{"ignored", "ignored"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	err = s.store.AddStateServer(srv2, env2)
	c.Assert(err, gc.IsNil)

	err = s.store.DeleteStateServer(srv2)
	c.Assert(err, gc.IsNil)
	srv3, err := s.store.StateServer(srvPath)
	c.Assert(srv3, gc.IsNil)
	env3, err := s.store.Environment(srvPath)
	c.Assert(env3, gc.IsNil)
}

func (s *jemSuite) TestDeleteEnvironemtn(c *gc.C) {
	srvPath := params.EntityPath{"dalek", "who"}
	srv := &mongodoc.StateServer{
		Id:        "ignored",
		Path:      srvPath,
		CACert:    "certainly",
		HostPorts: []string{"host1:1234", "host2:9999"},
	}
	env := &mongodoc.Environment{
		Id:            "dalek/who",
		Path:          params.EntityPath{"ignored", "ignored"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	err := s.store.AddStateServer(srv, env)
	c.Assert(err, gc.IsNil)

	err = s.store.DeleteEnvironment(env)
	c.Assert(err, gc.ErrorMatches, "environment \"dalek/who\" is a state server: failed")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrBadRequest)

	envPath := params.EntityPath{"dalek", "exterminate"}
	env2 := &mongodoc.Environment{
		Id:            "dalek/exterminate",
		Path:          envPath,
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	err = s.store.AddEnvironment(env2)
	c.Assert(err, gc.IsNil)

	err = s.store.DeleteEnvironment(env2)
	c.Assert(err, gc.IsNil)
	env3, err := s.store.Environment(envPath)
	c.Assert(env3, gc.IsNil)

	err = s.store.DeleteEnvironment(env2)
	c.Assert(err, gc.ErrorMatches, "environment \"dalek/exterminate\" not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *jemSuite) TestAddEnvironment(c *gc.C) {
	srvPath := params.EntityPath{"bob", "x"}
	env := &mongodoc.Environment{
		Id:            "ignored",
		Path:          srvPath,
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	err := s.store.AddEnvironment(env)
	c.Assert(err, gc.IsNil)
	c.Assert(env, jc.DeepEquals, &mongodoc.Environment{
		Id:            "bob/x",
		Path:          srvPath,
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	})

	env1, err := s.store.Environment(srvPath)
	c.Assert(err, gc.IsNil)
	c.Assert(env1, jc.DeepEquals, env)

	err = s.store.AddEnvironment(env)
	c.Assert(err, gc.ErrorMatches, "already exists")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)
}

func (s *jemSuite) TestAddTemplate(c *gc.C) {
	path := params.EntityPath{"bob", "x"}
	tmpl := &mongodoc.Template{
		Id:   "ignored",
		Path: path,
		Schema: environschema.Fields{
			"name": {
				Description: "name of environment",
				Type:        environschema.Tstring,
				Mandatory:   true,
				Values:      []interface{}{"venus", "pluto"},
			},
			"temperature": {
				Description: "temperature of environment",
				Type:        environschema.Tint,
				Example:     400,
				Values:      []interface{}{-400, 864.0},
			},
		},
		Config: map[string]interface{}{
			"name":        "pluto",
			"temperature": -400.0,
		},
	}
	err := s.store.AddTemplate(tmpl)
	c.Assert(err, gc.IsNil)
	c.Assert(tmpl.Id, gc.Equals, "bob/x")

	tmpl1, err := s.store.Template(path)
	c.Assert(err, gc.IsNil)
	c.Assert(tmpl1, jc.DeepEquals, tmpl)

	// Ensure that the schema still works even though some
	// values may have been transformed to float64 by the
	// JSON unmarshaler.
	fields, defaults, err := tmpl.Schema.ValidationSchema()
	c.Assert(err, gc.IsNil)
	config, err := schema.FieldMap(fields, defaults).Coerce(tmpl.Config, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(config, jc.DeepEquals, map[string]interface{}{
		"name":        "pluto",
		"temperature": -400,
	})
}

func (s *jemSuite) TestDeleteTemplate(c *gc.C) {
	path := params.EntityPath{"bob", "x"}
	tmpl := &mongodoc.Template{
		Id:   "ignored",
		Path: path,
		Schema: environschema.Fields{
			"name": {
				Description: "name of environment",
				Type:        environschema.Tstring,
				Mandatory:   true,
				Values:      []interface{}{"venus", "pluto"},
			},
			"temperature": {
				Description: "temperature of environment",
				Type:        environschema.Tint,
				Example:     400,
				Values:      []interface{}{-400, 864.0},
			},
		},
		Config: map[string]interface{}{
			"name":        "pluto",
			"temperature": -400.0,
		},
	}
	err := s.store.AddTemplate(tmpl)
	c.Assert(err, gc.IsNil)

	err = s.store.DeleteTemplate(tmpl)
	c.Assert(err, gc.IsNil)
	tmpl1, err := s.store.Template(path)
	c.Assert(tmpl1, gc.IsNil)

	err = s.store.DeleteTemplate(tmpl)
	c.Assert(err, gc.ErrorMatches, "template \"bob/x\" not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *jemSuite) TestSessionIsCopied(c *gc.C) {
	session := s.Session.Copy()
	pool, err := jem.NewPool(
		session.DB("jem"),
		bakery.NewServiceParams{
			Location: "here",
		},
		idmclient.New(idmclient.NewParams{
			BaseURL: s.idmSrv.URL.String(),
			Client:  s.idmSrv.Client("agent"),
		}),
	)
	c.Assert(err, gc.IsNil)

	store := pool.JEM()
	defer store.Close()
	// Check that we get an appropriate error when getting
	// a non-existent environment, indicating that database
	// access is going OK.
	_, err = store.Environment(params.EntityPath{"bob", "x"})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// Close the session and check that we still get the
	// same error.
	session.Close()

	_, err = store.Environment(params.EntityPath{"bob", "x"})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// Also check the macaroon storage as that also has its own session reference.
	m, err := store.Bakery.NewMacaroon("", nil, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.NotNil)
}
