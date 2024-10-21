// Copyright 2024 Canonical.

package jimm_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"gorm.io/gorm"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func TestSetCloudDefaults(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now()

	type testConfig struct {
		user             *dbmodel.Identity
		cloud            names.CloudTag
		region           string
		defaults         map[string]interface{}
		expectedError    string
		expectedDefaults *dbmodel.CloudDefaults
	}

	tests := []struct {
		about     string
		setup     func(c *qt.C, j *jimm.JIMM) testConfig
		assertion func(c *qt.C, db *db.Database)
	}{{
		about: "defaults do not exist yet - defaults created",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			user, err := dbmodel.NewIdentity("bob@canonical.com")
			c.Assert(err, qt.IsNil)
			c.Assert(j.Database.DB.Create(user).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud-1",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region",
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			defaults := map[string]interface{}{
				"key1": float64(42),
				"key2": "a test string",
			}

			cloud.Regions = nil
			expectedDefaults := dbmodel.CloudDefaults{
				IdentityName: user.Name,
				Identity:     *user,
				CloudID:      cloud.ID,
				Cloud:        cloud,
				Region:       "test-region",
				Defaults:     defaults,
			}

			return testConfig{
				user:             user,
				cloud:            names.NewCloudTag(cloud.Name),
				region:           "test-region",
				defaults:         defaults,
				expectedDefaults: &expectedDefaults,
			}
		},
	}, {
		about: "set defaults without region - defaults created",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			user, err := dbmodel.NewIdentity("bob@canonical.com")
			c.Assert(err, qt.IsNil)

			c.Assert(j.Database.DB.Create(user).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud-1",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region",
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			defaults := map[string]interface{}{
				"key1": float64(42),
				"key2": "a test string",
			}

			cloud.Regions = nil
			expectedDefaults := dbmodel.CloudDefaults{
				IdentityName: user.Name,
				Identity:     *user,
				CloudID:      cloud.ID,
				Cloud:        cloud,
				Defaults:     defaults,
			}

			return testConfig{
				user:             user,
				cloud:            names.NewCloudTag(cloud.Name),
				region:           "",
				defaults:         defaults,
				expectedDefaults: &expectedDefaults,
			}
		},
	}, {
		about: "defaults already exist - defaults updated",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			user, err := dbmodel.NewIdentity("bob@canonical.com")
			c.Assert(err, qt.IsNil)

			c.Assert(j.Database.DB.Create(user).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud-1",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region",
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			err = j.Database.SetCloudDefaults(ctx, &dbmodel.CloudDefaults{
				IdentityName: user.Name,
				Identity:     *user,
				CloudID:      cloud.ID,
				Cloud:        cloud,
				Region:       cloud.Regions[0].Name,
				Defaults: map[string]interface{}{
					"key1": float64(17),
					"key2": "a test string",
				},
			})
			c.Assert(err, qt.IsNil)

			defaults := map[string]interface{}{
				"key1": float64(42),
				"key2": "a changed string",
				"key3": "a new value",
			}

			cloud.Regions = nil
			expectedDefaults := dbmodel.CloudDefaults{
				IdentityName: user.Name,
				Identity:     *user,
				CloudID:      cloud.ID,
				Cloud:        cloud,
				Region:       "test-region",
				Defaults:     defaults,
			}

			return testConfig{
				user:             user,
				cloud:            names.NewCloudTag(cloud.Name),
				region:           "test-region",
				defaults:         defaults,
				expectedDefaults: &expectedDefaults,
			}
		},
	}, {
		about: "cloudregion does not exist",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			user, err := dbmodel.NewIdentity("bob@canonical.com")
			c.Assert(err, qt.IsNil)

			c.Assert(j.Database.DB.Create(user).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud-1",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region",
				}},
			}

			defaults := map[string]interface{}{
				"key1": float64(42),
				"key2": "a changed string",
				"key3": "a new value",
			}

			return testConfig{
				user:          user,
				cloud:         names.NewCloudTag(cloud.Name),
				region:        cloud.Regions[0].Name,
				defaults:      defaults,
				expectedError: `cloud "test-cloud-1" not found`,
			}
		},
	}, {
		about: "cannot set agent-version",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			user, err := dbmodel.NewIdentity("bob@canonical.com")
			c.Assert(err, qt.IsNil)

			c.Assert(j.Database.DB.Create(user).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud-1",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region",
				}},
			}

			defaults := map[string]interface{}{
				"agent-version": "2.0",
				"key2":          "a changed string",
				"key3":          "a new value",
			}

			return testConfig{
				user:          user,
				cloud:         names.NewCloudTag(cloud.Name),
				region:        cloud.Regions[0].Name,
				defaults:      defaults,
				expectedError: `agent-version cannot have a default value`,
			}
		},
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.PostgresDB(c, func() time.Time { return now }),
				},
			}
			err := j.Database.Migrate(ctx, true)
			c.Assert(err, qt.Equals, nil)

			testConfig := test.setup(c, j)

			err = j.SetModelDefaults(ctx, testConfig.user, testConfig.cloud, testConfig.region, testConfig.defaults)
			if testConfig.expectedError == "" {
				c.Assert(err, qt.Equals, nil)
				dbDefaults := dbmodel.CloudDefaults{
					IdentityName: testConfig.expectedDefaults.IdentityName,
					Cloud: dbmodel.Cloud{
						Name: testConfig.expectedDefaults.Cloud.Name,
					},
					Region: testConfig.expectedDefaults.Region,
				}
				err = j.Database.CloudDefaults(ctx, &dbDefaults)
				c.Assert(err, qt.Equals, nil)
				c.Assert(&dbDefaults, qt.CmpEquals(cmpopts.IgnoreTypes(gorm.Model{})), testConfig.expectedDefaults)
			} else {
				c.Assert(err, qt.ErrorMatches, testConfig.expectedError)
			}
		})
	}
}

