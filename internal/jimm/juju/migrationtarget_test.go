// Copyright 2025 Canonical.

package juju_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	descriptionv9 "github.com/juju/description/v9"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

const (
	migratingModelUUID = "00000000-0000-0000-0000-000000000001"
	offerUUID          = "00000012-0000-0000-0000-000000000001"
)

const testEnvWithIncomingMigration = `clouds:
- name: test
  type: test
  regions:
  - name: test-region
cloud-credentials:
- owner: alice@canonical.com
  name: test-cred
  cloud: test
controllers:
- name: test1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region-1
  agent-version: 3.6.9
  public-address: foo.com
users:
- username: alice@canonical.com
  controller-access: superuser
incoming-migrations:
- model-uuid: ` + migratingModelUUID + `
  target-controller: test1
  user-mapping:
  - local-user: bob
    external-user: alice@canonical.com
`

const testEnvNoIncomingMigration = `clouds:
- name: test
  type: test
  regions:
  - name: test-region
controllers:
- name: test1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region-1
  agent-version: 3.6.9
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

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigrationAndModel)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, j.OpenFGAClient)

	err := j.AbortMigration(ctx, user, migratingModelUUID)
	c.Assert(err, qt.IsNil)
	c.Assert(abortCalled, qt.IsTrue)

	// Check the model migration has been removed from the database.
	modelMigration := dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: migratingModelUUID,
			Valid:  true,
		},
	}
	err = j.Database.GetIncomingModelMigration(ctx, &modelMigration)
	c.Assert(err, qt.ErrorMatches, `.*model migration not found`)

	// Check the model has been deleted from the database.
	model := &dbmodel.Model{
		UUID: sql.NullString{
			String: migratingModelUUID,
			Valid:  true,
		},
	}
	err = j.Database.GetModel(ctx, model)
	c.Assert(err, qt.ErrorMatches, `.*model not found`)

	// Check that permissions for the user have been removed.
	ok, err := user.IsModelAdmin(ctx, names.NewModelTag(migratingModelUUID))
	c.Assert(err, qt.IsNil)
	c.Assert(ok, qt.IsFalse)
	ok, err = user.IsApplicationOfferConsumer(ctx, names.NewApplicationOfferTag(offerUUID))
	c.Assert(err, qt.IsNil)
	c.Assert(ok, qt.IsFalse)
}

func TestAbortMigration_MissingIncomingModel(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
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

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

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

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
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

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	// Expect an error when there is no incoming model migration.
	_, err := j.ControllerDetailsForIncomingModel(ctx, migratingModelUUID)
	c.Assert(err, qt.IsNotNil)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

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

func toJimmMigratingInfo(c *qt.C, modelInfo migration.ModelInfo, desc descriptionv9.Model) juju.MigratingModelInfo {
	rawDescription, err := descriptionv9.Serialize(desc)
	c.Assert(err, qt.IsNil)
	return juju.MigratingModelInfo{
		UUID:                   modelInfo.UUID,
		Owner:                  modelInfo.Owner,
		Name:                   modelInfo.Name,
		AgentVersion:           modelInfo.AgentVersion,
		ControllerAgentVersion: modelInfo.ControllerAgentVersion,
		RawModelDescription:    rawDescription,
	}
}

func TestPreChecks_NoUsersWithAccess(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	api := &jimmtest.API{
		Prechecks_: func(mi params.MigrationModelInfo) error {
			return nil
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, j.OpenFGAClient)

	// Below is a simple model description with no users that have access.
	descriptionArgs := modelDescriptionArgs{
		Owner:           "bob",
		CloudName:       "test",
		CloudRegionName: "test-region",
	}

	modelDescription := newModelDescription(descriptionArgs)
	modelDescription.SetUsers(nil)

	modelDescription.SetCloudCredential(descriptionv9.CloudCredentialArgs{
		Owner: names.NewUserTag("bob"),
		Name:  "test-cred",
		Cloud: names.NewCloudTag("test"),
	})

	rawDescription, err := descriptionv9.Serialize(modelDescription)
	c.Assert(err, qt.IsNil)

	modelInfo := juju.MigratingModelInfo{
		UUID:                   migratingModelUUID,
		Owner:                  names.NewUserTag("bob"),
		Name:                   "test-model",
		AgentVersion:           version.MustParse("3.6.9"),
		ControllerAgentVersion: version.MustParse("3.6.9"),
		RawModelDescription:    rawDescription,
	}
	err = j.Prechecks(ctx, user, modelInfo)
	c.Assert(err, qt.IsNil)
}

func TestPreChecks_ValidatesUserMapping(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	err := j.Prechecks(ctx, user, modelInfoWithUnmappedUsers(c))
	c.Assert(err, qt.ErrorMatches, `(?ms).*^expected user \"jane\" who has admin access to the model$.*`)
	c.Assert(err, qt.ErrorMatches, `(?ms).*^expected user \"jack\" who has admin access to offer "test-offer"$.*`)
}

