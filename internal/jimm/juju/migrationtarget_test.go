// Copyright 2025 Canonical.

package juju_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/description/v8"
	"github.com/juju/juju/core/migration"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
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
users:
- username: alice@canonical.com
  controller-access: superuser
`

func TestPrechecks_ModifiesModelDescription(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	// Validate that the API request to Juju is made with a modified version
	// of the model description, where the owner is replaced with an external user.
	api := &jimmtest.API{
		Prechecks_: func(mi migration.ModelInfo) error {
			c.Check(mi.UUID, qt.Equals, "00000001-0000-0000-0000-000000000001")
			c.Check(mi.Owner.Id(), qt.Equals, "alice@canonical.com")
			// TODO: Check the description has been modified to use the external user mapping.
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
			return errors.New("controller unreachable")
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

func newIncomingMigration(userMap map[string]string, ctl dbmodel.Controller) dbmodel.IncomingModelMigration {
	return dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: "00000001-0000-0000-0000-000000000001",
			Valid:  true,
		},
		TargetControllerID: ctl.ID,
		UserMapping:        userMap,
	}
}

func newMigrationInfo(owner string) migration.ModelInfo {
	descriptionArgs := description.ModelArgs{
		AgentVersion: "3.2.1",
		Owner:        names.NewUserTag(owner),
		Type:         description.IAAS,
		Cloud:        "test",
	}
	modelDescription := description.NewModel(descriptionArgs)
	userArgs := description.UserArgs{
		Name:        names.NewUserTag("bob"),
		DisplayName: "bob",
		Access:      "admin",
	}
	modelDescription.AddUser(userArgs)
	modelInfo := migration.ModelInfo{
		UUID:                   "00000001-0000-0000-0000-000000000001",
		Owner:                  names.NewUserTag(owner),
		Name:                   "test-model",
		AgentVersion:           version.MustParse("3.2.1"),
		ControllerAgentVersion: version.MustParse("3.2.1"),
		ModelDescription:       modelDescription,
	}
	return modelInfo
}
