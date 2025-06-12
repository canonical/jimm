// Copyright 2025 Canonical.

package jujuapi_test

import (
	"context"

	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

type jimmUnitTestSuite struct{}

var _ = gc.Suite(&jimmUnitTestSuite{})

func (s *jimmSuite) TestPrepareModelMigration_UnauthorizedUser(c *gc.C) {
	ctx := context.Background()

	root := newTestControllerRoot(mocks.JujuManager{}, "alice@canonical.com", false)

	err := root.PrepareModelMigration(ctx, apiparams.PrepareModelMigrationRequest{})

	c.Assert(err, gc.ErrorMatches, "unauthorized")
}

func (s *jimmSuite) TestPrepareModelMigration_InvalidModelTag(c *gc.C) {
	ctx := context.Background()

	root := newTestControllerRoot(mocks.JujuManager{}, "alice@canonical.com", true)

	err := root.PrepareModelMigration(ctx, apiparams.PrepareModelMigrationRequest{
		ModelTag: "blah",
	})

	c.Assert(err, gc.ErrorMatches, "invalid model tag")
}

func (s *jimmSuite) TestPrepareModelMigration_InvalidControllerName(c *gc.C) {
	ctx := context.Background()

	root := newTestControllerRoot(mocks.JujuManager{}, "alice@canonical.com", true)

	err := root.PrepareModelMigration(ctx, apiparams.PrepareModelMigrationRequest{
		ModelTag:             names.NewModelTag("5650ac3f-8332-437f-874f-089e0e447e7f").String(),
		TargetControllerName: "---bad wolf---",
	})

	c.Assert(err, gc.ErrorMatches, "invalid controller name")
}

func (s *jimmSuite) TestPrepareModelMigration_InvalidUserMapping(c *gc.C) {
	ctx := context.Background()

	root := newTestControllerRoot(mocks.JujuManager{}, "alice@canonical.com", true)

	err := root.PrepareModelMigration(ctx, apiparams.PrepareModelMigrationRequest{
		ModelTag:             names.NewModelTag("5650ac3f-8332-437f-874f-089e0e447e7f").String(),
		TargetControllerName: "controller",
		UserMapping:          map[string]string{"--bad local--": "alice@canonical.com"},
	})

	c.Assert(err, gc.ErrorMatches, `--bad local-- is not a valid local user name`)

	err = root.PrepareModelMigration(ctx, apiparams.PrepareModelMigrationRequest{
		ModelTag:             names.NewModelTag("5650ac3f-8332-437f-874f-089e0e447e7f").String(),
		TargetControllerName: "controller",
		UserMapping:          map[string]string{"alice": "alice"},
	})

	c.Assert(err, gc.ErrorMatches, `alice is not a valid external user name`)

	err = root.PrepareModelMigration(ctx, apiparams.PrepareModelMigrationRequest{
		ModelTag:             names.NewModelTag("5650ac3f-8332-437f-874f-089e0e447e7f").String(),
		TargetControllerName: "controller",
		UserMapping:          map[string]string{"alice": "--badwolf--@canonical.com"},
	})

	c.Assert(err, gc.ErrorMatches, `--badwolf--@canonical.com is not a valid external user name`)
}