func modelInfoWithUnmappedUsers(c *qt.C) juju.MigratingModelInfo {
	descriptionArgs := modelDescriptionArgs{
		Owner:           "bob",
		CloudName:       "test",
		CloudRegionName: "test-region",
	}
	modelDescription := newModelDescription(descriptionArgs)
	modelDescription.SetUsers(nil)

	// Add a user with admin access that is not mapped.
	userArgs := descriptionv9.UserArgs{
		Name:        names.NewUserTag("jane"),
		DisplayName: "jane",
		Access:      "admin",
	}
	modelDescription.AddUser(userArgs)
	modelDescription.SetCloudCredential(descriptionv9.CloudCredentialArgs{
		Owner: names.NewUserTag("bob"),
		Name:  "test-cred",
		Cloud: names.NewCloudTag("test"),
	})
	appArgs := descriptionv9.ApplicationArgs{
		Tag: names.NewApplicationTag("foo"),
	}
	app := modelDescription.AddApplication(appArgs)
	app.SetStatus(descriptionv9.StatusArgs{
		Value: "available",
	})

	// Add an offer with an ACL for the everyone@external user.
	offerArgs := descriptionv9.ApplicationOfferArgs{
		OfferUUID:              "86d97176-c9a7-4333-bb54-84f85f7d8aaa",
		OfferName:              "test-offer",
		ACL:                    map[string]string{"jack": "admin"},
		ApplicationName:        "foo",
		Endpoints:              map[string]string{"foo": "foo"},
		ApplicationDescription: "a description",
	}
	app.AddOffer(offerArgs)

	rawDescription, err := descriptionv9.Serialize(modelDescription)
	c.Assert(err, qt.IsNil)

	modelInfo := juju.MigratingModelInfo{
		UUID:                   migratingModelUUID,
		Owner:                  names.NewUserTag("bob"),
		Name:                   "test-model",
		AgentVersion:           version.MustParse("3.6.9"),
		ControllerAgentVersion: version.MustParse("3.6.9"),
		RawModelDescription:    rawDescription,
	}
	return modelInfo
}

