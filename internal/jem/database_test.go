// Copyright 2016 Canonical Ltd.

package jem_test

import (
	"fmt"
	"sort"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/state/multiwatcher"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jem/internal/auth"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/jemtest"
	"github.com/CanonicalLtd/jem/internal/mgosession"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

var testContext = context.Background()

type databaseSuite struct {
	jemtest.IsolatedMgoSuite
	database *jem.Database
}

var _ = gc.Suite(&databaseSuite{})

func (s *databaseSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	pool := mgosession.NewPool(context.TODO(), s.Session, 1)
	s.database = jem.NewDatabase(context.TODO(), pool, "jem")
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
	pool.Close()
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *databaseSuite) TearDownTest(c *gc.C) {
	s.database.Session.Close()
	s.database = nil
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *databaseSuite) checkDBOK(c *gc.C) {
	c.Check(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *databaseSuite) TestAddController(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
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
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "foo",
			}},
		},
		Location: map[string]string{
			"cloud":  "aws",
			"region": "foo",
		},
	}
	err := s.database.AddController(testContext, ctl)
	c.Assert(err, gc.IsNil)

	// Check that the fields have been mutated as expected.
	c.Assert(ctl, jc.DeepEquals, &mongodoc.Controller{
		Id:     "bob/x",
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
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "foo",
			}},
		},
		Location: map[string]string{
			"cloud":  "aws",
			"region": "foo",
		},
	})

	ctl1, err := s.database.Controller(testContext, ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl1, jc.DeepEquals, &mongodoc.Controller{
		Id:     "bob/x",
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
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "foo",
			}},
		},
		Location: map[string]string{
			"cloud":  "aws",
			"region": "foo",
		},
	})

	err = s.database.AddController(testContext, ctl)
	c.Assert(err, gc.ErrorMatches, "already exists")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)

	ctlPath2 := params.EntityPath{"bob", "y"}
	ctl2 := &mongodoc.Controller{
		Id:     "ignored",
		Path:   ctlPath2,
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
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "foo",
			}},
		},
	}
	err = s.database.AddController(testContext, ctl2)
	c.Assert(err, gc.IsNil)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestSetControllerAvailability(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Path: ctlPath,
	}
	err := s.database.AddController(testContext, ctl)

	// Check that we can mark it as unavailable.
	t0 := time.Now()
	err = s.database.SetControllerUnavailableAt(testContext, ctlPath, t0)
	c.Assert(err, gc.IsNil)

	ctl, err = s.database.Controller(testContext, ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.UnavailableSince.UTC(), jc.DeepEquals, mongodoc.Time(t0).UTC())

	// Check that if we mark it unavailable again, it doesn't
	// have any affect.
	err = s.database.SetControllerUnavailableAt(testContext, ctlPath, t0.Add(time.Second))
	c.Assert(err, gc.IsNil)

	ctl, err = s.database.Controller(testContext, ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.UnavailableSince.UTC(), jc.DeepEquals, mongodoc.Time(t0).UTC())

	// Check that we can mark it as available again.
	err = s.database.SetControllerAvailable(testContext, ctlPath)
	c.Assert(err, gc.IsNil)

	ctl, err = s.database.Controller(testContext, ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.UnavailableSince, jc.Satisfies, time.Time.IsZero)

	t1 := t0.Add(3 * time.Second)
	// ... and that we can mark it as unavailable after that.
	err = s.database.SetControllerUnavailableAt(testContext, ctlPath, t1)
	c.Assert(err, gc.IsNil)

	ctl, err = s.database.Controller(testContext, ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.UnavailableSince.UTC(), jc.DeepEquals, mongodoc.Time(t1).UTC())
	s.checkDBOK(c)
}

func (s *databaseSuite) TestSetControllerAvailabilityWithNotFoundController(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	err := s.database.SetControllerUnavailableAt(testContext, ctlPath, time.Now())
	c.Assert(err, gc.IsNil)
	err = s.database.SetControllerAvailable(testContext, ctlPath)
	c.Assert(err, gc.IsNil)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestSetControllerVersion(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Path: ctlPath,
	}
	err := s.database.AddController(testContext, ctl)
	c.Assert(err, gc.IsNil)

	testVersion := version.Number{Minor: 1}
	err = s.database.SetControllerVersion(testContext, ctlPath, testVersion)
	c.Assert(err, gc.IsNil)

	ctl, err = s.database.Controller(testContext, ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.Version, jc.DeepEquals, &testVersion)
}

func (s *databaseSuite) TestSetControllerVersionWithNotFoundController(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	err := s.database.SetControllerVersion(testContext, ctlPath, version.Number{Minor: 1})
	c.Assert(err, gc.IsNil)
}

func (s *databaseSuite) TestSetControllerRegions(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Path: ctlPath,
		Cloud: mongodoc.Cloud{
			Regions: []mongodoc.Region{{
				Name:             "test1",
				Endpoint:         "https://example.com/test1",
				IdentityEndpoint: "https://example.com/test1/identity",
				StorageEndpoint:  "https://example.com/test1/storage",
			}},
		},
	}
	err := s.database.AddController(testContext, ctl)
	c.Assert(err, gc.IsNil)

	testRegions := []mongodoc.Region{{
		Name:             "test2",
		Endpoint:         "https://example.com/test2",
		IdentityEndpoint: "https://example.com/test2/identity",
		StorageEndpoint:  "https://example.com/test2/storage",
	}}
	err = s.database.SetControllerRegions(testContext, ctlPath, testRegions)
	c.Assert(err, gc.IsNil)

	ctl, err = s.database.Controller(testContext, ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.Cloud.Regions, jc.DeepEquals, testRegions)
}

func (s *databaseSuite) TestSetControllerRegionsWithNotFoundController(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	err := s.database.SetControllerRegions(testContext, ctlPath, nil)
	c.Assert(err, gc.IsNil)
}

