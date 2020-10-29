// Copyright 2016 Canonical Ltd.

package jimmdb_test

import (
	"context"

	"github.com/google/go-cmp/cmp/cmpopts"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/jem/jimmdb"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

type modelSuite struct {
	jemtest.IsolatedMgoSuite
	database *jimmdb.Database
}

var _ = gc.Suite(&modelSuite{})

func (s *modelSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	pool := mgosession.NewPool(context.TODO(), s.Session, 1)
	s.database = jimmdb.NewDatabase(context.TODO(), pool, "jem")
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
	pool.Close()
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *modelSuite) TearDownTest(c *gc.C) {
	s.database.Session.Close()
	s.database = nil
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *modelSuite) checkDBOK(c *gc.C) {
	c.Check(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *modelSuite) TestInsertModel(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	m := mongodoc.Model{
		Id:   "ignored",
		Path: ctlPath,
	}
	err := s.database.InsertModel(testContext, &m)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m, jc.DeepEquals, mongodoc.Model{
		Id:   "bob/x",
		Path: ctlPath,
	})

	m1 := mongodoc.Model{Path: ctlPath}
	err = s.database.GetModel(testContext, &m1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m1, jemtest.CmpEquals(cmpopts.EquateEmpty()), m)

	err = s.database.InsertModel(testContext, &m)
	c.Assert(err, gc.ErrorMatches, "already exists")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)
	s.checkDBOK(c)
}