func TestPreChecks_SkipsEveryoneUser(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	api := &jimmtest.API{
		Prechecks_: func(mmi params.MigrationModelInfo) error {
			return nil
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	model, desc := newMigrationInfo(modelDescriptionArgs{
		Owner:               "bob",
		ModelName:           "test-model",
		CloudName:           "test",
		CloudCredentialName: "test-cred",
		CloudRegionName:     "test-region",
	})
	everyoneUserArgs := descriptionv9.UserArgs{
		Name:   names.NewUserTag("everyone@external"),
		Access: "read",
	}
	desc.AddUser(everyoneUserArgs)

	appArgs := descriptionv9.ApplicationArgs{
		Tag: names.NewApplicationTag("foo"),
	}
	app := desc.AddApplication(appArgs)
	app.SetStatus(descriptionv9.StatusArgs{
		Value: "available",
	})

	// Add an offer with an ACL for the everyone@external user.
	offerArgs := descriptionv9.ApplicationOfferArgs{
		OfferUUID:              "86d97176-c9a7-4333-bb54-84f85f7d8aaa",
		OfferName:              "test-offer",
		ACL:                    map[string]string{"everyone@external": "read"},
		ApplicationName:        "foo",
		Endpoints:              map[string]string{"foo": "foo"},
		ApplicationDescription: "a description",
	}
	app.AddOffer(offerArgs)

	err := j.Prechecks(ctx, user, toJimmMigratingInfo(c, model, desc))
	c.Assert(err, qt.IsNil)
}

func TestPrechecks_ModifiesModelDescription(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	// Validate that the API request to Juju is made with a modified version
	// of the model description, where the owner is replaced with an external user.
	api := &jimmtest.API{
		Prechecks_: func(mmi params.MigrationModelInfo) error {
			c.Check(mmi.UUID, qt.Equals, migratingModelUUID)
			c.Check(mmi.OwnerTag, qt.Equals, "user-alice@canonical.com")
			// Deserialize the model description and validate its contents.
			modelDescription, err := descriptionv9.Deserialize(mmi.ModelDescription)
			c.Check(err, qt.IsNil)
			if err != nil {
				return err
			}
			c.Check(modelDescription.Users(), qt.HasLen, 0)
			c.Check(modelDescription.Owner().String(), qt.Equals, "user-alice@canonical.com")
			return nil
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	model, desc := newMigrationInfo(modelDescriptionArgs{
		Owner:               "bob",
		ModelName:           "test-model",
		CloudName:           "test",
		CloudCredentialName: "test-cred",
		CloudRegionName:     "test-region",
	})
	err := j.Prechecks(ctx, user, toJimmMigratingInfo(c, model, desc))
	c.Assert(err, qt.IsNil)
}

func TestPrechecks_MissingCloudRegion(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	api := &jimmtest.API{}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	model, desc := newMigrationInfo(modelDescriptionArgs{
		Owner:               "bob",
		ModelName:           "test-model-2",
		CloudName:           "test",
		CloudCredentialName: "test-cred",
		CloudRegionName:     "test-region-not-found",
	})

	err := j.Prechecks(ctx, user, toJimmMigratingInfo(c, model, desc))
	c.Assert(err, qt.ErrorMatches, `^failed to find region for cloud "test".*`)
}

func TestPrechecks_MissingCloud(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	api := &jimmtest.API{}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	model, desc := newMigrationInfo(modelDescriptionArgs{
		Owner:               "bob",
		ModelName:           "test-model-2",
		CloudName:           "test-not-found",
		CloudCredentialName: "test-cred",
		CloudRegionName:     "test-region",
	})

	err := j.Prechecks(ctx, user, toJimmMigratingInfo(c, model, desc))
	c.Assert(err, qt.ErrorMatches, `^failed to find region for cloud "test-not-found".*`)
}

func TestPrechecks_MissingCloudCredential(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	api := &jimmtest.API{}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	model, desc := newMigrationInfo(modelDescriptionArgs{
		Owner:               "bob",
		ModelName:           "test-model-2",
		CloudName:           "test",
		CloudCredentialName: "test-cred-not-found",
		CloudRegionName:     "test-region",
	})

	err := j.Prechecks(ctx, user, toJimmMigratingInfo(c, model, desc))
	c.Assert(err, qt.ErrorMatches, `^cloudcredential "test/alice@canonical.com/test-cred-not-found" not found$`)
}

func TestPrechecks_ControllerUnreachable(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	api := &jimmtest.API{
		Prechecks_: func(mmi params.MigrationModelInfo) error {
			return errors.New("controller unreachable")
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	model, desc := newMigrationInfo(modelDescriptionArgs{
		Owner:               "bob",
		ModelName:           "test-model",
		CloudName:           "test",
		CloudCredentialName: "test-cred",
		CloudRegionName:     "test-region",
	})
	err := j.Prechecks(ctx, user, toJimmMigratingInfo(c, model, desc))
	c.Assert(err, qt.ErrorMatches, `.*controller unreachable`)
}

func TestPrechecks_MissingUserMapping(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	model, desc := newMigrationInfo(modelDescriptionArgs{
		Owner:               "not-found-user",
		ModelName:           "test-model",
		CloudName:           "test",
		CloudCredentialName: "test-cred",
		CloudRegionName:     "test-region",
	})
	err := j.Prechecks(ctx, user, toJimmMigratingInfo(c, model, desc))
	c.Assert(err, qt.ErrorMatches, `(?s).*user mapping is missing the following users.*`)
}

func TestPrechecks_InvalidOwner(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	incomingModel := env.IncomingMigrations[0].DBObject(c, j.Database)
	incomingModel.UserMapping["bob"] = ""
	err := j.Database.AddOrUpdateIncomingModelMigration(ctx, &incomingModel)
	c.Assert(err, qt.IsNil)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	model, desc := newMigrationInfo(modelDescriptionArgs{
		Owner:               "bob",
		ModelName:           "test-model",
		CloudName:           "test",
		CloudCredentialName: "test-cred",
		CloudRegionName:     "test-region",
	})
	err = j.Prechecks(ctx, user, toJimmMigratingInfo(c, model, desc))
	c.Assert(err, qt.ErrorMatches, `.*invalid external user mapping "" for local user "bob"`)
}

func TestPrechecks_NoIncomingModelMigration(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testEnvNoIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	model, desc := newMigrationInfo(modelDescriptionArgs{
		Owner:               "bob",
		ModelName:           "test-model",
		CloudName:           "test",
		CloudCredentialName: "test-cred",
		CloudRegionName:     "test-region",
	})
	err := j.Prechecks(ctx, user, toJimmMigratingInfo(c, model, desc))
	c.Assert(err, qt.ErrorMatches, `.*model migration not found`)
}

func TestAdoptResources_Success(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	// Note that adopt resources is called after the model
	// has been activated so the incoming model migration
	// row was deleted and the model has been created.

	controllerVersion := version.MustParse("3.6.9")
	modelUUID := "00000002-0000-0000-0000-000000000001"

	// Validate that the API request to Juju is made with a modified version
	// of the model description, where the owner is replaced with an external user.
	api := &jimmtest.API{
		AdoptResources_: func(uuid string, v version.Number) error {
			c.Check(uuid, qt.Equals, modelUUID)
			c.Check(v, qt.DeepEquals, controllerVersion)
			return nil
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, migratedModelEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	err := j.AdoptResources(ctx, user, modelUUID, controllerVersion)
	c.Assert(err, qt.IsNil)
}

func TestAdoptResources_NoModel(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	err := j.AdoptResources(ctx, nil, "foo", version.MustParse("3.6.9"))
	c.Assert(err, qt.ErrorMatches, `.*model not found`)
}

const testEnvWithIncomingMigrationAndModel = `clouds:
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
application-offers:
- name: offer-1
  url: test-offer-url
  uuid: 00000012-0000-0000-0000-000000000001
  model-name: model-1
  model-owner: alice@canonical.com
  application-name: application-1
  application-description: app description 1
users:
- username: alice@canonical.com
  controller-access: superuser
incoming-migrations:
- model-uuid: ` + migratingModelUUID + `
  target-controller: test1
  user-mapping:
  - local-user: bob
    external-user: alice@canonical.com
  - local-user: jane
    external-user: ""
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

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigrationAndModel)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	incomingModelMigration := dbmodel.IncomingModelMigration{}
	err := j.Database.DB.First(&incomingModelMigration).Error
	c.Assert(err, qt.IsNil)
	c.Assert(incomingModelMigration.UserMapping["jane"], qt.Equals, "")

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, j.OpenFGAClient)

	err = j.Activate(ctx, user, names.NewModelTag(migratingModelUUID), sourceInfo, relatedModels)
	c.Assert(err, qt.IsNil)

	modelMigration := dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: migratingModelUUID,
			Valid:  true,
		},
	}
	// Check the model migration has been removed from the database.
	err = j.Database.GetIncomingModelMigration(ctx, &modelMigration)
	c.Assert(err, qt.ErrorMatches, `.*model migration not found`)

	var count int64
	err = j.Database.DB.Model(&dbmodel.UserMapping{}).Count(&count).Error
	c.Assert(err, qt.IsNil)
	c.Assert(count, qt.Equals, int64(1))

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
	c.Assert(model.MigrationMode, qt.Equals, dbmodel.MigrationModeNone)
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
			return errors.New("API failure")
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, j.OpenFGAClient)

	err := j.Activate(ctx, user, names.NewModelTag(migratingModelUUID), sourceInfo, relatedModels)
	c.Assert(err, qt.ErrorMatches, `.*API failure`)

	modelMigration := dbmodel.IncomingModelMigration{
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

	env := jimmtest.ParseEnvironment(c, testEnvNoIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, j.OpenFGAClient)

	err := j.Activate(ctx, user, names.NewModelTag("foo"), migration.SourceControllerInfo{}, nil)
	c.Assert(err, qt.ErrorMatches, `.*model migration not found`)
}

type modelDescriptionArgs struct {
	Owner               string
	ModelName           string
	CloudName           string
	CloudCredentialName string
	CloudRegionName     string
}

func newModelDescription(args modelDescriptionArgs) descriptionv9.Model {
	descriptionArgs := descriptionv9.ModelArgs{
		AgentVersion: "3.6.9",
		Owner:        names.NewUserTag(args.Owner),
		Type:         descriptionv9.IAAS,
		Cloud:        args.CloudName,
		Config: map[string]interface{}{
			"uuid": migratingModelUUID,
			"name": args.ModelName,
		},
		CloudRegion: args.CloudRegionName,
	}
	modelDescription := descriptionv9.NewModel(descriptionArgs)
	userArgs := descriptionv9.UserArgs{
		Name:        names.NewUserTag(args.Owner),
		DisplayName: args.Owner,
		Access:      "admin",
	}
	modelDescription.AddUser(userArgs)
	modelDescription.SetCloudCredential(descriptionv9.CloudCredentialArgs{
		Owner: names.NewUserTag(args.Owner),
		Name:  args.CloudCredentialName,
		Cloud: names.NewCloudTag(args.CloudName),
	})
	modelDescription.SetStatus(descriptionv9.StatusArgs{Value: "available"})
	return modelDescription
}

func newMigrationInfo(args modelDescriptionArgs) (migration.ModelInfo, descriptionv9.Model) {
	modelInfo := migration.ModelInfo{
		UUID:                   migratingModelUUID,
		Owner:                  names.NewUserTag(args.Owner),
		Name:                   "test-model",
		AgentVersion:           version.MustParse("3.6.9"),
		ControllerAgentVersion: version.MustParse("3.6.9"),
	}

	return modelInfo, newModelDescription(args)
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
  agent-version: 3.6.9
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

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, j.OpenFGAClient)

	// Test a model UUID that does not exist
	_, err := j.LatestLogTime(ctx, user, "does-not-exist")
	c.Assert(err, qt.IsNotNil)

	// Test a model UUID that exists
	logTime, err := j.LatestLogTime(ctx, user, modelUUID)
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
  agent-version: 3.6.9
  public-address: foo.com
  cloud-regions:
  - cloud: test
    region: test-region
    priority: 1
users:
- username: alice@canonical.com
  controller-access: superuser
incoming-migrations:
- model-uuid: ` + migratingModelUUID + `
  target-controller: test1
  user-mapping:
  - local-user: bob
    external-user: alice@canonical.com
`

func TestImport_Success(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	// Validate that the API request to Juju is made with a modified version
	// of the model description, where the owner is replaced with an external user.
	api := &jimmtest.API{
		Import_: func(bytes []byte) error {
			desc, err := descriptionv9.Deserialize(bytes)
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

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, j.OpenFGAClient)

	desc := newModelDescription(modelDescriptionArgs{
		Owner:               "bob",
		ModelName:           "test-model",
		CloudName:           "test",
		CloudCredentialName: "test-cred",
		CloudRegionName:     "test-region",
	})

	app := desc.AddApplication(descriptionv9.ApplicationArgs{
		Tag: names.NewApplicationTag("test-app"),
	})
	appOfferUUID := "00000000-0000-0000-0000-000000000001"
	app.SetStatus(descriptionv9.StatusArgs{
		Value: "active",
	})
	app.AddOffer(descriptionv9.ApplicationOfferArgs{
		OfferUUID:              appOfferUUID,
		OfferName:              "test-offer",
		ApplicationName:        "test-app",
		ApplicationDescription: "some description",
		Endpoints:              map[string]string{"foo": "bar"},
		ACL:                    map[string]string{"bob": "write"},
	})
	bytes, err := descriptionv9.Serialize(desc)
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
	c.Assert(m.MigrationMode, qt.Equals, dbmodel.MigrationModeImporting)

	// Check that the application is created in the database.
	appOffer := &dbmodel.ApplicationOffer{
		ModelID: m.ID,
		UUID:    appOfferUUID,
	}
	err = j.Database.GetApplicationOffer(ctx, appOffer)
	c.Assert(err, qt.IsNil)
	c.Assert(appOffer.Name, qt.Equals, "test-offer")

	// Check that the user has access to the model and offer(s).
	ok, err := user.IsModelAdmin(ctx, m.ResourceTag())
	c.Assert(err, qt.IsNil)
	c.Assert(ok, qt.IsTrue)
	ok, err = user.IsApplicationOfferConsumer(ctx, appOffer.ResourceTag())
	c.Assert(err, qt.IsNil)
	c.Assert(ok, qt.IsTrue)
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

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	// Check cloud credential are checked.
	desc := newModelDescription(modelDescriptionArgs{
		Owner:               "bob",
		ModelName:           "test-model",
		CloudName:           "test",
		CloudCredentialName: "test-cred",
		CloudRegionName:     "test-region",
	})
	desc.SetStatus(descriptionv9.StatusArgs{Value: "available"})
	// Intentionally resetting cloud credential to simulate missing cloud credential.
	desc.SetCloudCredential(descriptionv9.CloudCredentialArgs{})
	bytes, err := descriptionv9.Serialize(desc)
	c.Assert(err, qt.IsNil)
	err = j.Import(ctx, user, params.SerializedModel{
		Bytes: bytes,
	})
	c.Assert(err, qt.ErrorMatches, ".*failed to modify model description.*")

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

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	// Check cloud credential are checked.
	desc := newModelDescription(modelDescriptionArgs{
		Owner:               "not-in-mapping",
		ModelName:           "test-model",
		CloudName:           "test",
		CloudCredentialName: "test-cred",
		CloudRegionName:     "test-region",
	})
	desc.SetStatus(descriptionv9.StatusArgs{Value: "available"})
	bytes, err := descriptionv9.Serialize(desc)
	c.Assert(err, qt.IsNil)
	err = j.Import(ctx, user, params.SerializedModel{
		Bytes: bytes,
	})
	c.Assert(err, qt.ErrorMatches, ".*failed to modify model description.*")

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
	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	// Check cloud credential are checked.
	desc := newModelDescription(modelDescriptionArgs{
		Owner:               "bob",
		ModelName:           "test-model",
		CloudName:           "test",
		CloudCredentialName: "test-cred-not-found",
		CloudRegionName:     "test-region",
	})
	desc.SetStatus(descriptionv9.StatusArgs{Value: "available"})
	bytes, err := descriptionv9.Serialize(desc)
	c.Assert(err, qt.IsNil)
	err = j.Import(ctx, user, params.SerializedModel{
		Bytes: bytes,
	})
	c.Assert(err, qt.ErrorMatches, `.*failed to import model from description: cloudcredential \S+ not found$`)

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
			return errors.New("API failure")
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testImportEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, j.OpenFGAClient)

	desc := newModelDescription(modelDescriptionArgs{
		Owner:               "bob",
		ModelName:           "test-model",
		CloudName:           "test",
		CloudCredentialName: "test-cred",
		CloudRegionName:     "test-region",
	})
	desc.SetStatus(descriptionv9.StatusArgs{Value: "available"})
	bytes, err := descriptionv9.Serialize(desc)
	c.Assert(err, qt.IsNil)
	err = j.Import(ctx, user, params.SerializedModel{
		Bytes: bytes,
	})
	c.Assert(err, qt.ErrorMatches, `.*failed to import model: API failure$`)

	// Check the model is created in the database with migration mode set to importing.
	m := &dbmodel.Model{
		UUID: sql.NullString{
			String: migratingModelUUID,
			Valid:  true,
		},
	}
	err = j.Database.GetModel(ctx, m)
	c.Assert(err, qt.IsNil)
	c.Assert(m.MigrationMode, qt.Equals, dbmodel.MigrationModeImporting)
}

// environment for testing cleanup of partial model migrations.
// It contains:
// - one valid incoming migration to be kept
// - one stale incoming migration without user-mapping and a model to be deleted
// - one stale incoming migration with model to be deleted
// - one stale incoming migration
const testCleanupEnv = `clouds:
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
  region: test-region
  agent-version: 3.6.9
  public-address: foo.com
models:
- name: test-model
  uuid: "00000000-0000-0000-0000-000000000002"
  controller: test1
  cloud: test
  region: test-region
  cloud-credential: test-cred
  owner: alice@canonical.com
  migration-mode: importing
- name: test-model-2
  uuid: "00000000-0000-0000-0000-000000000003"
  controller: test1
  cloud: test
  region: test-region
  cloud-credential: test-cred
  owner: alice@canonical.com
  migration-mode: importing
users:
- username: alice@canonical.com
  controller-access: superuser
incoming-migrations:
- model-uuid: "00000000-0000-0000-0000-000000000001"
  target-controller: test1
  user-mapping:
  - local-user: bob
    external-user: alice@canonical.com
- model-uuid: "00000000-0000-0000-0000-000000000002"
  target-controller: test1
  created-at: 2024-01-01T00:00:00Z
  user-mapping:
  - local-user: bob
    external-user: alice@canonical.com
  create-user-mapping: true
- model-uuid: "00000000-0000-0000-0000-000000000003"
  target-controller: test1
  created-at: 2024-01-01T00:00:00Z
  user-mapping:
  - local-user: bob
    external-user: alice@canonical.com
- model-uuid: "00000000-0000-0000-0000-000000000004"
  target-controller: test1
  created-at: 2024-01-01T00:00:00Z
  user-mapping:
  - local-user: bob
    external-user: alice@canonical.com
`

func TestCleanupPartialModelMigrations(t *testing.T) {
	c := qt.New(t)

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testCleanupEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	err := j.CleanupPartialModelMigrations(t.Context())
	c.Assert(err, qt.IsNil)

	// Check the first incoming migration is kept.
	modelMigration := dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: env.IncomingMigrations[0].ModelUUID,
			Valid:  true,
		},
	}
	err = j.Database.GetIncomingModelMigration(t.Context(), &modelMigration)
	c.Assert(err, qt.IsNil)

	// Check the second incoming migration has been deleted, with its mapping, and model.
	modelMigration = dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: env.IncomingMigrations[1].ModelUUID,
			Valid:  true,
		},
	}
	err = j.Database.GetIncomingModelMigration(t.Context(), &modelMigration)
	c.Assert(err, qt.ErrorMatches, `.*model migration not found`)

	userMapping := dbmodel.UserMapping{
		ModelUUID:        modelMigration.ModelUUID,
		LocalUser:        "bob",
		ExternalUserName: "alice@canonical.com",
	}
	err = j.Database.GetUserMapping(t.Context(), &userMapping)
	c.Assert(err, qt.ErrorMatches, `.*user mapping not found`)

	// Check the model has been deleted.
	model := &dbmodel.Model{
		UUID: sql.NullString{
			String: env.IncomingMigrations[1].ModelUUID,
			Valid:  true,
		},
	}
	err = j.Database.GetModel(t.Context(), model)
	c.Assert(err, qt.ErrorMatches, `.*model not found`)

	// Check the third incoming migration has been deleted, with its model.
	modelMigration = dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: env.IncomingMigrations[2].ModelUUID,
			Valid:  true,
		},
	}
	err = j.Database.GetIncomingModelMigration(t.Context(), &modelMigration)
	c.Assert(err, qt.ErrorMatches, `.*model migration not found`)

	model = &dbmodel.Model{
		UUID: sql.NullString{
			String: env.IncomingMigrations[2].ModelUUID,
			Valid:  true,
		},
	}
	err = j.Database.GetModel(t.Context(), model)
	c.Assert(err, qt.ErrorMatches, `.*model not found`)

	// Check the fourth incoming migration has been deleted.
	modelMigration = dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: env.IncomingMigrations[3].ModelUUID,
			Valid:  true,
		},
	}
	err = j.Database.GetIncomingModelMigration(t.Context(), &modelMigration)
	c.Assert(err, qt.ErrorMatches, `.*model migration not found`)
}
