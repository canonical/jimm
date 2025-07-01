// Copyright 2025 Canonical.

package juju_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/description/v9"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

const (
	migratingModelUUID = "00000000-0000-0000-0000-000000000001"
)

const testMigrationEnv = `clouds:
- name: test
  type: test
  regions:
  - name: test-region
controllers:
- name: test1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region-1
  agent-version: 3.2.1
  public-address: foo.com
users:
- username: alice@canonical.com
  controller-access: superuser
`

func TestAbortMigration_Success(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	abortCalled := false
	// Validate that the API request to Juju is made.
	api := &jimmtest.API{
		Abort_: func(uuid string) error {
			abortCalled = true
			c.Check(uuid, qt.Equals, migratingModelUUID)
			return nil
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testMigrationEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	userMap := map[string]string{"bob": "alice@canonical.com"}
	modelMigration := newIncomingMigration(userMap, env.Controller("test1").DBObject(c, j.Database))
	err := j.Database.AddIncomingModelMigration(ctx, &modelMigration)
	c.Assert(err, qt.IsNil)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	err = j.AbortMigration(ctx, user, migratingModelUUID)
	c.Assert(err, qt.IsNil)
	c.Assert(abortCalled, qt.IsTrue)
}

func TestAbortMigration_MissingIncomingModel(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testMigrationEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	err := j.AbortMigration(ctx, user, "foo")
	c.Assert(err, qt.ErrorMatches, `.*model migration not found`)
}

func TestCheckMachines_Success(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	checkMachinesCalled := false
	// Validate that the API request to Juju is made.
	api := &jimmtest.API{
		CheckMachines_: func(uuid string) ([]error, error) {
			checkMachinesCalled = true
			c.Check(uuid, qt.Equals, migratingModelUUID)
			return nil, nil
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testMigrationEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	userMap := map[string]string{"bob": "alice@canonical.com"}
	modelMigration := newIncomingMigration(userMap, env.Controller("test1").DBObject(c, j.Database))
	err := j.Database.AddIncomingModelMigration(ctx, &modelMigration)
	c.Assert(err, qt.IsNil)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	res, err := j.CheckMachines(ctx, user, migratingModelUUID)
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.IsNil)
	c.Assert(checkMachinesCalled, qt.IsTrue)
}

func TestCheckMachines_MissingIncomingModel(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testMigrationEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	_, err := j.CheckMachines(ctx, user, "foo")
	c.Assert(err, qt.ErrorMatches, `.*model migration not found`)
}

func TestControllerDetailsForIncomingModel(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testMigrationEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	// Expect an error when there is no incoming model migration.
	_, err := j.ControllerDetailsForIncomingModel(ctx, migratingModelUUID)
	c.Assert(err, qt.IsNotNil)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	// Create an incoming model migration in the database.
	userMap := map[string]string{"bob": "alice@canonical.com"}
	modelMigration := newIncomingMigration(userMap, env.Controller("test1").DBObject(c, j.Database))
	err = j.Database.AddIncomingModelMigration(ctx, &modelMigration)
	c.Assert(err, qt.IsNil)

	// Expect an error when the controller credentials are not set.
	_, err = j.ControllerDetailsForIncomingModel(ctx, migratingModelUUID)
	c.Assert(err, qt.IsNotNil)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	err = j.CredentialStore.PutControllerCredentials(ctx, "test1", "test-user", "test-password")
	c.Assert(err, qt.IsNil)

	// Expect to retrieve the controller details successfully.
	controllerDetails, err := j.ControllerDetailsForIncomingModel(ctx, migratingModelUUID)
	c.Assert(err, qt.IsNil)
	c.Assert(controllerDetails.PublicAddress, qt.Equals, "foo.com")
	c.Assert(controllerDetails.Credentials.AdminIdentityName, qt.Equals, "test-user")
	c.Assert(controllerDetails.Credentials.AdminPassword, qt.Equals, "test-password")
}

func TestPrechecks_ModifiesModelDescription(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	// Validate that the API request to Juju is made with a modified version
	// of the model description, where the owner is replaced with an external user.
	api := &jimmtest.API{
		Prechecks_: func(mi migration.ModelInfo) error {
			c.Check(mi.UUID, qt.Equals, migratingModelUUID)
			c.Check(mi.Owner.Id(), qt.Equals, "alice@canonical.com")
			c.Check(mi.ModelDescription.Users(), qt.HasLen, 0)
			c.Check(mi.ModelDescription.Owner().String(), qt.Equals, "user-alice@canonical.com")
			return nil
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testMigrationEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	userMap := map[string]string{"bob": "alice@canonical.com"}
	modelMigration := newIncomingMigration(userMap, env.Controller("test1").DBObject(c, j.Database))
	err := j.Database.AddIncomingModelMigration(ctx, &modelMigration)
	c.Assert(err, qt.IsNil)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	model := newMigrationInfo("bob")
	err = j.Prechecks(ctx, user, model)
	c.Assert(err, qt.IsNil)
}

func TestPrechecks_ControllerUnreachable(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	api := &jimmtest.API{
		Prechecks_: func(mi migration.ModelInfo) error {
			return errors.E("controller unreachable")
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testMigrationEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	userMap := map[string]string{"bob": "alice@canonical.com"}
	modelMigration := newIncomingMigration(userMap, env.Controller("test1").DBObject(c, j.Database))
	err := j.Database.AddIncomingModelMigration(ctx, &modelMigration)
	c.Assert(err, qt.IsNil)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	model := newMigrationInfo("bob")
	err = j.Prechecks(ctx, user, model)
	c.Assert(err, qt.ErrorMatches, `.*controller unreachable`)
}

func TestPrechecks_MissingUserMapping(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testMigrationEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	userMap := map[string]string{"random-user": "alice@canonical.com"}
	modelMigration := newIncomingMigration(userMap, env.Controller("test1").DBObject(c, j.Database))
	err := j.Database.AddIncomingModelMigration(ctx, &modelMigration)
	c.Assert(err, qt.IsNil)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	model := newMigrationInfo("bob")
	err = j.Prechecks(ctx, user, model)
	c.Assert(err, qt.ErrorMatches, `.*no external user mapping found for local user "bob"`)
}

func TestPrechecks_NoIncomingModelMigration(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testMigrationEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	model := newMigrationInfo("bob")
	err := j.Prechecks(ctx, user, model)
	c.Assert(err, qt.ErrorMatches, `.*model migration not found`)
}

func TestAdoptResources_Success(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	controllerVersion := version.MustParse("3.2.1")

	// Validate that the API request to Juju is made with a modified version
	// of the model description, where the owner is replaced with an external user.
	api := &jimmtest.API{
		AdoptResources_: func(uuid string, v version.Number) error {
			c.Check(uuid, qt.Equals, migratingModelUUID)
			c.Check(v, qt.DeepEquals, controllerVersion)
			return nil
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testMigrationEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	userMap := map[string]string{"bob": "alice@canonical.com"}
	modelMigration := newIncomingMigration(userMap, env.Controller("test1").DBObject(c, j.Database))
	err := j.Database.AddIncomingModelMigration(ctx, &modelMigration)
	c.Assert(err, qt.IsNil)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	err = j.AdoptResources(ctx, user, migratingModelUUID, controllerVersion)
	c.Assert(err, qt.IsNil)
}

func TestAdoptResources_NoIncomingModelMigration(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testMigrationEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	err := j.AdoptResources(ctx, user, "foo", version.MustParse("3.2.1"))
	c.Assert(err, qt.ErrorMatches, `.*model migration not found`)
}

const testActivateEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
controllers:
- name: test1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  uuid: 00000000-0000-0000-0000-000000000001
  controller: test1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: alive
  migration-mode: importing
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: write
users:
- username: alice@canonical.com
  controller-access: superuser
`

func TestActivate_Success(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	sourceInfo := migration.SourceControllerInfo{
		ControllerTag: names.NewControllerTag("00000001-0000-0000-0000-000000000002"),
	}
	relatedModels := []string{"related-model-1", "related-model-2"}

	// Validate that the API request to Juju is made with the correct parameters.
	api := &jimmtest.API{
		Activate_: func(modelUUID string, sourceInfo migration.SourceControllerInfo, relatedModels []string) error {
			c.Check(modelUUID, qt.Equals, modelUUID)
			c.Check(sourceInfo.ControllerTag.Id(), qt.Equals, "00000001-0000-0000-0000-000000000002")
			c.Check(relatedModels, qt.DeepEquals, []string{"related-model-1", "related-model-2"})
			return nil
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testActivateEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	userMap := map[string]string{"bob": "alice@canonical.com"}
	modelMigration := newIncomingMigration(userMap, env.Controller("test1").DBObject(c, j.Database))
	err := j.Database.AddIncomingModelMigration(ctx, &modelMigration)
	c.Assert(err, qt.IsNil)

	err = j.Activate(ctx, names.NewModelTag(migratingModelUUID), sourceInfo, relatedModels)
	c.Assert(err, qt.IsNil)

	modelMigration = dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: migratingModelUUID,
			Valid:  true,
		},
	}
	// Check the model migration has been removed from the database.
	err = j.Database.GetIncomingModelMigration(ctx, &modelMigration)
	c.Assert(err, qt.ErrorMatches, `.*model migration not found`)
	userMapping := dbmodel.UserMapping{
		ModelUUID: modelMigration.ModelUUID,
		LocalUser: "bob",
	}

	err = j.Database.GetUserMapping(ctx, &userMapping)
	c.Assert(err, qt.IsNil)
	c.Assert(userMapping.ExternalUserName, qt.Equals, "alice@canonical.com")
	model := &dbmodel.Model{
		UUID: sql.NullString{
			String: migratingModelUUID,
			Valid:  true,
		},
	}
	err = j.Database.GetModel(ctx, model)
	c.Assert(err, qt.IsNil)
	c.Assert(model.MigrationMode, qt.Equals, state.MigrationModeNone)
	c.Assert(model.Life, qt.Equals, state.Alive.String())
}

func TestActivate_APIFailure(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	sourceInfo := migration.SourceControllerInfo{}
	relatedModels := []string{"related-model-1", "related-model-2"}

	// Simulate an API failure.
	api := &jimmtest.API{
		Activate_: func(modelUUID string, sourceInfo migration.SourceControllerInfo, relatedModels []string) error {
			return errors.E("API failure")
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testMigrationEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	userMap := map[string]string{"bob": "alice@canonical.com"}
	modelMigration := newIncomingMigration(userMap, env.Controller("test1").DBObject(c, j.Database))
	err := j.Database.AddIncomingModelMigration(ctx, &modelMigration)
	c.Assert(err, qt.IsNil)

	err = j.Activate(ctx, names.NewModelTag(migratingModelUUID), sourceInfo, relatedModels)
	c.Assert(err, qt.ErrorMatches, `.*API failure`)

	modelMigration = dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: migratingModelUUID,
			Valid:  true,
		},
	}
	// Check the model migration has not been removed from the database.
	err = j.Database.GetIncomingModelMigration(ctx, &modelMigration)
	c.Assert(err, qt.IsNil)
}

func TestActivate_NoIncomingModelMigration(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testMigrationEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	err := j.Activate(ctx, names.NewModelTag("foo"), migration.SourceControllerInfo{}, nil)
	c.Assert(err, qt.ErrorMatches, `.*model migration not found`)
}

func newIncomingMigration(userMap map[string]string, ctl dbmodel.Controller) dbmodel.IncomingModelMigration {
	return dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: migratingModelUUID,
			Valid:  true,
		},
		TargetControllerID: ctl.ID,
		UserMapping:        userMap,
	}
}

func newModelDescription(owner string) description.Model {
	descriptionArgs := description.ModelArgs{
		AgentVersion: "3.2.1",
		Owner:        names.NewUserTag(owner),
		Type:         description.IAAS,
		Cloud:        "test",
		Config: map[string]interface{}{
			"uuid": migratingModelUUID,
			"name": "test-model",
		},
		CloudRegion: "test-region",
	}
	modelDescription := description.NewModel(descriptionArgs)
	userArgs := description.UserArgs{
		Name:        names.NewUserTag("bob"),
		DisplayName: "bob",
		Access:      "admin",
	}
	modelDescription.AddUser(userArgs)
	modelDescription.SetCloudCredential(description.CloudCredentialArgs{
		Owner: names.NewUserTag(owner),
		Name:  "test-cred",
		Cloud: names.NewCloudTag("test"),
	})
	return modelDescription
}

func newMigrationInfo(owner string) migration.ModelInfo {
	modelInfo := migration.ModelInfo{
		UUID:                   migratingModelUUID,
		Owner:                  names.NewUserTag(owner),
		Name:                   "test-model",
		AgentVersion:           version.MustParse("3.2.1"),
		ControllerAgentVersion: version.MustParse("3.2.1"),
		ModelDescription:       newModelDescription(owner),
	}
	return modelInfo
}

const migratedModelEnv = `clouds:
- name: test-cloud
  type: test
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
controllers:
- name: test-controller
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-region-1
  agent-version: 3.2.1
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: test-controller
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: alive
`

func TestLatestLogTime_Success(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	modelUUID := "00000002-0000-0000-0000-000000000001"
	latestLogTimeCalled := false
	// Validate that the API request to Juju is made.
	api := &jimmtest.API{
		LatestLogTime_: func(s string) (time.Time, error) {
			latestLogTimeCalled = true
			c.Check(s, qt.Equals, modelUUID)
			return time.Now(), nil
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, migratedModelEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	// Test a model UUID that does not exist
	_, err := j.LatestLogTime(ctx, "does-not-exist")
	c.Assert(err, qt.IsNotNil)

	// Test a model UUID that exists
	logTime, err := j.LatestLogTime(ctx, modelUUID)
	c.Assert(err, qt.IsNil)
	c.Assert(logTime, qt.Not(qt.IsNil))
	c.Assert(latestLogTimeCalled, qt.IsTrue)
}

const testImportEnv = `clouds:
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
  public-address: foo.com
  cloud-regions:
  - cloud: test
    region: test-region
    priority: 1
users:
- username: alice@canonical.com
  controller-access: superuser
`

func TestImport_Success(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	// Validate that the API request to Juju is made with a modified version
	// of the model description, where the owner is replaced with an external user.
	api := &jimmtest.API{
		Import_: func(bytes []byte) error {
			desc, err := description.Deserialize(bytes)
			c.Check(err, qt.IsNil)
			c.Check(desc.Tag().Id(), qt.Equals, migratingModelUUID)
			c.Check(desc.Owner(), qt.Equals, names.NewUserTag("alice@canonical.com"))
			c.Check(desc.Users(), qt.HasLen, 0)
			c.Check(desc.CloudCredential(), qt.Not(qt.IsNil))
			c.Check(desc.CloudCredential().Name(), qt.Equals, "test-cred")
			c.Check(desc.CloudCredential().Owner(), qt.Equals, "alice@canonical.com")
			return nil
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testImportEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	userMap := map[string]string{"bob": "alice@canonical.com"}
	modelMigration := newIncomingMigration(userMap, env.Controller("test1").DBObject(c, j.Database))
	err := j.Database.AddIncomingModelMigration(ctx, &modelMigration)
	c.Assert(err, qt.IsNil)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	desc := newModelDescription("bob")
	desc.SetStatus(description.StatusArgs{Value: "available"})
	bytes, err := description.Serialize(desc)
	c.Assert(err, qt.IsNil)
	err = j.Import(ctx, user, params.SerializedModel{
		Bytes: bytes,
	})
	c.Assert(err, qt.IsNil)

	// Check the model is created in the database with migration mode set to importing.
	m := &dbmodel.Model{
		UUID: sql.NullString{
			String: migratingModelUUID,
			Valid:  true,
		},
	}
	err = j.Database.GetModel(ctx, m)
	c.Assert(err, qt.IsNil)
	c.Assert(m.MigrationMode, qt.Equals, state.MigrationModeImporting)
}

func TestImport_MissingCloudCredentialsFromDescription(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{},
		},
	})

	env := jimmtest.ParseEnvironment(c, testImportEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	userMap := map[string]string{"bob": "alice@canonical.com"}
	modelMigration := newIncomingMigration(userMap, env.Controller("test1").DBObject(c, j.Database))
	err := j.Database.AddIncomingModelMigration(ctx, &modelMigration)
	c.Assert(err, qt.IsNil)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	// Check cloud credential are checked.
	desc := newModelDescription("bob")
	desc.SetStatus(description.StatusArgs{Value: "available"})
	// Intentionally resetting cloud credential to simulate missing cloud credential.
	desc.SetCloudCredential(description.CloudCredentialArgs{})
	bytes, err := description.Serialize(desc)
	c.Assert(err, qt.IsNil)
	err = j.Import(ctx, user, params.SerializedModel{
		Bytes: bytes,
	})
	c.Assert(err, qt.ErrorMatches, "^failed to modify model description.*")

	// Check the model is created in the database with migration mode set to importing.
	m := &dbmodel.Model{
		UUID: sql.NullString{
			String: migratingModelUUID,
			Valid:  true,
		},
	}
	err = j.Database.GetModel(ctx, m)
	c.Assert(err, qt.ErrorMatches, ".*not found.*")
}

func TestImport_UserNotFoundInUserMapping(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{},
		},
	})

	env := jimmtest.ParseEnvironment(c, testImportEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	userMap := map[string]string{"bob": "alice@canonical.com"}
	modelMigration := newIncomingMigration(userMap, env.Controller("test1").DBObject(c, j.Database))
	err := j.Database.AddIncomingModelMigration(ctx, &modelMigration)
	c.Assert(err, qt.IsNil)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	// Check cloud credential are checked.
	desc := newModelDescription("not-in-mapping")
	desc.SetStatus(description.StatusArgs{Value: "available"})
	bytes, err := description.Serialize(desc)
	c.Assert(err, qt.IsNil)
	err = j.Import(ctx, user, params.SerializedModel{
		Bytes: bytes,
	})
	c.Assert(err, qt.ErrorMatches, "^failed to modify model description.*")

	// Check the model is created in the database with migration mode set to importing.
	m := &dbmodel.Model{
		UUID: sql.NullString{
			String: migratingModelUUID,
			Valid:  true,
		},
	}
	err = j.Database.GetModel(ctx, m)
	c.Assert(err, qt.ErrorMatches, ".*not found.*")
}

func TestImport_MissingCloudCredentialFromJIMMState(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{},
		},
	})

	// we intentionally parse an environment that does not have the cloud credential.
	env := jimmtest.ParseEnvironment(c, testMigrationEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	userMap := map[string]string{"bob": "alice@canonical.com"}
	modelMigration := newIncomingMigration(userMap, env.Controller("test1").DBObject(c, j.Database))
	err := j.Database.AddIncomingModelMigration(ctx, &modelMigration)
	c.Assert(err, qt.IsNil)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	// Check cloud credential are checked.
	desc := newModelDescription("bob")
	desc.SetStatus(description.StatusArgs{Value: "available"})
	bytes, err := description.Serialize(desc)
	c.Assert(err, qt.IsNil)
	err = j.Import(ctx, user, params.SerializedModel{
		Bytes: bytes,
	})
	c.Assert(err, qt.ErrorMatches, `^failed to import model from description: cloudcredential \S+ not found$`)

	// Check the model is created in the database with migration mode set to importing.
	m := &dbmodel.Model{
		UUID: sql.NullString{
			String: migratingModelUUID,
			Valid:  true,
		},
	}
	err = j.Database.GetModel(ctx, m)
	c.Assert(err, qt.ErrorMatches, ".*not found.*")
}

func TestImport_APIFailure(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	// Validate that the API request to Juju is made with a modified version
	// of the model description, where the owner is replaced with an external user.
	api := &jimmtest.API{
		Import_: func(bytes []byte) error {
			return errors.E("API failure")
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testImportEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	userMap := map[string]string{"bob": "alice@canonical.com"}
	modelMigration := newIncomingMigration(userMap, env.Controller("test1").DBObject(c, j.Database))
	err := j.Database.AddIncomingModelMigration(ctx, &modelMigration)
	c.Assert(err, qt.IsNil)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	desc := newModelDescription("bob")
	desc.SetStatus(description.StatusArgs{Value: "available"})
	bytes, err := description.Serialize(desc)
	c.Assert(err, qt.IsNil)
	err = j.Import(ctx, user, params.SerializedModel{
		Bytes: bytes,
	})
	c.Assert(err, qt.ErrorMatches, `^failed to import model: API failure$`)

	// Check the model is created in the database with migration mode set to importing.
	m := &dbmodel.Model{
		UUID: sql.NullString{
			String: migratingModelUUID,
			Valid:  true,
		},
	}
	err = j.Database.GetModel(ctx, m)
	c.Assert(err, qt.IsNil)
	c.Assert(m.MigrationMode, qt.Equals, state.MigrationModeImporting)
}
