// Copyright 2026 Canonical.

package db_test

import (
	"context"
	"database/sql"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
)

func (s *dbSuite) TestControllerBootstrapCRUD(c *qt.C) {
	ctx := context.Background()
	c.Assert(s.Database.Migrate(ctx), qt.IsNil)

	bootstrap := dbmodel.ControllerBootstrap{
		Name:        "test-controller",
		CloudName:   "test-cloud",
		CloudRegion: "test-region",
	}
	c.Assert(s.Database.AddControllerBootstrap(ctx, &bootstrap), qt.IsNil)
	c.Assert(bootstrap.ID, qt.Not(qt.Equals), uint(0))

	byName := dbmodel.ControllerBootstrap{Name: bootstrap.Name}
	c.Assert(s.Database.GetControllerBootstrap(ctx, &byName), qt.IsNil)
	c.Check(byName, qt.DeepEquals, bootstrap)

	bootstrap.JobID = sql.NullInt64{Int64: 99, Valid: true}
	c.Assert(s.Database.UpdateControllerBootstrap(ctx, &bootstrap), qt.IsNil)

	byJob := dbmodel.ControllerBootstrap{JobID: sql.NullInt64{Int64: 99, Valid: true}}
	c.Assert(s.Database.GetControllerBootstrap(ctx, &byJob), qt.IsNil)
	c.Check(byJob, qt.DeepEquals, bootstrap)

	bootstraps, err := s.Database.ListControllerBootstraps(ctx)
	c.Assert(err, qt.IsNil)
	c.Check(bootstraps, qt.DeepEquals, []dbmodel.ControllerBootstrap{bootstrap})

	c.Assert(s.Database.DeleteControllerBootstrap(ctx, &bootstrap), qt.IsNil)
	c.Check(errors.ErrorCode(s.Database.GetControllerBootstrap(ctx, &dbmodel.ControllerBootstrap{Name: bootstrap.Name})), qt.Equals, errors.CodeNotFound)
	bootstraps, err = s.Database.ListControllerBootstraps(ctx)
	c.Assert(err, qt.IsNil)
	c.Check(bootstraps, qt.HasLen, 0)
}

func TestGetControllerBootstrapUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.GetControllerBootstrap(context.Background(), &dbmodel.ControllerBootstrap{Name: "test"})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}
