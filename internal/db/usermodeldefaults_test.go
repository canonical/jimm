// Copyright 2020 Canonical Ltd.

package db_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

func TestSetUserModelDefaults(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now()

	type testConfig struct {
		user             *dbmodel.User
		defaults         map[string]interface{}
		expectedError    string
		expectedDefaults *dbmodel.UserModelDefaults
	}

	tests := []struct {
		about     string
		setup     func(c *qt.C, j *jimm.JIMM) testConfig
		assertion func(c *qt.C, db *db.Database)
	}{{
		about: "defaults do not exist yet - defaults created",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			user := dbmodel.User{
				Username: "bob@external",
			}
			c.Assert(j.Database.DB.Create(&user).Error, qt.IsNil)

			defaults := map[string]interface{}{
				"key1": float64(42),
				"key2": "a test string",
			}

			expectedDefaults := dbmodel.UserModelDefaults{
				Username: user.Username,
				User:     user,
				Defaults: defaults,
			}

			return testConfig{
				user:             &user,
				defaults:         defaults,
				expectedDefaults: &expectedDefaults,
			}
		},
	}, {
		about: "defaults already exist - defaults updated",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			user := dbmodel.User{
				Username: "bob@external",
			}
			c.Assert(j.Database.DB.Create(&user).Error, qt.IsNil)

			j.Database.SetUserModelDefaults(ctx, &dbmodel.UserModelDefaults{
				Username: user.Username,
				User:     user,
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

			expectedDefaults := dbmodel.UserModelDefaults{
				Username: user.Username,
				User:     user,
				Defaults: defaults,
			}

			return testConfig{
				user:             &user,
				defaults:         defaults,
				expectedDefaults: &expectedDefaults,
			}
		},
	}, {
		about: "user does not exist",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			user := dbmodel.User{
				Username: "bob@external",
			}

			defaults := map[string]interface{}{
				"key1": float64(42),
				"key2": "a changed string",
				"key3": "a new value",
			}

			return testConfig{
				user:          &user,
				defaults:      defaults,
				expectedError: `FOREIGN KEY constraint failed`,
			}
		},
	}, {
		about: "cannot set agent-version",
		setup: func(c *qt.C, j *jimm.JIMM) testConfig {
			user := dbmodel.User{
				Username: "bob@external",
			}
			c.Assert(j.Database.DB.Create(&user).Error, qt.IsNil)

			defaults := map[string]interface{}{
				"agent-version": "2.0",
				"key2":          "a changed string",
				"key3":          "a new value",
			}

			return testConfig{
				user:          &user,
				defaults:      defaults,
				expectedError: `agent-version cannot have a default value`,
			}
		},
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
				},
			}
			err := j.Database.Migrate(ctx, true)
			c.Assert(err, qt.Equals, nil)

			testConfig := test.setup(c, j)

			err = j.SetUserModelDefaults(ctx, testConfig.user, testConfig.defaults)
			if testConfig.expectedError == "" {
				c.Assert(err, qt.Equals, nil)
				dbDefaults := dbmodel.UserModelDefaults{
					Username: testConfig.expectedDefaults.Username,
				}
				err = j.Database.UserModelDefaults(ctx, &dbDefaults)
				c.Assert(err, qt.Equals, nil)
				c.Assert(&dbDefaults, qt.CmpEquals(cmpopts.IgnoreTypes(gorm.Model{})), testConfig.expectedDefaults)
			} else {
				c.Assert(err, qt.ErrorMatches, testConfig.expectedError)
			}
		})
	}
}
