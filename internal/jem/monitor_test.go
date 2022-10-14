// Copyright 2020 Canonical Ltd.

package jem_test

import (
	"fmt"
	"sort"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jem/jimmdb"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

type monitorSuite struct {
	jemtest.JEMSuite
}

var _ = gc.Suite(&monitorSuite{})

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

func (s *monitorSuite) TestAcquireLease(c *gc.C) {
	for i, test := range acquireLeaseTests {
		c.Logf("test %d: %v", i, test.about)
		_, err := s.JEM.DB.Controllers().RemoveAll(bson.D{{"path", test.ctlPath}})
		c.Assert(err, gc.Equals, nil)
		_, err = s.JEM.DB.Models().RemoveAll(bson.D{{"path", test.ctlPath}})
		c.Assert(err, gc.Equals, nil)
		err = s.JEM.DB.InsertController(testContext, &mongodoc.Controller{
			Path:               test.ctlPath,
			UUID:               "fake-uuid",
			MonitorLeaseOwner:  test.actualOldOwner,
			MonitorLeaseExpiry: test.actualOldExpiry,
		})
		c.Assert(err, gc.Equals, nil)
		t, err := s.JEM.AcquireMonitorLease(testContext, test.ctlPath, test.oldExpiry, test.oldOwner, test.newExpiry, test.newOwner)
		if test.expectError != "" {
			if test.expectCause != nil {
				c.Check(errgo.Cause(err), gc.Equals, test.expectCause)
			}
			c.Assert(err, gc.ErrorMatches, test.expectError)
			c.Assert(t, jc.Satisfies, time.Time.IsZero)
			continue
		}
		c.Assert(err, gc.Equals, nil)
		c.Assert(t.UTC(), gc.DeepEquals, test.expectExpiry.UTC())
		ctl := &mongodoc.Controller{Path: test.ctlPath}
		err = s.JEM.DB.GetController(testContext, ctl)
		c.Assert(err, gc.Equals, nil)
		c.Assert(ctl.MonitorLeaseExpiry.UTC(), gc.DeepEquals, test.expectExpiry.UTC())
		c.Assert(ctl.MonitorLeaseOwner, gc.Equals, test.newOwner)
	}
}

func (s *monitorSuite) TestAcquireLeaseControllerNotFound(c *gc.C) {
	_, err := s.JEM.AcquireMonitorLease(testContext, params.EntityPath{"bob", "foo"}, time.Time{}, "", time.Now(), "jem1")
	c.Assert(err, gc.ErrorMatches, `controller removed`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *monitorSuite) TestSetModelInfoSuccess(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}

	// Add the controller model.
	err := s.JEM.DB.InsertModel(testContext, &mongodoc.Model{
		Path:       params.EntityPath{"bob", "foo"},
		UUID:       "fake-uuid",
		Controller: params.EntityPath{"bob", "foo"},
	})
	c.Assert(err, gc.Equals, nil)

	// Add another model with the same controller but a different UUID.
	err = s.JEM.DB.InsertModel(testContext, &mongodoc.Model{
		Path:       params.EntityPath{"alice", "baz"},
		UUID:       "another-uuid",
		Controller: ctlPath,
	})
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.SetModelInfo(testContext, ctlPath, "fake-uuid", &mongodoc.ModelInfo{
		Life: "alive",
	})
	c.Assert(err, gc.Equals, nil)

	m := mongodoc.Model{Path: ctlPath}
	err = s.JEM.DB.GetModel(testContext, &m)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m.Life(), gc.Equals, "alive")

	m.Path = params.EntityPath{"alice", "baz"}
	err = s.JEM.DB.GetModel(testContext, &m)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m.Life(), gc.Equals, "")
}

func (s *monitorSuite) TestSetModelInfoNotFound(c *gc.C) {
	err := s.JEM.SetModelInfo(testContext, params.EntityPath{"bob", "foo"}, "fake-uuid", &mongodoc.ModelInfo{})
	c.Assert(err, gc.Equals, nil)
}

func (s *monitorSuite) TestSetModelLifeSuccess(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}

	// Add the controller model.
	err := s.JEM.DB.InsertModel(testContext, &mongodoc.Model{
		Path:       params.EntityPath{"bob", "foo"},
		UUID:       "fake-uuid",
		Controller: params.EntityPath{"bob", "foo"},
	})
	c.Assert(err, gc.Equals, nil)

	// Add another model with the same controller but a different UUID.
	err = s.JEM.DB.InsertModel(testContext, &mongodoc.Model{
		Path:       params.EntityPath{"alice", "baz"},
		UUID:       "another-uuid",
		Controller: ctlPath,
	})
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.SetModelLife(testContext, ctlPath, "fake-uuid", "alive")
	c.Assert(err, gc.Equals, nil)

	m := mongodoc.Model{Path: ctlPath}
	err = s.JEM.DB.GetModel(testContext, &m)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m.Life(), gc.Equals, "alive")

	m.Path = params.EntityPath{"alice", "baz"}
	err = s.JEM.DB.GetModel(testContext, &m)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m.Life(), gc.Equals, "")
}

