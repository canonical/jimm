// Copyright 2025 Canonical.

package db_test

import (
	"context"
	"database/sql"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

func (s *dbSuite) TestAddUserMapping(c *qt.C) {
	ctx := context.Background()
	env := initTestEnvironment(c, s.Database)

	modelUUID := env.model.UUID
	localUser := "localuser1"
	identityName := env.u.Name

	userMapping := &dbmodel.UserMapping{
		ModelUUID:        modelUUID,
		LocalUser:        localUser,
		ExternalUserName: identityName,
	}

	err := s.Database.AddUserMapping(ctx, userMapping)
	c.Assert(err, qt.IsNil)
	c.Assert(userMapping.ID, qt.Not(qt.Equals), 0)

	userMapping.ID = 0 // Reset ID to ensure it is not reused
	err = s.Database.AddUserMapping(ctx, userMapping)
	c.Assert(err, qt.Not(qt.IsNil))
}

func (s *dbSuite) TestGetUserMapping(c *qt.C) {
	ctx := context.Background()
	env := initTestEnvironment(c, s.Database)

	modelUUID := env.model.UUID
	localUser := "localuser1"
	identityName := env.u.Name

	// Insert mapping first
	userMapping := &dbmodel.UserMapping{
		ModelUUID:        modelUUID,
		LocalUser:        localUser,
		ExternalUserName: identityName,
	}
	c.Assert(s.Database.AddUserMapping(ctx, userMapping), qt.IsNil)

	lookup := &dbmodel.UserMapping{
		ModelUUID: modelUUID,
		LocalUser: localUser,
	}
	err := s.Database.GetUserMapping(ctx, lookup)
	c.Assert(err, qt.IsNil)
	c.Assert(lookup.ExternalUserName, qt.Equals, identityName)
	c.Assert(lookup.ModelUUID.String, qt.Equals, modelUUID.String)
	c.Assert(lookup.LocalUser, qt.Equals, localUser)

	// Not found
	missing := &dbmodel.UserMapping{
		ModelUUID: sql.NullString{String: "no-such-uuid", Valid: true},
		LocalUser: "nouser",
	}
	err = s.Database.GetUserMapping(ctx, missing)
	c.Assert(err, qt.ErrorMatches, ".*user mapping not found.*")
}

func (s *dbSuite) TestDeleteUserMapping(c *qt.C) {
	ctx := context.Background()
	env := initTestEnvironment(c, s.Database)

	modelUUID := env.model.UUID
	localUser := "localuser1"
	identityName := env.u.Name

	userMapping := &dbmodel.UserMapping{
		ModelUUID:        modelUUID,
		LocalUser:        localUser,
		ExternalUserName: identityName,
	}
	c.Assert(s.Database.AddUserMapping(ctx, userMapping), qt.IsNil)

	// Delete
	err := s.Database.DeleteUserMapping(ctx, userMapping)
	c.Assert(err, qt.IsNil)

	// Read after delete
	err = s.Database.GetUserMapping(ctx, userMapping)
	c.Assert(err, qt.ErrorMatches, ".*user mapping not found.*")
}

func (s *dbSuite) TestDeleteUserMappingsByModelUUID(c *qt.C) {
	ctx := context.Background()
	env := initTestEnvironment(c, s.Database)

	modelUUID := env.model.UUID
	localUser := "localuser1"
	identityName := env.u.Name

	userMapping := &dbmodel.UserMapping{
		ModelUUID:        modelUUID,
		LocalUser:        localUser,
		ExternalUserName: identityName,
	}
	c.Assert(s.Database.AddUserMapping(ctx, userMapping), qt.IsNil)

	userMapping2 := &dbmodel.UserMapping{
		ModelUUID:        modelUUID,
		LocalUser:        "localuser2",
		ExternalUserName: identityName,
	}
	c.Assert(s.Database.AddUserMapping(ctx, userMapping2), qt.IsNil)

	err := s.Database.DeleteUserMappingsByModelUUID(ctx, modelUUID.String)
	c.Assert(err, qt.IsNil)

	// Check that both mappings are deleted
	err = s.Database.GetUserMapping(ctx, userMapping)
	c.Assert(err, qt.ErrorMatches, ".*user mapping not found.*")
	err = s.Database.GetUserMapping(ctx, userMapping2)
	c.Assert(err, qt.ErrorMatches, ".*user mapping not found.*")
}
