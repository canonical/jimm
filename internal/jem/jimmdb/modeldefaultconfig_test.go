// Copyright 20202 Canonical Ltd.

package jimmdb_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/canonical/jimm/internal/jem/jimmdb"
	"github.com/canonical/jimm/internal/jemtest"
	"github.com/canonical/jimm/internal/mgosession"
	"github.com/canonical/jimm/internal/mongodoc"
	"github.com/canonical/jimm/params"
)

type modelDefaultConfigSuite struct {
	jemtest.IsolatedMgoSuite
	database *jimmdb.Database
}

var _ = gc.Suite(&modelDefaultConfigSuite{})

func (s *modelDefaultConfigSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	pool := mgosession.NewPool(context.TODO(), s.Session, 1)
	s.database = jimmdb.NewDatabase(context.TODO(), pool, "jem")
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
	pool.Close()
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *modelDefaultConfigSuite) TearDownTest(c *gc.C) {
	s.database.Session.Close()
	s.database = nil
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *modelDefaultConfigSuite) checkDBOK(c *gc.C) {
	c.Check(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *modelDefaultConfigSuite) TestUpsertModelDefaultConfig(c *gc.C) {
	doc := mongodoc.CloudRegionDefaults{
		User:   "bob",
		Cloud:  "test-cloud",
		Region: "test-cloud-region",
		Defaults: map[string]interface{}{
			"a": "A",
			"b": 11,
		},
	}

	err := s.database.UpsertModelDefaultConfig(testContext, &doc)
	c.Assert(err, gc.Equals, nil)

	var doc2 mongodoc.CloudRegionDefaults
	q := jimmdb.And(jimmdb.Eq("user", doc.User), jimmdb.Eq("cloud", doc.Cloud), jimmdb.Eq("region", doc.Region))
	err = s.database.ModelDefaultConfigs().Find(q).One(&doc2)
	c.Assert(err, gc.Equals, nil)
	c.Check(doc2, jc.DeepEquals, doc)

	doc.Defaults["c"] = "sea"
	err = s.database.UpsertModelDefaultConfig(testContext, &doc)
	c.Assert(err, gc.Equals, nil)

	err = s.database.ModelDefaultConfigs().Find(q).One(&doc2)
	c.Assert(err, gc.Equals, nil)
	c.Check(doc2, jc.DeepEquals, doc)

	err = s.database.UpsertModelDefaultConfig(testContext, &mongodoc.CloudRegionDefaults{})
	c.Check(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	s.checkDBOK(c)
}

func (s *modelDefaultConfigSuite) TestForEachModelDefaultConfig(c *gc.C) {
	modelDefaultConfigs := []mongodoc.CloudRegionDefaults{{
		User:   "alice",
		Cloud:  "test-cloud",
		Region: "",
		Defaults: map[string]interface{}{
			"a": "a",
			"b": "b",
		},
	}, {
		User:   "alice",
		Cloud:  "aws",
		Region: "",
		Defaults: map[string]interface{}{
			"a": "A",
			"b": "B",
		},
	}, {
		User:   "alice",
		Cloud:  "test-cloud",
		Region: "test-cloud-region",
		Defaults: map[string]interface{}{
			"b": "B",
			"c": "C",
		},
	}}
	for i := range modelDefaultConfigs {
		err := s.database.UpsertModelDefaultConfig(testContext, &modelDefaultConfigs[i])
		c.Assert(err, gc.Equals, nil)
	}

	expect := []mongodoc.CloudRegionDefaults{modelDefaultConfigs[1], modelDefaultConfigs[0], modelDefaultConfigs[2]}
	err := s.database.ForEachModelDefaultConfig(testContext, nil, []string{"user", "cloud", "region"}, func(m *mongodoc.CloudRegionDefaults) error {
		if len(expect) < 1 {
			return errgo.Newf("unexpected cloud-region-defaults \"%s-%s-%s\"", m.User, m.Cloud, m.Region)
		}
		c.Check(m, jc.DeepEquals, &expect[0])
		expect = expect[1:]
		return nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Check(expect, gc.HasLen, 0)

	expect = []mongodoc.CloudRegionDefaults{modelDefaultConfigs[0], modelDefaultConfigs[2]}
	err = s.database.ForEachModelDefaultConfig(testContext, jimmdb.Eq("cloud", "test-cloud"), []string{"region"}, func(m *mongodoc.CloudRegionDefaults) error {
		if len(expect) < 1 {
			return errgo.Newf("unexpected cloud-region-defaults \"%s-%s-%s\"", m.User, m.Cloud, m.Region)
		}
		c.Check(m, jc.DeepEquals, &expect[0])
		expect = expect[1:]
		return nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Check(expect, gc.HasLen, 0)

	testError := errgo.New("test")
	err = s.database.ForEachModelDefaultConfig(testContext, nil, nil, func(m *mongodoc.CloudRegionDefaults) error {
		return testError
	})
	c.Check(errgo.Cause(err), gc.Equals, testError)

	s.checkDBOK(c)
}

func (s *modelDefaultConfigSuite) TestUpdateModelDefaultConfig(c *gc.C) {
	doc := mongodoc.CloudRegionDefaults{
		User:   "bob",
		Cloud:  "test-cloud",
		Region: "test-cloud-region",
		Defaults: map[string]interface{}{
			"a": "A",
			"b": 11,
		},
	}

	err := s.database.UpsertModelDefaultConfig(testContext, &doc)
	c.Assert(err, gc.Equals, nil)

	var doc2 mongodoc.CloudRegionDefaults
	q := jimmdb.And(jimmdb.Eq("user", doc.User), jimmdb.Eq("cloud", doc.Cloud), jimmdb.Eq("region", doc.Region))
	err = s.database.ModelDefaultConfigs().Find(q).One(&doc2)
	c.Assert(err, gc.Equals, nil)
	c.Check(doc2, jc.DeepEquals, doc)

	err = s.database.UpdateModelDefaultConfig(testContext, &doc, new(jimmdb.Update).Unset("defaults.b"), true)
	c.Assert(err, gc.Equals, nil)
	_, ok := doc.Defaults["b"]
	c.Assert(ok, gc.Equals, false)

	err = s.database.ModelDefaultConfigs().Find(q).One(&doc2)
	c.Assert(err, gc.Equals, nil)
	c.Check(doc2, jc.DeepEquals, doc)

	s.checkDBOK(c)
}
