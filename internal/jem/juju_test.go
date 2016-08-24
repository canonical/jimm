// Copyright 2016 Canonical Ltd.

package jem_test

import (
	"fmt"
	"time"

	"github.com/juju/idmclient"
	"github.com/juju/idmclient/idmtest"
	"github.com/juju/juju/api/controller"
	modelmanagerapi "github.com/juju/juju/api/modelmanager"
	corejujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/multiwatcher"
	jujutesting "github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"

	"github.com/CanonicalLtd/jem/internal/apiconn"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type jujuSuite struct {
	corejujutesting.JujuConnSuite
	idmSrv *idmtest.Server
	pool   *jem.Pool
	store  *jem.JEM
}

var _ = gc.Suite(&jujuSuite{})

func (s *jujuSuite) SetUpTest(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	s.pool = pool
	s.store = s.pool.JEM()
	s.PatchValue(&jem.APIOpenTimeout, time.Duration(0))
}

func (s *jujuSuite) TearDownTest(c *gc.C) {
	s.store.Close()
	s.pool.Close()
	s.JujuConnSuite.TearDownTest(c)
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
		Path:           params.EntityPath{"bob", "controller"},
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
}}

func (s *jujuSuite) TestCreateModel(c *gc.C) {
	ctlId := s.addController(c, params.EntityPath{"bob", "controller"})
	err := s.store.UpdateCredential(&mongodoc.Credential{
		User:  "bob",
		Cloud: "dummy",
		Name:  "cred1",
		Type:  "empty",
	})
	c.Assert(err, jc.ErrorIsNil)
	conn, err := s.store.OpenAPI(ctlId)
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()
	for i, test := range createModelTests {
		c.Logf("test %d. %s", i, test.about)
		if test.params.Path.Name == "" {
			test.params.Path.Name = params.Name(fmt.Sprintf("test-%d", i))
		}
		m, _, err := s.store.CreateModel(conn, test.params)
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

func (s *jujuSuite) TestGrantModel(c *gc.C) {
	conn, model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	defer conn.Close()
	err := s.store.GrantModel(conn, model, "alice", "write")
	c.Assert(err, jc.ErrorIsNil)
	model1, err := s.store.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{"alice"}})
}

func (s *jujuSuite) TestGrantModelControllerFailure(c *gc.C) {
	conn, model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	defer conn.Close()
	err := s.store.GrantModel(conn, model, "alice", "superpowers")
	c.Assert(err, gc.ErrorMatches, `invalid model access permission "superpowers"`)
	model1, err := s.store.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{}})
}

func (s *jujuSuite) TestRevokeModel(c *gc.C) {
	conn, model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	defer conn.Close()
	err := s.store.GrantModel(conn, model, "alice", "write")
	c.Assert(err, jc.ErrorIsNil)
	model1, err := s.store.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{"alice"}})
	err = s.store.RevokeModel(conn, model, "alice", "write")
	c.Assert(err, jc.ErrorIsNil)
	model1, err = s.store.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{}})
}

func (s *jujuSuite) TestRevokeModelControllerFailure(c *gc.C) {
	conn, model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	defer conn.Close()
	err := s.store.GrantModel(conn, model, "alice", "write")
	c.Assert(err, jc.ErrorIsNil)
	model1, err := s.store.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{"alice"}})
	err = s.store.RevokeModel(conn, model, "alice", "superpowers")
	c.Assert(err, gc.ErrorMatches, `invalid model access permission "superpowers"`)
	model1, err = s.store.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{}})
}

func (s *jujuSuite) TestDestroyModel(c *gc.C) {
	conn, model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
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

	err = s.store.DestroyModel(conn, model)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-ch:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("model not destroyed")
	}

	// Check the model is removed.
	_, err = s.store.Model(model.Path)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// Check that it cannot be destroyed twice
	err = s.store.DestroyModel(conn, model)
	c.Assert(err, gc.ErrorMatches, `model "bob/model" not found`)

	// Put the model back in the database
	err = s.store.AddModel(model)
	c.Assert(err, jc.ErrorIsNil)

	// Check that it can still be removed even if the contoller has no model.
	err = s.store.DestroyModel(conn, model)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the model is removed.
	_, err = s.store.Model(model.Path)
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

func (s *jujuSuite) addController(c *gc.C, path params.EntityPath) params.EntityPath {
	info := s.APIInfo(c)
	ctl := &mongodoc.Controller{
		Path:          path,
		HostPorts:     info.Addrs,
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	}
	m := &mongodoc.Model{
		UUID: info.ModelTag.Id(),
	}
	// Sanity check that we're really talking to the controller.
	minfo, err := s.APIState.Client().ModelInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(minfo.UUID, gc.Equals, s.ControllerConfig.ControllerUUID())

	err = s.store.AddController(ctl, m)
	c.Assert(err, jc.ErrorIsNil)
	return path
}

func (s *jujuSuite) bootstrapModel(c *gc.C, path params.EntityPath) (*apiconn.Conn, *mongodoc.Model) {
	ctlPath := s.addController(c, params.EntityPath{User: path.User, Name: "controller"})
	err := s.store.UpdateCredential(&mongodoc.Credential{
		User:  path.User,
		Cloud: "dummy",
		Name:  "cred",
		Type:  "empty",
	})
	c.Assert(err, jc.ErrorIsNil)
	conn, err := s.store.OpenAPI(ctlPath)
	c.Assert(err, jc.ErrorIsNil)
	model, _, err := s.store.CreateModel(conn, jem.CreateModelParams{
		Path:           path,
		ControllerPath: ctlPath,
		Credential:     "cred",
		Cloud:          "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)
	return conn, model
}