func (s *monitorSuite) TestSetModelLifeNotFound(c *gc.C) {
	err := s.JEM.SetModelLife(testContext, params.EntityPath{"bob", "foo"}, "fake-uuid", "alive")
	c.Assert(err, gc.Equals, nil)
}

func (s *monitorSuite) TestModelUUIDsForController(c *gc.C) {
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
		err := s.JEM.DB.InsertModel(testContext, &mongodoc.Model{
			Path:       m.path,
			Controller: m.ctlPath,
			UUID:       m.uuid,
		})
		c.Assert(err, gc.Equals, nil)
	}
	uuids, err := s.JEM.ModelUUIDsForController(testContext, params.EntityPath{"admin", "ctl1"})
	c.Assert(err, gc.Equals, nil)
	sort.Strings(uuids)
	c.Assert(uuids, jc.DeepEquals, []string{
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
	})
	uuids, err = s.JEM.ModelUUIDsForController(testContext, params.EntityPath{"admin", "ctl2"})
	c.Assert(err, gc.Equals, nil)
	sort.Strings(uuids)
	c.Assert(uuids, jc.DeepEquals, []string{
		"33333333-3333-3333-3333-333333333333",
	})
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

func (s *monitorSuite) TestUpdateModelCounts(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "controller"}
	for i, test := range updateModelCountsTests {
		c.Logf("test %d: %v", i, test.about)
		modelId := params.EntityPath{"bob", params.Name(fmt.Sprintf("model-%d", i))}
		uuid := fmt.Sprintf("uuid-%d", i)
		err := s.JEM.DB.InsertModel(testContext, &mongodoc.Model{
			Path:       modelId,
			Controller: ctlPath,
			UUID:       uuid,
			Counts:     test.before,
		})
		c.Assert(err, gc.Equals, nil)
		err = s.JEM.UpdateModelCounts(testContext, ctlPath, uuid, test.update, test.updateTime)
		c.Assert(err, gc.Equals, nil)
		model := mongodoc.Model{Path: modelId}
		err = s.JEM.DB.GetModel(testContext, &model)
		c.Assert(err, gc.Equals, nil)
		// Change all times to UTC for straightforward comparison.
		for name, count := range model.Counts {
			count.Time = count.Time.UTC()
			model.Counts[name] = count
		}
		c.Assert(model.Counts, jc.DeepEquals, test.expect)
	}
}

func (s *monitorSuite) TestUpdateModelCountsNotFoundUUID(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "controller"}
	modelId := params.EntityPath{"bob", "test-model"}
	uuid := "real-uuid"
	err := s.JEM.DB.InsertModel(testContext, &mongodoc.Model{
		Path:       modelId,
		Controller: ctlPath,
		UUID:       uuid,
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateModelCounts(testContext, ctlPath, "fake-uuid", nil, T(0))
	c.Assert(err, gc.ErrorMatches, `model not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *monitorSuite) TestUpdateModelCountsNotFoundController(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "controller"}
	modelId := params.EntityPath{"bob", "test-model"}
	uuid := "real-uuid"
	err := s.JEM.DB.InsertModel(testContext, &mongodoc.Model{
		Path:       modelId,
		Controller: ctlPath,
		UUID:       uuid,
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateModelCounts(testContext, params.EntityPath{"bob", "not-controller"}, "real-uuid", nil, T(0))
	c.Assert(err, gc.ErrorMatches, `model not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *monitorSuite) TestSetControllerAvailability(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{Path: ctlPath}
	err := s.JEM.DB.InsertController(testContext, ctl)

	// Check that we can mark it as unavailable.
	t0 := time.Now()
	err = s.JEM.SetControllerUnavailableAt(testContext, ctlPath, t0)
	c.Assert(err, gc.Equals, nil)

	ctl = &mongodoc.Controller{Path: ctlPath}
	err = s.JEM.DB.GetController(testContext, ctl)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ctl.UnavailableSince.UTC(), jc.DeepEquals, mongodoc.Time(t0).UTC())

	// Check that if we mark it unavailable again, it doesn't
	// have any affect.
	err = s.JEM.SetControllerUnavailableAt(testContext, ctlPath, t0.Add(time.Second))
	c.Assert(err, gc.Equals, nil)

	ctl = &mongodoc.Controller{Path: ctlPath}
	err = s.JEM.DB.GetController(testContext, ctl)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ctl.UnavailableSince.UTC(), jc.DeepEquals, mongodoc.Time(t0).UTC())

	// Check that we can mark it as available again.
	err = s.JEM.SetControllerAvailable(testContext, ctlPath)
	c.Assert(err, gc.Equals, nil)

	ctl = &mongodoc.Controller{Path: ctlPath}
	err = s.JEM.DB.GetController(testContext, ctl)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ctl.UnavailableSince, jc.Satisfies, time.Time.IsZero)

	t1 := t0.Add(3 * time.Second)
	// ... and that we can mark it as unavailable after that.
	err = s.JEM.SetControllerUnavailableAt(testContext, ctlPath, t1)
	c.Assert(err, gc.Equals, nil)

	ctl = &mongodoc.Controller{Path: ctlPath}
	err = s.JEM.DB.GetController(testContext, ctl)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ctl.UnavailableSince.UTC(), jc.DeepEquals, mongodoc.Time(t1).UTC())
}

