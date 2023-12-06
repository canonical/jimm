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

	"github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
)

func TestFindAuditEvents(t *testing.T) {
	c := qt.New(t)

	now := time.Now().UTC().Truncate(time.Microsecond)

	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
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
		Time:         now,
		UserTag:      admin.User.Tag().String(),
		FacadeMethod: "Login",
	}, {
		Time:         now.Add(time.Hour),
		UserTag:      admin.User.Tag().String(),
		FacadeMethod: "AddModel",
	}, {
		Time:         now.Add(2 * time.Hour),
		UserTag:      privileged.User.Tag().String(),
		Model:        "TestModel",
		FacadeMethod: "Deploy",
	}, {
		Time:         now.Add(3 * time.Hour),
		UserTag:      privileged.User.Tag().String(),
		Model:        "TestModel",
		FacadeMethod: "DestroyModel",
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
		about: "admin/privileged user is allowed to find audit events by user",
		users: []*openfga.User{admin, privileged},
		filter: db.AuditLogFilter{
			UserTag: admin.Tag().String(),
		},
		expectedEvents: []dbmodel.AuditLogEntry{events[0], events[1]},
	}, {
		about: "admin/privileged user is allowed to find audit events by method",
		users: []*openfga.User{admin, privileged},
		filter: db.AuditLogFilter{
			Method: "Deploy",
		},
		expectedEvents: []dbmodel.AuditLogEntry{events[2]},
	}, {
		about: "admin/privileged user is allowed to find audit events by model",
		users: []*openfga.User{admin, privileged},
		filter: db.AuditLogFilter{
			Model: "TestModel",
		},
		expectedEvents: []dbmodel.AuditLogEntry{events[2], events[3]},
	}, {
		about: "admin/privileged user is allowed to find audit events by model and sort by time",
		users: []*openfga.User{admin, privileged},
		filter: db.AuditLogFilter{
			Model:    "TestModel",
			SortTime: true,
		},
		expectedEvents: []dbmodel.AuditLogEntry{events[3], events[2]},
	}, {
		about: "admin/privileged user is allowed to find audit events with limit/offset",
		users: []*openfga.User{admin, privileged},
		filter: db.AuditLogFilter{
			Offset: 1,
			Limit:  2,
		},
		expectedEvents: []dbmodel.AuditLogEntry{events[1], events[2]},
	}, {
		about: "admin/privileged user - no events found",
		users: []*openfga.User{admin, privileged},
		filter: db.AuditLogFilter{
			UserTag: "no-such-user",
		},
	}, {
		about: "unprivileged user is not allowed to access audit events",
		users: []*openfga.User{unprivileged},
		filter: db.AuditLogFilter{
			UserTag: admin.Tag().String(),
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

	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
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
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

	tests := []struct {
		about               string
		user                dbmodel.User
		jimmAdmin           bool
		expectedControllers []dbmodel.Controller
		expectedError       string
	}{{
		about:     "superuser can list controllers",
		user:      env.User("alice@external").DBObject(c, j.Database),
		jimmAdmin: true,
		expectedControllers: []dbmodel.Controller{
			env.Controller("test1").DBObject(c, j.Database),
			env.Controller("test2").DBObject(c, j.Database),
			env.Controller("test3").DBObject(c, j.Database),
		},
	}, {
		about:         "add-model user can not list controllers",
		user:          env.User("bob@external").DBObject(c, j.Database),
		expectedError: "unauthorized",
	}, {
		about:         "user withouth access rights cannot list controllers",
		user:          env.User("eve@external").DBObject(c, j.Database),
		expectedError: "unauthorized",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			user := openfga.NewUser(&test.user, client)
			user.JimmAdmin = test.jimmAdmin
			controllers, err := j.ListControllers(ctx, user)
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

	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
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
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

	tests := []struct {
		about         string
		user          dbmodel.User
		jimmAdmin     bool
		deprecated    bool
		expectedError string
	}{{
		about:      "superuser can deprecate a controller",
		user:       env.User("alice@external").DBObject(c, j.Database),
		jimmAdmin:  true,
		deprecated: true,
	}, {
		about:      "superuser can deprecate a controller",
		user:       env.User("alice@external").DBObject(c, j.Database),
		jimmAdmin:  true,
		deprecated: false,
	}, {
		about:         "user withouth access rights cannot deprecate a controller",
		user:          env.User("eve@external").DBObject(c, j.Database),
		expectedError: "unauthorized",
		deprecated:    true,
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			user := openfga.NewUser(&test.user, client)
			user.JimmAdmin = test.jimmAdmin
			err := j.SetControllerDeprecated(ctx, user, "test1", test.deprecated)
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
		jimmAdmin        bool
		expectedError    string
	}{{
		about:            "superuser can remove an unavailable controller",
		user:             "alice@external",
		unavailableSince: &now,
		force:            true,
		jimmAdmin:        true,
	}, {
		about:     "superuser can remove a live controller with force",
		user:      "alice@external",
		force:     true,
		jimmAdmin: true,
	}, {
		about:         "superuser cannot remove a live controller",
		user:          "alice@external",
		force:         false,
		jimmAdmin:     true,
		expectedError: "controller is still alive",
	}, {
		about:         "add-model user cannot remove a controller",
		user:          "bob@external",
		expectedError: "unauthorized",
		jimmAdmin:     false,
		force:         false,
	}, {
		about:         "user withouth access rights cannot remove a controller",
		user:          "eve@external",
		expectedError: "unauthorized",
		jimmAdmin:     false,
		force:         false,
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {

			j := &jimm.JIMM{
				UUID: uuid.NewString(),
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
				},
			}

			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, removeControllerTestEnv)
			env.PopulateDB(c, j.Database)
			// env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

			dbUser := env.User(test.user).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, nil)
			user.JimmAdmin = test.jimmAdmin

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
		jimmAdmin      bool
		statusFunc     func(context.Context, []string) (*jujuparams.FullStatus, error)
		expectedStatus jujuparams.FullStatus
		expectedError  string
	}{{
		about:     "superuser allowed to see full model status",
		user:      "alice@external",
		modelUUID: "00000002-0000-0000-0000-000000000001",
		jimmAdmin: true,
		statusFunc: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
			return &fullStatus, nil
		},
		expectedStatus: fullStatus,
	}, {
		about:     "model not found",
		user:      "alice@external",
		modelUUID: "00000002-0000-0000-0000-000000000002",
		jimmAdmin: true,
		statusFunc: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
			return &fullStatus, nil
		},
		expectedError: "model not found",
	}, {
		about:     "controller returns an error",
		user:      "alice@external",
		modelUUID: "00000002-0000-0000-0000-000000000001",
		jimmAdmin: true,
		statusFunc: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
			return nil, errors.New("an error")
		},
		expectedError: "an error",
	}, {
		about:     "add-model user not allowed to see full model status",
		user:      "bob@external",
		modelUUID: "00000002-0000-0000-0000-000000000001",
		jimmAdmin: false,
		statusFunc: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
			return &fullStatus, nil
		},
		expectedError: "unauthorized",
	}, {
		about:     "no-access user not allowed to see full model status",
		user:      "eve@external",
		modelUUID: "00000002-0000-0000-0000-000000000001",
		jimmAdmin: false,
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

			j := &jimm.JIMM{
				UUID: uuid.NewString(),
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
				},
				Dialer: &jimmtest.Dialer{
					API: api,
				},
			}

			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, fullModelStatusTestEnv)
			env.PopulateDB(c, j.Database)

			dbUser := env.User(test.user).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, nil)
			user.JimmAdmin = test.jimmAdmin

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

const fillMigrationTargetTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
`

func TestFillMigrationTarget(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	tests := []struct {
		about          string
		userTag        string
		controllerName string
		expectedInfo   jujuparams.MigrationTargetInfo
		expectedError  string
	}{{
		about:          "controller exists",
		userTag:        "alice@external",
		controllerName: "controller-1",
		expectedInfo: jujuparams.MigrationTargetInfo{
			ControllerTag: "controller-00000001-0000-0000-0000-000000000001",
			Addrs:         nil,
			AuthTag:       "user-admin",
			Password:      "test-secret",
		},
	}, {
		about:          "controller doesn't exist",
		userTag:        "alice@external",
		controllerName: "controller-2",
		expectedError:  "controller not found",
	},
	}
	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			db := db.Database{
				DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
			}
			err := db.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			store := &jimmtest.InMemoryCredentialStore{}
			err = store.PutControllerCredentials(context.Background(), test.controllerName, "admin", "test-secret")
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, fillMigrationTargetTestEnv)
			env.PopulateDB(c, db)

			res, controllerID, err := jimm.FillMigrationTarget(db, store, test.controllerName)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
				c.Assert(controllerID, qt.Equals, uint(0))
			} else {
				c.Assert(controllerID, qt.Equals, env.Controllers[0].DBObject(c, db).ID)
				c.Assert(err, qt.IsNil)
				c.Assert(res, qt.DeepEquals, test.expectedInfo)
			}

		})
	}
}

const InitiateMigrationTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
  - owner: alice@external
    name: cred-1
    cloud: test-cloud
controllers:
- name: myController
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
  - name: model-1
    type: iaas
    uuid: 00000002-0000-0000-0000-000000000001
    controller: myController
    default-series: warty
    cloud: test-cloud
    region: test-cloud-region
    cloud-credential: cred-1
    owner: alice@external
    life: alive
users:
  - username: alice@external
    controller-access: superuser
`

func TestInitiateInternalMigration(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	tests := []struct {
		about         string
		user          string
		migrateInfo   params.MigrateModelInfo
		expectedError string
	}{{
		about:       "success",
		user:        "alice@external",
		migrateInfo: params.MigrateModelInfo{ModelTag: "model-00000002-0000-0000-0000-000000000001", TargetController: "myController"},
	}, {
		about:         "model doesn't exist",
		user:          "alice@external",
		migrateInfo:   params.MigrateModelInfo{ModelTag: "model-00000002-0000-0000-0000-000000000002", TargetController: "myController"},
		expectedError: "model not found",
	},
	}
	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {

			c.Patch(jimm.InitiateMigration, func(ctx context.Context, j *jimm.JIMM, user *openfga.User, spec jujuparams.MigrationSpec, targetID uint) (jujuparams.InitiateMigrationResult, error) {
				return jujuparams.InitiateMigrationResult{}, nil
			})
			store := &jimmtest.InMemoryCredentialStore{}
			err := store.PutControllerCredentials(context.Background(), test.migrateInfo.TargetController, "admin", "test-secret")
			c.Assert(err, qt.IsNil)

			j := &jimm.JIMM{
				UUID: uuid.NewString(),
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
				},
				CredentialStore: store,
			}
			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, InitiateMigrationTestEnv)
			env.PopulateDB(c, j.Database)
			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			dbUser := env.User(test.user).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, nil)
			mt, err := names.ParseModelTag(test.migrateInfo.ModelTag)
			c.Assert(err, qt.IsNil)
			res, err := j.InitiateInternalMigration(ctx, user, mt, test.migrateInfo.TargetController)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			} else {
				c.Assert(err, qt.IsNil)
				c.Assert(res, qt.DeepEquals, jujuparams.InitiateMigrationResult{})
			}
		})
	}
}
