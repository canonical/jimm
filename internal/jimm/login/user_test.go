// Copyright 2024 Canonical.
package login_test

import (
	"context"
	"time"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
)

func (s *loginManagerSuite) TestGetOrCreateUser(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	ofgaUser, err := s.manager.GetOrCreateUser(ctx, "bob@canonical.com.com")
	c.Assert(err, qt.IsNil)
	// Username -> email
	c.Assert(ofgaUser.Name, qt.Equals, "bob@canonical.com.com")
	// As no display name was set for this user as they're being created this time over
	c.Assert(ofgaUser.DisplayName, qt.Equals, "bob")
	// This user SHOULD NOT be an admin, so ensure admin check is OK
	c.Assert(ofgaUser.JimmAdmin, qt.IsFalse)

	// Next we'll update this user to an admin of JIMM and run the same tests.
	c.Assert(
		ofgaUser.SetControllerAccess(
			context.Background(),
			s.jimmTag,
			ofganames.AdministratorRelation,
		),
		qt.IsNil,
	)

	ofgaUser, err = s.manager.GetOrCreateUser(ctx, "bob@canonical.com.com")
	c.Assert(err, qt.IsNil)

	c.Assert(ofgaUser.Name, qt.Equals, "bob@canonical.com.com")
	c.Assert(ofgaUser.DisplayName, qt.Equals, "bob")
	// This user SHOULD be an admin, so ensure admin check is OK
	c.Assert(ofgaUser.JimmAdmin, qt.IsTrue)
}

func (s *loginManagerSuite) TestUpdateLastLogin(c *qt.C) {
	c.Parallel()

	ctx := context.Background()

	ofgaUser, err := s.manager.UpdateLastLogin(ctx, "bob@canonical.com.com")
	c.Assert(err, qt.IsNil)
	c.Assert(ofgaUser, qt.Not(qt.IsNil))

	user := dbmodel.Identity{Name: "bob@canonical.com.com"}
	err = s.db.GetIdentity(ctx, &user)
	c.Assert(err, qt.IsNil)
	c.Assert(user.DisplayName, qt.Equals, "bob")
	c.Assert(user.LastLogin.Time, qt.Not(qt.Equals), time.Time{})
	c.Assert(user.LastLogin.Valid, qt.IsTrue)
}
