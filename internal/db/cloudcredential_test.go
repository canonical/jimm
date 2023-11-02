// Copyright 2020 Canonical Ltd.

package db_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v4"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimmtest"
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
		Name:          "test-cred",
		OwnerUsername: u.Username,
		AuthType:      "empty",
	}
	err = s.Database.SetCloudCredential(context.Background(), &cred)
	c.Check(err, qt.ErrorMatches, fmt.Sprintf("invalid cloudcredential tag %q", cred.CloudName+"/"+cred.OwnerUsername+"/"+cred.Name))
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
		Name:          "test-cred",
		CloudName:     cloud.Name,
		OwnerUsername: u.Username,
		AuthType:      "empty",
	}
	c1 := cred
	err = s.Database.SetCloudCredential(context.Background(), &cred)
	c.Assert(err, qt.Equals, nil)

	var dbCred dbmodel.CloudCredential
	result := s.Database.DB.Where("cloud_name = ? AND owner_username = ? AND name = ?", cloud.Name, u.Username, cred.Name).First(&dbCred)
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
		Name:          "test-cred",
		CloudName:     cloud.Name,
		OwnerUsername: u.Username,
		AuthType:      "empty",
	}
	err = s.Database.SetCloudCredential(context.Background(), &cred)
	c.Assert(err, qt.Equals, nil)

	cred.Cloud = cloud
	cred.Cloud.Regions = nil

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
		CloudName:     cloud.Name,
		OwnerUsername: u.Username,
		Name:          cred.Name,
	}
	err = s.Database.GetCloudCredential(context.Background(), &dbCred)
	c.Assert(err, qt.Equals, nil)
	c.Assert(dbCred, jimmtest.DBObjectEquals, cred)
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
		Name:          "test-cred",
		CloudName:     cloud.Name,
		OwnerUsername: u.Username,
		AuthType:      "empty",
	}
	cred.Cloud.Regions = nil
	err = s.Database.SetCloudCredential(context.Background(), &cred)
	c.Assert(err, qt.Equals, nil)

	cred.Cloud = cloud
	cred.Cloud.Regions = nil

	dbCred := dbmodel.CloudCredential{
		CloudName:     cloud.Name,
		OwnerUsername: u.Username,
		Name:          cred.Name,
	}
	err = s.Database.GetCloudCredential(context.Background(), &dbCred)
	c.Assert(err, qt.Equals, nil)
	c.Assert(dbCred, jimmtest.DBObjectEquals, cred)
}

func TestForEachCloudCredentialUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.ForEachCloudCredential(context.Background(), "", "", nil)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

const forEachCloudCredentialEnv = `clouds:
- name: cloud-1
  regions:
  - name: default
- name: cloud-2
  regions:
  - name: default
cloud-credentials:
- name: cred-1
  cloud: cloud-1
  owner: alice@external
  attributes:
    k1: v1
    k2: v2
- name: cred-2
  cloud: cloud-1
  owner: bob@external
  attributes:
    k1: v1
    k2: v2
- name: cred-3
  cloud: cloud-2
  owner: alice@external
- name: cred-4
  cloud: cloud-2
  owner: bob@external
- name: cred-5
  cloud: cloud-1
  owner: alice@external
`

var forEachCloudCredentialTests = []struct {
	name              string
	username          string
	cloud             string
	f                 func(cred *dbmodel.CloudCredential) error
	expectCredentials []string
	expectError       string
	expectErrorCode   errors.Code
}{{
	name:     "UserCredentialsWithCloud",
	username: "alice@external",
	cloud:    "cloud-1",
	expectCredentials: []string{
		names.NewCloudCredentialTag("cloud-1/alice@external/cred-1").String(),
		names.NewCloudCredentialTag("cloud-1/alice@external/cred-5").String(),
	},
}, {
	name:     "UserCredentialsWithoutCloud",
	username: "bob@external",
	expectCredentials: []string{
		names.NewCloudCredentialTag("cloud-1/bob@external/cred-2").String(),
		names.NewCloudCredentialTag("cloud-2/bob@external/cred-4").String(),
	},
}, {
	name:     "IterationError",
	username: "alice@external",
	f: func(*dbmodel.CloudCredential) error {
		return errors.E("test error", errors.Code("test code"))
	},
	expectError:     "test error",
	expectErrorCode: "test code",
}}

func (s *dbSuite) TestForEachCloudCredential(c *qt.C) {
	ctx := context.Background()

	env := jimmtest.ParseEnvironment(c, forEachCloudCredentialEnv)
	err := s.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)
	env.PopulateDB(c, *s.Database)

	for _, test := range forEachCloudCredentialTests {
		c.Run(test.name, func(c *qt.C) {
			var credentials []string
			if test.f == nil {
				test.f = func(cred *dbmodel.CloudCredential) error {
					credentials = append(credentials, cred.Tag().String())
					return nil
				}
			}
			err = s.Database.ForEachCloudCredential(ctx, test.username, test.cloud, test.f)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			c.Check(credentials, qt.DeepEquals, test.expectCredentials)
		})
	}
}
