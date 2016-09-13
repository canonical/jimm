// Copyright 2016 Canonical Ltd.

package jem_test

import (
	"fmt"
	"sync"
	"time"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type databaseSuite struct {
	jujutesting.IsolatedMgoSuite
	database jem.Database
	gauge    gauge
}

var _ = gc.Suite(&databaseSuite{})

func (s *databaseSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	s.gauge.n = 1
	s.database = jem.MakeDatabase(s.Session.DB("jem"), &s.gauge)
}

func (s *databaseSuite) TestAddController(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Id:            "ignored",
		Path:          ctlPath,
		CACert:        "certainly",
		HostPorts:     []string{"host1:1234", "host2:9999"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "foo",
			}},
		},
	}
	err := s.database.AddController(ctl)
	c.Assert(err, gc.IsNil)

	// Check that the fields have been mutated as expected.
	c.Assert(ctl, jc.DeepEquals, &mongodoc.Controller{
		Id:            "bob/x",
		Path:          ctlPath,
		CACert:        "certainly",
		HostPorts:     []string{"host1:1234", "host2:9999"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "foo",
			}},
		},
	})

	ctl1, err := s.database.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl1, jc.DeepEquals, &mongodoc.Controller{
		Id:            "bob/x",
		Path:          ctlPath,
		CACert:        "certainly",
		HostPorts:     []string{"host1:1234", "host2:9999"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "foo",
			}},
		},
	})

	err = s.database.AddController(ctl)
	c.Assert(err, gc.ErrorMatches, "already exists")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)

	ctlPath2 := params.EntityPath{"bob", "y"}
	ctl2 := &mongodoc.Controller{
		Id:            "ignored",
		Path:          ctlPath2,
		CACert:        "certainly",
		HostPorts:     []string{"host1:1234", "host2:9999"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "foo",
			}},
		},
	}
	err = s.database.AddController(ctl2)
	c.Assert(err, gc.IsNil)
}

func (s *databaseSuite) TestSetControllerAvailability(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Path: ctlPath,
	}
	err := s.database.AddController(ctl)

	// Check that we can mark it as unavailable.
	t0 := time.Now()
	err = s.database.SetControllerUnavailableAt(ctlPath, t0)
	c.Assert(err, gc.IsNil)

	ctl, err = s.database.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.UnavailableSince.UTC(), jc.DeepEquals, mongodoc.Time(t0).UTC())

	// Check that if we mark it unavailable again, it doesn't
	// have any affect.
	err = s.database.SetControllerUnavailableAt(ctlPath, t0.Add(time.Second))
	c.Assert(err, gc.IsNil)

	ctl, err = s.database.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.UnavailableSince.UTC(), jc.DeepEquals, mongodoc.Time(t0).UTC())

	// Check that we can mark it as available again.
	err = s.database.SetControllerAvailable(ctlPath)
	c.Assert(err, gc.IsNil)

	ctl, err = s.database.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.UnavailableSince, jc.Satisfies, time.Time.IsZero)

	t1 := t0.Add(3 * time.Second)
	// ... and that we can mark it as unavailable after that.
	err = s.database.SetControllerUnavailableAt(ctlPath, t1)
	c.Assert(err, gc.IsNil)

	ctl, err = s.database.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.UnavailableSince.UTC(), jc.DeepEquals, mongodoc.Time(t1).UTC())
}

func (s *databaseSuite) TestSetControllerAvailabilityWithNotFoundController(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	err := s.database.SetControllerUnavailableAt(ctlPath, time.Now())
	c.Assert(err, gc.IsNil)
	err = s.database.SetControllerAvailable(ctlPath)
	c.Assert(err, gc.IsNil)
}

