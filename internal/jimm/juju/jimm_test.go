// Copyright 2025 Canonical.

package juju_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"gorm.io/gorm/clause"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/testdb"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

const testControllersEnv = `clouds:
- name: test
  type: test
  regions:
  - name: test-region
cloud-credentials:
- name: test-cred
  cloud: test
  owner: alice@canonical.com
  type: empty
controllers:
- name: test1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region-1
  agent-version: 3.2.1
  users:
    - user: bob@canonical.com
      access: add-model
    - user: eve@canonical.com
      access: add-model
- name: test2
  uuid: 00000001-0000-0000-0000-000000000002
  cloud: test
  region: test-region-2
  agent-version: 3.2.0
  users:
    - user: bob@canonical.com
      access: add-model
- name: test3
  uuid: 00000001-0000-0000-0000-000000000003
  cloud: test
  region: test-region-3
  agent-version: 2.1.0
  users:
    - user: eve@canonical.com
      access: add-model
users:
  - username: alice@canonical.com
    controller-access: superuser
`

func TestControllerInfo(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testControllersEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	alice := env.User("alice@canonical.com").DBObject(c, j.Database)
	bob := env.User("bob@canonical.com").DBObject(c, j.Database)
	notAllowed := env.User("notallowedanycontrollers@canonical.com").DBObject(c, j.Database)

	adminUser := openfga.NewUser(&alice, j.OpenFGAClient)
	bobUser := openfga.NewUser(&bob, j.OpenFGAClient)
	notAllowedUser := openfga.NewUser(&notAllowed, j.OpenFGAClient)

	tests := []struct {
		about         string
		user          *openfga.User
		controller    string
		expectedName  string
		expectedError string
	}{
		{
			about:        "jimm admin can access controller",
			user:         adminUser,
			controller:   "test1",
			expectedName: "test1",
		},
		{
			about:        "user with add-model access can access controller",
			user:         bobUser,
			controller:   "test1",
			expectedName: "test1",
		},
		{
			about:         "user without add-model access is unauthorized",
			user:          notAllowedUser,
			controller:    "test1",
			expectedError: "unauthorized",
		},
		{
			about:         "non-existent controller returns not found",
			user:          bobUser,
			controller:    "does-not-exist",
			expectedError: "controller not found",
		},
	}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			ctl, err := j.ControllerInfo(ctx, test.user, test.controller)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
				return
			}
			c.Assert(err, qt.IsNil)
			c.Assert(ctl.Name, qt.Equals, test.expectedName)
		})
	}
}

