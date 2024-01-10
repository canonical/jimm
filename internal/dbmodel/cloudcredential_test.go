// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v4"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/utils"
)

func TestCloudCredentialTag(t *testing.T) {
	c := qt.New(t)

	cred := dbmodel.CloudCredential{
		Name:          "test-credential",
		CloudName:     "test-cloud",
		OwnerUsername: utils.ToStringPtr("test-user"),
	}
	tag := cred.Tag()
	c.Check(tag.String(), qt.Equals, "cloudcred-test-cloud_test-user_test-credential")

	var cred2 dbmodel.CloudCredential
	cred2.SetTag(tag.(names.CloudCredentialTag))
	c.Check(cred, qt.DeepEquals, cred2)
}

func TestCloudCredentialsReferencesCloud(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	cred := dbmodel.CloudCredential{
		Name: "test-credential",
		Cloud: dbmodel.Cloud{
			Name: "test-cloud",
		},
		Owner: &dbmodel.User{
			Username: "bob@external",
		},
		AuthType: "empty",
	}
	result := db.Create(&cred)
	c.Assert(result.Error, qt.IsNil)
	c.Check(cred.CloudName, qt.Equals, cred.Cloud.Name)
}

func TestCloudCredentialsReferencesUserAsOwner(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	cred := dbmodel.CloudCredential{
		Name: "test-credential",
		Cloud: dbmodel.Cloud{
			Name: "test-cloud",
		},
		Owner: &dbmodel.User{
			Username: "bob@external",
		},
		AuthType: "empty",
	}
	result := db.Create(&cred)
	c.Assert(result.Error, qt.IsNil)
	c.Check(cred.CloudName, qt.Equals, cred.Cloud.Name)
	c.Check(cred.OwnerServiceAccount, qt.IsNil)
	c.Check(*cred.OwnerUsername, qt.Equals, cred.Owner.Username)
}

func TestCloudCredentialsReferencesServiceAccountAsOwner(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	cred := dbmodel.CloudCredential{
		Name: "test-credential",
		Cloud: dbmodel.Cloud{
			Name: "test-cloud",
		},
		OwnerServiceAccount: &dbmodel.ServiceAccount{
			ClientID:    "test-client-id",
			DisplayName: "test-display-name",
		},
		AuthType: "empty",
	}
	result := db.Create(&cred)
	c.Assert(result.Error, qt.IsNil)
	c.Check(cred.CloudName, qt.Equals, cred.Cloud.Name)
	c.Check(cred.OwnerUsername, qt.IsNil)
	c.Check(*cred.OwnerClientID, qt.Equals, cred.OwnerServiceAccount.ClientID)
}

func TestMultipleCloudCredentialsWithDifferentOwners(t *testing.T) {
	c := qt.New(t)

	// These entries are to be inserted sequentially in the same database, to
	// eliminate any misleading effect of isolation.
	entries := []struct {
		name          string
		cred          dbmodel.CloudCredential
		expectedError string
	}{{
		name: "owned by user alice",
		cred: dbmodel.CloudCredential{
			Name:     "credential-owned-by-user-alice",
			Cloud:    dbmodel.Cloud{Name: "test-cloud"},
			Owner:    &dbmodel.User{Username: "alice@external"},
			AuthType: "empty",
		},
	}, {
		name: "owned by user alice (duplicate)",
		cred: dbmodel.CloudCredential{
			Name:     "credential-owned-by-user-alice",
			Cloud:    dbmodel.Cloud{Name: "test-cloud"},
			Owner:    &dbmodel.User{Username: "alice@external"},
			AuthType: "empty",
		},
		expectedError: ".*violates unique constraint.*",
	}, {
		name: "owned by user bob",
		cred: dbmodel.CloudCredential{
			Name:     "credential-owned-by-user-bob",
			Cloud:    dbmodel.Cloud{Name: "test-cloud"},
			Owner:    &dbmodel.User{Username: "bob@external"},
			AuthType: "empty",
		},
	}, {
		name: "owned by service account a",
		cred: dbmodel.CloudCredential{
			Name:  "credential-owned-by-service-account-a",
			Cloud: dbmodel.Cloud{Name: "test-cloud"},
			OwnerServiceAccount: &dbmodel.ServiceAccount{
				ClientID:    "service-account-a",
				DisplayName: "service-account-a",
			},
			AuthType: "empty",
		},
	}, {
		name: "owned by service account a (duplicate)",
		cred: dbmodel.CloudCredential{
			Name:  "credential-owned-by-service-account-a",
			Cloud: dbmodel.Cloud{Name: "test-cloud"},
			OwnerServiceAccount: &dbmodel.ServiceAccount{
				ClientID:    "service-account-a",
				DisplayName: "service-account-a",
			},
			AuthType: "empty",
		},
		expectedError: ".*violates unique constraint.*",
	}, {
		name: "owned by service account b",
		cred: dbmodel.CloudCredential{
			Name:  "credential-owned-by-service-account-b",
			Cloud: dbmodel.Cloud{Name: "test-cloud"},
			OwnerServiceAccount: &dbmodel.ServiceAccount{
				ClientID:    "service-account-b",
				DisplayName: "service-account-b",
			},
			AuthType: "empty",
		},
	},
	}

	db := gormDB(c)
	for _, entry := range entries {
		cred := entry.cred
		result := db.Create(&cred)

		if entry.expectedError != "" {
			c.Assert(result.Error, qt.ErrorMatches, entry.expectedError)
			continue
		}

		c.Assert(result.Error, qt.IsNil)
		if cred.Owner != nil {
			c.Assert(cred.OwnerClientID, qt.IsNil)
			c.Assert(cred.OwnerServiceAccount, qt.IsNil)
			c.Assert(cred.Owner, qt.IsNotNil)
			c.Assert(*cred.OwnerUsername, qt.Equals, cred.Owner.Username)
		} else {
			c.Assert(cred.OwnerUsername, qt.IsNil)
			c.Assert(cred.Owner, qt.IsNil)
			c.Assert(cred.OwnerClientID, qt.IsNotNil)
			c.Assert(*cred.OwnerClientID, qt.Equals, cred.OwnerServiceAccount.ClientID)
		}
	}
}

// TestCloudCredentialsCascadeOnDelete As of database version 1.3 (see migrations),
// the foreign key relationship to the clouds, should be a cascade-on-delete relationship.
func TestCloudCredentialsCascadeOnDelete(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
	}
	result := db.Create(&cloud)
	c.Assert(result.Error, qt.IsNil)
	c.Check(result.RowsAffected, qt.Equals, int64(1))

	cred := dbmodel.CloudCredential{
		Name:  "test-credential",
		Cloud: cloud,
		Owner: &dbmodel.User{
			Username: "bob@external",
		},
	}
	result = db.Create(&cred)
	c.Assert(result.Error, qt.IsNil)
	c.Check(result.RowsAffected, qt.Equals, int64(1))
	c.Check(cred.CloudName, qt.Equals, "test-cloud")
	c.Check(*cred.OwnerUsername, qt.Equals, "bob@external")

	result = db.Delete(&cloud)
	c.Assert(result.Error, qt.IsNil)
	c.Check(result.RowsAffected, qt.Equals, int64(1))

	deletedCred := dbmodel.CloudCredential{
		Name: "test-credential",
	}
	result = db.Find(&deletedCred)
	c.Assert(result.Error, qt.IsNil)
	c.Assert(result.RowsAffected, qt.Equals, int64(0))
}
