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
		Owner: dbmodel.User{
			Username: "bob@external",
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
	c.Check(cred.OwnerUsername, qt.Equals, cred.Owner.Username)
}
