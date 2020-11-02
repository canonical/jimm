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

type cloudRegionSuite struct {
	jemtest.IsolatedMgoSuite
	database *jimmdb.Database
}

var _ = gc.Suite(&cloudRegionSuite{})

func (s *cloudRegionSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	pool := mgosession.NewPool(context.TODO(), s.Session, 1)
	s.database = jimmdb.NewDatabase(context.TODO(), pool, "jem")
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
	pool.Close()
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *cloudRegionSuite) TearDownTest(c *gc.C) {
	s.database.Session.Close()
	s.database = nil
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *cloudRegionSuite) checkDBOK(c *gc.C) {
	c.Check(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *cloudRegionSuite) TestInsertCloudRegion(c *gc.C) {
	cr := mongodoc.CloudRegion{
		Id:    "ignored",
		Cloud: "test-cloud",
	}
	err := s.database.InsertCloudRegion(testContext, &cr)
	c.Assert(err, gc.Equals, nil)
	c.Assert(cr, jc.DeepEquals, mongodoc.CloudRegion{
		Id:    "test-cloud/",
		Cloud: "test-cloud",
	})

	cr1 := mongodoc.CloudRegion{Cloud: "test-cloud"}
	err = s.database.GetCloudRegion(testContext, &cr1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(cr1, jemtest.CmpEquals(cmpopts.EquateEmpty()), cr)

	err = s.database.InsertCloudRegion(testContext, &cr)
	c.Assert(err, gc.ErrorMatches, "already exists")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)
	s.checkDBOK(c)
}

func (s *cloudRegionSuite) TestGetCloudRegion(c *gc.C) {
	ctx := context.Background()
	cr1 := mongodoc.CloudRegion{
		Cloud:        "test-cloud",
		ProviderType: "dummy",
	}
	err := s.database.InsertCloudRegion(testContext, &cr1)
	c.Assert(err, gc.Equals, nil)
	cr2 := mongodoc.CloudRegion{
		Cloud:        "test-cloud",
		Region:       "dummy-region",
		ProviderType: "dummy",
	}
	err = s.database.InsertCloudRegion(testContext, &cr2)
	c.Assert(err, gc.Equals, nil)

	cr3 := mongodoc.CloudRegion{Cloud: "test-cloud"}
	err = s.database.GetCloudRegion(ctx, &cr3)
	c.Assert(err, gc.Equals, nil)
	c.Check(cr3, jc.DeepEquals, cr1)

	cr4 := mongodoc.CloudRegion{Cloud: "test-cloud", Region: "dummy-region"}
	err = s.database.GetCloudRegion(ctx, &cr4)
	c.Assert(err, gc.Equals, nil)
	c.Check(cr4, jc.DeepEquals, cr2)

	cr5 := mongodoc.CloudRegion{ProviderType: "dummy", Region: "dummy-region"}
	err = s.database.GetCloudRegion(ctx, &cr5)
	c.Assert(err, gc.Equals, nil)
	c.Check(cr5, jc.DeepEquals, cr2)

	cr6 := mongodoc.CloudRegion{ProviderType: "dummy"}
	err = s.database.GetCloudRegion(ctx, &cr6)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	cr7 := mongodoc.CloudRegion{Cloud: "not-test-cloud"}
	err = s.database.GetCloudRegion(ctx, &cr7)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	cr8 := mongodoc.CloudRegion{Cloud: "test-cloud", Region: "no-such-region"}
	err = s.database.GetCloudRegion(ctx, &cr8)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	cr9 := mongodoc.CloudRegion{ProviderType: "no-such-provider", Region: "dummy-region"}
	err = s.database.GetCloudRegion(ctx, &cr9)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	cr10 := mongodoc.CloudRegion{ProviderType: "dummy", Region: "not-dummy-region"}
	err = s.database.GetCloudRegion(ctx, &cr10)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *cloudRegionSuite) TestForEachCloudRegion(c *gc.C) {
	ctx := context.Background()

	cr1 := mongodoc.CloudRegion{
		Cloud:        "dummy-1",
		ProviderType: "dummy",
	}
	err := s.database.InsertCloudRegion(testContext, &cr1)
	c.Assert(err, gc.Equals, nil)

	cr2 := mongodoc.CloudRegion{
		Cloud:        "dummy-1",
		Region:       "default",
		ProviderType: "dummy",
	}
	err = s.database.InsertCloudRegion(testContext, &cr2)
	c.Assert(err, gc.Equals, nil)

	cr3 := mongodoc.CloudRegion{
		Cloud:        "dummy-2",
		ProviderType: "dummy",
	}
	err = s.database.InsertCloudRegion(testContext, &cr3)
	c.Assert(err, gc.Equals, nil)

	cr4 := mongodoc.CloudRegion{
		Cloud:        "dummy-2",
		Region:       "default",
		ProviderType: "dummy",
	}
	err = s.database.InsertCloudRegion(testContext, &cr4)
	c.Assert(err, gc.Equals, nil)

	expect := []mongodoc.CloudRegion{cr1, cr2}
	err = s.database.ForEachCloudRegion(ctx, jimmdb.Eq("cloud", "dummy-1"), []string{"region"}, func(cr *mongodoc.CloudRegion) error {
		if len(expect) < 1 {
			return errgo.Newf("unexpected result %q", cr.Id)
		}
		c.Check(*cr, jc.DeepEquals, expect[0])
		expect = expect[1:]
		return nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Check(expect, gc.HasLen, 0)

	expect = []mongodoc.CloudRegion{cr1, cr2, cr3, cr4}
	err = s.database.ForEachCloudRegion(ctx, jimmdb.Eq("providertype", "dummy"), []string{"cloud", "region"}, func(cr *mongodoc.CloudRegion) error {
		if len(expect) < 1 {
			return errgo.Newf("unexpected result %q", cr.Id)
		}
		c.Check(*cr, jc.DeepEquals, expect[0])
		expect = expect[1:]
		return nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Check(expect, gc.HasLen, 0)

	expect = []mongodoc.CloudRegion{}
	err = s.database.ForEachCloudRegion(ctx, jimmdb.Eq("providertype", "not-dummy"), []string{"cloud", "region"}, func(cr *mongodoc.CloudRegion) error {
		if len(expect) < 1 {
			return errgo.Newf("unexpected result %q", cr.Id)
		}
		c.Check(*cr, jc.DeepEquals, expect[0])
		expect = expect[1:]
		return nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Check(expect, gc.HasLen, 0)
}

func (s *cloudRegionSuite) TestUpsertCloudRegion(c *gc.C) {
	ctx := context.Background()

	cr1 := mongodoc.CloudRegion{
		Cloud:  "dummy-1",
		Region: "dummy-region",
		ACL: params.ACL{
			Read: []string{"everyone@external"},
		},
		ProviderType:         "dummy",
		AuthTypes:            []string{"empty"},
		Endpoint:             "https://example.com/endpoint",
		IdentityEndpoint:     "https://example.com/identity-endpoint",
		StorageEndpoint:      "https://example.com/storage-endpoint",
		CACertificates:       []string{"cert1"},
		PrimaryControllers:   []params.EntityPath{{User: jemtest.ControllerAdmin, Name: "test1"}},
		SecondaryControllers: []params.EntityPath{{User: jemtest.ControllerAdmin, Name: "test2"}},
	}
	err := s.database.UpsertCloudRegion(ctx, &cr1)
	c.Assert(err, gc.Equals, nil)

	cr2 := mongodoc.CloudRegion{
		Cloud:  "dummy-1",
		Region: "dummy-region",
	}
	err = s.database.GetCloudRegion(ctx, &cr2)
	c.Assert(err, gc.Equals, nil)
	c.Assert(cr2, jc.DeepEquals, cr1)

	cr3 := mongodoc.CloudRegion{
		Cloud:  "dummy-1",
		Region: "dummy-region",
		ACL: params.ACL{
			Read: []string{"noone@external"},
		},
		ProviderType:         "dummy-2",
		AuthTypes:            []string{"empty", "userpass"},
		Endpoint:             "https://example.com/endpoint-2",
		IdentityEndpoint:     "https://example.com/identity-endpoint-2",
		StorageEndpoint:      "https://example.com/storage-endpoint-2",
		CACertificates:       []string{"cert2"},
		PrimaryControllers:   []params.EntityPath{{User: jemtest.ControllerAdmin, Name: "test3"}},
		SecondaryControllers: []params.EntityPath{{User: jemtest.ControllerAdmin, Name: "test4"}},
	}
	err = s.database.UpsertCloudRegion(ctx, &cr3)
	c.Assert(err, gc.Equals, nil)

	cr4 := mongodoc.CloudRegion{
		Cloud:  "dummy-1",
		Region: "dummy-region",
	}
	err = s.database.GetCloudRegion(ctx, &cr4)
	c.Assert(err, gc.Equals, nil)

	c.Check(cr4, jc.DeepEquals, mongodoc.CloudRegion{
		Id:     cr1.Id,
		Cloud:  "dummy-1",
		Region: "dummy-region",
		ACL: params.ACL{
			Read: []string{"everyone@external"},
		},
		ProviderType:     "dummy-2",
		AuthTypes:        []string{"empty", "userpass"},
		Endpoint:         "https://example.com/endpoint-2",
		IdentityEndpoint: "https://example.com/identity-endpoint-2",
		StorageEndpoint:  "https://example.com/storage-endpoint-2",
		CACertificates:   []string{"cert2"},
		PrimaryControllers: []params.EntityPath{
			{User: jemtest.ControllerAdmin, Name: "test1"},
			{User: jemtest.ControllerAdmin, Name: "test3"},
		},
		SecondaryControllers: []params.EntityPath{
			{User: jemtest.ControllerAdmin, Name: "test2"},
			{User: jemtest.ControllerAdmin, Name: "test4"},
		},
	})
}

func (s *cloudRegionSuite) TestRemoveCloudRegion(c *gc.C) {
	cr := mongodoc.CloudRegion{Cloud: "test-cloud"}
	err := s.database.InsertCloudRegion(testContext, &cr)
	c.Assert(err, gc.Equals, nil)
	err = s.database.RemoveCloudRegion(testContext, &cr)
	c.Assert(err, gc.Equals, nil)
	err = s.database.InsertCloudRegion(testContext, &cr)
	c.Assert(err, gc.Equals, nil)
}

func (s *cloudRegionSuite) TestRemoveCloudRegions(c *gc.C) {
	cr1 := mongodoc.CloudRegion{Cloud: "test-cloud"}
	err := s.database.InsertCloudRegion(testContext, &cr1)
	c.Assert(err, gc.Equals, nil)
	cr2 := mongodoc.CloudRegion{Cloud: "test-cloud", Region: "test-region"}
	err = s.database.InsertCloudRegion(testContext, &cr2)
	c.Assert(err, gc.Equals, nil)
	cr3 := mongodoc.CloudRegion{Cloud: "test-cloud-2"}
	err = s.database.InsertCloudRegion(testContext, &cr3)
	c.Assert(err, gc.Equals, nil)

	count, err := s.database.RemoveCloudRegions(testContext, jimmdb.Eq("cloud", "test-cloud"))
	c.Assert(err, gc.Equals, nil)
	c.Check(count, gc.Equals, 2)

	expect := []mongodoc.CloudRegion{cr3}
	err = s.database.ForEachCloudRegion(testContext, nil, nil, func(cr *mongodoc.CloudRegion) error {
		if len(expect) < 1 {
			return errgo.Newf("unexpected cloudregion %q", cr.Id)
		}
		c.Check(*cr, jc.DeepEquals, expect[0])
		expect = expect[1:]
		return nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Check(expect, gc.HasLen, 0)
}
