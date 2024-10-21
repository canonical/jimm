// Copyright 2024 Canonical.

package jimm_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gorm.io/gorm"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
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
			identity, err := dbmodel.NewIdentity("bob@canonical.com")
			c.Assert(err, qt.IsNil)

			c.Assert(j.Database.DB.Create(identity).Error, qt.IsNil)

			defaults := map[string]interface{}{
				"key1": float64(42),
				"key2": "a test string",
			}

			expectedDefaults := dbmodel.IdentityModelDefaults{
				IdentityName: identity.Name,
				Identity:     *identity,
				Defaults:     defaults,
			}

			return testConfig{
				identity:         identity,
				defaults:         defaults,
				expectedDefaults: &expectedDefaults,
			}
		},
	}, {
		about: "defaults already exist - defaults updated",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			identity, err := dbmodel.NewIdentity("bob@canonical.com")
			c.Assert(err, qt.IsNil)

			c.Assert(j.Database.DB.Create(identity).Error, qt.IsNil)

			err = j.Database.SetIdentityModelDefaults(ctx, &dbmodel.IdentityModelDefaults{
				IdentityName: identity.Name,
				Identity:     *identity,
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

			expectedDefaults := dbmodel.IdentityModelDefaults{
				IdentityName: identity.Name,
				Identity:     *identity,
				Defaults:     defaults,
			}

			return testConfig{
				identity:         identity,
				defaults:         defaults,
				expectedDefaults: &expectedDefaults,
			}
		},
	}, {
		about: "cannot set agent-version",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			identity, err := dbmodel.NewIdentity("bob@canonical.com")
			c.Assert(err, qt.IsNil)

			c.Assert(j.Database.DB.Create(identity).Error, qt.IsNil)

			defaults := map[string]interface{}{
				"agent-version": "2.0",
				"key2":          "a changed string",
				"key3":          "a new value",
			}

			return testConfig{
				identity:      identity,
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

func TestIdentityModelDefaults(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now()

	type testConfig struct {
		identity         *dbmodel.Identity
		expectedError    string
		expectedDefaults map[string]interface{}
	}

	tests := []struct {
		about     string
		setup     func(c *qt.C, j *jimm.JIMM) testConfig
		assertion func(c *qt.C, db *db.Database)
	}{{
		about: "defaults do not exist",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			identity, err := dbmodel.NewIdentity("bob@canonical.com")
			c.Assert(err, qt.IsNil)

			c.Assert(j.Database.DB.Create(identity).Error, qt.IsNil)

			return testConfig{
				identity:      identity,
				expectedError: "identitymodeldefaults not found",
			}
		},
	}, {
		about: "defaults exist",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			identity, err := dbmodel.NewIdentity("bob@canonical.com")
			c.Assert(err, qt.IsNil)

			c.Assert(j.Database.DB.Create(identity).Error, qt.IsNil)

			err = j.Database.SetIdentityModelDefaults(ctx, &dbmodel.IdentityModelDefaults{
				IdentityName: identity.Name,
				Identity:     *identity,
				Defaults: map[string]interface{}{
					"key1": float64(42),
					"key2": "a changed string",
					"key3": "a new value",
				},
			})
			c.Assert(err, qt.IsNil)

			return testConfig{
				identity: identity,
				expectedDefaults: map[string]interface{}{
					"key1": float64(42),
					"key2": "a changed string",
					"key3": "a new value",
				},
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

			defaults, err := j.IdentityModelDefaults(ctx, testConfig.identity)
			if testConfig.expectedError == "" {
				c.Assert(err, qt.Equals, nil)
				c.Assert(defaults, qt.CmpEquals(cmpopts.IgnoreTypes(gorm.Model{})), testConfig.expectedDefaults)
			} else {
				c.Assert(err, qt.ErrorMatches, testConfig.expectedError)
			}
		})
	}
}
