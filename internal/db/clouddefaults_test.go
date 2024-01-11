// Copyright 2021 Canonical Ltd.

package db_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/names/v4"
	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
)

func (s *dbSuite) TestModelDefaults(c *qt.C) {
	ctx := context.Background()

	err := s.Database.Migrate(ctx, true)
	c.Assert(err, qt.Equals, nil)

	u := dbmodel.Identity{
		Username: "bob@external",
	}
	c.Assert(s.Database.DB.Create(&u).Error, qt.IsNil)

	cloud1 := dbmodel.Cloud{
		Name: "test-cloud-1",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region",
		}},
	}
	c.Assert(s.Database.DB.Create(&cloud1).Error, qt.IsNil)

	cloud2 := dbmodel.Cloud{
		Name: "test-cloud-2",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region",
		}},
	}
	c.Assert(s.Database.DB.Create(&cloud2).Error, qt.IsNil)

	cloud := cloud1
	cloud.Regions = nil
	defaults := dbmodel.CloudDefaults{
		Username: u.Username,
		User:     u,
		CloudID:  cloud.ID,
		Cloud:    cloud,
		Region:   cloud1.Regions[0].Name,
		Defaults: map[string]interface{}{
			"key1": float64(17),
			"key2": "some other data",
		},
	}
	err = s.Database.SetCloudDefaults(ctx, &defaults)
	c.Check(err, qt.Equals, nil)

	d, err := s.Database.ModelDefaultsForCloud(ctx, &u, names.NewCloudTag("test-cloud-1"))
	c.Assert(err, qt.Equals, nil)
	c.Assert(d, qt.HasLen, 1)
	c.Assert(d[0], qt.DeepEquals, defaults)

	defaults.Defaults["key1"] = float64(71)
	defaults.Defaults["key3"] = "more data"
	err = s.Database.SetCloudDefaults(ctx, &defaults)
	c.Check(err, qt.Equals, nil)

	d, err = s.Database.ModelDefaultsForCloud(ctx, &u, names.NewCloudTag("test-cloud-1"))
	c.Assert(err, qt.Equals, nil)
	c.Assert(d, qt.HasLen, 1)
	c.Assert(d[0].Defaults, qt.DeepEquals, dbmodel.Map{
		"key1": float64(71),
		"key2": "some other data",
		"key3": "more data",
	})

	dbDefaults := dbmodel.CloudDefaults{
		Username: u.Username,
		CloudID:  cloud2.ID,
		Cloud:    cloud2,
		Region:   cloud2.Regions[0].Name,
	}
	err = s.Database.CloudDefaults(ctx, &dbDefaults)
	c.Assert(err, qt.ErrorMatches, "cloudregiondefaults not found")
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	dbDefaults = dbmodel.CloudDefaults{
		Username: u.Username,
		CloudID:  cloud1.ID,
		Cloud:    cloud1,
		Region:   cloud1.Regions[0].Name,
	}
	err = s.Database.CloudDefaults(ctx, &dbDefaults)
	c.Assert(err, qt.Equals, nil)
	c.Assert(dbDefaults, qt.DeepEquals, defaults)

	err = s.Database.UnsetCloudDefaults(ctx, &defaults, []string{"key1", "key2", "unknown-key"})
	c.Assert(err, qt.Equals, nil)

	err = s.Database.CloudDefaults(ctx, &dbDefaults)
	c.Assert(err, qt.Equals, nil)
	c.Assert(dbDefaults, qt.CmpEquals(cmpopts.IgnoreTypes([]dbmodel.CloudRegion{}, gorm.Model{})), dbmodel.CloudDefaults{
		Username: u.Username,
		User:     u,
		CloudID:  cloud1.ID,
		Cloud:    cloud1,
		Region:   cloud1.Regions[0].Name,
		Defaults: map[string]interface{}{
			"key3": "more data",
		},
	})

	err = s.Database.UnsetCloudDefaults(ctx, &dbmodel.CloudDefaults{
		Username: u.Username,
		CloudID:  cloud2.ID,
		Region:   "no-such-region",
	}, []string{"key1", "key2", "unknown-key"})
	c.Assert(err, qt.ErrorMatches, "cloudregiondefaults not found")
}

func TestSetCloudDefaultsUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.SetCloudDefaults(context.Background(), &dbmodel.CloudDefaults{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func TestUnsetCloudDefaultsUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.UnsetCloudDefaults(context.Background(), &dbmodel.CloudDefaults{}, nil)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func TestCloudDefaultsUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.CloudDefaults(context.Background(), &dbmodel.CloudDefaults{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func TestModelDefaultsForCloudUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	_, err := d.ModelDefaultsForCloud(context.Background(), &dbmodel.Identity{}, names.NewCloudTag("test-cloud"))
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}
