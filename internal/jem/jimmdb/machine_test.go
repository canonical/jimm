// Copyright 2016 Canonical Ltd.

package jimmdb_test

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"
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

type machineSuite struct {
	jemtest.IsolatedMgoSuite
	database *jimmdb.Database
}

var _ = gc.Suite(&machineSuite{})

func (s *machineSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	pool := mgosession.NewPool(context.TODO(), s.Session, 1)
	s.database = jimmdb.NewDatabase(context.TODO(), pool, "jem")
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
	pool.Close()
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *machineSuite) TearDownTest(c *gc.C) {
	s.database.Session.Close()
	s.database = nil
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *machineSuite) checkDBOK(c *gc.C) {
	c.Check(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *machineSuite) TestUpsertMachine(c *gc.C) {
	m := mongodoc.Machine{
		Controller: "alice/controller-1",
		Cloud:      "test-cloud",
		Region:     "test-cloud-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: "00000000-0000-0000-0000-00000000000a",
			Id:        "0",
			Life:      life.Alive,
		},
	}

	err := s.database.UpsertMachine(testContext, &m)
	c.Assert(err, gc.Equals, nil)

	var m2 mongodoc.Machine
	err = s.database.Machines().FindId("alice/controller-1 00000000-0000-0000-0000-00000000000a 0").One(&m2)
	c.Assert(err, gc.Equals, nil)
	c.Check(m2, jc.DeepEquals, m)

	m.Info.Life = life.Dying
	err = s.database.UpsertMachine(testContext, &m)
	c.Assert(err, gc.Equals, nil)

	err = s.database.Machines().FindId("alice/controller-1 00000000-0000-0000-0000-00000000000a 0").One(&m2)
	c.Assert(err, gc.Equals, nil)
	c.Check(m2, jc.DeepEquals, m)

	err = s.database.UpsertMachine(testContext, &mongodoc.Machine{})
	c.Check(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	s.checkDBOK(c)
}

func (s *machineSuite) TestForEachMachine(c *gc.C) {
	machines := []mongodoc.Machine{{
		Controller: "alice/controller-1",
		Cloud:      "test-cloud",
		Region:     "test-cloud-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: "00000000-0000-0000-0000-00000000000a",
			Id:        "0",
			Life:      life.Alive,
		},
	}, {
		Controller: "alice/controller-1",
		Cloud:      "test-cloud",
		Region:     "test-cloud-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: "00000000-0000-0000-0000-00000000000b",
			Id:        "0",
			Life:      life.Alive,
		},
	}, {
		Controller: "alice/controller-1",
		Cloud:      "test-cloud",
		Region:     "test-cloud-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: "00000000-0000-0000-0000-00000000000a",
			Id:        "1",
			Life:      life.Alive,
		},
	}}
	for i := range machines {
		err := s.database.UpsertMachine(testContext, &machines[i])
		c.Assert(err, gc.Equals, nil)
	}

	expect := []mongodoc.Machine{machines[0], machines[2], machines[1]}
	err := s.database.ForEachMachine(testContext, nil, []string{"_id"}, func(m *mongodoc.Machine) error {
		if len(expect) < 1 {
			return errgo.Newf("unexpected machine %q", m.Id)
		}
		c.Check(m, jc.DeepEquals, &expect[0])
		expect = expect[1:]
		return nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Check(expect, gc.HasLen, 0)

	expect = []mongodoc.Machine{machines[0], machines[2]}
	err = s.database.ForEachMachine(testContext, jimmdb.Eq("info.modeluuid", "00000000-0000-0000-0000-00000000000a"), []string{"_id"}, func(m *mongodoc.Machine) error {
		if len(expect) < 1 {
			return errgo.Newf("unexpected machine %q", m.Id)
		}
		c.Check(m, jc.DeepEquals, &expect[0])
		expect = expect[1:]
		return nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Check(expect, gc.HasLen, 0)

	testError := errgo.New("test")
	err = s.database.ForEachMachine(testContext, nil, []string{"_id"}, func(m *mongodoc.Machine) error {
		return testError
	})
	c.Check(errgo.Cause(err), gc.Equals, testError)

	s.checkDBOK(c)
}

func (s *machineSuite) TestRemoveMachine(c *gc.C) {
	m := mongodoc.Machine{
		Controller: "alice/controller-1",
		Cloud:      "test-cloud",
		Region:     "test-cloud-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: "00000000-0000-0000-0000-00000000000a",
			Id:        "0",
			Life:      life.Alive,
		},
	}

	err := s.database.UpsertMachine(testContext, &m)
	c.Assert(err, gc.Equals, nil)

	var m2 mongodoc.Machine
	err = s.database.Machines().FindId("alice/controller-1 00000000-0000-0000-0000-00000000000a 0").One(&m2)
	c.Assert(err, gc.Equals, nil)
	c.Check(m2, jc.DeepEquals, m)

	err = s.database.RemoveMachine(testContext, &m)
	c.Assert(err, gc.Equals, nil)

	err = s.database.Machines().FindId("alice/controller-1 00000000-0000-0000-0000-00000000000a 0").One(&m2)
	c.Assert(err, gc.Equals, mgo.ErrNotFound)

	err = s.database.RemoveMachine(testContext, &m)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	s.checkDBOK(c)
}

func (s *machineSuite) TestRemoveMachines(c *gc.C) {
	machines := []mongodoc.Machine{{
		Controller: "alice/controller-1",
		Cloud:      "test-cloud",
		Region:     "test-cloud-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: "00000000-0000-0000-0000-00000000000a",
			Id:        "0",
			Life:      life.Alive,
		},
	}, {
		Controller: "alice/controller-1",
		Cloud:      "test-cloud",
		Region:     "test-cloud-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: "00000000-0000-0000-0000-00000000000b",
			Id:        "0",
			Life:      life.Alive,
		},
	}, {
		Controller: "alice/controller-1",
		Cloud:      "test-cloud",
		Region:     "test-cloud-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: "00000000-0000-0000-0000-00000000000a",
			Id:        "1",
			Life:      life.Alive,
		},
	}}
	for i := range machines {
		err := s.database.UpsertMachine(testContext, &machines[i])
		c.Assert(err, gc.Equals, nil)
	}

	count, err := s.database.RemoveMachines(testContext, jimmdb.Eq("info.modeluuid", "00000000-0000-0000-0000-00000000000a"))
	c.Assert(err, gc.Equals, nil)
	c.Check(count, gc.Equals, 2)

	count, err = s.database.RemoveMachines(testContext, jimmdb.Eq("info.modeluuid", "00000000-0000-0000-0000-00000000000a"))
	c.Assert(err, gc.Equals, nil)
	c.Check(count, gc.Equals, 0)

	expect := []mongodoc.Machine{machines[1]}
	err = s.database.ForEachMachine(testContext, nil, []string{"_id"}, func(m *mongodoc.Machine) error {
		if len(expect) < 1 {
			return errgo.Newf("unexpected machine %q", m.Id)
		}
		c.Check(m, jc.DeepEquals, &expect[0])
		expect = expect[1:]
		return nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Check(expect, gc.HasLen, 0)

	s.checkDBOK(c)
}
