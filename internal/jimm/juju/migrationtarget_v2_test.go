// Copyright 2026 Canonical.

package juju_test

import (
	"context"
	"database/sql"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

// testPayload is a minimal model database payload, holding a single offer
// alongside a table JIMM does not read.
const testPayload = `model:
- uuid: ` + migratingModelUUID + `
  name: test-model
offer:
- uuid: ` + offerUUID + `
  name: test-offer
  description: null
`

// newTestEnvelope returns a SerializedModelV2 envelope for a model owned by
// the local user "bob", as sent by a Juju 4.1 source controller.
func newTestEnvelope() params.SerializedModelV2 {
	return params.SerializedModelV2{
		Payload: []byte(testPayload),
		ModelInfo: params.SerializedModelInfo{
			UUID:                migratingModelUUID,
			Name:                "test-model",
			Qualifier:           "bob",
			Type:                "iaas",
			Cloud:               "test",
			CloudRegion:         "test-region",
			CredentialName:      "test-cred",
			CredentialOwner:     "bob",
			Life:                "alive",
			SourceMigrationUUID: "00000000-0000-0000-0000-000000000002",
		},
		Users: []params.ModelUser{{
			Name:        "bob",
			DisplayName: "Bob",
		}},
		ModelCredential: &params.ModelCloudCredential{
			Cloud:    "test",
			Owner:    "bob",
			Name:     "test-cred",
			AuthType: "empty",
		},
		Permissions: []params.ModelPermission{{
			ObjectType:  "model",
			GrantOn:     migratingModelUUID,
			SubjectName: "bob",
			Access:      "admin",
		}, {
			ObjectType:  "offer",
			GrantOn:     offerUUID,
			SubjectName: "bob",
			Access:      "admin",
		}},
		AuthorizedKeys: []params.ModelAuthorizedKey{{
			Username:  "bob",
			PublicKey: "ssh-ed25519 AAAA bob@host",
		}},
	}
}

func TestPrechecksV2_ModifiesEnvelope(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	// Validate that the API request to Juju is made with a modified version
	// of the envelope, where local users are replaced with their external mapping.
	prechecksCalled := false
	api := &jimmtest.API{
		PrechecksV2_: func(_ context.Context, envelope params.SerializedModelV2) error {
			prechecksCalled = true
			c.Check(envelope.ModelInfo.UUID, qt.Equals, migratingModelUUID)
			c.Check(envelope.ModelInfo.Qualifier, qt.Equals, "alice@canonical.com")
			c.Check(envelope.ModelInfo.CredentialOwner, qt.Equals, "alice@canonical.com")
			c.Check(envelope.ModelCredential.Owner, qt.Equals, "alice@canonical.com")

			// All permissions are dropped.
			c.Check(envelope.Permissions, qt.HasLen, 0)

			// SSH keys are dropped.
			c.Check(envelope.AuthorizedKeys, qt.HasLen, 0)

			// The user list contains only the mapped users, as external users.
			c.Check(envelope.Users, qt.HasLen, 1)
			if len(envelope.Users) == 1 {
				c.Check(envelope.Users[0].Name, qt.Equals, "alice@canonical.com")
				c.Check(envelope.Users[0].External, qt.IsTrue)
			}

			// The payload is passed through untouched.
			c.Check(string(envelope.Payload), qt.Equals, testPayload)
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

	envelope := newTestEnvelope()
	err := j.PrechecksV2(ctx, user, envelope)
	c.Assert(err, qt.IsNil)
	c.Assert(prechecksCalled, qt.IsTrue)

	// Validate that the caller's envelope is not modified.
	c.Assert(envelope, qt.DeepEquals, newTestEnvelope())
}

func TestPrechecksV2_ValidatesUserMapping(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	// Add a model user, an offer user and a user with access to another
	// object type that are not mapped.
	envelope := newTestEnvelope()
	envelope.Permissions = append(envelope.Permissions, params.ModelPermission{
		ObjectType:  "model",
		GrantOn:     migratingModelUUID,
		SubjectName: "jane",
		Access:      "admin",
	}, params.ModelPermission{
		ObjectType:  "offer",
		GrantOn:     offerUUID,
		SubjectName: "jack",
		Access:      "consume",
	}, params.ModelPermission{
		ObjectType:  "future-object",
		GrantOn:     "some-object",
		SubjectName: "jill",
		Access:      "read",
	})

	err := j.PrechecksV2(ctx, user, envelope)
	c.Assert(err, qt.ErrorMatches, `(?ms).*^expected user \"jane\" who has admin access to the model$.*`)
	c.Assert(err, qt.ErrorMatches, `(?ms).*^expected user \"jack\" who has consume access to offer "test-offer"$.*`)
	c.Assert(err, qt.ErrorMatches, `(?ms).*^expected user \"jill\" who has read access to future-object "some-object"$.*`)
}

func TestPrechecksV2_ExternalUsersNotInMapping(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	// Validate that external users with access to the model or its offers,
	// including the everyone user, don't need to be in the user mapping.
	prechecksCalled := false
	api := &jimmtest.API{
		PrechecksV2_: func(_ context.Context, envelope params.SerializedModelV2) error {
			prechecksCalled = true
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

	envelope := newTestEnvelope()
	envelope.Permissions = append(envelope.Permissions, params.ModelPermission{
		ObjectType:  "model",
		GrantOn:     migratingModelUUID,
		SubjectName: "dave@ubuntu.com",
		Access:      "read",
	}, params.ModelPermission{
		ObjectType:  "offer",
		GrantOn:     offerUUID,
		SubjectName: "dave@ubuntu.com",
		Access:      "consume",
	}, params.ModelPermission{
		ObjectType:  "offer",
		GrantOn:     offerUUID,
		SubjectName: "everyone@external",
		Access:      "read",
	})

	err := j.PrechecksV2(ctx, user, envelope)
	c.Assert(err, qt.IsNil)
	c.Assert(prechecksCalled, qt.IsTrue)
}

// testEnvV2 holds the resources the tests below need, without an incoming
// migration. Tests that need a specific user mapping add their own
// incoming migration to it.
const testEnvV2 = `clouds:
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
`

// testEnvWithSkippedUser has an incoming migration where jack is intentionally left unmapped.
const testEnvWithSkippedUser = testEnvV2 + `incoming-migrations:
- model-uuid: ` + migratingModelUUID + `
  target-controller: test1
  user-mapping:
  - local-user: bob
    external-user: alice@canonical.com
  - local-user: jack
    external-user: ""
`

func TestPrechecksV2_SkippedUser(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	// Validate that a local user intentionally left unmapped passes the user
	// mapping validation, and isn't added to the user list.
	api := &jimmtest.API{
		PrechecksV2_: func(_ context.Context, envelope params.SerializedModelV2) error {
			c.Check(envelope.Permissions, qt.HasLen, 0)
			for _, u := range envelope.Users {
				c.Check(u.Name, qt.Not(qt.Equals), "jack")
			}
			return nil
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testEnvWithSkippedUser)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	envelope := newTestEnvelope()
	envelope.Permissions = append(envelope.Permissions, params.ModelPermission{
		ObjectType:  "offer",
		GrantOn:     offerUUID,
		SubjectName: "jack",
		Access:      "consume",
	})

	err := j.PrechecksV2(ctx, user, envelope)
	c.Assert(err, qt.IsNil)
}

// testEnvWithDuplicateMapping has an incoming migration where bob and carol are both mapped to alice@canonical.com.
const testEnvWithDuplicateMapping = testEnvV2 + `incoming-migrations:
- model-uuid: ` + migratingModelUUID + `
  target-controller: test1
  user-mapping:
  - local-user: bob
    external-user: alice@canonical.com
  - local-user: carol
    external-user: alice@canonical.com
`

func TestPrechecksV2_UserListDeduplicated(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	// Validate that when several local users are mapped to the same external
	// user, that user appears only once in the envelope's user list. Below, the
	// model owner (bob) and credential owner (carol) both map to it.
	api := &jimmtest.API{
		PrechecksV2_: func(_ context.Context, envelope params.SerializedModelV2) error {
			c.Check(envelope.Users, qt.HasLen, 1)
			if len(envelope.Users) == 1 {
				c.Check(envelope.Users[0].Name, qt.Equals, "alice@canonical.com")
			}
			return nil
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testEnvWithDuplicateMapping)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	envelope := newTestEnvelope()
	envelope.Users = append(envelope.Users, params.ModelUser{Name: "carol"})
	envelope.ModelInfo.CredentialOwner = "carol"
	envelope.ModelCredential.Owner = "carol"

	err := j.PrechecksV2(ctx, user, envelope)
	c.Assert(err, qt.IsNil)
}

func TestPrechecksV2_MissingCloudRegion(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{},
		},
	})

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	envelope := newTestEnvelope()
	envelope.ModelInfo.CloudRegion = "test-region-not-found"

	err := j.PrechecksV2(ctx, user, envelope)
	c.Assert(err, qt.ErrorMatches, `^failed to find region for cloud "test".*`)
}

func TestPrechecksV2_MissingCloudCredential(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{},
		},
	})

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	// #nosec G101 No fields are secret
	envelope := newTestEnvelope()
	envelope.ModelInfo.CredentialName = "test-cred-not-found"

	err := j.PrechecksV2(ctx, user, envelope)
	c.Assert(err, qt.ErrorMatches, `^cloudcredential "test/alice@canonical.com/test-cred-not-found" not found$`)
}

func TestPrechecksV2_NoCloudCredential(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	envelope := newTestEnvelope()
	envelope.ModelCredential = nil

	err := j.PrechecksV2(ctx, user, envelope)
	c.Assert(err, qt.ErrorMatches, `failed to modify migration info: model must have a cloud credential`)
}

// testEnvWithUnmappedOwner has an incoming migration where the model owner, bob, is left unmapped.
const testEnvWithUnmappedOwner = testEnvV2 + `incoming-migrations:
- model-uuid: ` + migratingModelUUID + `
  target-controller: test1
  user-mapping:
  - local-user: bob
    external-user: ""
`

func TestPrechecksV2_InvalidOwner(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testEnvWithUnmappedOwner)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	err := j.PrechecksV2(ctx, user, newTestEnvelope())
	c.Assert(err, qt.ErrorMatches, `failed to modify migration info: cannot map model owner "bob" to an external user: not a valid user or no external user mapping found`)
}