func TestListControllers(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testControllersEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	tests := []struct {
		about               string
		user                dbmodel.Identity
		jimmAdmin           bool
		expectedControllers []dbmodel.Controller
		expectedError       string
	}{
		{
			about:     "superuser can list controllers",
			user:      env.User("alice@canonical.com").DBObject(c, j.Database),
			jimmAdmin: true,
			expectedControllers: []dbmodel.Controller{
				env.Controller("test1").DBObject(c, j.Database),
				env.Controller("test2").DBObject(c, j.Database),
				env.Controller("test3").DBObject(c, j.Database),
			},
		},
		{
			about:     "bob has can_addmodel to test1 and test2 controllers",
			user:      env.User("bob@canonical.com").DBObject(c, j.Database),
			jimmAdmin: false,
			expectedControllers: []dbmodel.Controller{
				env.Controller("test1").DBObject(c, j.Database),
				env.Controller("test2").DBObject(c, j.Database),
			},
		},
		{
			about:     "eve has can_addmodel to test1 and test3 controllers",
			user:      env.User("eve@canonical.com").DBObject(c, j.Database),
			jimmAdmin: false,
			expectedControllers: []dbmodel.Controller{
				env.Controller("test1").DBObject(c, j.Database),
				env.Controller("test3").DBObject(c, j.Database),
			},
		},
		{
			about:               "user without access cannot list any controllers",
			user:                env.User("notallowedanycontrollers@canonical.com").DBObject(c, j.Database),
			jimmAdmin:           false,
			expectedControllers: nil,
		},
	}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			user := openfga.NewUser(&test.user, j.OpenFGAClient)
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
  owner: alice@canonical.com
  type: empty
controllers:
- name: test1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region-1
  agent-version: 3.2.1
users:
- username: alice@canonical.com
  controller-access: superuser
- username: bob@canonical.com
  controller-access: login
- username: eve@canonical.com
  controller-access: "no-access"
`

func TestSetControllerDeprecated(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testSetControllerDeprecatedEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	tests := []struct {
		about         string
		user          dbmodel.Identity
		jimmAdmin     bool
		deprecated    bool
		expectedError string
	}{{
		about:      "superuser can deprecate a controller",
		user:       env.User("alice@canonical.com").DBObject(c, j.Database),
		jimmAdmin:  true,
		deprecated: true,
	}, {
		about:      "superuser can deprecate a controller",
		user:       env.User("alice@canonical.com").DBObject(c, j.Database),
		jimmAdmin:  true,
		deprecated: false,
	}, {
		about:         "user withouth access rights cannot deprecate a controller",
		user:          env.User("eve@canonical.com").DBObject(c, j.Database),
		expectedError: "unauthorized",
		deprecated:    true,
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			user := openfga.NewUser(&test.user, j.OpenFGAClient)
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

const removeControllerWithModelsTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
users:
- username: alice@canonical.com
  controller-access: superuser
- username: bob@canonical.com
  controller-access: login
- username: eve@canonical.com
  controller-access: "no-access"
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: alive
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: write
  - user: charlie@canonical.com
    access: read
`

const removeControllerWithoutModelsTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
users:
- username: alice@canonical.com
  controller-access: superuser
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
`

func TestRemoveController(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	tests := []struct {
		about         string
		user          string
		force         bool
		env           string
		expectedError string
	}{{
		about: "remove a controller without models",
		user:  "alice@canonical.com",
		force: true,
		env:   removeControllerWithoutModelsTestEnv,
	}, {
		about: "remove a controller with models with force",
		user:  "alice@canonical.com",
		force: true,
		env:   removeControllerWithModelsTestEnv,
	}, {
		about:         "error when removing a controller with models",
		user:          "alice@canonical.com",
		force:         false,
		expectedError: "controller still has models",
		env:           removeControllerWithModelsTestEnv,
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			j := newTestJujuManager(c, nil)

			env := jimmtest.ParseEnvironment(c, test.env)
			env.PopulateDB(c, j.Database)

			dbUser := env.User(test.user).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, nil)

			err := j.RemoveController(ctx, user, "controller-1", test.force)
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

const removeAndAddControllerTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
users:
- username: alice@canonical.com
  controller-access: superuser
- username: bob@canonical.com
  controller-access: login
- username: eve@canonical.com
  controller-access: "no-access"
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: alive
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: write
  - user: charlie@canonical.com
    access: read
`

func TestRemoveAndAddController(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, removeAndAddControllerTestEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)
	controller := env.Controllers[0]

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, j.OpenFGAClient)
	user.JimmAdmin = true

	err := j.RemoveController(ctx, user, "controller-1", true)
	c.Assert(err, qt.Equals, nil)
	ctls, err := j.ListControllers(ctx, user)
	c.Assert(err, qt.Equals, nil)
	c.Assert(len(ctls), qt.Equals, 0)
	// Recreate the controller.
	ctlDbObject := controller.DBObject(c, j.Database)
	ctlDbObject.ID = 0
	err = j.Database.AddController(ctx, &ctlDbObject)
	c.Assert(err, qt.Equals, nil)
	ctls, err = j.ListControllers(ctx, user)
	c.Assert(err, qt.Equals, nil)
	c.Assert(len(ctls), qt.Equals, 1)
}

const fullModelStatusTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
users:
- username: alice@canonical.com
  controller-access: superuser
- username: bob@canonical.com
  controller-access: login
