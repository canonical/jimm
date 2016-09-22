// Copyright 2016 Canonical Ltd.

package jem_test

import (
	"fmt"
	"time"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jem/internal/auth"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mgosession"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type databaseSuite struct {
	jujutesting.IsolatedMgoSuite
	database *jem.Database
}

var _ = gc.Suite(&databaseSuite{})

func (s *databaseSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	pool := mgosession.NewPool(s.Session, 1)
	s.database = jem.NewDatabase(pool.Session(), "jem")
	pool.Close()
}

func (s *databaseSuite) TearDownTest(c *gc.C) {
	jem.DatabaseClose(s.database)
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *databaseSuite) checkDBOK(c *gc.C) {
	c.Check(jem.DatabaseSessionIsDead(s.database), gc.Equals, false)
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
	s.checkDBOK(c)
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
	s.checkDBOK(c)
}

func (s *databaseSuite) TestSetControllerAvailabilityWithNotFoundController(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	err := s.database.SetControllerUnavailableAt(ctlPath, time.Now())
	c.Assert(err, gc.IsNil)
	err = s.database.SetControllerAvailable(ctlPath)
	c.Assert(err, gc.IsNil)
	s.checkDBOK(c)
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
	s.checkDBOK(c)
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
	s.checkDBOK(c)
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
	s.checkDBOK(c)
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
		s.checkDBOK(c)
	}
}

