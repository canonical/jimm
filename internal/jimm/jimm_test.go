// Copyright 2021 Canonical Ltd.

package jimm_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
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

const testListCoControllersEnv = `clouds:
- name: test
  type: test
  regions:
  - name: test-region
cloud-credentials:
- name: test-cred
  cloud: test
  owner: alice@external
  type: empty
controllers:
- name: test1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region-1
  agent-version: 3.2.1
- name: test2
  uuid: 00000001-0000-0000-0000-000000000002
  cloud: test
  region: test-region-2
  agent-version: 3.2.0
- name: test3
  uuid: 00000001-0000-0000-0000-000000000003
  cloud: test
  region: test-region-3
  agent-version: 2.1.0
users:
- username: alice@external
  controller-access: superuser
- username: bob@external
  controller-access: add-model
- username: eve@external
  controller-access: "no-access"
`

func TestListControllers(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	j := &jimm.JIMM{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
		},
	}

	err := j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, testListCoControllersEnv)
	env.PopulateDB(c, j.Database)

	tests := []struct {
		about               string
		user                dbmodel.User
		expectedControllers []dbmodel.Controller
		expectedError       string
	}{{
		about: "superuser can list controllers",
		user:  env.User("alice@external").DBObject(c, j.Database),
		expectedControllers: []dbmodel.Controller{
			env.Controller("test1").DBObject(c, j.Database),
			env.Controller("test2").DBObject(c, j.Database),
			env.Controller("test3").DBObject(c, j.Database),
		},
	}, {
		about:         "add-model user can not list controllers",
		user:          env.User("bob@external").DBObject(c, j.Database),
		expectedError: "unauthorized access",
	}, {
		about:         "user withouth access rights cannot list controllers",
		user:          env.User("eve@external").DBObject(c, j.Database),
		expectedError: "unauthorized access",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			controllers, err := j.ListControllers(ctx, &test.user)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			} else {
				c.Assert(err, qt.Equals, nil)
				c.Assert(controllers, qt.CmpEquals(cmpopts.IgnoreTypes([]dbmodel.CloudRegionControllerPriority{})), test.expectedControllers)
			}
		})
	}
}

const testSetControllerDeprecatedEnv = `clouds:
- name: test
  type: test
  regions:
  - name: test-region
cloud-credentials:
- name: test-cred
  cloud: test
  owner: alice@external
  type: empty
controllers:
- name: test1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region-1
  agent-version: 3.2.1
users:
- username: alice@external
  controller-access: superuser
- username: bob@external
  controller-access: add-model
- username: eve@external
  controller-access: "no-access"
`

func TestSetControllerDeprecated(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	j := &jimm.JIMM{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
		},
	}

	err := j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, testSetControllerDeprecatedEnv)
	env.PopulateDB(c, j.Database)

	tests := []struct {
		about         string
		user          dbmodel.User
		deprecated    bool
		expectedError string
	}{{
		about:      "superuser can deprecate a controller",
		user:       env.User("alice@external").DBObject(c, j.Database),
		deprecated: true,
	}, {
		about:      "superuser can deprecate a controller",
		user:       env.User("alice@external").DBObject(c, j.Database),
		deprecated: false,
	}, {
		about:         "add-model user cannot deprecate a controller",
		expectedError: "unauthorized access",
		deprecated:    true,
	}, {
		about:         "user withouth access rights cannot deprecate a controller",
		user:          env.User("eve@external").DBObject(c, j.Database),
		expectedError: "unauthorized access",
		deprecated:    true,
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			err := j.SetControllerDeprecated(ctx, &test.user, "test1", test.deprecated)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			} else {
				c.Assert(err, qt.Equals, nil)
				controller := dbmodel.Controller{
					Name: "test1",
				}
				err = j.Database.GetController(ctx, &controller)
				c.Assert(err, qt.Equals, nil)
				c.Assert(controller.Deprecated, qt.Equals, test.deprecated)
			}
		})
	}
}

const removeControllerTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@external
  name: cred-1
  cloud: test-cloud
users:
- username: alice@external
  controller-access: superuser
- username: bob@external
  controller-access: add-model
- username: eve@external
  controller-access: "no-access"
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
models:
- name: model-1
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@external
  life: alive
  status:
    status: available
    info: "OK!"
    since: 2020-02-20T20:02:20Z
  users:
  - user: alice@external
    access: admin
  - user: bob@external
    access: write
  - user: charlie@external
    access: read
  machines:
  - id: 0
    hardware:
      arch: amd64
      mem: 8096
      root-disk: 10240
      cores: 1
    instance-id: 00000009-0000-0000-0000-0000000000000
    display-name: Machine 0
    status: available
    message: OK!
    has-vote: true
    wants-vote: false
    ha-primary: false
  - id: 1
    hardware:
      arch: amd64
      mem: 8096
      root-disk: 10240
      cores: 2
    instance-id: 00000009-0000-0000-0000-0000000000001
    display-name: Machine 1
    status: available
    message: OK!
    has-vote: true
    wants-vote: false
    ha-primary: false
  sla:
    level: unsupported
  agent-version: 1.2.3
`

func TestRemoveController(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	tests := []struct {
		about            string
		user             string
		unavailableSince *time.Time
		force            bool
		expectedError    string
	}{{
		about:            "superuser can remove and unavailable controller",
		user:             "alice@external",
		unavailableSince: &now,
		force:            true,
	}, {
		about: "superuser can remove a live controller with force",
		user:  "alice@external",
		force: true,
	}, {
		about:         "superuser cannot remove a live controller",
		user:          "alice@external",
		force:         false,
		expectedError: "controller is still alive",
	}, {
		about:         "add-model user cannot remove a controller",
		user:          "bob@external",
		expectedError: "unauthorized access",
		force:         false,
	}, {
		about:         "user withouth access rights cannot remove a controller",
		user:          "eve@external",
		expectedError: "unauthorized access",
		force:         false,
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
				},
			}

			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, removeControllerTestEnv)
			env.PopulateDB(c, j.Database)

			u := env.User(test.user).DBObject(c, j.Database)

			if test.unavailableSince != nil {
				// make the controller unavailable
				controller := env.Controller("controller-1").DBObject(c, j.Database)
				controller.UnavailableSince = sql.NullTime{
					Valid: true,
					Time:  *test.unavailableSince,
				}
				err = j.Database.UpdateController(ctx, &controller)
				c.Assert(err, qt.Equals, nil)
			}

			err = j.RemoveController(ctx, &u, "controller-1", test.force)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			} else {
				c.Assert(err, qt.Equals, nil)
				controller := dbmodel.Controller{
					Name: "test1",
				}
				err = j.Database.GetController(ctx, &controller)
				c.Assert(err, qt.ErrorMatches, "record not found")
			}
		})
	}
}