- username: eve@canonical.com
  controller-access: "no-access"
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: alive
`

func TestFullModelStatus(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

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
				Data:   map[string]any{},
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
		user:      "alice@canonical.com",
		modelUUID: "00000002-0000-0000-0000-000000000001",
		jimmAdmin: true,
		statusFunc: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
			return &fullStatus, nil
		},
		expectedStatus: fullStatus,
	}, {
		about:     "model not found",
		user:      "alice@canonical.com",
		modelUUID: "00000002-0000-0000-0000-000000000002",
		jimmAdmin: true,
		statusFunc: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
			return &fullStatus, nil
		},
		expectedError: "model not found",
	}, {
		about:     "controller returns an error",
		user:      "alice@canonical.com",
		modelUUID: "00000002-0000-0000-0000-000000000001",
		jimmAdmin: true,
		statusFunc: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
			return nil, errors.New("an error")
		},
		expectedError: "an error",
	}, {
		about:     "add-model user not allowed to see full model status",
		user:      "bob@canonical.com",
		modelUUID: "00000002-0000-0000-0000-000000000001",
		jimmAdmin: false,
		statusFunc: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
			return &fullStatus, nil
		},
		expectedError: "unauthorized",
	}, {
		about:     "no-access user not allowed to see full model status",
		user:      "eve@canonical.com",
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

			j := newTestJujuManager(c, &parameters{
				Dialer: &jimmtest.Dialer{
					API: api,
				},
			})

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

	tests := []struct {
		about          string
		userTag        string
		controllerName string
		expectedInfo   jujuparams.MigrationTargetInfo
		expectedError  string
	}{{
		about:          "controller exists",
		userTag:        "alice@canonical.com",
		controllerName: "controller-1",
		expectedInfo: jujuparams.MigrationTargetInfo{
			ControllerAlias: "controller-1",
			ControllerTag:   "controller-00000001-0000-0000-0000-000000000001",
			Addrs:           nil,
			AuthTag:         "user-admin",
			Password:        "test-secret",
		},
	}, {
		about:          "controller doesn't exist",
		userTag:        "alice@canonical.com",
		controllerName: "controller-2",
		expectedError:  "controller not found",
	},
	}
	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			db := &db.Database{
				DB: testdb.PostgresDB(c, func() time.Time { return now }),
			}
			err := db.Migrate(ctx)
			c.Assert(err, qt.IsNil)

			store := jimmtest.NewInMemoryCredentialStore()
			err = store.PutControllerCredentials(context.Background(), test.controllerName, "admin", "test-secret")
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, fillMigrationTargetTestEnv)
			env.PopulateDB(c, db)

			res, controllerID, err := juju.FillMigrationTarget(db, store, test.controllerName)
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
  - owner: alice@canonical.com
    name: cred-1
    cloud: test-cloud
controllers:
- name: myController
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
  - name: model-1
    uuid: 00000002-0000-0000-0000-000000000001
    controller: myController
    cloud: test-cloud
    region: test-cloud-region
    cloud-credential: cred-1
    owner: alice@canonical.com
    life: alive
users:
  - username: alice@canonical.com
    controller-access: superuser
`