func TestPrechecksV2_InvalidPayload(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testEnvWithIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	envelope := newTestEnvelope()
	envelope.Payload = []byte("offer: [not valid")

	err := j.PrechecksV2(ctx, user, envelope)
	c.Assert(err, qt.ErrorMatches, `failed to read offers from model payload: .*`)
}

func TestPrechecksV2_ControllerUnreachable(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	api := &jimmtest.API{
		PrechecksV2_: func(_ context.Context, envelope params.SerializedModelV2) error {
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

	err := j.PrechecksV2(ctx, user, newTestEnvelope())
	c.Assert(err, qt.ErrorMatches, `failed to run pre-checks for migration: controller unreachable`)
}

func TestPrechecksV2_NoIncomingModelMigration(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testEnvNoIncomingMigration)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, nil)

	err := j.PrechecksV2(ctx, user, newTestEnvelope())
	c.Assert(err, qt.ErrorMatches, `.*model migration not found`)
}

func TestImportV2_Success(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	// Validate that the API request to Juju is made with a modified version
	// of the envelope, where the owner is replaced with an external user.
	importCalled := false
	api := &jimmtest.API{
		ImportV2_: func(_ context.Context, envelope params.SerializedModelV2) error {
			importCalled = true
			c.Check(envelope.ModelInfo.UUID, qt.Equals, migratingModelUUID)
			c.Check(envelope.ModelInfo.Qualifier, qt.Equals, "alice@canonical.com")
			c.Check(envelope.ModelCredential.Owner, qt.Equals, "alice@canonical.com")
			c.Check(string(envelope.Payload), qt.Equals, testPayload)
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

	err := j.ImportV2(ctx, user, newTestEnvelope())
	c.Assert(err, qt.IsNil)
	c.Assert(importCalled, qt.IsTrue)

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
	c.Assert(m.Name, qt.Equals, "test-model")
	c.Assert(m.OwnerIdentityName, qt.Equals, "alice@canonical.com")

	// Check that the offer from the payload is created in the database.
	appOffer := &dbmodel.ApplicationOffer{
		ModelID: m.ID,
		UUID:    offerUUID,
	}
	err = j.Database.GetApplicationOffer(ctx, appOffer)
	c.Assert(err, qt.IsNil)
	c.Assert(appOffer.Name, qt.Equals, "test-offer")
	c.Assert(appOffer.URL, qt.Equals, crossmodel.MakeURL("alice@canonical.com", "test-model", "test-offer", ""))

	// Check that the user has access to the model and offer(s).
	ok, err := user.IsModelAdmin(ctx, m.ResourceTag())
	c.Assert(err, qt.IsNil)
	c.Assert(ok, qt.IsTrue)
	ok, err = user.IsApplicationOfferConsumer(ctx, appOffer.ResourceTag())
	c.Assert(err, qt.IsNil)
	c.Assert(ok, qt.IsTrue)
}

func TestImportV2_UserNotFoundInUserMapping(t *testing.T) {
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

	envelope := newTestEnvelope()
	envelope.ModelInfo.Qualifier = "not-in-mapping"

	err := j.ImportV2(ctx, user, envelope)
	c.Assert(err, qt.ErrorMatches, `failed to modify migration info: cannot map model owner "not-in-mapping" to an external user: not a valid user or no external user mapping found`)

	// Check the model is not created in the database.
	m := &dbmodel.Model{
		UUID: sql.NullString{
			String: migratingModelUUID,
			Valid:  true,
		},
	}
	err = j.Database.GetModel(ctx, m)
	c.Assert(err, qt.ErrorMatches, ".*not found.*")
}

//nolint:gosec // Test fixtures intentionally include cloud credential names.
func TestImportV2_MissingCloudCredentialFromJIMMState(t *testing.T) {
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

	envelope := newTestEnvelope()
	envelope.ModelInfo.CredentialName = "test-cred-not-found"

	err := j.ImportV2(ctx, user, envelope)
	c.Assert(err, qt.ErrorMatches, `failed to import model from envelope: cloudcredential \S+ not found$`)

	// Check the model is not created in the database.
	m := &dbmodel.Model{
		UUID: sql.NullString{
			String: migratingModelUUID,
			Valid:  true,
		},
	}
	err = j.Database.GetModel(ctx, m)
	c.Assert(err, qt.ErrorMatches, ".*not found.*")
}

func TestImportV2_APIFailure(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	api := &jimmtest.API{
		ImportV2_: func(_ context.Context, envelope params.SerializedModelV2) error {
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

	err := j.ImportV2(ctx, user, newTestEnvelope())
	c.Assert(err, qt.ErrorMatches, `failed to import model: API failure$`)

	// Check the model is left in the database with migration mode set to importing,
	// matching the behaviour of Import.
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
