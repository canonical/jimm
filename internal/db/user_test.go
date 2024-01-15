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

func TestGetIdentityUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.GetIdentity(context.Background(), &dbmodel.Identity{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetIdentity(c *qt.C) {
	ctx := context.Background()
	err := s.Database.GetIdentity(ctx, &dbmodel.Identity{})
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	err = s.Database.GetIdentity(ctx, &dbmodel.Identity{})
	c.Check(err, qt.ErrorMatches, `invalid identity name ""`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	u := dbmodel.Identity{
		Name: "bob@external",
	}
	err = s.Database.GetIdentity(ctx, &u)
	c.Assert(err, qt.IsNil)

	u2 := dbmodel.Identity{
		Name: u.Name,
	}
	err = s.Database.GetIdentity(ctx, &u2)
	c.Assert(err, qt.IsNil)
	c.Check(u2, qt.DeepEquals, u)
}

func TestUpdateIdentityUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.UpdateIdentity(context.Background(), &dbmodel.Identity{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestUpdateIdentity(c *qt.C) {
	ctx := context.Background()
	err := s.Database.UpdateIdentity(ctx, &dbmodel.Identity{})
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	err = s.Database.UpdateIdentity(ctx, &dbmodel.Identity{})
	c.Check(err, qt.ErrorMatches, `invalid identity name ""`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	u := dbmodel.Identity{
		Name: "bob@external",
	}
	err = s.Database.GetIdentity(ctx, &u)
	c.Assert(err, qt.IsNil)

	err = s.Database.UpdateIdentity(ctx, &u)
	c.Assert(err, qt.IsNil)

	u2 := dbmodel.Identity{
		Name: u.Name,
	}
	err = s.Database.GetIdentity(ctx, &u2)
	c.Assert(err, qt.IsNil)
	c.Check(u2, qt.DeepEquals, u)
}

func TestGetIdentityCloudCredentialsUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	_, err := d.GetIdentityCloudCredentials(context.Background(), &dbmodel.Identity{}, "")
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetIdentityCloudCredentials(c *qt.C) {
	ctx := context.Background()

	err := s.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	_, err = s.Database.GetIdentityCloudCredentials(ctx, &dbmodel.Identity{}, "")
	c.Check(err, qt.ErrorMatches, `cloudcredential not found`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	_, err = s.Database.GetIdentityCloudCredentials(ctx, &dbmodel.Identity{
		Name: "test",
	}, "ec2")
	c.Check(err, qt.IsNil)

	u := dbmodel.Identity{
		Name: "bob@external",
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
		Name:              "test-cred-1",
		CloudName:         cloud.Name,
		OwnerIdentityName: u.Name,
		AuthType:          "empty",
	}
	err = s.Database.SetCloudCredential(context.Background(), &cred1)
	c.Assert(err, qt.Equals, nil)

	cred2 := dbmodel.CloudCredential{
		Name:              "test-cred-2",
		CloudName:         cloud.Name,
		OwnerIdentityName: u.Name,
		AuthType:          "empty",
	}
	err = s.Database.SetCloudCredential(context.Background(), &cred2)
	c.Assert(err, qt.Equals, nil)

	credentials, err := s.Database.GetIdentityCloudCredentials(ctx, &u, cloud.Name)
	c.Check(err, qt.IsNil)
	c.Assert(credentials, qt.DeepEquals, []dbmodel.CloudCredential{cred1, cred2})
}