func (s *monitorSuite) TestSetControllerAvailabilityWithNotFoundController(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	err := s.JEM.SetControllerUnavailableAt(testContext, ctlPath, time.Now())
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.SetControllerAvailable(testContext, ctlPath)
	c.Assert(err, gc.Equals, nil)
}

func (s *monitorSuite) TestSetControllerVersion(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Path: ctlPath,
	}
	err := s.JEM.DB.InsertController(testContext, ctl)
	c.Assert(err, gc.Equals, nil)

	testVersion := version.Number{Minor: 1}
	err = s.JEM.SetControllerVersion(testContext, ctlPath, testVersion)
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.DB.GetController(testContext, ctl)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ctl.Version, jc.DeepEquals, &testVersion)
}

func (s *monitorSuite) TestSetControllerVersionWithNotFoundController(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	err := s.JEM.SetControllerVersion(testContext, ctlPath, version.Number{Minor: 1})
	c.Assert(err, gc.Equals, nil)
}

func (s *monitorSuite) TestSetControllerStatsNotFound(c *gc.C) {
	err := s.JEM.SetControllerStats(testContext, params.EntityPath{"bob", "foo"}, &mongodoc.ControllerStats{})
	c.Assert(err, gc.ErrorMatches, "controller not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *monitorSuite) TestSetControllerStats(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.JEM.DB.InsertController(testContext, &mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.Equals, nil)

	stats := &mongodoc.ControllerStats{
		UnitCount:    1,
		ModelCount:   2,
		ServiceCount: 3,
		MachineCount: 4,
	}
	err = s.JEM.SetControllerStats(testContext, ctlPath, stats)
	c.Assert(err, gc.Equals, nil)
	ctl := mongodoc.Controller{Path: ctlPath}
	err = s.JEM.DB.GetController(testContext, &ctl)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ctl.Stats, jc.DeepEquals, *stats)
}

type monitorUpdateSuite struct {
	jemtest.BootstrapSuite
}

var _ = gc.Suite(&monitorUpdateSuite{})

func (s *monitorUpdateSuite) TestUpdateMachineInfo(c *gc.C) {
	err := s.JEM.UpdateMachineInfo(testContext, s.Controller.Path, &jujuparams.MachineInfo{
		ModelUUID: s.Model.UUID,
		Id:        "0",
		Series:    "quantal",
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateMachineInfo(testContext, s.Controller.Path, &jujuparams.MachineInfo{
		ModelUUID: s.Model.UUID,
		Id:        "1",
		Series:    "precise",
	})
	c.Assert(err, gc.Equals, nil)

	var docs []mongodoc.Machine
	err = s.JEM.DB.ForEachMachine(testContext, jimmdb.Eq("info.modeluuid", s.Model.UUID), []string{"_id"}, func(m *mongodoc.Machine) error {
		cleanMachineDoc(m)
		docs = append(docs, *m)
		return nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.Machine{{
		Id:         s.Controller.Path.String() + " " + s.Model.UUID + " 0",
		Controller: s.Controller.Path.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: s.Model.UUID,
			Id:        "0",
			Series:    "quantal",
			Config:    map[string]interface{}{},
		},
	}, {
		Id:         s.Controller.Path.String() + " " + s.Model.UUID + " 1",
		Controller: s.Controller.Path.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: s.Model.UUID,
			Id:        "1",
			Series:    "precise",
			Config:    map[string]interface{}{},
		},
	}})

	// Check that we can update one of the documents.
	err = s.JEM.UpdateMachineInfo(testContext, s.Controller.Path, &jujuparams.MachineInfo{
		ModelUUID: s.Model.UUID,
		Id:        "0",
		Series:    "quantal",
		Life:      "dying",
	})
	c.Assert(err, gc.Equals, nil)

	// Check that setting a machine dead removes it.
	err = s.JEM.UpdateMachineInfo(testContext, s.Controller.Path, &jujuparams.MachineInfo{
		ModelUUID: s.Model.UUID,
		Id:        "1",
		Series:    "precise",
		Life:      "dead",
	})
	c.Assert(err, gc.Equals, nil)

	docs = docs[:0]
	err = s.JEM.DB.ForEachMachine(testContext, jimmdb.Eq("info.modeluuid", s.Model.UUID), []string{"_id"}, func(m *mongodoc.Machine) error {
		cleanMachineDoc(m)
		docs = append(docs, *m)
		return nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.Machine{{
		Id:         s.Controller.Path.String() + " " + s.Model.UUID + " 0",
		Controller: s.Controller.Path.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: s.Model.UUID,
			Id:        "0",
			Series:    "quantal",
			Config:    map[string]interface{}{},
			Life:      "dying",
		},
	}})
}

func (s *monitorUpdateSuite) TestUpdateMachineUnknownModel(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "controller"}

	err := s.JEM.UpdateMachineInfo(testContext, ctlPath, &jujuparams.MachineInfo{
		ModelUUID: "no-such-uuid",
		Id:        "1",
		Series:    "precise",
	})
	c.Assert(err, gc.Equals, nil)
}

