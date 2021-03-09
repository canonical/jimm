// Copyright 2021 Canonical Ltd.

package db_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

func TestUpsertCloudRegionDefaultsUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.UpsertCloudRegionDefaults(context.Background(), &dbmodel.CloudRegionDefaults{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestModelDefaults(c *qt.C) {
	ctx := context.Background()

	err := s.Database.Migrate(ctx, true)
	c.Assert(err, qt.Equals, nil)

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

	defaults := dbmodel.CloudRegionDefaults{
		UserID:        u.Username,
		User:          u,
		CloudRegionID: cloud.Regions[0].ID,
		CloudRegion:   cloud.Regions[0],
		Defaults: map[string]interface{}{
			"key1": float64(17),
			"key2": "some other data",
		},
	}
	err = s.Database.UpsertCloudRegionDefaults(ctx, &defaults)
	c.Check(err, qt.Equals, nil)

	d, err := s.Database.ModelDefaultsForCloud(ctx, &u, &cloud)
	c.Assert(err, qt.Equals, nil)
	c.Assert(d, qt.HasLen, 1)
	c.Assert(d[0], qt.DeepEquals, defaults)

	delete(defaults.Defaults, "key1")
	err = s.Database.UpsertCloudRegionDefaults(ctx, &defaults)
	c.Check(err, qt.Equals, nil)

	d, err = s.Database.ModelDefaultsForCloud(ctx, &u, &cloud)
	c.Assert(err, qt.Equals, nil)
	c.Assert(d, qt.HasLen, 1)
	c.Assert(d[0].Defaults, qt.DeepEquals, dbmodel.Map{
		"key2": "some other data",
	})
}

func TestModelDefaultsForCloudUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	_, err := d.ModelDefaultsForCloud(context.Background(), &dbmodel.User{}, &dbmodel.Cloud{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}