func TestInitiateInternalMigration(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	tests := []struct {
		about         string
		user          string
		migrateInfo   params.MigrateModelInfo
		expectedError string
	}{{
		about:       "success with uuid",
		user:        "alice@canonical.com",
		migrateInfo: params.MigrateModelInfo{TargetModelNameOrUUID: "00000002-0000-0000-0000-000000000001", TargetController: "myController"},
	}, {
		about:       "a success with name",
		user:        "alice@canonical.com",
		migrateInfo: params.MigrateModelInfo{TargetModelNameOrUUID: "alice@canonical.com/model-1", TargetController: "myController"},
	}, {
		about:         "empty controller name",
		user:          "alice@canonical.com",
		migrateInfo:   params.MigrateModelInfo{TargetModelNameOrUUID: "alice@canonical.com/model-1", TargetController: ""},
		expectedError: `failed to get controller with name "": controller UUID or name must be provided`,
	}, {
		about:         "model doesn't exist",
		user:          "alice@canonical.com",
		migrateInfo:   params.MigrateModelInfo{TargetModelNameOrUUID: "00000002-0000-0000-0000-000000000002", TargetController: "myController"},
		expectedError: "model not found",
	}, {
		about:         "model doesn't exist",
		user:          "alice@canonical.com",
		migrateInfo:   params.MigrateModelInfo{TargetModelNameOrUUID: "00000002-0000-0000-0000-000000000002", TargetController: "myController"},
		expectedError: "model not found",
	}, {
		about:         "a missing model target",
		user:          "alice@canonical.com",
		migrateInfo:   params.MigrateModelInfo{TargetModelNameOrUUID: "alice@canonical.com", TargetController: "myController"},
		expectedError: "invalid model target",
	}, {
		about:         "using an invalid user name",
		user:          "alice@canonical.com",
		migrateInfo:   params.MigrateModelInfo{TargetModelNameOrUUID: "*bad wolf*@canonical.com/model-1", TargetController: "myController"},
		expectedError: "invalid user name",
	}, {
		about:         "using an invalid model name",
		user:          "alice@canonical.com",
		migrateInfo:   params.MigrateModelInfo{TargetModelNameOrUUID: "alice@canonical.com/*bad wolf*", TargetController: "myController"},
		expectedError: "invalid model name",
	}}
	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			c.Patch(juju.InitiateInternalMigration, func(ctx context.Context, j *juju.JujuManager, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error) {
				return jujuparams.InitiateMigrationResult{}, nil
			})
			store := jimmtest.NewInMemoryCredentialStore()
			err := store.PutControllerCredentials(context.Background(), test.migrateInfo.TargetController, "admin", "test-secret")
			c.Assert(err, qt.IsNil)

			j := newTestJujuManager(c, &parameters{
				CredentialStore: store,
			})

			env := jimmtest.ParseEnvironment(c, InitiateMigrationTestEnv)
			env.PopulateDB(c, j.Database)

			dbUser := env.User(test.user).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, nil)

			res, err := j.InitiateInternalMigration(
				ctx,
				user,
				test.migrateInfo.TargetModelNameOrUUID,
				test.migrateInfo.TargetController,
			)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			} else {
				c.Assert(err, qt.IsNil)
				c.Assert(res, qt.DeepEquals, jujuparams.InitiateMigrationResult{})
			}
		})
	}
}

const prepareModelMigrationTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
  - owner: alice@canonical.com
    name: cred-1
    cloud: test-cloud
controllers:
- name: myController
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
users:
  - username: alice@canonical.com
    controller-access: superuser
