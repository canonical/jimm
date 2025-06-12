// Copyright 2025 Canonical.

package jujuapi_test

import (
	"context"

	"github.com/juju/juju/core/migration"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
)

type migrationTargetUnitSuite struct {
}

var _ = gc.Suite(&migrationTargetUnitSuite{})

func (s *migrationTargetUnitSuite) TestMigrationTarget(c *gc.C) {
	ctx := context.Background()

	preChecksCalled := false
	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			Prechecks_: func(ctx context.Context, user *openfga.User, model migration.ModelInfo) error {
				preChecksCalled = true
				c.Assert(model.UUID, gc.Equals, "00000001-0000-0000-0000-000000000001")
				c.Assert(model.Owner.Id(), gc.Equals, "bob")
				return nil
			},
		}}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jimm.JujuManager {
			return &jujuManager
		},
	}

	var u dbmodel.Identity
	u.SetTag(names.NewUserTag("alice@canonical.com"))
	user := openfga.NewUser(&u, nil)

	cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(cr, user)

	args := jujuparams.MigrationModelInfo{
		UUID:     "00000001-0000-0000-0000-000000000001",
		Name:     "test-model",
		OwnerTag: names.NewUserTag("bob").String(),
	}

	// Validate access denied without JIMM admin permissions.
	err := cr.Prechecks(ctx, args)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(preChecksCalled, gc.Equals, false)

	// Validate the precheck method is called when the user is a JIMM admin.
	user.JimmAdmin = true
	err = cr.Prechecks(ctx, args)
	c.Assert(err, gc.IsNil)
	c.Assert(preChecksCalled, gc.Equals, true)

	// Validate that an invalid owner tag is rejected.
	args.OwnerTag = "invalid-owner-tag"
	err = cr.Prechecks(ctx, args)
	c.Assert(err, gc.ErrorMatches, `"invalid-owner-tag" is not a valid tag`)
}