func (s *monitorUpdateSuite) TestUpdateMachineIncorrectController(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "controller2"}

	err := s.JEM.UpdateMachineInfo(testContext, ctlPath, &jujuparams.MachineInfo{
		ModelUUID: s.Model.UUID,
		Id:        "1",
		Series:    "precise",
	})
	c.Assert(err, gc.Equals, nil)
}

func (s *monitorUpdateSuite) TestUpdateApplicationInfo(c *gc.C) {
	err := s.JEM.UpdateApplicationInfo(testContext, s.Controller.Path, &jujuparams.ApplicationInfo{
		ModelUUID: s.Model.UUID,
		Name:      "0",
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateApplicationInfo(testContext, s.Controller.Path, &jujuparams.ApplicationInfo{
		ModelUUID: s.Model.UUID,
		Name:      "1",
	})
	c.Assert(err, gc.Equals, nil)

	var docs []mongodoc.Application
	err = s.JEM.DB.ForEachApplication(testContext, jimmdb.Eq("info.modeluuid", s.Model.UUID), []string{"_id"}, func(app *mongodoc.Application) error {
		cleanApplicationDoc(app)
		docs = append(docs, *app)
		return nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.Application{{
		Id:         s.Controller.Path.String() + " " + s.Model.UUID + " 0",
		Controller: s.Controller.Path.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: s.Model.UUID,
			Name:      "0",
		},
	}, {
		Id:         s.Controller.Path.String() + " " + s.Model.UUID + " 1",
		Controller: s.Controller.Path.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: s.Model.UUID,
			Name:      "1",
		},
	}})

	// Check that we can update one of the documents.
	err = s.JEM.UpdateApplicationInfo(testContext, s.Controller.Path, &jujuparams.ApplicationInfo{
		ModelUUID: s.Model.UUID,
		Name:      "0",
		Life:      "dying",
	})
	c.Assert(err, gc.Equals, nil)

	// Check that setting an application dead removes it.
	err = s.JEM.UpdateApplicationInfo(testContext, s.Controller.Path, &jujuparams.ApplicationInfo{
		ModelUUID: s.Model.UUID,
		Name:      "1",
		Life:      "dead",
	})
	c.Assert(err, gc.Equals, nil)

	docs = docs[:0]
	err = s.JEM.DB.ForEachApplication(testContext, jimmdb.Eq("info.modeluuid", s.Model.UUID), []string{"_id"}, func(app *mongodoc.Application) error {
		cleanApplicationDoc(app)
		docs = append(docs, *app)
		return nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.Application{{
		Id:         s.Controller.Path.String() + " " + s.Model.UUID + " 0",
		Controller: s.Controller.Path.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: s.Model.UUID,
			Name:      "0",
			Life:      "dying",
		},
	}})
}

func (s *monitorUpdateSuite) TestUpdateApplicationUnknownModel(c *gc.C) {
	err := s.JEM.UpdateApplicationInfo(testContext, s.Controller.Path, &jujuparams.ApplicationInfo{
		ModelUUID: s.Model.UUID,
		Name:      "1",
	})
	c.Assert(err, gc.Equals, nil)
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

// cleanApplicationDoc cleans up the application document so
// that we can use a DeepEqual comparison without worrying
// about non-nil vs nil map comparisons.
func cleanApplicationDoc(doc *mongodoc.Application) {
	if len(doc.Info.Status.Data) == 0 {
		doc.Info.Status.Data = nil
	}
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

var epoch = parseTime("2016-01-01T12:00:00Z")

func T(n int) time.Time {
	return epoch.Add(time.Duration(n) * time.Millisecond)
}
