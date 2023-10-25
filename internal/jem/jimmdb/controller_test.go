// Copyright 2016 Canonical Ltd.

package jimmdb_test

import (
	"context"

	"github.com/google/go-cmp/cmp/cmpopts"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/canonical/jimm/internal/jem/jimmdb"
	"github.com/canonical/jimm/internal/jemtest"
	"github.com/canonical/jimm/internal/mgosession"
	"github.com/canonical/jimm/internal/mongodoc"
	"github.com/canonical/jimm/params"
)

type controllerSuite struct {
	jemtest.IsolatedMgoSuite
	database *jimmdb.Database
}

var _ = gc.Suite(&controllerSuite{})

func (s *controllerSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	pool := mgosession.NewPool(context.TODO(), s.Session, 1)
	s.database = jimmdb.NewDatabase(context.TODO(), pool, "jem")
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
	pool.Close()
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *controllerSuite) TearDownTest(c *gc.C) {
	s.database.Session.Close()
	s.database = nil
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *controllerSuite) checkDBOK(c *gc.C) {
	c.Check(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *controllerSuite) TestInsertController(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	ctl := mongodoc.Controller{
		Id:   "ignored",
		Path: ctlPath,
	}
	err := s.database.InsertController(testContext, &ctl)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ctl, jc.DeepEquals, mongodoc.Controller{
		Id:   "bob/x",
		Path: ctlPath,
	})

	ctl1 := mongodoc.Controller{Path: ctlPath}
	err = s.database.GetController(testContext, &ctl1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ctl1, jemtest.CmpEquals(cmpopts.EquateEmpty()), ctl)

	err = s.database.InsertController(testContext, &ctl)
	c.Assert(err, gc.ErrorMatches, "already exists")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)
	s.checkDBOK(c)
}

func (s *controllerSuite) TestRemoveController(c *gc.C) {
	ctlPath := params.EntityPath{"dalek", "who"}
	ctl := &mongodoc.Controller{
		Id:     "ignored",
		Path:   ctlPath,
		CACert: "certainly",
		HostPorts: [][]mongodoc.HostPort{{{
			Host: "host1",
			Port: 1234,
		}}, {{
			Host: "host2",
			Port: 9999,
		}}},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	err := s.database.InsertController(testContext, ctl)
	c.Assert(err, gc.Equals, nil)

	ctl2 := &mongodoc.Controller{
		Path: ctlPath,
	}
	err = s.database.RemoveController(testContext, ctl2)
	c.Assert(err, gc.Equals, nil)

	ctl3 := &mongodoc.Controller{
		Path: ctlPath,
	}
	err = s.database.GetController(testContext, ctl3)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	err = s.database.RemoveController(testContext, ctl2)
	c.Assert(err, gc.ErrorMatches, "controller not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
}

func (s *controllerSuite) TestRemoveControllerWithUUID(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.InsertController(testContext, &mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.Equals, nil)

	ctl := mongodoc.Controller{
		UUID: "fake-uuid",
	}
	err = s.database.RemoveController(testContext, &ctl)
	c.Assert(err, gc.Equals, nil)

	ctl2 := mongodoc.Controller{Path: ctlPath}
	err = s.database.GetController(testContext, &ctl2)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *controllerSuite) TestGetControllerFromUUID(c *gc.C) {
	uuid := "99999999-9999-9999-9999-999999999999"
	path := params.EntityPath{"bob", "x"}
	ctl := mongodoc.Controller{
		Id:   "ignored",
		Path: path,
		UUID: uuid,
	}
	err := s.database.InsertController(testContext, &ctl)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ctl, jc.DeepEquals, mongodoc.Controller{
		Id:   "bob/x",
		Path: path,
		UUID: uuid,
	})

	ctl1 := mongodoc.Controller{UUID: uuid}
	err = s.database.GetController(testContext, &ctl1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ctl1, jemtest.CmpEquals(cmpopts.EquateEmpty()), ctl)

	ctl2 := mongodoc.Controller{UUID: "no-such-uuid"}
	err = s.database.GetController(testContext, &ctl2)
	c.Assert(err, gc.ErrorMatches, `controller not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	s.checkDBOK(c)
}

func (s *controllerSuite) TestForEachController(c *gc.C) {
	ctlPath1 := params.EntityPath{User: "bob", Name: "c1"}
	ctlPath2 := params.EntityPath{User: "bob", Name: "c2"}

	err := s.database.InsertController(testContext, &mongodoc.Controller{
		Path: ctlPath1,
		UUID: "00000000-0000-0000-0000-000000000001",
	})
	c.Assert(err, gc.Equals, nil)
	err = s.database.InsertController(testContext, &mongodoc.Controller{
		Path: ctlPath2,
		UUID: "00000000-0000-0000-0000-000000000002",
	})
	c.Assert(err, gc.Equals, nil)

	paths := []params.EntityPath{
		{User: "bob", Name: "c2"},
		{User: "bob", Name: "c1"},
	}
	f := func(ctl *mongodoc.Controller) error {
		if len(paths) == 0 || ctl.Path != paths[0] {
			return errgo.Newf("unexpected controller, %s", ctl.Path)
		}
		paths = paths[1:]
		return nil
	}

	err = s.database.ForEachController(testContext, nil, []string{"-uuid"}, f)
	c.Assert(err, gc.Equals, nil)
	c.Assert(paths, gc.HasLen, 0)

	s.checkDBOK(c)
}

func (s *controllerSuite) TestForEachControllerReturnsError(c *gc.C) {
	ctlPath := params.EntityPath{User: "bob", Name: "c1"}

	err := s.database.InsertController(testContext, &mongodoc.Controller{
		Path: ctlPath,
		UUID: "00000000-0000-0000-0000-000000000001",
	})
	c.Assert(err, gc.Equals, nil)

	testError := errgo.New("test")

	f := func(_ *mongodoc.Controller) error {
		return testError
	}

	err = s.database.ForEachController(testContext, nil, []string{"-uuid"}, f)
	c.Assert(errgo.Cause(err), gc.Equals, testError)

	s.checkDBOK(c)
}

func (s *controllerSuite) TestCountController(c *gc.C) {
	ctlPath1 := params.EntityPath{User: "bob", Name: "c1"}
	ctlPath2 := params.EntityPath{User: "bob", Name: "c2"}
	ctlPath3 := params.EntityPath{User: "bob", Name: "c3"}

	err := s.database.InsertController(testContext, &mongodoc.Controller{
		Path:   ctlPath1,
		UUID:   "00000000-0000-0000-0000-000000000001",
		Public: true,
	})
	c.Assert(err, gc.Equals, nil)
	err = s.database.InsertController(testContext, &mongodoc.Controller{
		Path:   ctlPath2,
		UUID:   "00000000-0000-0000-0000-000000000002",
		Public: false,
	})
	c.Assert(err, gc.Equals, nil)
	err = s.database.InsertController(testContext, &mongodoc.Controller{
		Path:   ctlPath3,
		UUID:   "00000000-0000-0000-0000-000000000003",
		Public: true,
	})
	c.Assert(err, gc.Equals, nil)

	i, err := s.database.CountControllers(testContext, jimmdb.Eq("public", true))
	c.Assert(err, gc.Equals, nil)
	c.Check(i, gc.Equals, 2)
	i, err = s.database.CountControllers(testContext, jimmdb.NotExists("public"))
	c.Assert(err, gc.Equals, nil)
	c.Check(i, gc.Equals, 1)

	s.checkDBOK(c)
}