`

func TestPrepareModelMigration_ControllerDoesNotExist(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	store := jimmtest.NewInMemoryCredentialStore()
	j := newTestJujuManager(c, &parameters{
		CredentialStore: store,
	})

	env := jimmtest.ParseEnvironment(c, prepareModelMigrationTestEnv)
	// We delete the controller from the env, and check for this UUID
	targetControllerName := env.Controllers[0].Name
	env.Controllers = env.Controllers[:0]
	env.PopulateDB(c, j.Database)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	user.JimmAdmin = true

	userMapping := map[string]string{"alice": "alice@canonical.com"}

	fakeModelUUID := "d9a0bd29-a76e-451f-a186-7216cac77e29"

	_, err := j.PrepareModelMigration(
		ctx,
		user,
		fakeModelUUID,
		targetControllerName,
		userMapping,
	)
	c.Assert(err, qt.ErrorMatches, "failed to add incoming model migration details: controller not found")
}

func TestPrepareModelMigration_Success(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	store := jimmtest.NewInMemoryCredentialStore()
	j := newTestJujuManager(c, &parameters{
		CredentialStore: store,
	})

	env := jimmtest.ParseEnvironment(c, prepareModelMigrationTestEnv)
	env.PopulateDB(c, j.Database)
	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)
	user.JimmAdmin = true

	fakeModelUUID := "d9a0bd29-a76e-451f-a186-7216cac77e29"
	userMapping := map[string]string{"alice": "alice@canonical.com"}
	targetController := env.Controllers[0].DBObject(c, j.Database)

	migrationToken, err := j.PrepareModelMigration(
		ctx,
		user,
		fakeModelUUID,
		targetController.Name,
		userMapping,
	)
	c.Assert(err, qt.IsNil)
	c.Assert(string(migrationToken), qt.Equals, "test-migration-token")

	incomingMigration := &dbmodel.IncomingModelMigration{
		ModelUUID:          sql.NullString{String: fakeModelUUID, Valid: true},
		TargetControllerID: targetController.ID,
	}
	err = j.Database.GetIncomingModelMigration(ctx, incomingMigration)
	c.Assert(err, qt.IsNil)

	c.Assert(incomingMigration.ModelUUID.String, qt.Equals, fakeModelUUID)
	c.Assert(incomingMigration.TargetController.UUID, qt.Equals, targetController.UUID)
	c.Assert(incomingMigration.UserMapping, qt.DeepEquals, dbmodel.StringMap(userMapping))
}

const prepareMigrationModelExistsEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
  - owner: alice@canonical.com
    name: cred-1
    cloud: test-cloud
controllers:
- name: myController
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: test-1
  uuid: 00000002-0000-0000-0000-000000000001
  owner: alice@canonical.com
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  controller: myController
`

func TestPrepareMigration_ModelExists(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	store := jimmtest.NewInMemoryCredentialStore()
	j := newTestJujuManager(c, &parameters{
		CredentialStore: store,
	})

	env := jimmtest.ParseEnvironment(c, prepareMigrationModelExistsEnv)
	env.PopulateDB(c, j.Database)
	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	fakeModelUUID := env.Models[0].UUID
	userMapping := map[string]string{"alice": "alice@canonical.com"}
	targetController := env.Controllers[0].DBObject(c, j.Database)

	_, err := j.PrepareModelMigration(ctx, user, fakeModelUUID, targetController.Name, userMapping)
	c.Assert(err, qt.IsNotNil)
}

func TestPrepareMigration_MigrationLocked(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	store := jimmtest.NewInMemoryCredentialStore()
	j := newTestJujuManager(c, &parameters{
		CredentialStore: store,
	})

	env := jimmtest.ParseEnvironment(c, prepareModelMigrationTestEnv)
	env.PopulateDB(c, j.Database)
	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	fakeModelUUID := "d9a0bd29-a76e-451f-a186-7216cac77e29"
	userMapping := map[string]string{"alice": "alice@canonical.com"}
	targetController := env.Controllers[0].DBObject(c, j.Database)

	// First call to PrepareModelMigration should succeed
	_, err := j.PrepareModelMigration(ctx, user, fakeModelUUID, targetController.Name, userMapping)
	c.Assert(err, qt.IsNil)

	notifyChan := make(chan struct{})
	go func() {
		// Lock the migration
		err := j.Database.Transaction(func(d *db.Database) error {
			db := d.DB
			db = db.Clauses(clause.Locking{Strength: "UPDATE"})
			mig := &dbmodel.IncomingModelMigration{ModelUUID: sql.NullString{String: fakeModelUUID, Valid: true}}
			err := db.First(mig).Error
			c.Check(err, qt.IsNil)
			close(notifyChan)
			<-ctx.Done()
			return nil
		})
		c.Check(err, qt.IsNil)
	}()

	<-notifyChan // Wait for the migration to be locked
	_, err = j.PrepareModelMigration(ctx, user, fakeModelUUID, targetController.Name, userMapping)
	c.Assert(err, qt.ErrorMatches, `.*could not obtain lock on row.*`)
}

