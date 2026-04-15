// Copyright 2025 Canonical.

package db_test

import (
	"context"
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
)

func TestGetIdentityUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	i, err := dbmodel.NewIdentity("bob")
	c.Assert(err, qt.IsNil)
	err = d.GetIdentity(context.Background(), i)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetIdentity(c *qt.C) {
	ctx := context.Background()
	i, err := dbmodel.NewIdentity("bob")
	c.Assert(err, qt.IsNil)
	err = s.Database.GetIdentity(ctx, i)
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx)
	c.Assert(err, qt.IsNil)

	u, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	err = s.Database.GetIdentity(ctx, u)
	c.Assert(err, qt.IsNil)

	u2, err := dbmodel.NewIdentity(u.Name)
	c.Assert(err, qt.IsNil)
	err = s.Database.GetIdentity(ctx, u2)
	c.Assert(err, qt.IsNil)
	c.Check(u2, qt.DeepEquals, u)

	u3, err := dbmodel.NewIdentity("jimm_test@canonical.com")
	c.Assert(err, qt.IsNil)
	err = s.Database.GetIdentity(ctx, u3)
	c.Assert(err, qt.IsNil)
	c.Check(u3.Name, qt.DeepEquals, "jimm-test43cc8c@canonical.com")

	// Test get on the sanitised email returns ONLY the sanitised user
	// and doesn't create a new user
	u4, err := dbmodel.NewIdentity("jimm-test43cc8c@canonical.com")
	c.Assert(err, qt.IsNil)
	err = s.Database.GetIdentity(ctx, u4)
	c.Assert(err, qt.IsNil)
	c.Check(u4, qt.DeepEquals, u3)
}

func (s *dbSuite) TestGetIdentityConcurrent(c *qt.C) {
	ctx := context.Background()
	i, err := dbmodel.NewIdentity("bob")
	c.Assert(err, qt.IsNil)
	err = s.Database.GetIdentity(ctx, i)
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx)
	c.Assert(err, qt.IsNil)

	N := 50
	errorChannel := make(chan error, N)
	for range N {
		go func() {
			u, err := dbmodel.NewIdentity("bob@canonical.com")
			c.Check(err, qt.IsNil)

			errorChannel <- s.Database.GetIdentity(ctx, u)
		}()
	}

	for range N {
		err = <-errorChannel
		c.Assert(err, qt.IsNil)
	}
}

func (s *dbSuite) TestIdentityUserCreatedLogging(c *qt.C) {
	ctx := context.Background()

	err := s.Database.Migrate(ctx)
	c.Assert(err, qt.IsNil)

	core, logs := observer.New(zap.InfoLevel)
	ctx = zapctx.WithLogger(ctx, zap.New(core))

	i, err := dbmodel.NewIdentity("bob")
	c.Assert(err, qt.IsNil)
	err = s.Database.GetIdentity(ctx, i)
	c.Assert(err, qt.IsNil)
	// Logging creation
	c.Assert(logs.Len(), qt.Equals, 1)
	c.Assert(logs.All()[0].Message, qt.Equals, "user_created:bob")

	i, err = dbmodel.NewIdentity("bob")
	c.Assert(err, qt.IsNil)
	err = s.Database.GetIdentity(ctx, i)
	c.Assert(err, qt.IsNil)
	// No new creation, no extra logging
	c.Assert(logs.Len(), qt.Equals, 1)
}

func TestUpdateIdentityUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database

	i, err := dbmodel.NewIdentity("bob")
	c.Assert(err, qt.IsNil)
	err = d.UpdateIdentity(context.Background(), i)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestUpdateIdentity(c *qt.C) {
	ctx := context.Background()

	i, err := dbmodel.NewIdentity("bob")
	c.Assert(err, qt.IsNil)
	err = s.Database.UpdateIdentity(ctx, i)
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx)
	c.Assert(err, qt.IsNil)

	u, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	err = s.Database.GetIdentity(ctx, u)
	c.Assert(err, qt.IsNil)

	err = s.Database.UpdateIdentity(ctx, u)
	c.Assert(err, qt.IsNil)

	u2, err := dbmodel.NewIdentity(u.Name)
	c.Assert(err, qt.IsNil)
	err = s.Database.GetIdentity(ctx, u2)
	c.Assert(err, qt.IsNil)
	c.Check(u2, qt.DeepEquals, u)

	u3, err := dbmodel.NewIdentity("jimm_test@canonical.com")
	c.Assert(err, qt.IsNil)
	err = s.Database.GetIdentity(ctx, u3)
	c.Assert(err, qt.IsNil)
	c.Check(u3.Name, qt.DeepEquals, "jimm-test43cc8c@canonical.com")

	u3.AccessToken = "REMOVED-ACCESS-TOKEN-EXAMPLE"
	err = s.Database.UpdateIdentity(ctx, u3)
	c.Assert(err, qt.IsNil)

	// Do a final get just to be super clear the updates have taken effect on the
	// sanitised user
	u4, err := dbmodel.NewIdentity(u3.Name)
	c.Assert(err, qt.IsNil)
	err = s.Database.GetIdentity(ctx, u4)
	c.Assert(err, qt.IsNil)
	c.Assert(u4, qt.DeepEquals, u3)
}

func TestGetIdentityCloudCredentialsUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	i, err := dbmodel.NewIdentity("bob")
	c.Assert(err, qt.IsNil)
	_, err = d.GetIdentityCloudCredentials(context.Background(), i, "test-cloud")
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetIdentityCloudCredentials(c *qt.C) {
	ctx := context.Background()

	err := s.Database.Migrate(ctx)
	c.Assert(err, qt.IsNil)

	i, err := dbmodel.NewIdentity("idontexist")
	c.Assert(err, qt.IsNil)
	_, err = s.Database.GetIdentityCloudCredentials(ctx, i, "")
	c.Check(err, qt.ErrorMatches, `cloudcredential not found`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	i, err = dbmodel.NewIdentity("test")
	c.Assert(err, qt.IsNil)
	_, err = s.Database.GetIdentityCloudCredentials(ctx, i, "ec2")
	c.Check(err, qt.IsNil)

	i, err = dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(s.Database.DB.Create(i).Error, qt.IsNil)

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
		OwnerIdentityName: i.Name,
		AuthType:          "empty",
	}
	err = s.Database.SetCloudCredential(context.Background(), &cred1)
	c.Assert(err, qt.Equals, nil)

	cred2 := dbmodel.CloudCredential{
		Name:              "test-cred-2",
		CloudName:         cloud.Name,
		OwnerIdentityName: i.Name,
		AuthType:          "empty",
	}
	err = s.Database.SetCloudCredential(context.Background(), &cred2)
	c.Assert(err, qt.Equals, nil)

	credentials, err := s.Database.GetIdentityCloudCredentials(ctx, i, cloud.Name)
	c.Check(err, qt.IsNil)
	c.Assert(credentials, qt.DeepEquals, []dbmodel.CloudCredential{cred1, cred2})
}

func (s *dbSuite) TestListIdentities(c *qt.C) {
	err := s.Database.Migrate(context.Background())
	c.Assert(err, qt.IsNil)

	for i := range 10 {
		id, _ := dbmodel.NewIdentity(fmt.Sprintf("bob%d@canonical.com", i))
		err = s.Database.GetIdentity(context.Background(), id)
		c.Assert(err, qt.IsNil)
	}
	ctx := context.Background()
	firstIdentities, err := s.Database.ListIdentities(ctx, 5, 0, "")
	c.Assert(err, qt.IsNil)
	for i := range 5 {
		c.Assert(firstIdentities[i].Name, qt.Equals, fmt.Sprintf("bob%d@canonical.com", i))
	}
	secondIdentities, err := s.Database.ListIdentities(ctx, 5, 5, "")
	c.Assert(err, qt.IsNil)
	for i := range 5 {
		c.Assert(secondIdentities[i].Name, qt.Equals, fmt.Sprintf("bob%d@canonical.com", i+5))
	}

	filteredIdentities, err := s.Database.ListIdentities(ctx, 5, 0, "bob0")
	c.Assert(err, qt.IsNil)
	c.Assert(filteredIdentities, qt.HasLen, 1)
	c.Assert(filteredIdentities[0].Name, qt.Equals, "bob0@canonical.com")
}