func (s *databaseSuite) TestDeleteController(c *gc.C) {
	ctlPath := params.EntityPath{"dalek", "who"}
	ctl := &mongodoc.Controller{
		Id:            "ignored",
		Path:          ctlPath,
		CACert:        "certainly",
		HostPorts:     []string{"host1:1234", "host2:9999"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	err := s.database.AddController(ctl)
	c.Assert(err, gc.IsNil)
	err = s.database.DeleteController(ctlPath)
	c.Assert(err, gc.IsNil)

	ctl1, err := s.database.Controller(ctlPath)
	c.Assert(ctl1, gc.IsNil)
	m1, err := s.database.Model(ctlPath)
	c.Assert(m1, gc.IsNil)

	err = s.database.DeleteController(ctlPath)
	c.Assert(err, gc.ErrorMatches, "controller \"dalek/who\" not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// Test with non-existing model.
	ctl2 := &mongodoc.Controller{
		Id:            "dalek/who",
		Path:          ctlPath,
		CACert:        "certainly",
		HostPorts:     []string{"host1:1234", "host2:9999"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	err = s.database.AddController(ctl2)
	c.Assert(err, gc.IsNil)

	err = s.database.DeleteController(ctlPath)
	c.Assert(err, gc.IsNil)
	ctl3, err := s.database.Controller(ctlPath)
	c.Assert(ctl3, gc.IsNil)
	m3, err := s.database.Model(ctlPath)
	c.Assert(m3, gc.IsNil)
}

func (s *databaseSuite) TestDeleteModel(c *gc.C) {
	ctlPath := params.EntityPath{"dalek", "who"}
	ctl := &mongodoc.Controller{
		Id:            "ignored",
		Path:          ctlPath,
		CACert:        "certainly",
		HostPorts:     []string{"host1:1234", "host2:9999"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	err := s.database.AddController(ctl)
	c.Assert(err, gc.IsNil)

	modelPath := params.EntityPath{"dalek", "exterminate"}
	m2 := &mongodoc.Model{
		Id:   "dalek/exterminate",
		Path: modelPath,
	}
	err = s.database.AddModel(m2)
	c.Assert(err, gc.IsNil)

	err = s.database.DeleteModel(m2.Path)
	c.Assert(err, gc.IsNil)
	m3, err := s.database.Model(modelPath)
	c.Assert(m3, gc.IsNil)

	err = s.database.DeleteModel(m2.Path)
	c.Assert(err, gc.ErrorMatches, "model \"dalek/exterminate\" not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *databaseSuite) TestAddModel(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	m := &mongodoc.Model{
		Id:   "ignored",
		Path: ctlPath,
	}
	err := s.database.AddModel(m)
	c.Assert(err, gc.IsNil)
	c.Assert(m, jc.DeepEquals, &mongodoc.Model{
		Id:   "bob/x",
		Path: ctlPath,
	})

	m1, err := s.database.Model(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(m1, jc.DeepEquals, m)

	err = s.database.AddModel(m)
	c.Assert(err, gc.ErrorMatches, "already exists")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)
}

func (s *databaseSuite) TestModelFromUUID(c *gc.C) {
	uuid := "99999999-9999-9999-9999-999999999999"
	path := params.EntityPath{"bob", "x"}
	m := &mongodoc.Model{
		Id:   "ignored",
		Path: path,
		UUID: uuid,
	}
	err := s.database.AddModel(m)
	c.Assert(err, gc.IsNil)
	c.Assert(m, jc.DeepEquals, &mongodoc.Model{
		Id:   "bob/x",
		Path: path,
		UUID: uuid,
	})

	m1, err := s.database.ModelFromUUID(uuid)
	c.Assert(err, gc.IsNil)
	c.Assert(m1, jc.DeepEquals, m)

	m2, err := s.database.ModelFromUUID("no-such-uuid")
	c.Assert(err, gc.ErrorMatches, `model "no-such-uuid" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(m2, gc.IsNil)
}

func (s *databaseSuite) TestCopy(c *gc.C) {
	db := s.database.Copy()
	c.Assert(s.gauge.n, gc.Equals, 2)
	db.Close()
	c.Assert(s.gauge.n, gc.Equals, 1)
	_, err := s.database.Model(params.EntityPath{"bob", "x"})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *databaseSuite) TestClone(c *gc.C) {
	db := s.database.Clone()
	c.Assert(s.gauge.n, gc.Equals, 2)
	db.Close()
	c.Assert(s.gauge.n, gc.Equals, 1)
	_, err := s.database.Model(params.EntityPath{"bob", "x"})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

var epoch = parseTime("2016-01-01T12:00:00Z")

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
		err = s.database.AddController(&mongodoc.Controller{
			Path:               test.ctlPath,
			UUID:               "fake-uuid",
			MonitorLeaseOwner:  test.actualOldOwner,
			MonitorLeaseExpiry: test.actualOldExpiry,
		})
		c.Assert(err, gc.IsNil)
		t, err := s.database.AcquireMonitorLease(test.ctlPath, test.oldExpiry, test.oldOwner, test.newExpiry, test.newOwner)
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
		ctl, err := s.database.Controller(test.ctlPath)
		c.Assert(err, gc.IsNil)
		c.Assert(ctl.MonitorLeaseExpiry.UTC(), gc.DeepEquals, test.expectExpiry.UTC())
		c.Assert(ctl.MonitorLeaseOwner, gc.Equals, test.newOwner)
	}
}

func (s *databaseSuite) TestSetControllerStatsNotFound(c *gc.C) {
	err := s.database.SetControllerStats(params.EntityPath{"bob", "foo"}, &mongodoc.ControllerStats{})
	c.Assert(err, gc.ErrorMatches, "controller not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *databaseSuite) TestSetControllerStats(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.AddController(&mongodoc.Controller{
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
	err = s.database.SetControllerStats(ctlPath, stats)
	c.Assert(err, gc.IsNil)
	ctl, err := s.database.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.Stats, jc.DeepEquals, *stats)
}

func (s *databaseSuite) TestSetModelLifeNotFound(c *gc.C) {
	err := s.database.SetModelLife(params.EntityPath{"bob", "foo"}, "fake-uuid", "alive")
	c.Assert(err, gc.IsNil)
}

func (s *databaseSuite) TestSetModelLifeSuccess(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.AddController(&mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.IsNil)

	// Add the controller model.
	err = s.database.AddModel(&mongodoc.Model{
		Path:       params.EntityPath{"bob", "foo"},
		UUID:       "fake-uuid",
		Controller: params.EntityPath{"bob", "foo"},
	})
	c.Assert(err, gc.IsNil)

	// Add another model with the same UUID but a different controller.
	err = s.database.AddModel(&mongodoc.Model{
		Path:       params.EntityPath{"bar", "baz"},
		UUID:       "fake-uuid",
		Controller: params.EntityPath{"bar", "zzz"},
	})
	c.Assert(err, gc.IsNil)

	// Add another model with the same controller but a different UUID.
	err = s.database.AddModel(&mongodoc.Model{
		Path:       params.EntityPath{"alice", "baz"},
		UUID:       "another-uuid",
		Controller: ctlPath,
	})
	c.Assert(err, gc.IsNil)

	err = s.database.SetModelLife(ctlPath, "fake-uuid", "alive")
	c.Assert(err, gc.IsNil)

	m, err := s.database.Model(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life, gc.Equals, "alive")

	m, err = s.database.Model(params.EntityPath{"bar", "baz"})
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life, gc.Equals, "")

	m, err = s.database.Model(params.EntityPath{"alice", "baz"})
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life, gc.Equals, "")
}

func (s *databaseSuite) TestAcquireLeaseControllerNotFound(c *gc.C) {
	_, err := s.database.AcquireMonitorLease(params.EntityPath{"bob", "foo"}, time.Time{}, "", time.Now(), "jem1")
	c.Assert(err, gc.ErrorMatches, `controller removed`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *databaseSuite) TestAddAndGetCredential(c *gc.C) {
	user := params.User("test-user")
	cloud := params.Cloud("test-cloud")
	name := params.Name("test-credential")
	expectId := fmt.Sprintf("%s/%s/%s", user, cloud, name)
	cred, err := s.database.Credential(user, cloud, name)
	c.Assert(cred, gc.IsNil)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(err, gc.ErrorMatches, `credential "test-user/test-cloud/test-credential" not found`)

	attrs := map[string]string{
		"attr1": "val1",
		"attr2": "val2",
	}
	err = jem.UpdateCredential(s.database, &mongodoc.Credential{
		User:       user,
		Cloud:      cloud,
		Name:       name,
		Type:       "credtype",
		Label:      "Test Label",
		Attributes: attrs,
	})
	c.Assert(err, gc.IsNil)

	cred, err = s.database.Credential(user, cloud, name)
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

	err = jem.UpdateCredential(s.database, &mongodoc.Credential{
		User:       user,
		Cloud:      cloud,
		Name:       name,
		Type:       "credtype",
		Label:      "Test Label 2",
		Attributes: attrs,
	})
	c.Assert(err, gc.IsNil)

	cred, err = s.database.Credential(user, cloud, name)
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
}

func (s *databaseSuite) TestCredentialAddController(c *gc.C) {
	user := params.User("test-user")
	cloud := params.Cloud("test-cloud")
	name := params.Name("test-credential")
	expectId := fmt.Sprintf("%s/%s/%s", user, cloud, name)
	err := jem.UpdateCredential(s.database, &mongodoc.Credential{
		User:  user,
		Cloud: cloud,
		Name:  name,
		Type:  "empty",
	})
	c.Assert(err, gc.IsNil)

	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Path: ctlPath,
	}
	err = s.database.AddController(ctl)
	c.Assert(err, gc.IsNil)

	err = jem.CredentialAddController(s.database, user, cloud, name, ctlPath)
	c.Assert(err, gc.IsNil)

	cred, err := s.database.Credential(user, cloud, name)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:         expectId,
		User:       user,
		Cloud:      cloud,
		Name:       name,
		Type:       "empty",
		Attributes: map[string]string{},
		Controllers: []params.EntityPath{
			ctlPath,
		},
	})

	// Add a second time
	err = jem.CredentialAddController(s.database, user, cloud, name, ctlPath)
	c.Assert(err, gc.IsNil)

	cred, err = s.database.Credential(user, cloud, name)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:         expectId,
		User:       user,
		Cloud:      cloud,
		Name:       name,
		Type:       "empty",
		Attributes: map[string]string{},
		Controllers: []params.EntityPath{
			ctlPath,
		},
	})

	// Add to a non-existant credential
	err = jem.CredentialAddController(s.database, user, cloud, "no-such-cred", ctlPath)
	c.Assert(err, gc.ErrorMatches, `credential "test-user/test-cloud/no-such-cred" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *databaseSuite) TestCredentialRemoveController(c *gc.C) {
	user := params.User("test-user")
	cloud := params.Cloud("test-cloud")
	name := params.Name("test-credential")
	expectId := fmt.Sprintf("%s/%s/%s", user, cloud, name)
	err := jem.UpdateCredential(s.database, &mongodoc.Credential{
		User:  user,
		Cloud: cloud,
		Name:  name,
		Type:  "empty",
	})
	c.Assert(err, gc.IsNil)

	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Path: ctlPath,
	}
	err = s.database.AddController(ctl)
	c.Assert(err, gc.IsNil)

	err = jem.CredentialAddController(s.database, user, cloud, name, ctlPath)
	c.Assert(err, gc.IsNil)

	// sanity check the controller is there.
	cred, err := s.database.Credential(user, cloud, name)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:         expectId,
		User:       user,
		Cloud:      cloud,
		Name:       name,
		Type:       "empty",
		Attributes: map[string]string{},
		Controllers: []params.EntityPath{
			ctlPath,
		},
	})

	err = jem.CredentialRemoveController(s.database, user, cloud, name, ctlPath)
	c.Assert(err, gc.IsNil)

	cred, err = s.database.Credential(user, cloud, name)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:         expectId,
		User:       user,
		Cloud:      cloud,
		Name:       name,
		Type:       "empty",
		Attributes: map[string]string{},
	})

	// Remove again
	err = jem.CredentialRemoveController(s.database, user, cloud, name, ctlPath)
	c.Assert(err, gc.IsNil)

	cred, err = s.database.Credential(user, cloud, name)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:         expectId,
		User:       user,
		Cloud:      cloud,
		Name:       name,
		Type:       "empty",
		Attributes: map[string]string{},
	})

	// remove from a non-existant credential
	err = jem.CredentialRemoveController(s.database, user, cloud, "no-such-cred", ctlPath)
	c.Assert(err, gc.ErrorMatches, `credential "test-user/test-cloud/no-such-cred" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *databaseSuite) TestSetACL(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.AddController(&mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.IsNil)

	err = jem.SetACL(s.database.Controllers(), ctlPath, params.ACL{
		Read: []string{"t1", "t2"},
	})
	c.Assert(err, gc.IsNil)
	var cnt mongodoc.Controller
	err = s.database.Controllers().FindId(ctlPath.String()).One(&cnt)
	c.Assert(err, gc.IsNil)
	c.Assert(cnt.ACL, jc.DeepEquals, params.ACL{
		Read: []string{"t1", "t2"},
	})

	err = jem.SetACL(s.database.Controllers(), params.EntityPath{"bob", "bar"}, params.ACL{
		Read: []string{"t2", "t1"},
	})
	c.Assert(err, gc.ErrorMatches, `"bob/bar" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *databaseSuite) TestGrant(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.AddController(&mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.IsNil)

	err = jem.Grant(s.database.Controllers(), ctlPath, "t1")
	c.Assert(err, gc.IsNil)
	var cnt mongodoc.Controller
	err = s.database.Controllers().FindId(ctlPath.String()).One(&cnt)
	c.Assert(err, gc.IsNil)
	c.Assert(cnt.ACL, jc.DeepEquals, params.ACL{
		Read: []string{"t1"},
	})

	err = jem.Grant(s.database.Controllers(), params.EntityPath{"bob", "bar"}, "t1")
	c.Assert(err, gc.ErrorMatches, `"bob/bar" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *databaseSuite) TestRevoke(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.AddController(&mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.IsNil)

	err = jem.SetACL(s.database.Controllers(), ctlPath, params.ACL{
		Read: []string{"t1", "t2"},
	})
	c.Assert(err, gc.IsNil)
	err = jem.Revoke(s.database.Controllers(), ctlPath, "t2")
	c.Assert(err, gc.IsNil)
	var cnt mongodoc.Controller
	err = s.database.Controllers().FindId(ctlPath.String()).One(&cnt)
	c.Assert(err, gc.IsNil)
	c.Assert(cnt.ACL, jc.DeepEquals, params.ACL{
		Read: []string{"t1"},
	})

	err = jem.Revoke(s.database.Controllers(), params.EntityPath{"bob", "bar"}, "t2")
	c.Assert(err, gc.ErrorMatches, `"bob/bar" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *databaseSuite) TestCloud(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.AddController(&mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
		Cloud: mongodoc.Cloud{
			Name:         "my-cloud",
			ProviderType: "ec2",
		},
	})
	c.Assert(err, gc.IsNil)
	cld, err := s.database.Cloud("my-cloud")
	c.Assert(err, gc.IsNil)
	c.Assert(cld, jc.DeepEquals, &mongodoc.Cloud{
		Name:         "my-cloud",
		ProviderType: "ec2",
	})
	cld, err = s.database.Cloud("not-my-cloud")
	c.Assert(err, gc.ErrorMatches, `cloud "not-my-cloud" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

type gauge struct {
	mu sync.Mutex
	n  int
}

func (g *gauge) Inc() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
}

func (g *gauge) Dec() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n--
}