func TestPrepareMigration_MultipleCalls(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	store := jimmtest.NewInMemoryCredentialStore()
	j := newTestJujuManager(c, &parameters{
		CredentialStore: store,
	})

	env := jimmtest.ParseEnvironment(c, prepareModelMigrationTestEnv)
	env.PopulateDB(c, j.Database)
	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	fakeModelUUID := "d9a0bd29-a76e-451f-a186-7216cac77e29"
	userMapping := map[string]string{"alice": "alice@canonical.com"}
	targetController := env.Controllers[0].DBObject(c, j.Database)

	// First call to PrepareModelMigration should succeed
	_, err := j.PrepareModelMigration(ctx, user, fakeModelUUID, targetController.Name, userMapping)
	c.Assert(err, qt.IsNil)

	// Second call with an updated mapping should succeed
	userMapping["bob"] = "alice@canonical.com"
	_, err = j.PrepareModelMigration(ctx, user, fakeModelUUID, targetController.Name, userMapping)
	c.Assert(err, qt.IsNil)
}

const testMigrationTargetsEnv = `clouds:
- name: test
  type: test
  regions:
  - name: test-region-a
  - name: test-region-b
cloud-credentials:
- name: test-cred
  cloud: test
  owner: alice@canonical.com
  type: empty
controllers:
- name: test1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region-a
  cloud-regions:
    - cloud: test
      region: test-region-a
      priority: 1
  agent-version: 3.2.1
- name: test2
  uuid: 00000001-0000-0000-0000-000000000002
  cloud: test
  region: test-region-b
  cloud-regions:
    - cloud: test
      region: test-region-b
      priority: 1
  agent-version: 3.2.0
- name: test3
  uuid: 00000001-0000-0000-0000-000000000003
  cloud: test
  region: test-region-b
  cloud-regions:
    - cloud: test
      region: test-region-b
      priority: 1
  agent-version: 3.10.0
- name: test4
  uuid: 00000001-0000-0000-0000-000000000004
  cloud: test
  region: test-region-b
  cloud-regions:
    - cloud: test
      region: test-region-b
      priority: 1
  agent-version: 2.1.0
- name: test5
  uuid: 00000001-0000-0000-0000-000000000005
  cloud: test
  region: test-region-b
  cloud-regions:
    - cloud: test
      region: test-region-a
      priority: 1
    - cloud: test
      region: test-region-b
      priority: 1
  agent-version: 3.2.0
models:
- name: test-migratable-1
  uuid: 00000002-0000-0000-0000-000000000001
  owner: alice@canonical.com
  cloud: test
  region: test-region-b
  cloud-credential: test-cred
  controller: test2
- name: test-migratable-3
  uuid: 00000002-0000-0000-0000-000000000003
  owner: alice@canonical.com
  cloud: test
  region: test-region-b
  cloud-credential: test-cred
  controller: test3
users:
- username: alice@canonical.com
  controller-access: superuser
- username: bob@canonical.com
  controller-access: login
- username: eve@canonical.com
  controller-access: "no-access"
`

func TestListMigrationTargets(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testMigrationTargetsEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	tests := []struct {
		about               string
		user                dbmodel.Identity
		expectedControllers []dbmodel.Controller
		expectedError       string
		modelTag            names.ModelTag
	}{{
		about:    "list migratable controllers",
		user:     env.User("alice@canonical.com").DBObject(c, j.Database),
		modelTag: names.NewModelTag(env.Models[0].UUID),
		expectedControllers: []dbmodel.Controller{
			env.Controller("test3").DBObject(c, j.Database),
			env.Controller("test5").DBObject(c, j.Database),
		},
	}, {
		about:         "fails to list controllers for missing model",
		user:          env.User("alice@canonical.com").DBObject(c, j.Database),
		modelTag:      names.NewModelTag("00000002-0000-0000-0000-000000000002"),
		expectedError: `model not found`,
	}, {
		about:               "empty list of controllers for too-new model",
		user:                env.User("alice@canonical.com").DBObject(c, j.Database),
		modelTag:            names.NewModelTag(env.Models[1].UUID),
		expectedControllers: nil,
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			user := openfga.NewUser(&test.user, j.OpenFGAClient)
			controllers, err := j.ListMigrationTargets(ctx, user, test.modelTag)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			} else {
				c.Assert(err, qt.Equals, nil)
				c.Assert(controllers, qt.CmpEquals(cmpopts.IgnoreTypes([]dbmodel.CloudRegionControllerPriority{})), test.expectedControllers)
			}
		})
	}
}

