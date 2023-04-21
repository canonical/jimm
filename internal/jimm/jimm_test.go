// Copyright 2021 Canonical Ltd.

package jimm_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	"github.com/CanonicalLtd/jimm/internal/openfga"
	ofganames "github.com/CanonicalLtd/jimm/internal/openfga/names"
)

func TestFindAuditEvents(t *testing.T) {
	c := qt.New(t)

	now := time.Now().UTC()

	_, client, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	j := &jimm.JIMM{
		UUID: uuid.NewString(),
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, nil),
		},
		OpenFGAClient: client,
	}

	ctx := context.Background()

	err = j.Database.Migrate(ctx, true)
	c.Assert(err, qt.Equals, nil)

	admin := openfga.NewUser(&dbmodel.User{Username: "alice@external"}, client)
	err = admin.SetControllerAccess(ctx, j.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	privileged := openfga.NewUser(&dbmodel.User{Username: "bob@external"}, client)
	err = privileged.SetControllerAccess(ctx, j.ResourceTag(), ofganames.AuditLogViewerRelation)
	c.Assert(err, qt.IsNil)

	unprivileged := openfga.NewUser(&dbmodel.User{Username: "eve@external"}, client)

	events := []dbmodel.AuditLogEntry{{
		Time:    now,
		Tag:     "tag-1",
		UserTag: admin.User.Tag().String(),
		Action:  "test-action-1",
		Success: true,
		Params: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}, {
		Time:    now.Add(time.Hour),
		Tag:     "tag-2",
		UserTag: admin.User.Tag().String(),
		Action:  "test-action-2",
		Success: true,
		Params: map[string]string{
			"key3": "value3",
			"key4": "value4",
		},
	}, {
		Time:    now.Add(2 * time.Hour),
		Tag:     "tag-1",
		UserTag: privileged.User.Tag().String(),
		Action:  "test-action-3",
		Success: true,
		Params: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}, {
		Time:    now.Add(3 * time.Hour),
		Tag:     "tag-2",
		UserTag: privileged.User.Tag().String(),
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
		users          []*openfga.User
		filter         db.AuditLogFilter
		expectedEvents []dbmodel.AuditLogEntry
		expectedError  string
	}{{
		about: "admin/privileged user is allowed to find audit events by time",
		users: []*openfga.User{admin, privileged},
		filter: db.AuditLogFilter{
			Start: now.Add(-time.Hour),
			End:   now.Add(time.Minute),
		},
		expectedEvents: []dbmodel.AuditLogEntry{events[0]},
	}, {
		about: "admin/privileged user is allowed to find audit events by action",
		users: []*openfga.User{admin, privileged},
		filter: db.AuditLogFilter{
			Action: "test-action-2",
		},
		expectedEvents: []dbmodel.AuditLogEntry{events[1], events[3]},
	}, {
		about: "admin/privileged user is allowed to find audit events by tag",
		users: []*openfga.User{admin, privileged},
		filter: db.AuditLogFilter{
			Tag: "tag-1",
		},
		expectedEvents: []dbmodel.AuditLogEntry{events[0], events[2]},
	}, {
		about: "admin/privileged user - no events found",
		users: []*openfga.User{admin, privileged},
		filter: db.AuditLogFilter{
			Tag: "no-such-tag",
		},
	}, {
		about: "unprivileged user is not allowed to access audit events",
		users: []*openfga.User{unprivileged},
		filter: db.AuditLogFilter{
			Tag: "tag-1",
		},
		expectedError: "unauthorized",
	}}
	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			for _, user := range test.users {
				events, err := j.FindAuditEvents(context.Background(), user, test.filter)
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
  controller-access: login
- username: eve@external
  controller-access: "no-access"
`

func TestListControllers(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	_, client, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	j := &jimm.JIMM{
		UUID: uuid.NewString(),
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
		},
		OpenFGAClient: client,
	}

	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, testListCoControllersEnv)
	env.PopulateDB(c, j.Database, client)
	env.AddJIMMRelations(c, j.ResourceTag(), j.Database, client)

	tests := []struct {
		about               string
		user                dbmodel.User
		expectedControllers []dbmodel.Controller
		expectedError       string
	}{{
		about: "superuser can list controllers",
		user:  env.User("alice@external").DBObject(c, j.Database, client),
		expectedControllers: []dbmodel.Controller{
			env.Controller("test1").DBObject(c, j.Database, client),
			env.Controller("test2").DBObject(c, j.Database, client),
			env.Controller("test3").DBObject(c, j.Database, client),
		},
	}, {
		about:         "add-model user can not list controllers",
		user:          env.User("bob@external").DBObject(c, j.Database, client),
		expectedError: "unauthorized",
	}, {
		about:         "user withouth access rights cannot list controllers",
		user:          env.User("eve@external").DBObject(c, j.Database, client),
		expectedError: "unauthorized",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			controllers, err := j.ListControllers(ctx, openfga.NewUser(&test.user, client))
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
  controller-access: login
- username: eve@external
  controller-access: "no-access"
`

func TestSetControllerDeprecated(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	_, client, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	j := &jimm.JIMM{
		UUID: uuid.NewString(),
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
		},
		OpenFGAClient: client,
	}

	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, testSetControllerDeprecatedEnv)
	env.PopulateDB(c, j.Database, client)
	env.AddJIMMRelations(c, j.ResourceTag(), j.Database, client)

	tests := []struct {
		about         string
		user          dbmodel.User
		deprecated    bool
		expectedError string
	}{{
		about:      "superuser can deprecate a controller",
		user:       env.User("alice@external").DBObject(c, j.Database, client),
		deprecated: true,
	}, {
		about:      "superuser can deprecate a controller",
		user:       env.User("alice@external").DBObject(c, j.Database, client),
		deprecated: false,
	}, {
		about:         "user withouth access rights cannot deprecate a controller",
		user:          env.User("eve@external").DBObject(c, j.Database, client),
		expectedError: "unauthorized",
		deprecated:    true,
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			err := j.SetControllerDeprecated(ctx, openfga.NewUser(&test.user, client), "test1", test.deprecated)
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
  controller-access: login
- username: eve@external
  controller-access: "no-access"
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
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
		about:            "superuser can remove an unavailable controller",
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
		expectedError: "unauthorized",
		force:         false,
	}, {
		about:         "user withouth access rights cannot remove a controller",
		user:          "eve@external",
		expectedError: "unauthorized",
		force:         false,
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			_, client, _, err := jimmtest.SetupTestOFGAClient(c.Name())
			c.Assert(err, qt.IsNil)

			j := &jimm.JIMM{
				UUID: uuid.NewString(),
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
				},
				OpenFGAClient: client,
			}

			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, removeControllerTestEnv)
			env.PopulateDB(c, j.Database, client)
			env.AddJIMMRelations(c, j.ResourceTag(), j.Database, client)

			dbUser := env.User(test.user).DBObject(c, j.Database, client)
			user := openfga.NewUser(&dbUser, client)

			if test.unavailableSince != nil {
				// make the controller unavailable
				controller := env.Controller("controller-1").DBObject(c, j.Database, client)
				controller.UnavailableSince = sql.NullTime{
					Valid: true,
					Time:  *test.unavailableSince,
				}
				err = j.Database.UpdateController(ctx, &controller)
				c.Assert(err, qt.Equals, nil)
			}

			err = j.RemoveController(ctx, user, "controller-1", test.force)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			} else {
				c.Assert(err, qt.Equals, nil)
				controller := dbmodel.Controller{
					Name: "test1",
				}
				err = j.Database.GetController(ctx, &controller)
				c.Assert(err, qt.ErrorMatches, "controller not found")
			}
		})
	}
}

