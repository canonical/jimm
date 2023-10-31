// Copyright 2020 Canonical Ltd.

package db_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
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
	c.Check(u.ControllerAccess, qt.Equals, "login")

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
	c.Check(u.ControllerAccess, qt.Equals, "login")

	u.ControllerAccess = "superuser"
	err = s.Database.UpdateUser(ctx, &u)
	c.Assert(err, qt.IsNil)

	u2 := dbmodel.User{
		Username: u.Username,
	}
	err = s.Database.GetUser(ctx, &u2)
	c.Assert(err, qt.IsNil)
	c.Check(u2, qt.DeepEquals, u)
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
		Name:          "test-cred-1",
		CloudName:     cloud.Name,
		OwnerUsername: u.Username,
		AuthType:      "empty",
	}
	err = s.Database.SetCloudCredential(context.Background(), &cred1)
	c.Assert(err, qt.Equals, nil)

	cred2 := dbmodel.CloudCredential{
		Name:          "test-cred-2",
		CloudName:     cloud.Name,
		OwnerUsername: u.Username,
		AuthType:      "empty",
	}
	err = s.Database.SetCloudCredential(context.Background(), &cred2)
	c.Assert(err, qt.Equals, nil)

	credentials, err := s.Database.GetUserCloudCredentials(ctx, &u, cloud.Name)
	c.Check(err, qt.IsNil)
	c.Assert(credentials, qt.DeepEquals, []dbmodel.CloudCredential{cred1, cred2})
}
