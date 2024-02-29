// Copyright 2020 Canonical Ltd.

package db_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
)

func TestSetIdentityModelDefaults(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now()

	type testConfig struct {
		identity         *dbmodel.Identity
		defaults         map[string]interface{}
		expectedError    string
		expectedDefaults *dbmodel.IdentityModelDefaults
	}

	tests := []struct {
		about     string
		setup     func(c *qt.C, j *jimm.JIMM) testConfig
		assertion func(c *qt.C, db *db.Database)
	}{{
		about: "defaults do not exist yet - defaults created",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			identity := dbmodel.Identity{
				Name: "bob@canonical.com",
			}
			c.Assert(j.Database.DB.Create(&identity).Error, qt.IsNil)

			defaults := map[string]interface{}{
				"key1": float64(42),
				"key2": "a test string",
			}

			expectedDefaults := dbmodel.IdentityModelDefaults{
				IdentityName: identity.Name,
				Identity:     identity,
				Defaults:     defaults,
			}

			return testConfig{
				identity:         &identity,
				defaults:         defaults,
				expectedDefaults: &expectedDefaults,
			}
		},
	}, {
		about: "defaults already exist - defaults updated",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			identity := dbmodel.Identity{
				Name: "bob@canonical.com",
			}
			c.Assert(j.Database.DB.Create(&identity).Error, qt.IsNil)

			j.Database.SetIdentityModelDefaults(ctx, &dbmodel.IdentityModelDefaults{
				IdentityName: identity.Name,
				Identity:     identity,
				Defaults: map[string]interface{}{
					"key1": float64(17),
					"key2": "a test string",
				},
			})

			defaults := map[string]interface{}{
				"key1": float64(42),
				"key2": "a changed string",
				"key3": "a new value",
			}

			expectedDefaults := dbmodel.IdentityModelDefaults{
				IdentityName: identity.Name,
				Identity:     identity,
				Defaults:     defaults,
			}

			return testConfig{
				identity:         &identity,
				defaults:         defaults,
				expectedDefaults: &expectedDefaults,
			}
		},
	}, {
		about: "identity does not exist",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			identity := dbmodel.Identity{
				Name: "bob@canonical.com",
			}

			defaults := map[string]interface{}{
				"key1": float64(42),
				"key2": "a changed string",
				"key3": "a new value",
			}

			return testConfig{
				identity:      &identity,
				defaults:      defaults,
				expectedError: `.*violates foreign key constraint.*`,
			}
		},
	}, {
		about: "cannot set agent-version",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			identity := dbmodel.Identity{
				Name: "bob@canonical.com",
			}
			c.Assert(j.Database.DB.Create(&identity).Error, qt.IsNil)

			defaults := map[string]interface{}{
				"agent-version": "2.0",
				"key2":          "a changed string",
				"key3":          "a new value",
			}

			return testConfig{
				identity:      &identity,
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

			err = j.SetIdentityModelDefaults(ctx, testConfig.identity, testConfig.defaults)
			if testConfig.expectedError == "" {
				c.Assert(err, qt.Equals, nil)
				dbDefaults := dbmodel.IdentityModelDefaults{
					IdentityName: testConfig.expectedDefaults.IdentityName,
				}
				err = j.Database.IdentityModelDefaults(ctx, &dbDefaults)
				c.Assert(err, qt.Equals, nil)
				c.Assert(&dbDefaults, qt.CmpEquals(cmpopts.IgnoreTypes(gorm.Model{})), testConfig.expectedDefaults)
			} else {
				c.Assert(err, qt.ErrorMatches, testConfig.expectedError)
			}
		})
	}
}
