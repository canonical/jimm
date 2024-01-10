// Copyright 2024 Canonical Ltd.

package db_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
)

func TestGetServiceAccountUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.GetServiceAccount(context.Background(), &dbmodel.ServiceAccount{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetServiceAccount(c *qt.C) {
	ctx := context.Background()
	err := s.Database.GetServiceAccount(ctx, &dbmodel.ServiceAccount{})
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	err = s.Database.GetServiceAccount(ctx, &dbmodel.ServiceAccount{})
	c.Check(err, qt.ErrorMatches, `invalid client id ""`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	sa := dbmodel.ServiceAccount{
		ClientID: "some-client-id",
	}
	err = s.Database.GetServiceAccount(ctx, &sa)
	c.Assert(err, qt.IsNil)

	sa2 := dbmodel.ServiceAccount{
		ClientID: sa.ClientID,
	}
	err = s.Database.GetServiceAccount(ctx, &sa2)
	c.Assert(err, qt.IsNil)
	c.Check(sa2, qt.DeepEquals, sa)
}

func TestUpdateServiceAccountUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.UpdateServiceAccount(context.Background(), &dbmodel.ServiceAccount{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestUpdateServiceAccount(c *qt.C) {
	ctx := context.Background()
	err := s.Database.UpdateServiceAccount(ctx, &dbmodel.ServiceAccount{})
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	err = s.Database.UpdateServiceAccount(ctx, &dbmodel.ServiceAccount{})
	c.Check(err, qt.ErrorMatches, `invalid client id ""`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	u := dbmodel.ServiceAccount{
		ClientID: "some-client-id",
	}
	err = s.Database.GetServiceAccount(ctx, &u)
	c.Assert(err, qt.IsNil)

	// Apply some change on the entity.
	u.DisplayName = "new-display-name"

	err = s.Database.UpdateServiceAccount(ctx, &u)
	c.Assert(err, qt.IsNil)

	u2 := dbmodel.ServiceAccount{
		ClientID: u.ClientID,
	}
	err = s.Database.GetServiceAccount(ctx, &u2)
	c.Assert(err, qt.IsNil)
	c.Check(u2, qt.DeepEquals, u)
}

func TestGetServiceAccountCloudCredentialsUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	_, err := d.GetServiceAccountCloudCredentials(context.Background(), &dbmodel.ServiceAccount{}, "")
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetServiceAccountCloudCredentials(c *qt.C) {
	ctx := context.Background()

	err := s.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	_, err = s.Database.GetServiceAccountCloudCredentials(ctx, &dbmodel.ServiceAccount{}, "")
	c.Check(err, qt.ErrorMatches, `cloudcredential not found`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	_, err = s.Database.GetServiceAccountCloudCredentials(ctx, &dbmodel.ServiceAccount{
		ClientID: "some-client-id",
	}, "ec2")
	c.Check(err, qt.IsNil)

	sa := dbmodel.ServiceAccount{
		ClientID: "some-client-id",
	}
	c.Assert(s.Database.DB.Create(&sa).Error, qt.IsNil)

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
		OwnerClientID: &sa.ClientID,
		AuthType:      "empty",
	}
	err = s.Database.SetCloudCredential(context.Background(), &cred1)
	c.Assert(err, qt.Equals, nil)

	cred2 := dbmodel.CloudCredential{
		Name:          "test-cred-2",
		CloudName:     cloud.Name,
		OwnerClientID: &sa.ClientID,
		AuthType:      "empty",
	}
	err = s.Database.SetCloudCredential(context.Background(), &cred2)
	c.Assert(err, qt.Equals, nil)

	credentials, err := s.Database.GetServiceAccountCloudCredentials(ctx, &sa, cloud.Name)
	c.Check(err, qt.IsNil)
	c.Assert(credentials, qt.DeepEquals, []dbmodel.CloudCredential{cred1, cred2})
}