func (s *databaseSuite) TestDeleteController(c *gc.C) {
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
	err := s.database.AddController(testContext, ctl)
	c.Assert(err, gc.IsNil)
	err = s.database.DeleteController(testContext, ctlPath)
	c.Assert(err, gc.IsNil)

	ctl1, err := s.database.Controller(testContext, ctlPath)
	c.Assert(ctl1, gc.IsNil)
	m1, err := s.database.Model(testContext, ctlPath)
	c.Assert(m1, gc.IsNil)

	err = s.database.DeleteController(testContext, ctlPath)
	c.Assert(err, gc.ErrorMatches, "controller \"dalek/who\" not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// Test with non-existing model.
	ctl2 := &mongodoc.Controller{
		Id:     "dalek/who",
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
	err = s.database.AddController(testContext, ctl2)
	c.Assert(err, gc.IsNil)

	err = s.database.DeleteController(testContext, ctlPath)
	c.Assert(err, gc.IsNil)
	ctl3, err := s.database.Controller(testContext, ctlPath)
	c.Assert(ctl3, gc.IsNil)
	m3, err := s.database.Model(testContext, ctlPath)
	c.Assert(m3, gc.IsNil)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestDeleteModel(c *gc.C) {
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
	err := s.database.AddController(testContext, ctl)
	c.Assert(err, gc.IsNil)

	modelPath := params.EntityPath{"dalek", "exterminate"}
	m2 := &mongodoc.Model{
		Path: modelPath,
	}
	err = s.database.AddModel(testContext, m2)
	c.Assert(err, gc.IsNil)

	err = s.database.DeleteModel(testContext, m2.Path)
	c.Assert(err, gc.IsNil)
	m3, err := s.database.Model(testContext, modelPath)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(m3, gc.IsNil)

	err = s.database.DeleteModel(testContext, m2.Path)
	c.Assert(err, gc.ErrorMatches, "model \"dalek/exterminate\" not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestAddModel(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	m := &mongodoc.Model{
		Id:   "ignored",
		Path: ctlPath,
	}
	err := s.database.AddModel(testContext, m)
	c.Assert(err, gc.IsNil)
	c.Assert(m, jc.DeepEquals, &mongodoc.Model{
		Id:   "bob/x",
		Path: ctlPath,
	})

	m1, err := s.database.Model(testContext, ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(m1, jemtest.CmpEquals(cmpopts.EquateEmpty()), m)

	err = s.database.AddModel(testContext, m)
	c.Assert(err, gc.ErrorMatches, "already exists")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestModelUUIDsForController(c *gc.C) {
	models := []struct {
		path    params.EntityPath
		ctlPath params.EntityPath
		uuid    string
	}{{
		path:    params.EntityPath{"bob", "m1"},
		ctlPath: params.EntityPath{"admin", "ctl1"},
		uuid:    "11111111-1111-1111-1111-111111111111",
	}, {
		path:    params.EntityPath{"bob", "m2"},
		ctlPath: params.EntityPath{"admin", "ctl1"},
		uuid:    "22222222-2222-2222-2222-222222222222",
	}, {
		path:    params.EntityPath{"bob", "m3"},
		ctlPath: params.EntityPath{"admin", "ctl2"},
		uuid:    "33333333-3333-3333-3333-333333333333",
	}}
	for _, m := range models {
		err := s.database.AddModel(testContext, &mongodoc.Model{
			Path:       m.path,
			Controller: m.ctlPath,
			UUID:       m.uuid,
		})
		c.Assert(err, gc.Equals, nil)
	}
	uuids, err := s.database.ModelUUIDsForController(testContext, params.EntityPath{"admin", "ctl1"})
	c.Assert(err, gc.Equals, nil)
	sort.Strings(uuids)
	c.Assert(uuids, jc.DeepEquals, []string{
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
	})
	uuids, err = s.database.ModelUUIDsForController(testContext, params.EntityPath{"admin", "ctl2"})
	c.Assert(err, gc.Equals, nil)
	sort.Strings(uuids)
	c.Assert(uuids, jc.DeepEquals, []string{
		"33333333-3333-3333-3333-333333333333",
	})
}

func (s *databaseSuite) TestUpdateLegacyModel(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	m := &mongodoc.Model{
		Id:   "ignored",
		Path: ctlPath,
	}
	err := s.database.AddModel(testContext, m)
	c.Assert(err, gc.IsNil)
	c.Assert(m, jc.DeepEquals, &mongodoc.Model{
		Id:   "bob/x",
		Path: ctlPath,
	})

	m.Cloud = "bob-cloud"
	m.CloudRegion = "bob-region"
	m.DefaultSeries = "trusty"
	m.Credential = params.CredentialPath{
		Cloud: "bob-cloud",
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "cred",
		},
	}
	err = s.database.UpdateLegacyModel(testContext, m)
	c.Assert(err, gc.IsNil)

	m1, err := s.database.Model(testContext, ctlPath)
	c.Assert(m1, jemtest.CmpEquals(cmpopts.EquateEmpty()), m)

	m2 := &mongodoc.Model{
		Id:   "ignored",
		Path: params.EntityPath{"bob", "y"},
	}
	err = s.database.UpdateLegacyModel(testContext, m2)
	c.Assert(err, gc.ErrorMatches, "cannot update bob/y: not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestModelFromUUID(c *gc.C) {
	uuid := "99999999-9999-9999-9999-999999999999"
	path := params.EntityPath{"bob", "x"}
	m := &mongodoc.Model{
		Id:   "ignored",
		Path: path,
		UUID: uuid,
	}
	err := s.database.AddModel(testContext, m)
	c.Assert(err, gc.IsNil)
	c.Assert(m, jc.DeepEquals, &mongodoc.Model{
		Id:   "bob/x",
		Path: path,
		UUID: uuid,
	})

	m1, err := s.database.ModelFromUUID(testContext, uuid)
	c.Assert(err, gc.IsNil)
	c.Assert(m1, jemtest.CmpEquals(cmpopts.EquateEmpty()), m)

	m2, err := s.database.ModelFromUUID(testContext, "no-such-uuid")
	c.Assert(err, gc.ErrorMatches, `model "no-such-uuid" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(m2, gc.IsNil)
	s.checkDBOK(c)
}

var epoch = parseTime("2016-01-01T12:00:00Z")

func T(n int) time.Time {
	return epoch.Add(time.Duration(n) * time.Millisecond)
}

const leaseExpiryDuration = 15 * time.Second

var acquireLeaseTests = []struct {
	about           string
	now             time.Time
	ctlPath         params.EntityPath
	oldExpiry       time.Time
	oldOwner        string
	newExpiry       time.Time
	newOwner        string
	actualOldExpiry time.Time
	actualOldOwner  string
	expectExpiry    time.Time
	expectError     string
	expectCause     error
}{{
	about:           "initial lease acquisition",
	ctlPath:         params.EntityPath{"bob", "foo"},
	oldExpiry:       time.Time{},
	newExpiry:       epoch.Add(leaseExpiryDuration),
	oldOwner:        "",
	newOwner:        "jem1",
	actualOldExpiry: time.Time{},
	actualOldOwner:  "",
	expectExpiry:    epoch.Add(leaseExpiryDuration),
}, {
	about:           "renewal",
	ctlPath:         params.EntityPath{"bob", "foo"},
	oldExpiry:       epoch.Add(leaseExpiryDuration),
	oldOwner:        "jem1",
	newExpiry:       epoch.Add(leaseExpiryDuration/2 + leaseExpiryDuration),
	newOwner:        "jem1",
	actualOldExpiry: epoch.Add(leaseExpiryDuration),
	actualOldOwner:  "jem1",
	expectExpiry:    epoch.Add(leaseExpiryDuration/2 + leaseExpiryDuration),
}, {
	about:           "renewal with time mismatch",
	ctlPath:         params.EntityPath{"bob", "foo"},
	oldExpiry:       epoch.Add(leaseExpiryDuration),
	oldOwner:        "jem1",
	newExpiry:       epoch.Add(leaseExpiryDuration * 3),
	newOwner:        "jem1",
	actualOldExpiry: epoch.Add(leaseExpiryDuration * 2),
	actualOldOwner:  "jem1",
	expectError:     `controller has lease taken out by "jem1" expiring at 2016-01-01 12:00:30 \+0000 UTC`,
	expectCause:     jem.ErrLeaseUnavailable,
}, {
	about:           "renewal with owner mismatch",
	ctlPath:         params.EntityPath{"bob", "foo"},
	oldExpiry:       epoch.Add(leaseExpiryDuration),
	oldOwner:        "jem1",
	newOwner:        "jem1",
	actualOldExpiry: epoch.Add(leaseExpiryDuration),
	actualOldOwner:  "jem0",
	expectError:     `controller has lease taken out by "jem0" expiring at 2016-01-01 12:00:15 \+0000 UTC`,
	expectCause:     jem.ErrLeaseUnavailable,
}, {
	about:           "drop lease",
	now:             epoch.Add(leaseExpiryDuration / 2),
	ctlPath:         params.EntityPath{"bob", "foo"},
	oldExpiry:       epoch.Add(leaseExpiryDuration),
	oldOwner:        "jem1",
	newOwner:        "",
	actualOldExpiry: epoch.Add(leaseExpiryDuration),
	actualOldOwner:  "jem1",
	expectExpiry:    time.Time{},
}, {
	about:           "drop never-acquired lease",
	now:             epoch,
	ctlPath:         params.EntityPath{"bob", "foo"},
	oldOwner:        "",
	newOwner:        "",
	actualOldExpiry: time.Time{},
	actualOldOwner:  "",
	expectExpiry:    time.Time{},
}}

func (s *databaseSuite) TestAcquireLease(c *gc.C) {
	for i, test := range acquireLeaseTests {
		c.Logf("test %d: %v", i, test.about)
		_, err := s.database.Controllers().RemoveAll(bson.D{{"path", test.ctlPath}})
		c.Assert(err, gc.IsNil)
		_, err = s.database.Models().RemoveAll(bson.D{{"path", test.ctlPath}})
		c.Assert(err, gc.IsNil)
		err = s.database.AddController(testContext, &mongodoc.Controller{
			Path:               test.ctlPath,
			UUID:               "fake-uuid",
			MonitorLeaseOwner:  test.actualOldOwner,
			MonitorLeaseExpiry: test.actualOldExpiry,
		})
		c.Assert(err, gc.IsNil)
		t, err := s.database.AcquireMonitorLease(testContext, test.ctlPath, test.oldExpiry, test.oldOwner, test.newExpiry, test.newOwner)
		if test.expectError != "" {
			if test.expectCause != nil {
				c.Check(errgo.Cause(err), gc.Equals, test.expectCause)
			}
			c.Assert(err, gc.ErrorMatches, test.expectError)
			c.Assert(t, jc.Satisfies, time.Time.IsZero)
			continue
		}
		c.Assert(err, gc.IsNil)
		c.Assert(t.UTC(), gc.DeepEquals, test.expectExpiry.UTC())
		ctl, err := s.database.Controller(testContext, test.ctlPath)
		c.Assert(err, gc.IsNil)
		c.Assert(ctl.MonitorLeaseExpiry.UTC(), gc.DeepEquals, test.expectExpiry.UTC())
		c.Assert(ctl.MonitorLeaseOwner, gc.Equals, test.newOwner)
		s.checkDBOK(c)
	}
}

func (s *databaseSuite) TestSetControllerStatsNotFound(c *gc.C) {
	err := s.database.SetControllerStats(testContext, params.EntityPath{"bob", "foo"}, &mongodoc.ControllerStats{})
	c.Assert(err, gc.ErrorMatches, "controller not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestSetControllerStats(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.AddController(testContext, &mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.IsNil)

	stats := &mongodoc.ControllerStats{
		UnitCount:    1,
		ModelCount:   2,
		ServiceCount: 3,
		MachineCount: 4,
	}
	err = s.database.SetControllerStats(testContext, ctlPath, stats)
	c.Assert(err, gc.IsNil)
	ctl, err := s.database.Controller(testContext, ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.Stats, jc.DeepEquals, *stats)
	s.checkDBOK(c)
}

var updateModelCountsTests = []struct {
	about      string
	before     map[params.EntityCount]params.Count
	update     map[params.EntityCount]int
	updateTime time.Time
	expect     map[params.EntityCount]params.Count
}{{
	about: "empty counts",
	update: map[params.EntityCount]int{
		"foo": 5,
		"bar": 20,
	},
	updateTime: T(1000),
	expect: map[params.EntityCount]params.Count{
		"foo": {
			Time:    T(1000),
			Current: 5,
			Max:     5,
			Total:   5,
		},
		"bar": {
			Time:    T(1000),
			Current: 20,
			Max:     20,
			Total:   20,
		},
	},
}, {
	about: "existing counts",
	before: map[params.EntityCount]params.Count{
		"foo": {
			Time:    T(1000),
			Current: 5,
			Max:     5,
			Total:   5,
		},
		"bar": {
			Time:      T(1000),
			Current:   20,
			Max:       20,
			Total:     100,
			TotalTime: 9000,
		},
		"baz": {
			Time:    T(500),
			Current: 2,
			Max:     3,
			Total:   200,
		},
	},
	updateTime: T(5000),
	update: map[params.EntityCount]int{
		"foo": 2,
		"bar": 50,
	},
	expect: map[params.EntityCount]params.Count{
		"foo": {
			Time:      T(5000),
			Current:   2,
			Max:       5,
			Total:     5,
			TotalTime: (5000 - 1000) * 5,
		},
		"bar": {
			Time:      T(5000),
			Current:   50,
			Max:       50,
			Total:     130,
			TotalTime: 9000 + (5000-1000)*20,
		},
		"baz": {
			Time:    T(500),
			Current: 2,
			Max:     3,
			Total:   200,
		},
	},
}}

func (s *databaseSuite) TestUpdateModelCounts(c *gc.C) {
	for i, test := range updateModelCountsTests {
		c.Logf("test %d: %v", i, test.about)
		modelId := params.EntityPath{"bob", params.Name(fmt.Sprintf("model-%d", i))}
		uuid := fmt.Sprintf("uuid-%d", i)
		err := s.database.AddModel(testContext, &mongodoc.Model{
			Path:       modelId,
			Controller: params.EntityPath{"bob", "controller"},
			UUID:       uuid,
			Counts:     test.before,
		})
		c.Assert(err, gc.IsNil)
		err = s.database.UpdateModelCounts(testContext, uuid, test.update, test.updateTime)
		c.Assert(err, gc.IsNil)
		model, err := s.database.Model(testContext, modelId)
		c.Assert(err, gc.IsNil)
		// Change all times to UTC for straightforward comparison.
		for name, count := range model.Counts {
			count.Time = count.Time.UTC()
			model.Counts[name] = count
		}
		c.Assert(model.Counts, jc.DeepEquals, test.expect)
	}
}

func (s *databaseSuite) TestUpdateModelCountsNotFound(c *gc.C) {
	err := s.database.UpdateModelCounts(testContext, "fake-uuid", nil, T(0))
	c.Assert(err, gc.ErrorMatches, `cannot update model counts: not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *databaseSuite) TestUpdateMachineInfo(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	err := s.database.UpdateMachineInfo(testContext, &mongodoc.Machine{
		Controller: ctlPath,
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &multiwatcher.MachineInfo{
			ModelUUID: "fake-uuid",
			Id:        "0",
			Series:    "quantal",
		},
	})
	c.Assert(err, gc.IsNil)
	err = s.database.UpdateMachineInfo(testContext, &mongodoc.Machine{
		Controller: ctlPath,
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &multiwatcher.MachineInfo{
			ModelUUID: "another-uuid",
			Id:        "0",
			Series:    "blah",
		},
	})
	c.Assert(err, gc.IsNil)
	err = s.database.UpdateMachineInfo(testContext, &mongodoc.Machine{
		Controller: ctlPath,
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &multiwatcher.MachineInfo{
			ModelUUID: "fake-uuid",
			Id:        "1",
			Series:    "precise",
		},
	})
	c.Assert(err, gc.IsNil)

	docs, err := s.database.MachinesForModel(testContext, "fake-uuid")
	c.Assert(err, gc.IsNil)
	for i := range docs {
		cleanMachineDoc(&docs[i])
	}
	c.Assert(docs, jc.DeepEquals, []mongodoc.Machine{{
		Id:         ctlPath.String() + " fake-uuid 0",
		Controller: ctlPath,
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &multiwatcher.MachineInfo{
			ModelUUID: "fake-uuid",
			Id:        "0",
			Series:    "quantal",
			Config:    map[string]interface{}{},
		},
	}, {
		Id:         ctlPath.String() + " fake-uuid 1",
		Controller: ctlPath,
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &multiwatcher.MachineInfo{
			ModelUUID: "fake-uuid",
			Id:        "1",
			Series:    "precise",
			Config:    map[string]interface{}{},
		},
	}})

	// Check that we can update one of the documents.
	err = s.database.UpdateMachineInfo(testContext, &mongodoc.Machine{
		Controller: ctlPath,
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &multiwatcher.MachineInfo{
			ModelUUID: "fake-uuid",
			Id:        "0",
			Series:    "foo",
			Life:      "dying",
		},
	})
	c.Assert(err, gc.IsNil)

	// Check that setting a machine dead removes it.
	err = s.database.UpdateMachineInfo(testContext, &mongodoc.Machine{
		Controller: ctlPath,
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &multiwatcher.MachineInfo{
			ModelUUID: "fake-uuid",
			Id:        "1",
			Series:    "foo",
			Life:      "dead",
		},
	})
	c.Assert(err, gc.IsNil)

	docs, err = s.database.MachinesForModel(testContext, "fake-uuid")
	c.Assert(err, gc.IsNil)
	for i := range docs {
		cleanMachineDoc(&docs[i])
	}
	c.Assert(docs, jc.DeepEquals, []mongodoc.Machine{{
		Id:         ctlPath.String() + " fake-uuid 0",
		Controller: ctlPath,
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &multiwatcher.MachineInfo{
			ModelUUID: "fake-uuid",
			Id:        "0",
			Series:    "foo",
			Config:    map[string]interface{}{},
			Life:      "dying",
		},
	}})
}

// cleanMachineDoc cleans up the machine document so
// that we can use a DeepEqual comparison without worrying
// about non-nil vs nil map comparisons.
func cleanMachineDoc(doc *mongodoc.Machine) {
	if len(doc.Info.AgentStatus.Data) == 0 {
		doc.Info.AgentStatus.Data = nil
	}
	if len(doc.Info.InstanceStatus.Data) == 0 {
		doc.Info.InstanceStatus.Data = nil
	}
}

func (s *databaseSuite) TestSetModelControllerNotFound(c *gc.C) {
	err := s.database.SetModelController(testContext, params.EntityPath{"bob", "foo"}, params.EntityPath{"x", "y"})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *databaseSuite) TestSetModelControllerSuccess(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.AddController(testContext, &mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.IsNil)

	modelPath := params.EntityPath{"bob", "foo"}
	err = s.database.AddModel(testContext, &mongodoc.Model{
		Path:       modelPath,
		UUID:       "fake-uuid",
		Controller: params.EntityPath{"bob", "foo"},
	})
	c.Assert(err, gc.IsNil)
	origDoc, err := s.database.Model(testContext, modelPath)
	c.Assert(err, gc.IsNil)

	err = s.database.SetModelController(testContext, params.EntityPath{"bob", "foo"}, params.EntityPath{"x", "y"})
	c.Assert(err, gc.Equals, nil)

	newDoc, err := s.database.Model(testContext, modelPath)
	c.Assert(err, gc.Equals, nil)

	origDoc.Controller = params.EntityPath{"x", "y"}

	c.Assert(newDoc, gc.DeepEquals, origDoc)
}

func (s *databaseSuite) TestSetModelLifeNotFound(c *gc.C) {
	err := s.database.SetModelLife(testContext, params.EntityPath{"bob", "foo"}, "fake-uuid", "alive")
	c.Assert(err, gc.IsNil)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestSetModelInfoNotFound(c *gc.C) {
	err := s.database.SetModelInfo(testContext, params.EntityPath{"bob", "foo"}, "fake-uuid", &mongodoc.ModelInfo{})
	c.Assert(err, gc.IsNil)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestSetControllerDeprecated(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}

	err := s.database.SetControllerDeprecated(testContext, ctlPath, true)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	err = s.database.SetControllerDeprecated(testContext, ctlPath, false)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	err = s.database.AddController(testContext, &mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.IsNil)

	// When first added, the deprecated field is not present.
	var doc map[string]interface{}
	err = s.database.Controllers().FindId(ctlPath.String()).One(&doc)
	c.Assert(err, gc.Equals, nil)
	_, ok := doc["deprecated"]
	c.Assert(ok, gc.Equals, false)

	// Set the controller to deprecated and check that the field
	// is set to true.
	err = s.database.SetControllerDeprecated(testContext, ctlPath, true)
	c.Assert(err, gc.Equals, nil)

	doc = nil
	err = s.database.Controllers().FindId(ctlPath.String()).One(&doc)
	c.Assert(err, gc.Equals, nil)
	c.Assert(doc["deprecated"], gc.Equals, true)

	// Check that we've used the right field name by unmarshaling into
	// the usual document.
	ctl, err := s.database.Controller(testContext, ctlPath)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ctl.Deprecated, gc.Equals, true)

	// Set it back to non-deprecated and check that the field is removed.
	err = s.database.SetControllerDeprecated(testContext, ctlPath, false)
	c.Assert(err, gc.Equals, nil)

	doc = nil
	err = s.database.Controllers().FindId(ctlPath.String()).One(&doc)
	c.Assert(err, gc.Equals, nil)
	_, ok = doc["deprecated"]
	c.Assert(ok, gc.Equals, false)
}

func (s *databaseSuite) TestSetModelLifeSuccess(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.AddController(testContext, &mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.IsNil)

	// Add the controller model.
	err = s.database.AddModel(testContext, &mongodoc.Model{
		Path:       params.EntityPath{"bob", "foo"},
		UUID:       "fake-uuid",
		Controller: params.EntityPath{"bob", "foo"},
	})
	c.Assert(err, gc.IsNil)

	// Add another model with the same UUID but a different controller.
	err = s.database.AddModel(testContext, &mongodoc.Model{
		Path:       params.EntityPath{"bar", "baz"},
		UUID:       "fake-uuid",
		Controller: params.EntityPath{"bar", "zzz"},
	})
	c.Assert(err, gc.IsNil)

	// Add another model with the same controller but a different UUID.
	err = s.database.AddModel(testContext, &mongodoc.Model{
		Path:       params.EntityPath{"alice", "baz"},
		UUID:       "another-uuid",
		Controller: ctlPath,
	})
	c.Assert(err, gc.IsNil)

	err = s.database.SetModelLife(testContext, ctlPath, "fake-uuid", "alive")
	c.Assert(err, gc.IsNil)

	m, err := s.database.Model(testContext, ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life(), gc.Equals, "alive")

	m, err = s.database.Model(testContext, params.EntityPath{"bar", "baz"})
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life(), gc.Equals, "")

	m, err = s.database.Model(testContext, params.EntityPath{"alice", "baz"})
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life(), gc.Equals, "")
	s.checkDBOK(c)
}

func (s *databaseSuite) TestSetModelInfoSuccess(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.AddController(testContext, &mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.IsNil)

	// Add the controller model.
	err = s.database.AddModel(testContext, &mongodoc.Model{
		Path:       params.EntityPath{"bob", "foo"},
		UUID:       "fake-uuid",
		Controller: params.EntityPath{"bob", "foo"},
	})
	c.Assert(err, gc.IsNil)

	// Add another model with the same UUID but a different controller.
	err = s.database.AddModel(testContext, &mongodoc.Model{
		Path:       params.EntityPath{"bar", "baz"},
		UUID:       "fake-uuid",
		Controller: params.EntityPath{"bar", "zzz"},
	})
	c.Assert(err, gc.IsNil)

	// Add another model with the same controller but a different UUID.
	err = s.database.AddModel(testContext, &mongodoc.Model{
		Path:       params.EntityPath{"alice", "baz"},
		UUID:       "another-uuid",
		Controller: ctlPath,
	})
	c.Assert(err, gc.IsNil)

	err = s.database.SetModelInfo(testContext, ctlPath, "fake-uuid", &mongodoc.ModelInfo{
		Life: "alive",
	})
	c.Assert(err, gc.IsNil)

	m, err := s.database.Model(testContext, ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life(), gc.Equals, "alive")

	m, err = s.database.Model(testContext, params.EntityPath{"bar", "baz"})
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life(), gc.Equals, "")

	m, err = s.database.Model(testContext, params.EntityPath{"alice", "baz"})
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life(), gc.Equals, "")
	s.checkDBOK(c)
}

func (s *databaseSuite) TestDeleteModelWithUUID(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.AddController(testContext, &mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.IsNil)

	// Add the controller model.
	err = s.database.AddModel(testContext, &mongodoc.Model{
		Path:       params.EntityPath{"bob", "foo"},
		UUID:       "fake-uuid",
		Controller: params.EntityPath{"bob", "foo"},
	})
	c.Assert(err, gc.IsNil)

	err = s.database.DeleteModelWithUUID(testContext, ctlPath, "fake-uuid")
	c.Assert(err, gc.IsNil)

	_, err = s.database.Model(testContext, ctlPath)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *databaseSuite) TestAcquireLeaseControllerNotFound(c *gc.C) {
	_, err := s.database.AcquireMonitorLease(testContext, params.EntityPath{"bob", "foo"}, time.Time{}, "", time.Now(), "jem1")
	c.Assert(err, gc.ErrorMatches, `controller removed`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestAddAndGetCredential(c *gc.C) {
	path := credentialPath("test-cloud", "test-user", "test-credential")
	expectId := path.String()
	cred, err := s.database.Credential(testContext, path)
	c.Assert(cred, gc.IsNil)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(err, gc.ErrorMatches, `credential "test-cloud/test-user/test-credential" not found`)

	attrs := map[string]string{
		"attr1": "val1",
		"attr2": "val2",
	}
	err = jem.UpdateCredential(s.database, testContext, &mongodoc.Credential{
		Path:       path,
		Type:       "credtype",
		Label:      "Test Label",
		Attributes: attrs,
	})
	c.Assert(err, gc.IsNil)

	cred, err = s.database.Credential(testContext, path)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:         expectId,
		Path:       path,
		Type:       "credtype",
		Label:      "Test Label",
		Attributes: attrs,
	})

	err = jem.UpdateCredential(s.database, testContext, &mongodoc.Credential{
		Path:       path,
		Type:       "credtype",
		Label:      "Test Label 2",
		Attributes: attrs,
	})
	c.Assert(err, gc.IsNil)

	cred, err = s.database.Credential(testContext, path)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:         expectId,
		Path:       path,
		Type:       "credtype",
		Label:      "Test Label 2",
		Attributes: attrs,
	})

	err = jem.UpdateCredential(s.database, testContext, &mongodoc.Credential{
		Path:    path,
		Revoked: true,
	})
	c.Assert(err, gc.IsNil)
	cred, err = s.database.Credential(testContext, path)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:         expectId,
		Path:       path,
		Attributes: map[string]string{},
		Revoked:    true,
	})
	s.checkDBOK(c)
}

