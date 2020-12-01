// Copyright 2020 Canonical Ltd.

package db_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

func TestSetCloudCredentialUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.SetCloudCredential(context.Background(), &dbmodel.CloudCredential{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestSetCloudCredentialInvalidTag(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	u := dbmodel.User{
		Username: "bob@external",
	}
	c.Assert(s.Database.DB.Create(&u).Error, qt.IsNil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region",
		}},
	}
	c.Assert(s.Database.DB.Create(&cloud).Error, qt.IsNil)

	cred := dbmodel.CloudCredential{
		Name:     "test-cred",
		OwnerID:  u.Username,
		AuthType: "empty",
	}
	err = s.Database.SetCloudCredential(context.Background(), &cred)
	c.Check(err, qt.ErrorMatches, fmt.Sprintf("invalid cloudcredential tag %q", cred.CloudName+"/"+cred.OwnerID+"/"+cred.Name))
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeBadRequest)
}

func (s *dbSuite) TestSetCloudCredential(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	u := dbmodel.User{
		Username: "bob@external",
	}
	c.Assert(s.Database.DB.Create(&u).Error, qt.IsNil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region",
		}},
	}
	c.Assert(s.Database.DB.Create(&cloud).Error, qt.IsNil)

	cred := dbmodel.CloudCredential{
		Name:      "test-cred",
		CloudName: cloud.Name,
		OwnerID:   u.Username,
		AuthType:  "empty",
	}
	c1 := cred
	err = s.Database.SetCloudCredential(context.Background(), &cred)
	c.Assert(err, qt.Equals, nil)

	var dbCred dbmodel.CloudCredential
	result := s.Database.DB.Where("cloud_name = ? AND owner_id = ? AND name = ?", cloud.Name, u.Username, cred.Name).First(&dbCred)
	c.Assert(result.Error, qt.Equals, nil)
	c.Assert(dbCred, qt.DeepEquals, cred)

	err = s.Database.SetCloudCredential(context.Background(), &c1)
	c.Assert(err, qt.IsNil)
}

func (s *dbSuite) TestSetCloudCredentialUpdate(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	u := dbmodel.User{
		Username: "bob@external",
	}
	c.Assert(s.Database.DB.Create(&u).Error, qt.IsNil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region",
		}},
	}
	c.Assert(s.Database.DB.Create(&cloud).Error, qt.IsNil)

	cred := dbmodel.CloudCredential{
		Name:      "test-cred",
		CloudName: cloud.Name,
		OwnerID:   u.Username,
		AuthType:  "empty",
	}
	err = s.Database.SetCloudCredential(context.Background(), &cred)
	c.Assert(err, qt.Equals, nil)

	cred.Label = "test label"
	cred.Attributes = dbmodel.StringMap{
		"key1": "value1",
		"key2": "value2",
	}
	cred.AttributesInVault = true
	cred.Valid = sql.NullBool{
		Bool:  true,
		Valid: true,
	}
	err = s.Database.SetCloudCredential(context.Background(), &cred)
	c.Assert(err, qt.Equals, nil)

	dbCred := dbmodel.CloudCredential{
		CloudName: cloud.Name,
		OwnerID:   u.Username,
		Name:      cred.Name,
	}
	err = s.Database.GetCloudCredential(context.Background(), &dbCred)
	c.Assert(err, qt.Equals, nil)
	c.Assert(dbCred, qt.DeepEquals, cred)
	c.Assert(dbCred.Attributes, qt.DeepEquals, dbmodel.StringMap{
		"key1": "value1",
		"key2": "value2",
	})
	c.Assert(dbCred.Label, qt.Equals, "test label")
	c.Assert(dbCred.AttributesInVault, qt.IsTrue)
	c.Assert(dbCred.Valid.Valid, qt.IsTrue)
	c.Assert(dbCred.Valid.Bool, qt.IsTrue)
}

func TestGetCloudCredentialUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.GetCloudCredential(context.Background(), &dbmodel.CloudCredential{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetCloudCredential(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	u := dbmodel.User{
		Username: "bob@external",
	}
	c.Assert(s.Database.DB.Create(&u).Error, qt.IsNil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region",
		}},
	}
	c.Assert(s.Database.DB.Create(&cloud).Error, qt.IsNil)

	cred := dbmodel.CloudCredential{
		Name:      "test-cred",
		CloudName: cloud.Name,
		OwnerID:   u.Username,
		AuthType:  "empty",
	}
	err = s.Database.SetCloudCredential(context.Background(), &cred)
	c.Assert(err, qt.Equals, nil)

	dbCred := dbmodel.CloudCredential{
		CloudName: cloud.Name,
		OwnerID:   u.Username,
		Name:      cred.Name,
	}
	err = s.Database.GetCloudCredential(context.Background(), &dbCred)
	c.Assert(err, qt.Equals, nil)
	c.Assert(dbCred, qt.DeepEquals, cred)
}
