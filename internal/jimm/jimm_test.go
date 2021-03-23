// Copyright 2021 Canonical Ltd.

package jimm_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	qt "github.com/frankban/quicktest"
)

func TestMain(m *testing.M) {
	code := m.Run()
	jimmtest.VaultStop()
	os.Exit(code)
}

func TestFindAuditEvents(t *testing.T) {
	c := qt.New(t)

	now := time.Now().UTC()

	j := &jimm.JIMM{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, nil),
		},
	}

	err := j.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	users := []dbmodel.User{{
		Username:         "alice@external",
		ControllerAccess: "superuser",
	}, {
		Username: "eve@external",
	}}
	for i := range users {
		c.Assert(j.Database.DB.Create(&users[i]).Error, qt.IsNil)
	}

	events := []dbmodel.AuditLogEntry{{
		Time:    now,
		Tag:     "tag-1",
		UserTag: users[0].Tag().String(),
		Action:  "test-action-1",
		Success: true,
		Params: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}, {
		Time:    now.Add(time.Hour),
		Tag:     "tag-2",
		UserTag: users[0].Tag().String(),
		Action:  "test-action-2",
		Success: true,
		Params: map[string]string{
			"key3": "value3",
			"key4": "value4",
		},
	}, {
		Time:    now.Add(2 * time.Hour),
		Tag:     "tag-1",
		UserTag: users[1].Tag().String(),
		Action:  "test-action-3",
		Success: true,
		Params: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}, {
		Time:    now.Add(3 * time.Hour),
		Tag:     "tag-2",
		UserTag: users[1].Tag().String(),
		Action:  "test-action-2",
		Success: true,
		Params: map[string]string{
			"key2": "value3",
			"key5": "value5",
		},
	}}
	for i, event := range events {
		e := event
		j.AddAuditLogEntry(&e)
		events[i] = e
	}

	tests := []struct {
		about          string
		user           *dbmodel.User
		filter         db.AuditLogFilter
		expectedEvents []dbmodel.AuditLogEntry
		expectedError  string
	}{{
		about: "superuser is allower to find audit events by time",
		user:  &users[0],
		filter: db.AuditLogFilter{
			Start: now.Add(-time.Hour),
			End:   now.Add(time.Minute),
		},
		expectedEvents: []dbmodel.AuditLogEntry{events[0]},
	}, {
		about: "superuser is allower to find audit events by action",
		user:  &users[0],
		filter: db.AuditLogFilter{
			Action: "test-action-2",
		},
		expectedEvents: []dbmodel.AuditLogEntry{events[1], events[3]},
	}, {
		about: "superuser is allower to find audit events by tag",
		user:  &users[0],
		filter: db.AuditLogFilter{
			Tag: "tag-1",
		},
		expectedEvents: []dbmodel.AuditLogEntry{events[0], events[2]},
	}, {
		about: "superuser - no events found",
		user:  &users[0],
		filter: db.AuditLogFilter{
			Tag: "no-such-tag",
		},
	}, {
		about: "user is not allowed to access audit events",
		user:  &users[1],
		filter: db.AuditLogFilter{
			Tag: "tag-1",
		},
		expectedError: "unauthorized access",
	}}
	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			events, err := j.FindAuditEvents(context.Background(), test.user, test.filter)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			} else {
				c.Assert(err, qt.Equals, nil)
				c.Assert(events, qt.DeepEquals, test.expectedEvents)
			}
		})
	}
}