const testModelControllerInfoEnv = `clouds:
- name: test-cloud
  type: test
  regions:
  - name: test-cloud-region
cloud-credentials:
- name: cred-1
  owner: alice@canonical.com
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
- name: controller-2
  uuid: 00000001-0000-0000-0000-000000000002
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: alive
  users:
  - user: alice@canonical.com
    access: admin
- name: model-2
  uuid: 00000002-0000-0000-0000-000000000002
  controller: controller-2
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: bob@canonical.com
  life: alive
  users:
  - user: bob@canonical.com
    access: admin
users:
- username: alice@canonical.com
  controller-access: superuser
- username: bob@canonical.com
  controller-access: login
`

func TestModelControllerInfo(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testModelControllerInfoEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	tests := []struct {
		about         string
		user          dbmodel.Identity
		jimmAdmin     bool
		useModelTag   bool
		modelTag      string
		ownerName     string
		modelName     string
		expectedInfo  *params.ModelControllerInfo
		expectedError string
	}{{
		about:       "model admin can get model controller info by model uuid",
		user:        env.User("alice@canonical.com").DBObject(c, j.Database),
		jimmAdmin:   false,
		useModelTag: true,
		modelTag:    "00000002-0000-0000-0000-000000000001",
		expectedInfo: &params.ModelControllerInfo{
			ModelName:      "model-1",
			ModelUUID:      "00000002-0000-0000-0000-000000000001",
			ControllerName: "controller-1",
			ControllerUUID: "00000001-0000-0000-0000-000000000001",
		},
	}, {
		about:       "model admin can get model controller info for different controller by model uuid",
		user:        env.User("bob@canonical.com").DBObject(c, j.Database),
		jimmAdmin:   false,
		useModelTag: true,
		modelTag:    "00000002-0000-0000-0000-000000000002",
		expectedInfo: &params.ModelControllerInfo{
			ModelName:      "model-2",
			ModelUUID:      "00000002-0000-0000-0000-000000000002",
			ControllerName: "controller-2",
			ControllerUUID: "00000001-0000-0000-0000-000000000002",
		},
	}, {
		about:         "non-model-admin user cannot get model controller info by model uuid",
		user:          env.User("bob@canonical.com").DBObject(c, j.Database),
		jimmAdmin:     false,
		useModelTag:   true,
		modelTag:      "00000002-0000-0000-0000-000000000001",
		expectedError: "unauthorized",
	}, {
		about:         "model admin fails for non-existent model by model uuid",
		user:          env.User("alice@canonical.com").DBObject(c, j.Database),
		jimmAdmin:     false,
		useModelTag:   true,
		modelTag:      "00000002-0000-0000-0000-000000000999",
		expectedError: "model not found",
	}, {
		about:       "model admin can get model controller info by owner and name",
		user:        env.User("alice@canonical.com").DBObject(c, j.Database),
		jimmAdmin:   false,
		useModelTag: false,
		ownerName:   "alice@canonical.com",
		modelName:   "model-1",
		expectedInfo: &params.ModelControllerInfo{
			ModelName:      "model-1",
			ModelUUID:      "00000002-0000-0000-0000-000000000001",
			ControllerName: "controller-1",
			ControllerUUID: "00000001-0000-0000-0000-000000000001",
		},
	}, {
		about:       "model admin can get model controller info for different model by owner and name",
		user:        env.User("bob@canonical.com").DBObject(c, j.Database),
		jimmAdmin:   false,
		useModelTag: false,
		ownerName:   "bob@canonical.com",
		modelName:   "model-2",
		expectedInfo: &params.ModelControllerInfo{
			ModelName:      "model-2",
			ModelUUID:      "00000002-0000-0000-0000-000000000002",
			ControllerName: "controller-2",
			ControllerUUID: "00000001-0000-0000-0000-000000000002",
		},
	}, {
		about:         "non-model-admin user cannot get model controller info by owner and name",
		user:          env.User("bob@canonical.com").DBObject(c, j.Database),
		jimmAdmin:     false,
		useModelTag:   false,
		ownerName:     "alice@canonical.com",
		modelName:     "model-1",
		expectedError: "unauthorized",
	}, {
		about:         "model admin fails for non-existent model by owner and name",
		user:          env.User("alice@canonical.com").DBObject(c, j.Database),
		jimmAdmin:     false,
		useModelTag:   false,
		ownerName:     "alice@canonical.com",
		modelName:     "non-existent-model",
		expectedError: "model not found",
	}, {
		about:         "fails when neither model uuid nor owner/name provided",
		user:          env.User("alice@canonical.com").DBObject(c, j.Database),
		jimmAdmin:     false,
		useModelTag:   false,
		ownerName:     "",
		modelName:     "",
		expectedError: "either model uuid or both model name and owner must be provided",
	}, {
		about:         "fails when only owner name provided",
		user:          env.User("alice@canonical.com").DBObject(c, j.Database),
		jimmAdmin:     false,
		useModelTag:   false,
		ownerName:     "alice@canonical.com",
		modelName:     "",
		expectedError: "either model uuid or both model name and owner must be provided",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			user := openfga.NewUser(&test.user, j.OpenFGAClient)
			user.JimmAdmin = test.jimmAdmin

			var info *params.ModelControllerInfo
			var err error
			if test.useModelTag {
				info, err = j.ModelControllerInfo(ctx, user, juju.WithModelUUID(test.modelTag))
			} else {
				info, err = j.ModelControllerInfo(ctx, user, juju.WithOwnerAndModelName(test.ownerName, test.modelName))
			}

			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
				c.Assert(info, qt.IsNil)
			} else {
				c.Assert(err, qt.IsNil)
				c.Assert(info, qt.DeepEquals, test.expectedInfo)
			}
		})
	}
}