func (s *databaseSuite) TestCredentialAddController(c *gc.C) {
	path := credentialPath("test-cloud", "test-user", "test-credential")
	expectId := path.String()
	err := jem.UpdateCredential(s.database, testContext, &mongodoc.Credential{
		Path: path,
		Type: "empty",
	})
	c.Assert(err, gc.IsNil)

	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Path: ctlPath,
	}
	err = s.database.AddController(testContext, ctl)
	c.Assert(err, gc.IsNil)

	err = jem.CredentialAddController(s.database, testContext, path, ctlPath)
	c.Assert(err, gc.IsNil)

	cred, err := s.database.Credential(testContext, path)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:         expectId,
		Path:       path,
		Type:       "empty",
		Attributes: map[string]string{},
		Controllers: []params.EntityPath{
			ctlPath,
		},
	})

	// Add a second time
	err = jem.CredentialAddController(s.database, testContext, path, ctlPath)
	c.Assert(err, gc.IsNil)

	cred, err = s.database.Credential(testContext, path)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:         expectId,
		Path:       path,
		Type:       "empty",
		Attributes: map[string]string{},
		Controllers: []params.EntityPath{
			ctlPath,
		},
	})
	path2 := params.CredentialPath{
		Cloud: "test-cloud",
		EntityPath: params.EntityPath{
			User: "test-user",
			Name: "no-such-cred",
		},
	}
	// Add to a non-existant credential
	err = jem.CredentialAddController(s.database, testContext, path2, ctlPath)
	c.Assert(err, gc.ErrorMatches, `credential "test-cloud/test-user/no-such-cred" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestCredentialRemoveController(c *gc.C) {
	path := credentialPath("test-cloud", "test-user", "test-credential")
	expectId := path.String()
	err := jem.UpdateCredential(s.database, testContext, &mongodoc.Credential{
		Path: path,
		Type: "empty",
	})
	c.Assert(err, gc.IsNil)

	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Path: ctlPath,
	}
	err = s.database.AddController(testContext, ctl)
	c.Assert(err, gc.IsNil)

	err = jem.CredentialAddController(s.database, testContext, path, ctlPath)
	c.Assert(err, gc.IsNil)

	// sanity check the controller is there.
	cred, err := s.database.Credential(testContext, path)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:         expectId,
		Path:       path,
		Type:       "empty",
		Attributes: map[string]string{},
		Controllers: []params.EntityPath{
			ctlPath,
		},
	})

	err = jem.CredentialRemoveController(s.database, testContext, path, ctlPath)
	c.Assert(err, gc.IsNil)

	cred, err = s.database.Credential(testContext, path)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:         expectId,
		Path:       path,
		Type:       "empty",
		Attributes: map[string]string{},
	})

	// Remove again
	err = jem.CredentialRemoveController(s.database, testContext, path, ctlPath)
	c.Assert(err, gc.IsNil)

	cred, err = s.database.Credential(testContext, path)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:         expectId,
		Path:       path,
		Type:       "empty",
		Attributes: map[string]string{},
	})
	path2 := credentialPath("test-cloud", "test-user", "no-such-cred")
	// remove from a non-existant credential
	err = jem.CredentialRemoveController(s.database, testContext, path2, ctlPath)
	c.Assert(err, gc.ErrorMatches, `credential "test-cloud/test-user/no-such-cred" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestSetACL(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.AddController(testContext, &mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.IsNil)

	err = s.database.SetACL(testContext, s.database.Controllers(), ctlPath, params.ACL{
		Read: []string{"t1", "t2"},
	})
	c.Assert(err, gc.IsNil)
	var cnt mongodoc.Controller
	err = s.database.Controllers().FindId(ctlPath.String()).One(&cnt)
	c.Assert(err, gc.IsNil)
	c.Assert(cnt.ACL, jc.DeepEquals, params.ACL{
		Read: []string{"t1", "t2"},
	})

	err = s.database.SetACL(testContext, s.database.Controllers(), params.EntityPath{"bob", "bar"}, params.ACL{
		Read: []string{"t2", "t1"},
	})
	c.Assert(err, gc.ErrorMatches, `"bob/bar" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestGrant(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.AddController(testContext, &mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.IsNil)

	err = s.database.Grant(testContext, s.database.Controllers(), ctlPath, "t1")
	c.Assert(err, gc.IsNil)
	var cnt mongodoc.Controller
	err = s.database.Controllers().FindId(ctlPath.String()).One(&cnt)
	c.Assert(err, gc.IsNil)
	c.Assert(cnt.ACL, jc.DeepEquals, params.ACL{
		Read: []string{"t1"},
	})

	err = s.database.Grant(testContext, s.database.Controllers(), params.EntityPath{"bob", "bar"}, "t1")
	c.Assert(err, gc.ErrorMatches, `"bob/bar" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestRevoke(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.AddController(testContext, &mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.IsNil)

	err = s.database.SetACL(testContext, s.database.Controllers(), ctlPath, params.ACL{
		Read: []string{"t1", "t2"},
	})
	c.Assert(err, gc.IsNil)
	err = s.database.Revoke(testContext, s.database.Controllers(), ctlPath, "t2")
	c.Assert(err, gc.IsNil)
	var cnt mongodoc.Controller
	err = s.database.Controllers().FindId(ctlPath.String()).One(&cnt)
	c.Assert(err, gc.IsNil)
	c.Assert(cnt.ACL, jc.DeepEquals, params.ACL{
		Read: []string{"t1"},
	})

	err = s.database.Revoke(testContext, s.database.Controllers(), params.EntityPath{"bob", "bar"}, "t2")
	c.Assert(err, gc.ErrorMatches, `"bob/bar" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestCloud(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.AddController(testContext, &mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
		Cloud: mongodoc.Cloud{
			Name:         "my-cloud",
			ProviderType: "ec2",
		},
	})
	c.Assert(err, gc.IsNil)
	cld, err := s.database.Cloud(testContext, "my-cloud")
	c.Assert(err, gc.IsNil)
	c.Assert(cld, jc.DeepEquals, &mongodoc.Cloud{
		Name:         "my-cloud",
		ProviderType: "ec2",
	})
	cld, err = s.database.Cloud(testContext, "not-my-cloud")
	c.Assert(err, gc.ErrorMatches, `cloud "not-my-cloud" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestGetACL(c *gc.C) {
	m := &mongodoc.Model{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "model",
		},
		ACL: params.ACL{
			Read: []string{"fred", "jim"},
		},
	}
	err := s.database.AddModel(testContext, m)
	c.Assert(err, gc.IsNil)
	acl, err := s.database.GetACL(testContext, s.database.Models(), m.Path)
	c.Assert(err, gc.IsNil)
	c.Assert(acl, jc.DeepEquals, m.ACL)
}

func (s *databaseSuite) TestGetACLNotFound(c *gc.C) {
	m := &mongodoc.Model{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "model",
		},
	}
	acl, err := s.database.GetACL(testContext, s.database.Models(), m.Path)
	c.Assert(err, gc.ErrorMatches, "not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(acl, jc.DeepEquals, m.ACL)
}

var checkReadACLTests = []struct {
	about            string
	owner            string
	acl              []string
	user             string
	groups           []string
	skipCreateEntity bool
	expectError      string
	expectCause      error
}{{
	about: "user is owner",
	owner: "bob",
	user:  "bob",
}, {
	about:  "owner is user group",
	owner:  "bobgroup",
	user:   "bob",
	groups: []string{"bobgroup"},
}, {
	about: "acl contains user",
	owner: "fred",
	acl:   []string{"bob"},
	user:  "bob",
}, {
	about:  "acl contains user's group",
	owner:  "fred",
	acl:    []string{"bobgroup"},
	user:   "bob",
	groups: []string{"bobgroup"},
}, {
	about:       "user not in acl",
	owner:       "fred",
	acl:         []string{"fredgroup"},
	user:        "bob",
	expectError: "unauthorized",
	expectCause: params.ErrUnauthorized,
}, {
	about:            "no entity and not owner",
	owner:            "fred",
	user:             "bob",
	skipCreateEntity: true,
	expectError:      "unauthorized",
	expectCause:      params.ErrUnauthorized,
}}

func (s *databaseSuite) TestCheckReadACL(c *gc.C) {
	for i, test := range checkReadACLTests {
		c.Logf("%d. %s", i, test.about)
		ctx := auth.ContextWithUser(testContext, test.user, test.groups...)
		entity := params.EntityPath{
			User: params.User(test.owner),
			Name: params.Name(fmt.Sprintf("test%d", i)),
		}
		if !test.skipCreateEntity {
			err := s.database.AddModel(ctx, &mongodoc.Model{
				Path: entity,
				ACL: params.ACL{
					Read: test.acl,
				},
			})
			c.Assert(err, gc.IsNil)
		}
		err := s.database.CheckReadACL(ctx, s.database.Models(), entity)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			if test.expectCause != nil {
				c.Assert(errgo.Cause(err), gc.Equals, test.expectCause)
			} else {
				c.Assert(errgo.Cause(err), gc.Equals, err)
			}
		} else {
			c.Assert(err, gc.IsNil)
		}
	}
}

func (s *databaseSuite) TestCanReadIter(c *gc.C) {
	testModels := []mongodoc.Model{{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "m1",
		},
	}, {
		Path: params.EntityPath{
			User: params.User("fred"),
			Name: "m2",
		},
	}, {
		Path: params.EntityPath{
			User: params.User("fred"),
			Name: "m3",
		},
		ACL: params.ACL{
			Read: []string{"bob"},
		},
	}}
	for i := range testModels {
		err := s.database.AddModel(testContext, &testModels[i])
		c.Assert(err, gc.IsNil)
	}
	ctx := auth.ContextWithUser(testContext, "bob", "bob-group")
	it := s.database.Models().Find(nil).Sort("_id").Iter()
	crit := s.database.NewCanReadIter(ctx, it)
	var models []mongodoc.Model
	var m mongodoc.Model
	for crit.Next(&m) {
		models = append(models, m)
	}
	c.Assert(crit.Err(), gc.IsNil)
	c.Assert(models, jemtest.CmpEquals(cmpopts.EquateEmpty()), []mongodoc.Model{
		testModels[0],
		testModels[2],
	})
}

var (
	fakeEntityPath = params.EntityPath{"bob", "foo"}
	fakeCredPath   = credentialPath("test-cloud", "test-user", "test-credential")
)

var setDeadTests = []struct {
	about string
	run   func(db *jem.Database)
}{{
	about: "AddController",
	run: func(db *jem.Database) {
		db.AddController(testContext, &mongodoc.Controller{
			Path: params.EntityPath{"bob", "foo"},
			UUID: "fake-uuid",
			Cloud: mongodoc.Cloud{
				Name:         "my-cloud",
				ProviderType: "ec2",
			},
		})
	},
}, {
	about: "AddModel",
	run: func(db *jem.Database) {
		db.AddModel(testContext, &mongodoc.Model{
			Path: fakeEntityPath,
		})
	},
}, {
	about: "AcquireMonitorLease",
	run: func(db *jem.Database) {
		db.AcquireMonitorLease(testContext, fakeEntityPath, time.Now(), "foo", time.Now(), "bar")
	},
}, {
	about: "CanReadIter",
	run: func(db *jem.Database) {
		it := db.Models().Find(nil).Sort("_id").Iter()
		ctx := auth.ContextWithUser(testContext, "bob", "bob-group")
		crit := db.NewCanReadIter(ctx, it)
		crit.Next(&mongodoc.Model{})
		crit.Err()
	},
}, {
	about: "CanReadIter with Close",
	run: func(db *jem.Database) {
		it := db.Models().Find(nil).Sort("_id").Iter()
		ctx := auth.ContextWithUser(testContext, "bob", "bob-group")
		crit := db.NewCanReadIter(ctx, it)
		crit.Next(&mongodoc.Model{})
		crit.Close()
	},
}, {
	about: "clearCredentialUpdate",
	run: func(db *jem.Database) {
		jem.ClearCredentialUpdate(db, testContext, fakeEntityPath, fakeCredPath)
	},
}, {
	about: "Cloud",
	run: func(db *jem.Database) {
		db.Cloud(testContext, "my-cloud")
	},
}, {
	about: "Controller",
	run: func(db *jem.Database) {
		db.Controller(testContext, fakeEntityPath)
	},
}, {
	about: "Credential",
	run: func(db *jem.Database) {
		db.Credential(testContext, fakeCredPath)
	},
}, {
	about: "credentialAddController",
	run: func(db *jem.Database) {
		jem.CredentialAddController(db, testContext, fakeCredPath, fakeEntityPath)
	},
}, {
	about: "credentialRemoveController",
	run: func(db *jem.Database) {
		jem.CredentialRemoveController(db, testContext, fakeCredPath, fakeEntityPath)
	},
}, {
	about: "DeleteModel",
	run: func(db *jem.Database) {
		db.DeleteModel(testContext, fakeEntityPath)
	},
}, {
	about: "GetACL",
	run: func(db *jem.Database) {
		db.GetACL(testContext, db.Models(), fakeEntityPath)
	},
}, {
	about: "Grant",
	run: func(db *jem.Database) {
		db.Grant(testContext, db.Controllers(), fakeEntityPath, "t1")
	},
}, {
	about: "Model",
	run: func(db *jem.Database) {
		db.Model(testContext, fakeEntityPath)
	},
}, {
	about: "MachinesForModel",
	run: func(db *jem.Database) {
		db.Model(testContext, fakeEntityPath)
	},
}, {
	about: "ModelFromUUID",
	run: func(db *jem.Database) {
		db.ModelFromUUID(testContext, "99999999-9999-9999-9999-999999999999")
	},
}, {
	about: "Revoke",
	run: func(db *jem.Database) {
		db.Revoke(testContext, db.Controllers(), fakeEntityPath, "t1")
	},
}, {
	about: "SetACL",
	run: func(db *jem.Database) {
		db.SetACL(testContext, db.Models(), fakeEntityPath, params.ACL{
			Read: []string{"t1", "t2"},
		})
	},
}, {
	about: "SetControllerAvailable",
	run: func(db *jem.Database) {
		db.SetControllerAvailable(testContext, fakeEntityPath)
	},
}, {
	about: "SetControllerStats",
	run: func(db *jem.Database) {
		db.SetControllerStats(testContext, fakeEntityPath, &mongodoc.ControllerStats{})
	},
}, {
	about: "SetControllerUnavailableAt",
	run: func(db *jem.Database) {
		db.SetControllerUnavailableAt(testContext, fakeEntityPath, time.Now())
	},
}, {
	about: "SetControllerVersion",
	run: func(db *jem.Database) {
		db.SetControllerVersion(testContext, fakeEntityPath, version.Number{})
	},
}, {
	about: "setCredentialUpdates",
	run: func(db *jem.Database) {
		jem.SetCredentialUpdates(db, testContext, []params.EntityPath{fakeEntityPath}, fakeCredPath)
	},
}, {
	about: "SetModelInfo",
	run: func(db *jem.Database) {
		db.SetModelInfo(testContext, fakeEntityPath, "fake-uuid", &mongodoc.ModelInfo{})
	},
}, {
	about: "SetModelLife",
	run: func(db *jem.Database) {
		db.SetModelLife(testContext, fakeEntityPath, "fake-uuid", "fake-life")
	},
}, {
	about: "SetModelController",
	run: func(db *jem.Database) {
		db.SetModelController(testContext, fakeEntityPath, fakeEntityPath)
	},
}, {
	about: "UpdateCredential",
	run: func(db *jem.Database) {
		jem.UpdateCredential(db, testContext, &mongodoc.Credential{
			Path:  fakeCredPath,
			Type:  "credtype",
			Label: "Test Label",
		})
	},
}, {
	about: "UpdateMachineInfo",
	run: func(db *jem.Database) {
		db.UpdateMachineInfo(testContext, &mongodoc.Machine{
			Controller: params.EntityPath{"test", "test"},
			Info: &multiwatcher.MachineInfo{
				ModelUUID: "xxx",
				Id:        "yyy",
			},
		})
	},
}, {
	about: "UpdateModelCounts",
	run: func(db *jem.Database) {
		db.UpdateModelCounts(testContext, "fake-uuid", nil, T(0))
	},
}}

func (s *databaseSuite) TestSetDead(c *gc.C) {
	session := jujutesting.NewProxiedSession(c)
	defer session.Close()

	pool := mgosession.NewPool(context.TODO(), session.Session, 1)
	defer pool.Close()
	for i, test := range setDeadTests {
		c.Logf("test %d: %s", i, test.about)
		testSetDead(c, session.TCPProxy, pool, test.run)
	}
}

func testSetDead(c *gc.C, proxy *jujutesting.TCPProxy, pool *mgosession.Pool, run func(db *jem.Database)) {
	db := jem.NewDatabase(context.TODO(), pool, "jem")
	defer db.Session.Close()
	// Use the session so that it's bound to the socket.
	err := db.Session.Ping()
	c.Assert(err, gc.IsNil)
	proxy.CloseConns() // Close the existing socket so that the connection is broken.

	// Sanity check that getting another session from the pool also
	// gives us a broken session (note that we know that the
	// pool only contains one session).
	s1 := pool.Session(context.TODO())
	defer s1.Close()
	c.Assert(s1.Ping(), gc.NotNil)

	run(db)

	// Check that another session from the pool is OK to use now
	// because the operation has reset the pool.
	s2 := pool.Session(context.TODO())
	defer s2.Close()
	c.Assert(s2.Ping(), gc.Equals, nil)
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func credentialPath(cloud, user, name string) params.CredentialPath {
	return params.CredentialPath{
		Cloud: params.Cloud(cloud),
		EntityPath: params.EntityPath{
			User: params.User(user),
			Name: params.Name(name),
		},
	}
}

func (s *databaseSuite) TestGetModelStatuses(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	m := &mongodoc.Model{
		Id:   "ignored",
		Path: ctlPath,
	}
	err := s.database.AddModel(testContext, m)
	c.Assert(err, gc.IsNil)
	c.Assert(m, jc.DeepEquals, &mongodoc.Model{
		Id:   "bob/x",
		Path: ctlPath,
	})

	m1, err := s.database.Model(testContext, ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(m1, jemtest.CmpEquals(cmpopts.EquateEmpty()), m)
	s.checkDBOK(c)

	st, err := s.database.GetModelStatuses(testContext)
	c.Assert(err, gc.IsNil)
	c.Assert(st, gc.DeepEquals, params.ModelStatuses{{
		Status:     "unknown",
		ID:         "bob/x",
		Controller: "/",
	}})
}