const fullModelStatusTestEnv = `clouds:
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
  controller-access: login
- username: eve@external
  controller-access: "no-access"
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
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
`

func TestFullModelStatus(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	fullStatus := jujuparams.FullStatus{
		Model: jujuparams.ModelStatusInfo{
			Name:             "model-1",
			Type:             "iaas",
			CloudTag:         "cloud-test-cloud",
			CloudRegion:      "test-cloud-region",
			Version:          "2.9-rc7",
			AvailableVersion: "",
			ModelStatus: jujuparams.DetailedStatus{
				Status: "available",
				Info:   "",
				Data:   map[string]interface{}{},
			},
			SLA: "unsupported",
		},
		Machines:           map[string]jujuparams.MachineStatus{},
		Applications:       map[string]jujuparams.ApplicationStatus{},
		RemoteApplications: map[string]jujuparams.RemoteApplicationStatus{},
		Offers:             map[string]jujuparams.ApplicationOfferStatus{},
		Relations:          []jujuparams.RelationStatus(nil),
		Branches:           map[string]jujuparams.BranchStatus{},
	}

	tests := []struct {
		about          string
		user           string
		modelUUID      string
		statusFunc     func(context.Context, []string) (*jujuparams.FullStatus, error)
		expectedStatus jujuparams.FullStatus
		expectedError  string
	}{{
		about:     "superuser allowed to see full model status",
		user:      "alice@external",
		modelUUID: "00000002-0000-0000-0000-000000000001",
		statusFunc: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
			return &fullStatus, nil
		},
		expectedStatus: fullStatus,
	}, {
		about:     "model not found",
		user:      "alice@external",
		modelUUID: "00000002-0000-0000-0000-000000000002",
		statusFunc: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
			return &fullStatus, nil
		},
		expectedError: "model not found",
	}, {
		about:     "controller returns an error",
		user:      "alice@external",
		modelUUID: "00000002-0000-0000-0000-000000000001",
		statusFunc: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
			return nil, errors.New("an error")
		},
		expectedError: "an error",
	}, {
		about:     "add-model user not allowed to see full model status",
		user:      "bob@external",
		modelUUID: "00000002-0000-0000-0000-000000000001",
		statusFunc: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
			return &fullStatus, nil
		},
		expectedError: "unauthorized",
	}, {
		about:     "no-access user not allowed to see full model status",
		user:      "eve@external",
		modelUUID: "00000002-0000-0000-0000-000000000001",
		statusFunc: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
			return &fullStatus, nil
		},
		expectedError: "unauthorized",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			api := &jimmtest.API{
				Status_: test.statusFunc,
			}

			_, client, _, err := jimmtest.SetupTestOFGAClient(c.Name())
			c.Assert(err, qt.IsNil)

			j := &jimm.JIMM{
				UUID: uuid.NewString(),
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
				},
				Dialer: &jimmtest.Dialer{
					API: api,
				},
				OpenFGAClient: client,
			}

			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, fullModelStatusTestEnv)
			env.PopulateDB(c, j.Database, client)
			env.AddJIMMRelations(c, j.ResourceTag(), j.Database, client)

			dbUser := env.User(test.user).DBObject(c, j.Database, client)
			user := openfga.NewUser(&dbUser, client)

			status, err := j.FullModelStatus(ctx, user, names.NewModelTag(test.modelUUID), nil)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			} else {
				c.Assert(err, qt.Equals, nil)
				c.Assert(status, qt.DeepEquals, &test.expectedStatus)
			}
		})
	}
}