func TestListModelControllerInfo(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testModelControllerInfoEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	userIdentity := env.User("bob@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&userIdentity, j.OpenFGAClient)

	info, err := j.ListModelControllerInfo(ctx, user)
	c.Assert(err, qt.IsNil)
	c.Assert(info, qt.DeepEquals, []params.ModelControllerInfoListItem{{
		ModelName:      "model-2",
		ModelUUID:      "00000002-0000-0000-0000-000000000002",
		ControllerName: "controller-2",
		ControllerUUID: "00000001-0000-0000-0000-000000000002",
	}})
}

const testControllerModelCountEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region
cloud-credentials:
- name: test-cred
  cloud: test-cloud
  owner: alice@canonical.com
  type: empty
controllers:
- name: controller-with-models
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-region
- name: controller-without-models
  uuid: 00000001-0000-0000-0000-000000000002
  cloud: test-cloud
  region: test-region
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-with-models
  cloud: test-cloud
  region: test-region
  cloud-credential: test-cred
  owner: alice@canonical.com
- name: model-2
  uuid: 00000002-0000-0000-0000-000000000002
  controller: controller-with-models
  cloud: test-cloud
  region: test-region
  cloud-credential: test-cred
  owner: alice@canonical.com
`

func TestControllerModelCount(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testControllerModelCountEnv)
	env.PopulateDB(c, j.Database)

	withModels := env.Controllers[0].DBObject(c, j.Database)
	count, err := j.ControllerModelCount(ctx, withModels)
	c.Assert(err, qt.IsNil)
	c.Assert(count, qt.Equals, 2)

	withoutModels := env.Controllers[1].DBObject(c, j.Database)
	count, err = j.ControllerModelCount(ctx, withoutModels)
	c.Assert(err, qt.IsNil)
	c.Assert(count, qt.Equals, 0)
}
