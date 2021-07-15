// Copyright 2016 Canonical Ltd.

package jimmdb_test

import (
	"context"

	"github.com/juju/juju/core/life"
	"github.com/juju/mgo/v2"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/jem/jimmdb"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

type applicationSuite struct {
	jemtest.IsolatedMgoSuite
	database *jimmdb.Database
}

var _ = gc.Suite(&applicationSuite{})

func (s *applicationSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	pool := mgosession.NewPool(context.TODO(), s.Session, 1)
	s.database = jimmdb.NewDatabase(context.TODO(), pool, "jem")
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
	pool.Close()
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *applicationSuite) TearDownTest(c *gc.C) {
	s.database.Session.Close()
	s.database = nil
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *applicationSuite) checkDBOK(c *gc.C) {
	c.Check(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *applicationSuite) TestUpsertApplication(c *gc.C) {
	m := mongodoc.Application{
		Controller: "alice/controller-1",
		Cloud:      "test-cloud",
		Region:     "test-cloud-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: "00000000-0000-0000-0000-00000000000a",
			Name:      "app0",
			Life:      life.Alive,
		},
	}

	err := s.database.UpsertApplication(testContext, &m)
	c.Assert(err, gc.Equals, nil)

	var m2 mongodoc.Application
	err = s.database.Applications().FindId("alice/controller-1 00000000-0000-0000-0000-00000000000a app0").One(&m2)
	c.Assert(err, gc.Equals, nil)
	c.Check(m2, jc.DeepEquals, m)

	m.Info.Life = life.Dying
	err = s.database.UpsertApplication(testContext, &m)
	c.Assert(err, gc.Equals, nil)

	err = s.database.Applications().FindId("alice/controller-1 00000000-0000-0000-0000-00000000000a app0").One(&m2)
	c.Assert(err, gc.Equals, nil)
	c.Check(m2, jc.DeepEquals, m)

	err = s.database.UpsertApplication(testContext, &mongodoc.Application{})
	c.Check(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	s.checkDBOK(c)
}

func (s *applicationSuite) TestForEachApplication(c *gc.C) {
	applications := []mongodoc.Application{{
		Controller: "alice/controller-1",
		Cloud:      "test-cloud",
		Region:     "test-cloud-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: "00000000-0000-0000-0000-00000000000a",
			Name:      "app0",
			Life:      life.Alive,
		},
	}, {
		Controller: "alice/controller-1",
		Cloud:      "test-cloud",
		Region:     "test-cloud-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: "00000000-0000-0000-0000-00000000000b",
			Name:      "app0",
			Life:      life.Alive,
		},
	}, {
		Controller: "alice/controller-1",
		Cloud:      "test-cloud",
		Region:     "test-cloud-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: "00000000-0000-0000-0000-00000000000a",
			Name:      "app1",
			Life:      life.Alive,
		},
	}}
	for i := range applications {
		err := s.database.UpsertApplication(testContext, &applications[i])
		c.Assert(err, gc.Equals, nil)
	}

	expect := []mongodoc.Application{applications[0], applications[2], applications[1]}
	err := s.database.ForEachApplication(testContext, nil, []string{"_id"}, func(m *mongodoc.Application) error {
		if len(expect) < 1 {
			return errgo.Newf("unexpected application %q", m.Id)
		}
		c.Check(m, jc.DeepEquals, &expect[0])
		expect = expect[1:]
		return nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Check(expect, gc.HasLen, 0)

	expect = []mongodoc.Application{applications[0], applications[2]}
	err = s.database.ForEachApplication(testContext, jimmdb.Eq("info.modeluuid", "00000000-0000-0000-0000-00000000000a"), []string{"_id"}, func(m *mongodoc.Application) error {
		if len(expect) < 1 {
			return errgo.Newf("unexpected application %q", m.Id)
		}
		c.Check(m, jc.DeepEquals, &expect[0])
		expect = expect[1:]
		return nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Check(expect, gc.HasLen, 0)

	testError := errgo.New("test")
	err = s.database.ForEachApplication(testContext, nil, []string{"_id"}, func(m *mongodoc.Application) error {
		return testError
	})
	c.Check(errgo.Cause(err), gc.Equals, testError)

	s.checkDBOK(c)
}

func (s *applicationSuite) TestRemoveApplication(c *gc.C) {
	m := mongodoc.Application{
		Controller: "alice/controller-1",
		Cloud:      "test-cloud",
		Region:     "test-cloud-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: "00000000-0000-0000-0000-00000000000a",
			Name:      "app0",
			Life:      life.Alive,
		},
	}

	err := s.database.UpsertApplication(testContext, &m)
	c.Assert(err, gc.Equals, nil)

	var m2 mongodoc.Application
	err = s.database.Applications().FindId("alice/controller-1 00000000-0000-0000-0000-00000000000a app0").One(&m2)
	c.Assert(err, gc.Equals, nil)
	c.Check(m2, jc.DeepEquals, m)

	err = s.database.RemoveApplication(testContext, &m)
	c.Assert(err, gc.Equals, nil)

	err = s.database.Applications().FindId("alice/controller-1 00000000-0000-0000-0000-00000000000a app0").One(&m2)
	c.Assert(err, gc.Equals, mgo.ErrNotFound)

	err = s.database.RemoveApplication(testContext, &m)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	s.checkDBOK(c)
}

func (s *applicationSuite) TestRemoveApplications(c *gc.C) {
	applications := []mongodoc.Application{{
		Controller: "alice/controller-1",
		Cloud:      "test-cloud",
		Region:     "test-cloud-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: "00000000-0000-0000-0000-00000000000a",
			Name:      "app0",
			Life:      life.Alive,
		},
	}, {
		Controller: "alice/controller-1",
		Cloud:      "test-cloud",
		Region:     "test-cloud-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: "00000000-0000-0000-0000-00000000000b",
			Name:      "app0",
			Life:      life.Alive,
		},
	}, {
		Controller: "alice/controller-1",
		Cloud:      "test-cloud",
		Region:     "test-cloud-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: "00000000-0000-0000-0000-00000000000a",
			Name:      "app1",
			Life:      life.Alive,
		},
	}}
	for i := range applications {
		err := s.database.UpsertApplication(testContext, &applications[i])
		c.Assert(err, gc.Equals, nil)
	}

	count, err := s.database.RemoveApplications(testContext, jimmdb.Eq("info.modeluuid", "00000000-0000-0000-0000-00000000000a"))
	c.Assert(err, gc.Equals, nil)
	c.Check(count, gc.Equals, 2)

	count, err = s.database.RemoveApplications(testContext, jimmdb.Eq("info.modeluuid", "00000000-0000-0000-0000-00000000000a"))
	c.Assert(err, gc.Equals, nil)
	c.Check(count, gc.Equals, 0)

	expect := []mongodoc.Application{applications[1]}
	err = s.database.ForEachApplication(testContext, nil, []string{"_id"}, func(m *mongodoc.Application) error {
		if len(expect) < 1 {
			return errgo.Newf("unexpected application %q", m.Id)
		}
		c.Check(m, jc.DeepEquals, &expect[0])
		expect = expect[1:]
		return nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Check(expect, gc.HasLen, 0)

	s.checkDBOK(c)
}
