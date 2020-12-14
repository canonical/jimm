// Copyright 2020 Canonical Ltd.

package db_test

import (
	"context"
	"database/sql"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

func TestGetUserUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.GetUser(context.Background(), &dbmodel.User{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetUser(c *qt.C) {
	ctx := context.Background()
	err := s.Database.GetUser(ctx, &dbmodel.User{})
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	err = s.Database.GetUser(ctx, &dbmodel.User{})
	c.Check(err, qt.ErrorMatches, `invalid username ""`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	u := dbmodel.User{
		Username: "bob@external",
	}
	err = s.Database.GetUser(ctx, &u)
	c.Assert(err, qt.IsNil)
	c.Check(u.ControllerAccess, qt.Equals, "add-model")

	u2 := dbmodel.User{
		Username: u.Username,
	}
	err = s.Database.GetUser(ctx, &u2)
	c.Assert(err, qt.IsNil)
	c.Check(u2, qt.DeepEquals, u)
}

func TestUpdateUserUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.UpdateUser(context.Background(), &dbmodel.User{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestUpdateUser(c *qt.C) {
	ctx := context.Background()
	err := s.Database.UpdateUser(ctx, &dbmodel.User{})
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	err = s.Database.UpdateUser(ctx, &dbmodel.User{})
	c.Check(err, qt.ErrorMatches, `invalid username ""`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	u := dbmodel.User{
		Username: "bob@external",
	}
	err = s.Database.GetUser(ctx, &u)
	c.Assert(err, qt.IsNil)
	c.Check(u.ControllerAccess, qt.Equals, "add-model")

	u.ControllerAccess = "superuser"
	u.Models = []dbmodel.UserModelAccess{{
		Model_: dbmodel.Model{
			Name:  "model-1",
			Owner: u,
			UUID: sql.NullString{
				String: "00000001-0000-0000-0000-0000-00000000001",
				Valid:  true,
			},
		},
		Access: "admin",
	}}
	err = s.Database.UpdateUser(ctx, &u)
	c.Assert(err, qt.IsNil)

	u.Models = nil

	u2 := dbmodel.User{
		Username: u.Username,
	}
	err = s.Database.GetUser(ctx, &u2)
	c.Assert(err, qt.IsNil)
	c.Check(u2, qt.DeepEquals, u)
}

func TestGetUserCloudsUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	_, err := d.GetUserClouds(context.Background(), &dbmodel.User{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetUserClouds(c *qt.C) {
	ctx := context.Background()

	u := dbmodel.User{
		Username:    "everyone@external",
		DisplayName: "everyone",
	}

	_, err := s.Database.GetUserClouds(ctx, &u)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	clouds, err := s.Database.GetUserClouds(ctx, &u)
	c.Assert(err, qt.IsNil)
	c.Check(clouds, qt.HasLen, 0)

	cl := dbmodel.Cloud{
		Name:             "test-cloud",
		Type:             "dummy",
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://identity.example.com",
		StorageEndpoint:  "https://storage.example.com",
		Regions: []dbmodel.CloudRegion{{
			Name: "dummy-region",
		}},
		CACertificates: dbmodel.Strings{"CACERT 1", "CACERT 2"},
		Users: []dbmodel.UserCloudAccess{{
			User:   u,
			Access: "add-model",
		}},
	}

	err = s.Database.AddCloud(ctx, &cl)
	c.Assert(err, qt.IsNil)

	clouds, err = s.Database.GetUserClouds(ctx, &u)
	c.Assert(err, qt.IsNil)
	c.Check(clouds, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(gorm.Model{})), []dbmodel.UserCloudAccess{{
		Username:  "everyone@external",
		CloudName: "test-cloud",
		Cloud:     cl,
		Access:    "add-model",
	}})
}

func TestGetUserCloudCredentialsUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	_, err := d.GetUserCloudCredentials(context.Background(), &dbmodel.User{}, "")
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetUserCloudCredentials(c *qt.C) {
	ctx := context.Background()

	err := s.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	_, err = s.Database.GetUserCloudCredentials(ctx, &dbmodel.User{}, "")
	c.Check(err, qt.ErrorMatches, `cloudcredential not found`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	_, err = s.Database.GetUserCloudCredentials(ctx, &dbmodel.User{
		Username: "test",
	}, "ec2")
	c.Check(err, qt.IsNil)

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

	cred1 := dbmodel.CloudCredential{
		Name:      "test-cred-1",
		CloudName: cloud.Name,
		OwnerID:   u.Username,
		AuthType:  "empty",
	}
	err = s.Database.SetCloudCredential(context.Background(), &cred1)
	c.Assert(err, qt.Equals, nil)

	cred2 := dbmodel.CloudCredential{
		Name:      "test-cred-2",
		CloudName: cloud.Name,
		OwnerID:   u.Username,
		AuthType:  "empty",
	}
	err = s.Database.SetCloudCredential(context.Background(), &cred2)
	c.Assert(err, qt.Equals, nil)

	credentials, err := s.Database.GetUserCloudCredentials(ctx, &u, cloud.Name)
	c.Check(err, qt.IsNil)
	c.Assert(credentials, qt.DeepEquals, []dbmodel.CloudCredential{cred1, cred2})
}
