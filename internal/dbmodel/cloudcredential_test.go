// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v4"

	"github.com/canonical/jimm/internal/dbmodel"
)

func TestCloudCredentialTag(t *testing.T) {
	c := qt.New(t)

	cred := dbmodel.CloudCredential{
		Name:          "test-credential",
		CloudName:     "test-cloud",
		OwnerUsername: "test-user",
	}
	tag := cred.Tag()
	c.Check(tag.String(), qt.Equals, "cloudcred-test-cloud_test-user_test-credential")

	var cred2 dbmodel.CloudCredential
	cred2.SetTag(tag.(names.CloudCredentialTag))
	c.Check(cred, qt.DeepEquals, cred2)
}

func TestCloudCredential(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	cred := dbmodel.CloudCredential{
		Name: "test-credential",
		Cloud: dbmodel.Cloud{
			Name: "test-cloud",
		},
		Owner: dbmodel.Identity{
			Name: "bob@external",
		},
		AuthType: "empty",
		Label:    "test label",
		Attributes: dbmodel.StringMap{
			"a": "b",
			"c": "d",
		},
	}
	result := db.Create(&cred)
	c.Assert(result.Error, qt.IsNil)
	c.Check(cred.CloudName, qt.Equals, cred.Cloud.Name)
	c.Check(cred.OwnerUsername, qt.Equals, cred.Owner.Name)
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
		Owner: dbmodel.Identity{
			Name: "bob@external",
		},
	}
	result = db.Create(&cred)
	c.Assert(result.Error, qt.IsNil)
	c.Check(result.RowsAffected, qt.Equals, int64(1))
	c.Check(cred.CloudName, qt.Equals, "test-cloud")
	c.Check(cred.OwnerUsername, qt.Equals, "bob@external")

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