func (s *databaseSuite) TestSetControllerStatsNotFound(c *gc.C) {
	err := s.database.SetControllerStats(params.EntityPath{"bob", "foo"}, &mongodoc.ControllerStats{})
	c.Assert(err, gc.ErrorMatches, "controller not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
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
	s.checkDBOK(c)
}

func (s *databaseSuite) TestSetModelLifeNotFound(c *gc.C) {
	err := s.database.SetModelLife(params.EntityPath{"bob", "foo"}, "fake-uuid", "alive")
	c.Assert(err, gc.IsNil)
	s.checkDBOK(c)
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
	s.checkDBOK(c)
}

func (s *databaseSuite) TestAcquireLeaseControllerNotFound(c *gc.C) {
	_, err := s.database.AcquireMonitorLease(params.EntityPath{"bob", "foo"}, time.Time{}, "", time.Now(), "jem1")
	c.Assert(err, gc.ErrorMatches, `controller removed`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestAddAndGetCredential(c *gc.C) {
	path := credentialPath("test-cloud", "test-user", "test-credential")
	expectId := path.String()
	cred, err := s.database.Credential(path)
	c.Assert(cred, gc.IsNil)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(err, gc.ErrorMatches, `credential "test-cloud/test-user/test-credential" not found`)

	attrs := map[string]string{
		"attr1": "val1",
		"attr2": "val2",
	}
	err = jem.UpdateCredential(s.database, &mongodoc.Credential{
		Path:       path,
		Type:       "credtype",
		Label:      "Test Label",
		Attributes: attrs,
	})
	c.Assert(err, gc.IsNil)

	cred, err = s.database.Credential(path)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:         expectId,
		Path:       path,
		Type:       "credtype",
		Label:      "Test Label",
		Attributes: attrs,
	})

	err = jem.UpdateCredential(s.database, &mongodoc.Credential{
		Path:       path,
		Type:       "credtype",
		Label:      "Test Label 2",
		Attributes: attrs,
	})
	c.Assert(err, gc.IsNil)

	cred, err = s.database.Credential(path)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:         expectId,
		Path:       path,
		Type:       "credtype",
		Label:      "Test Label 2",
		Attributes: attrs,
	})

	err = jem.UpdateCredential(s.database, &mongodoc.Credential{
		Path:    path,
		Revoked: true,
	})
	c.Assert(err, gc.IsNil)
	cred, err = s.database.Credential(path)
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
	err := jem.UpdateCredential(s.database, &mongodoc.Credential{
		Path: path,
		Type: "empty",
	})
	c.Assert(err, gc.IsNil)

	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Path: ctlPath,
	}
	err = s.database.AddController(ctl)
	c.Assert(err, gc.IsNil)

	err = jem.CredentialAddController(s.database, path, ctlPath)
	c.Assert(err, gc.IsNil)

	cred, err := s.database.Credential(path)
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
	err = jem.CredentialAddController(s.database, path, ctlPath)
	c.Assert(err, gc.IsNil)

	cred, err = s.database.Credential(path)
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
	err = jem.CredentialAddController(s.database, path2, ctlPath)
	c.Assert(err, gc.ErrorMatches, `credential "test-cloud/test-user/no-such-cred" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestCredentialRemoveController(c *gc.C) {
	path := credentialPath("test-cloud", "test-user", "test-credential")
	expectId := path.String()
	err := jem.UpdateCredential(s.database, &mongodoc.Credential{
		Path: path,
		Type: "empty",
	})
	c.Assert(err, gc.IsNil)

	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Path: ctlPath,
	}
	err = s.database.AddController(ctl)
	c.Assert(err, gc.IsNil)

	err = jem.CredentialAddController(s.database, path, ctlPath)
	c.Assert(err, gc.IsNil)

	// sanity check the controller is there.
	cred, err := s.database.Credential(path)
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

	err = jem.CredentialRemoveController(s.database, path, ctlPath)
	c.Assert(err, gc.IsNil)

	cred, err = s.database.Credential(path)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:         expectId,
		Path:       path,
		Type:       "empty",
		Attributes: map[string]string{},
	})

	// Remove again
	err = jem.CredentialRemoveController(s.database, path, ctlPath)
	c.Assert(err, gc.IsNil)

	cred, err = s.database.Credential(path)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:         expectId,
		Path:       path,
		Type:       "empty",
		Attributes: map[string]string{},
	})
	path2 := credentialPath("test-cloud", "test-user", "no-such-cred")
	// remove from a non-existant credential
	err = jem.CredentialRemoveController(s.database, path2, ctlPath)
	c.Assert(err, gc.ErrorMatches, `credential "test-cloud/test-user/no-such-cred" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestSetACL(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.AddController(&mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.IsNil)

	err = s.database.SetACL(s.database.Controllers(), ctlPath, params.ACL{
		Read: []string{"t1", "t2"},
	})
	c.Assert(err, gc.IsNil)
	var cnt mongodoc.Controller
	err = s.database.Controllers().FindId(ctlPath.String()).One(&cnt)
	c.Assert(err, gc.IsNil)
	c.Assert(cnt.ACL, jc.DeepEquals, params.ACL{
		Read: []string{"t1", "t2"},
	})

	err = s.database.SetACL(s.database.Controllers(), params.EntityPath{"bob", "bar"}, params.ACL{
		Read: []string{"t2", "t1"},
	})
	c.Assert(err, gc.ErrorMatches, `"bob/bar" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestGrant(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.AddController(&mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.IsNil)

	err = s.database.Grant(s.database.Controllers(), ctlPath, "t1")
	c.Assert(err, gc.IsNil)
	var cnt mongodoc.Controller
	err = s.database.Controllers().FindId(ctlPath.String()).One(&cnt)
	c.Assert(err, gc.IsNil)
	c.Assert(cnt.ACL, jc.DeepEquals, params.ACL{
		Read: []string{"t1"},
	})

	err = s.database.Grant(s.database.Controllers(), params.EntityPath{"bob", "bar"}, "t1")
	c.Assert(err, gc.ErrorMatches, `"bob/bar" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestRevoke(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.AddController(&mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.IsNil)

	err = s.database.SetACL(s.database.Controllers(), ctlPath, params.ACL{
		Read: []string{"t1", "t2"},
	})
	c.Assert(err, gc.IsNil)
	err = s.database.Revoke(s.database.Controllers(), ctlPath, "t2")
	c.Assert(err, gc.IsNil)
	var cnt mongodoc.Controller
	err = s.database.Controllers().FindId(ctlPath.String()).One(&cnt)
	c.Assert(err, gc.IsNil)
	c.Assert(cnt.ACL, jc.DeepEquals, params.ACL{
		Read: []string{"t1"},
	})

	err = s.database.Revoke(s.database.Controllers(), params.EntityPath{"bob", "bar"}, "t2")
	c.Assert(err, gc.ErrorMatches, `"bob/bar" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
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
	err := s.database.AddModel(m)
	c.Assert(err, gc.IsNil)
	acl, err := s.database.GetACL(s.database.Models(), m.Path)
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
	acl, err := s.database.GetACL(s.database.Models(), m.Path)
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
		gs := append(test.groups, test.user)
		ctx := auth.AuthenticateForTest(context.Background(), gs...)
		entity := params.EntityPath{
			User: params.User(test.owner),
			Name: params.Name(fmt.Sprintf("test%d", i)),
		}
		if !test.skipCreateEntity {
			err := s.database.AddModel(&mongodoc.Model{
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
		err := s.database.AddModel(&testModels[i])
		c.Assert(err, gc.IsNil)
	}
	ctx := auth.AuthenticateForTest(context.Background(), "bob", "bob-group")
	it := s.database.Models().Find(nil).Sort("_id").Iter()
	crit := s.database.NewCanReadIter(ctx, it)
	var models []mongodoc.Model
	var m mongodoc.Model
	for crit.Next(&m) {
		models = append(models, m)
	}
	c.Assert(crit.Err(), gc.IsNil)
	c.Assert(models, jc.DeepEquals, []mongodoc.Model{
		testModels[0],
		testModels[2],
	})
	c.Assert(crit.Count(), gc.Equals, 3)
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
		db.AddController(&mongodoc.Controller{
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
		db.AddModel(&mongodoc.Model{
			Path: fakeEntityPath,
		})
	},
}, {
	about: "DeleteModel",
	run: func(db *jem.Database) {
		db.DeleteModel(fakeEntityPath)
	},
}, {
	about: "Model",
	run: func(db *jem.Database) {
		db.Model(fakeEntityPath)
	},
}, {
	about: "Controller",
	run: func(db *jem.Database) {
		db.Controller(fakeEntityPath)
	},
}, {
	about: "ModelFromUUID",
	run: func(db *jem.Database) {
		db.ModelFromUUID("99999999-9999-9999-9999-999999999999")
	},
}, {
	about: "SetControllerAvailable",
	run: func(db *jem.Database) {
		db.SetControllerAvailable(fakeEntityPath)
	},
}, {
	about: "SetControllerUnavailableAt",
	run: func(db *jem.Database) {
		db.SetControllerUnavailableAt(fakeEntityPath, time.Now())
	},
}, {
	about: "AcquireMonitorLease",
	run: func(db *jem.Database) {
		db.AcquireMonitorLease(fakeEntityPath, time.Now(), "foo", time.Now(), "bar")
	},
}, {
	about: "SetControllerStats",
	run: func(db *jem.Database) {
		db.SetModelLife(fakeEntityPath, "fake-uuid", "alive")
	},
}, {
	about: "SetModelLife",
	run: func(db *jem.Database) {
		db.SetModelLife(fakeEntityPath, "fake-uuid", "alive")
	},
}, {
	about: "UpdateCredential",
	run: func(db *jem.Database) {
		jem.UpdateCredential(db, &mongodoc.Credential{
			Path:  fakeCredPath,
			Type:  "credtype",
			Label: "Test Label",
		})
	},
}, {
	about: "Credential",
	run: func(db *jem.Database) {
		db.Credential(fakeCredPath)
	},
}, {
	about: "credentialAddController",
	run: func(db *jem.Database) {
		jem.CredentialAddController(db, fakeCredPath, fakeEntityPath)
	},
}, {
	about: "credentialRemoveController",
	run: func(db *jem.Database) {
		jem.CredentialRemoveController(db, fakeCredPath, fakeEntityPath)
	},
}, {
	about: "Cloud",
	run: func(db *jem.Database) {
		db.Cloud("my-cloud")
	},
}, {
	about: "setCredentialUpdates",
	run: func(db *jem.Database) {
		jem.SetCredentialUpdates(db, []params.EntityPath{fakeEntityPath}, fakeCredPath)
	},
}, {
	about: "clearCredentialUpdate",
	run: func(db *jem.Database) {
		jem.ClearCredentialUpdate(db, fakeEntityPath, fakeCredPath)
	},
}, {
	about: "GetACL",
	run: func(db *jem.Database) {
		db.GetACL(db.Models(), fakeEntityPath)
	},
}, {
	about: "SetACL",
	run: func(db *jem.Database) {
		db.SetACL(db.Models(), fakeEntityPath, params.ACL{
			Read: []string{"t1", "t2"},
		})
	},
}, {
	about: "Grant",
	run: func(db *jem.Database) {
		db.Grant(db.Controllers(), fakeEntityPath, "t1")
	},
}, {
	about: "Revoke",
	run: func(db *jem.Database) {
		db.Revoke(db.Controllers(), fakeEntityPath, "t1")
	},
}, {
	about: "CanReadIter",
	run: func(db *jem.Database) {
		it := db.Models().Find(nil).Sort("_id").Iter()
		ctx := auth.AuthenticateForTest(context.Background(), "bob", "bob-group")
		crit := db.NewCanReadIter(ctx, it)
		crit.Next(&mongodoc.Model{})
		crit.Err()
	},
}, {
	about: "CanReadIter with Close",
	run: func(db *jem.Database) {
		it := db.Models().Find(nil).Sort("_id").Iter()
		ctx := auth.AuthenticateForTest(context.Background(), "bob", "bob-group")
		crit := db.NewCanReadIter(ctx, it)
		crit.Next(&mongodoc.Model{})
		crit.Close()
	},
}}

func (s *databaseSuite) TestSetDead(c *gc.C) {
	session := jujutesting.NewProxiedSession(c)
	defer session.Close()

	pool := mgosession.NewPool(session.Session, 1)
	defer pool.Close()
	for i, test := range setDeadTests {
		c.Logf("test %d: %s", i, test.about)
		testSetDead(c, session.TCPProxy, pool, test.run)
	}
}

func testSetDead(c *gc.C, proxy *jujutesting.TCPProxy, pool *mgosession.Pool, run func(db *jem.Database)) {
	session := pool.Session()
	defer session.Close()
	// Use the session so that it's bound to the socket.
	err := session.Ping()
	c.Assert(err, gc.IsNil)
	proxy.CloseConns() // Close the existing socket so that the connection is broken.

	c.Check(session.MayReuse(), gc.Equals, true) // Sanity check.
	db := jem.NewDatabase(session, "jem")
	run(db)
	c.Check(session.MayReuse(), gc.Equals, false)
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