func TestUnsetCloudDefaults(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now()

	type testConfig struct {
		user             *dbmodel.Identity
		cloud            names.CloudTag
		region           string
		keys             []string
		expectedError    string
		expectedDefaults *dbmodel.CloudDefaults
	}

	tests := []struct {
		about     string
		setup     func(c *qt.C, j *jimm.JIMM) testConfig
		assertion func(c *qt.C, db *db.Database)
	}{{
		about: "all ok - keys removed from the defaults map",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			user, err := dbmodel.NewIdentity("bob@canonical.com")
			c.Assert(err, qt.IsNil)

			c.Assert(j.Database.DB.Create(user).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud-1",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region",
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			err = j.Database.SetCloudDefaults(ctx, &dbmodel.CloudDefaults{
				IdentityName: user.Name,
				CloudID:      cloud.ID,
				Region:       cloud.Regions[0].Name,
				Defaults: map[string]interface{}{
					"key1": float64(17),
					"key2": "a test string",
					"key3": "some value",
				},
			})
			c.Assert(err, qt.Equals, nil)

			keys := []string{
				"key1",
				"key3",
				"unknown-key",
			}

			cloud.Regions = nil
			expectedDefaults := dbmodel.CloudDefaults{
				IdentityName: user.Name,
				Identity:     *user,
				CloudID:      cloud.ID,
				Cloud:        cloud,
				Region:       "test-region",
				Defaults: map[string]interface{}{
					"key2": "a test string",
				},
			}

			return testConfig{
				user:             user,
				cloud:            names.NewCloudTag(cloud.Name),
				region:           "test-region",
				keys:             keys,
				expectedDefaults: &expectedDefaults,
			}
		},
	}, {
		about: "unset without region - keys removed from the defaults map",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			user, err := dbmodel.NewIdentity("bob@canonical.com")
			c.Assert(err, qt.IsNil)

			c.Assert(j.Database.DB.Create(user).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud-1",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region",
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			err = j.Database.SetCloudDefaults(ctx, &dbmodel.CloudDefaults{
				IdentityName: user.Name,
				CloudID:      cloud.ID,
				Defaults: map[string]interface{}{
					"key1": float64(17),
					"key2": "a test string",
					"key3": "some value",
				},
			})
			c.Assert(err, qt.Equals, nil)

			keys := []string{
				"key1",
				"key3",
				"unknown-key",
			}

			cloud.Regions = nil
			expectedDefaults := dbmodel.CloudDefaults{
				IdentityName: user.Name,
				Identity:     *user,
				CloudID:      cloud.ID,
				Cloud:        cloud,
				Region:       "",
				Defaults: map[string]interface{}{
					"key2": "a test string",
				},
			}

			return testConfig{
				user:             user,
				cloud:            names.NewCloudTag(cloud.Name),
				region:           "",
				keys:             keys,
				expectedDefaults: &expectedDefaults,
			}
		},
	}, {
		about: "cloudregiondefaults not found",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			user, err := dbmodel.NewIdentity("bob@canonical.com")
			c.Assert(err, qt.IsNil)

			c.Assert(j.Database.DB.Create(user).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud-1",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region",
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			keys := []string{
				"key1",
				"key3",
				"unknown-key",
			}

			return testConfig{
				user:          user,
				cloud:         names.NewCloudTag(cloud.Name),
				region:        cloud.Name,
				keys:          keys,
				expectedError: "cloudregiondefaults not found",
			}
		},
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.PostgresDB(c, func() time.Time { return now }),
				},
			}
			err := j.Database.Migrate(ctx, true)
			c.Assert(err, qt.Equals, nil)

			testConfig := test.setup(c, j)

			err = j.UnsetModelDefaults(ctx, testConfig.user, testConfig.cloud, testConfig.region, testConfig.keys)
			if testConfig.expectedError == "" {
				c.Assert(err, qt.Equals, nil)
				dbDefaults := dbmodel.CloudDefaults{
					IdentityName: testConfig.expectedDefaults.IdentityName,
					Cloud: dbmodel.Cloud{
						Name: testConfig.cloud.Id(),
					},
					Region: testConfig.expectedDefaults.Region,
				}
				err = j.Database.CloudDefaults(ctx, &dbDefaults)
				c.Assert(err, qt.Equals, nil)
				c.Assert(&dbDefaults, qt.CmpEquals(cmpopts.IgnoreTypes(gorm.Model{})), testConfig.expectedDefaults)
			} else {
				c.Assert(err, qt.ErrorMatches, testConfig.expectedError)
			}
		})
	}
}

