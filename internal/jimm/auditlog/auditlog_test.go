// Copyright 2025 Canonical.

package auditlog_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/auditlog"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type auditLogManagerSuite struct {
	manager        *auditlog.AuditLogManager
	adminUser      *openfga.User
	priveligedUser *openfga.User
	user           *openfga.User
	db             *db.Database
	ofgaClient     *openfga.OFGAClient
	jimmTag        names.ControllerTag
}

func (s *auditLogManagerSuite) Init(c *qt.C) {
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	err := db.Migrate(context.Background())
	c.Assert(err, qt.IsNil)

	s.db = db

	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	s.ofgaClient = ofgaClient

	s.jimmTag = names.NewControllerTag("foo")

	s.manager, err = auditlog.NewAuditLogManager(db, ofgaClient, s.jimmTag, 1)
	c.Assert(err, qt.IsNil)

	// Create test identity
	i, err := dbmodel.NewIdentity("alice")
	c.Assert(err, qt.IsNil)
	s.adminUser = openfga.NewUser(i, ofgaClient)
	err = s.adminUser.SetControllerAccess(context.Background(), s.jimmTag, ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)
	s.adminUser.JimmAdmin = true

	i2, err := dbmodel.NewIdentity("bob")
	c.Assert(err, qt.IsNil)
	s.priveligedUser = openfga.NewUser(i2, ofgaClient)
	err = s.priveligedUser.SetControllerAccess(context.Background(), s.jimmTag, ofganames.AuditLogViewerRelation)
	c.Assert(err, qt.IsNil)

	i3, err := dbmodel.NewIdentity("eve")
	c.Assert(err, qt.IsNil)
	s.user = openfga.NewUser(i3, ofgaClient)
}

func (s *auditLogManagerSuite) TestFindAuditEvents(c *qt.C) {
	c.Parallel()

	now := (time.Time{}).UTC().Round(time.Millisecond)

	events := []dbmodel.AuditLogEntry{{
		Time:         now,
		IdentityTag:  s.adminUser.Identity.Tag().String(),
		FacadeMethod: "Login",
	}, {
		Time:         now.Add(time.Hour),
		IdentityTag:  s.adminUser.Identity.Tag().String(),
		FacadeMethod: "AddModel",
	}, {
		Time:         now.Add(2 * time.Hour),
		IdentityTag:  s.priveligedUser.Identity.Tag().String(),
		Model:        "TestModel",
		FacadeMethod: "Deploy",
	}, {
		Time:         now.Add(3 * time.Hour),
		IdentityTag:  s.priveligedUser.Identity.Tag().String(),
		Model:        "TestModel",
		FacadeMethod: "DestroyModel",
	}}
	for i, event := range events {
		e := event
		s.manager.AddAuditLogEntry(&e)
		events[i] = e
	}

	found, err := s.manager.FindAuditEvents(context.Background(), s.adminUser, db.AuditLogFilter{})
	c.Assert(err, qt.IsNil)
	c.Assert(found, qt.HasLen, len(events))

	tests := []struct {
		about          string
		users          []*openfga.User
		filter         db.AuditLogFilter
		expectedEvents []dbmodel.AuditLogEntry
		expectedError  string
	}{{
		about: "admin/privileged user is allowed to find audit events by time",
		users: []*openfga.User{s.adminUser, s.priveligedUser},
		filter: db.AuditLogFilter{
			Start: now.Add(-time.Hour),
			End:   now.Add(time.Minute),
		},
		expectedEvents: []dbmodel.AuditLogEntry{events[0]},
	}, {
		about: "admin/privileged user is allowed to find audit events by user",
		users: []*openfga.User{s.adminUser, s.priveligedUser},
		filter: db.AuditLogFilter{
			IdentityTag: s.adminUser.Tag().String(),
		},
		expectedEvents: []dbmodel.AuditLogEntry{events[0], events[1]},
	}, {
		about: "admin/privileged user is allowed to find audit events by method",
		users: []*openfga.User{s.adminUser, s.priveligedUser},
		filter: db.AuditLogFilter{
			Method: "Deploy",
		},
		expectedEvents: []dbmodel.AuditLogEntry{events[2]},
	}, {
		about: "admin/privileged user is allowed to find audit events by model",
		users: []*openfga.User{s.adminUser, s.priveligedUser},
		filter: db.AuditLogFilter{
			Model: "TestModel",
		},
		expectedEvents: []dbmodel.AuditLogEntry{events[2], events[3]},
	}, {
		about: "admin/privileged user is allowed to find audit events by model and sort by time",
		users: []*openfga.User{s.adminUser, s.priveligedUser},
		filter: db.AuditLogFilter{
			Model:    "TestModel",
			SortTime: true,
		},
		expectedEvents: []dbmodel.AuditLogEntry{events[3], events[2]},
	}, {
		about: "admin/privileged user is allowed to find audit events with limit/offset",
		users: []*openfga.User{s.adminUser, s.priveligedUser},
		filter: db.AuditLogFilter{
			Offset: 1,
			Limit:  2,
		},
		expectedEvents: []dbmodel.AuditLogEntry{events[1], events[2]},
	}, {
		about: "admin/privileged user - no events found",
		users: []*openfga.User{s.adminUser, s.priveligedUser},
		filter: db.AuditLogFilter{
			IdentityTag: "no-such-user",
		},
	}, {
		about: "unprivileged user is not allowed to access audit events",
		users: []*openfga.User{s.user},
		filter: db.AuditLogFilter{
			IdentityTag: s.adminUser.Tag().String(),
		},
		expectedError: "unauthorized",
	}}
	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			for _, user := range test.users {
				events, err := s.manager.FindAuditEvents(context.Background(), user, test.filter)
				if test.expectedError != "" {
					c.Assert(err, qt.ErrorMatches, test.expectedError)
				} else {
					c.Assert(err, qt.Equals, nil)
					c.Assert(events, qt.DeepEquals, test.expectedEvents)
				}
			}
		})
	}
}

func TestAuditLogManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &auditLogManagerSuite{})
}