func (s *modelSuite) TestRemoveModel(c *gc.C) {
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

	modelPath := params.EntityPath{"dalek", "exterminate"}
	m2 := &mongodoc.Model{
		Path: modelPath,
	}
	err = s.database.InsertModel(testContext, m2)
	c.Assert(err, gc.Equals, nil)

	err = s.database.RemoveModel(testContext, m2)
	c.Assert(err, gc.Equals, nil)
	m3 := mongodoc.Model{Path: modelPath}
	err = s.database.GetModel(testContext, &m3)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	err = s.database.RemoveModel(testContext, m2)
	c.Assert(err, gc.ErrorMatches, "model not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
}

func (s *modelSuite) TestRemoveModelWithUUID(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.InsertController(testContext, &mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.Equals, nil)

	// Add the controller model.
	err = s.database.InsertModel(testContext, &mongodoc.Model{
		Path:       ctlPath,
		UUID:       "fake-uuid",
		Controller: ctlPath,
	})
	c.Assert(err, gc.Equals, nil)

	m := mongodoc.Model{
		Controller: ctlPath,
		UUID:       "fake-uuid",
	}
	err = s.database.RemoveModel(testContext, &m)
	c.Assert(err, gc.Equals, nil)

	m = mongodoc.Model{Path: ctlPath}
	err = s.database.GetModel(testContext, &m)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *modelSuite) TestGetModelFromUUID(c *gc.C) {
	uuid := "99999999-9999-9999-9999-999999999999"
	path := params.EntityPath{"bob", "x"}
	m := mongodoc.Model{
		Id:   "ignored",
		Path: path,
		UUID: uuid,
	}
	err := s.database.InsertModel(testContext, &m)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m, jc.DeepEquals, mongodoc.Model{
		Id:   "bob/x",
		Path: path,
		UUID: uuid,
	})

	m1 := mongodoc.Model{UUID: uuid}
	err = s.database.GetModel(testContext, &m1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m1, jemtest.CmpEquals(cmpopts.EquateEmpty()), m)

	m2 := mongodoc.Model{UUID: "no-such-uuid"}
	err = s.database.GetModel(testContext, &m2)
	c.Assert(err, gc.ErrorMatches, `model not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	m3 := mongodoc.Model{
		Controller: params.EntityPath{User: "bob", Name: "no-such-controller"},
		UUID:       uuid,
	}
	err = s.database.GetModel(testContext, &m3)
	c.Assert(err, gc.ErrorMatches, `model not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	s.checkDBOK(c)
}

func (s *modelSuite) TestForEachModel(c *gc.C) {
	ctlPath1 := params.EntityPath{User: "bob", Name: "c1"}
	ctlPath2 := params.EntityPath{User: "bob", Name: "c2"}

	err := s.database.InsertModel(testContext, &mongodoc.Model{
		Path:       params.EntityPath{User: "bob", Name: "m1"},
		UUID:       "00000000-0000-0000-0000-000000000001",
		Controller: ctlPath1,
	})
	c.Assert(err, gc.Equals, nil)
	err = s.database.InsertModel(testContext, &mongodoc.Model{
		Path:       params.EntityPath{User: "bob", Name: "m2"},
		UUID:       "00000000-0000-0000-0000-000000000002",
		Controller: ctlPath2,
	})
	c.Assert(err, gc.Equals, nil)
	err = s.database.InsertModel(testContext, &mongodoc.Model{
		Path:       params.EntityPath{User: "bob", Name: "m3"},
		UUID:       "00000000-0000-0000-0000-000000000003",
		Controller: ctlPath1,
	})
	c.Assert(err, gc.Equals, nil)

	paths := []params.EntityPath{
		{User: "bob", Name: "m3"},
		{User: "bob", Name: "m1"},
		{User: "bob", Name: "m2"},
	}
	f := func(m *mongodoc.Model) error {
		if len(paths) == 0 || m.Path != paths[0] {
			return errgo.Newf("unexpected model, %s", m.Path)
		}
		paths = paths[1:]
		return nil
	}

	err = s.database.ForEachModel(testContext, jimmdb.Eq("controller", ctlPath1), []string{"-uuid"}, f)
	c.Assert(err, gc.Equals, nil)
	err = s.database.ForEachModel(testContext, jimmdb.Eq("controller", ctlPath2), []string{"-uuid"}, f)
	c.Assert(err, gc.Equals, nil)
	c.Assert(paths, gc.HasLen, 0)

	s.checkDBOK(c)
}

func (s *modelSuite) TestForEachModelReturnsError(c *gc.C) {
	ctlPath := params.EntityPath{User: "bob", Name: "c1"}

	err := s.database.InsertModel(testContext, &mongodoc.Model{
		Path:       params.EntityPath{User: "bob", Name: "m1"},
		UUID:       "00000000-0000-0000-0000-000000000001",
		Controller: ctlPath,
	})
	c.Assert(err, gc.Equals, nil)

	testError := errgo.New("test")

	f := func(m *mongodoc.Model) error {
		return testError
	}

	err = s.database.ForEachModel(testContext, jimmdb.Eq("controller", ctlPath), []string{"-uuid"}, f)
	c.Assert(errgo.Cause(err), gc.Equals, testError)

	s.checkDBOK(c)
}

func (s *modelSuite) TestCountModels(c *gc.C) {
	ctlPath1 := params.EntityPath{User: "bob", Name: "c1"}
	ctlPath2 := params.EntityPath{User: "bob", Name: "c2"}

	err := s.database.InsertModel(testContext, &mongodoc.Model{
		Path:       params.EntityPath{User: "bob", Name: "m1"},
		UUID:       "00000000-0000-0000-0000-000000000001",
		Controller: ctlPath1,
	})
	c.Assert(err, gc.Equals, nil)
	err = s.database.InsertModel(testContext, &mongodoc.Model{
		Path:       params.EntityPath{User: "bob", Name: "m2"},
		UUID:       "00000000-0000-0000-0000-000000000002",
		Controller: ctlPath2,
	})
	c.Assert(err, gc.Equals, nil)
	err = s.database.InsertModel(testContext, &mongodoc.Model{
		Path:       params.EntityPath{User: "bob", Name: "m3"},
		UUID:       "00000000-0000-0000-0000-000000000003",
		Controller: ctlPath1,
	})
	c.Assert(err, gc.Equals, nil)

	i, err := s.database.CountModels(testContext, jimmdb.Eq("controller", ctlPath1))
	c.Assert(err, gc.Equals, nil)
	c.Check(i, gc.Equals, 2)
	i, err = s.database.CountModels(testContext, jimmdb.Eq("controller", ctlPath2))
	c.Assert(err, gc.Equals, nil)
	c.Check(i, gc.Equals, 1)

	s.checkDBOK(c)
}

func (s *modelSuite) TestLegacyModelCredentials(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	m := struct {
		Id            string `bson:"_id"`
		Path          params.EntityPath
		Cloud         params.Cloud
		CloudRegion   string `bson:",omitempty"`
		DefaultSeries string
		Credential    legacyCredentialPath
	}{
		Id:            ctlPath.String(),
		Path:          ctlPath,
		Cloud:         "bob-cloud",
		CloudRegion:   "bob-region",
		DefaultSeries: "trusty",
		Credential: legacyCredentialPath{
			Cloud: "bob-cloud",
			EntityPath: params.EntityPath{
				User: params.User("bob"),
				Name: params.Name("test-credentials"),
			},
		},
	}
	err := s.database.Models().Insert(m)
	c.Assert(err, gc.Equals, nil)

	m1 := mongodoc.Model{Path: ctlPath}
	err = s.database.GetModel(testContext, &m1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m1, jemtest.CmpEquals(cmpopts.EquateEmpty()), mongodoc.Model{
		Id:            m.Id,
		Path:          m.Path,
		Cloud:         m.Cloud,
		CloudRegion:   m.CloudRegion,
		DefaultSeries: m.DefaultSeries,
		Credential: mongodoc.CredentialPath{
			Cloud: "bob-cloud",
			EntityPath: mongodoc.EntityPath{
				User: "bob",
				Name: "test-credentials",
			},
		},
	})
}