func TestModelDefaultsForCloud(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now()

	j := &jimm.JIMM{
		Database: db.Database{
			DB: jimmtest.PostgresDB(c, func() time.Time { return now }),
		},
	}
	err := j.Database.Migrate(ctx, true)
	c.Assert(err, qt.Equals, nil)

	user, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(j.Database.DB.Create(user).Error, qt.IsNil)

	user1, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(j.Database.DB.Create(user1).Error, qt.IsNil)

	cloud1 := dbmodel.Cloud{
		Name: "test-cloud-1",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region-1",
		}, {
			Name: "test-region-2",
		}},
	}
	c.Assert(j.Database.DB.Create(&cloud1).Error, qt.IsNil)

	cloud2 := dbmodel.Cloud{
		Name: "test-cloud-2",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region",
		}},
	}
	c.Assert(j.Database.DB.Create(&cloud2).Error, qt.IsNil)

	err = j.Database.SetCloudDefaults(ctx, &dbmodel.CloudDefaults{
		IdentityName: user.Name,
		CloudID:      cloud1.ID,
		Region:       cloud1.Regions[0].Name,
		Defaults: map[string]interface{}{
			"key1": float64(17),
			"key2": "a test string",
			"key3": "some value",
		},
	})
	c.Assert(err, qt.Equals, nil)

	err = j.Database.SetCloudDefaults(ctx, &dbmodel.CloudDefaults{
		IdentityName: user.Name,
		CloudID:      cloud1.ID,
		Region:       cloud1.Regions[1].Name,
		Defaults: map[string]interface{}{
			"key2": "a different string",
			"key4": float64(42),
		},
	})
	c.Assert(err, qt.Equals, nil)

	err = j.Database.SetCloudDefaults(ctx, &dbmodel.CloudDefaults{
		IdentityName: user.Name,
		CloudID:      cloud2.ID,
		Region:       cloud2.Regions[0].Name,
		Defaults: map[string]interface{}{
			"key2": "a different string",
			"key4": float64(42),
			"key5": "test",
		},
	})
	c.Assert(err, qt.Equals, nil)

	err = j.Database.SetCloudDefaults(ctx, &dbmodel.CloudDefaults{
		IdentityName: user.Name,
		CloudID:      cloud2.ID,
		Region:       "",
		Defaults: map[string]interface{}{
			"key1": "value",
			"key4": float64(37),
		},
	})
	c.Assert(err, qt.Equals, nil)

	result, err := j.ModelDefaultsForCloud(ctx, user, names.NewCloudTag(cloud1.Name))
	c.Assert(err, qt.Equals, nil)
	c.Assert(result, qt.DeepEquals, jujuparams.ModelDefaultsResult{
		Config: map[string]jujuparams.ModelDefaults{
			"key1": {
				Regions: []jujuparams.RegionDefaults{{
					RegionName: "test-region-1",
					Value:      float64(17),
				}},
			},
			"key2": {
				Regions: []jujuparams.RegionDefaults{{
					RegionName: "test-region-1",
					Value:      "a test string",
				}, {
					RegionName: "test-region-2",
					Value:      "a different string",
				}},
			},
			"key3": {
				Regions: []jujuparams.RegionDefaults{{
					RegionName: "test-region-1",
					Value:      "some value",
				}},
			},
			"key4": {
				Regions: []jujuparams.RegionDefaults{{
					RegionName: "test-region-2",
					Value:      float64(42),
				}},
			},
		},
	})

	result, err = j.ModelDefaultsForCloud(ctx, user, names.NewCloudTag(cloud2.Name))
	c.Assert(err, qt.Equals, nil)
	c.Assert(result, qt.DeepEquals, jujuparams.ModelDefaultsResult{
		Config: map[string]jujuparams.ModelDefaults{
			"key1": {
				Default: "value",
			},
			"key2": {
				Regions: []jujuparams.RegionDefaults{{
					RegionName: "test-region",
					Value:      "a different string",
				}},
			},
			"key4": {
				Default: float64(37),
				Regions: []jujuparams.RegionDefaults{{
					RegionName: "test-region",
					Value:      float64(42),
				}},
			},
			"key5": {
				Regions: []jujuparams.RegionDefaults{{
					RegionName: "test-region",
					Value:      "test",
				}},
			},
		},
	})

	result, err = j.ModelDefaultsForCloud(ctx, user1, names.NewCloudTag(cloud2.Name))
	c.Assert(err, qt.Equals, nil)
	c.Assert(result, qt.DeepEquals, jujuparams.ModelDefaultsResult{
		Config: map[string]jujuparams.ModelDefaults{},
	})

}
