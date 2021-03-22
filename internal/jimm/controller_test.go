// Copyright 2020 Canonical Ltd.

package jimm_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/version"
	"github.com/juju/names/v4"
	semversion "github.com/juju/version"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

func TestAddController(t *testing.T) {
	c := qt.New(t)

	now := time.Now().UTC().Round(time.Millisecond)
	api := &jimmtest.API{
		Clouds_: func(context.Context) (map[names.CloudTag]jujuparams.Cloud, error) {
			clouds := map[names.CloudTag]jujuparams.Cloud{
				names.NewCloudTag("aws"): jujuparams.Cloud{
					Type:             "ec2",
					AuthTypes:        []string{"userpass"},
					Endpoint:         "https://example.com",
					IdentityEndpoint: "https://identity.example.com",
					StorageEndpoint:  "https://storage.example.com",
					Regions: []jujuparams.CloudRegion{{
						Name:             "eu-west-1",
						Endpoint:         "https://eu-west-1.example.com",
						IdentityEndpoint: "https://eu-west-1.identity.example.com",
						StorageEndpoint:  "https://eu-west-1.storage.example.com",
					}, {
						Name:             "eu-west-2",
						Endpoint:         "https://eu-west-2.example.com",
						IdentityEndpoint: "https://eu-west-2.identity.example.com",
						StorageEndpoint:  "https://eu-west-2.storage.example.com",
					}},
					CACertificates: []string{"CA CERT 1", "CA CERT 2"},
					Config: map[string]interface{}{
						"A": "a",
						"B": 0xb,
					},
					RegionConfig: map[string]map[string]interface{}{
						"eu-west-1": map[string]interface{}{
							"B": 0xb0,
							"C": "C",
						},
						"eu-west-2": map[string]interface{}{
							"B": 0xb1,
							"D": "D",
						},
					},
				},
				names.NewCloudTag("k8s"): jujuparams.Cloud{
					Type:      "kubernetes",
					AuthTypes: []string{"userpass"},
					Endpoint:  "https://k8s.example.com",
					Regions: []jujuparams.CloudRegion{{
						Name: "default",
					}},
				},
			}
			return clouds, nil
		},
		CloudInfo_: func(_ context.Context, tag names.CloudTag, ci *jujuparams.CloudInfo) error {
			if tag.Id() != "k8s" {
				c.Errorf("CloudInfo called for unexpected cloud %q", tag)
				return errors.E("unexpected cloud")
			}
			ci.Type = "kubernetes"
			ci.AuthTypes = []string{"userpass"}
			ci.Endpoint = "https://k8s.example.com"
			ci.Regions = []jujuparams.CloudRegion{{
				Name: "default",
			}}
			ci.Users = []jujuparams.CloudUserInfo{{
				UserName:    "alice@external",
				DisplayName: "Alice",
				Access:      "admin",
			}, {
				UserName:    "bob@external",
				DisplayName: "Bob",
				Access:      "add-model",
			}}
			return nil
		},
		ControllerModelSummary_: func(_ context.Context, ms *jujuparams.ModelSummary) error {
			ms.Name = "controller"
			ms.UUID = "5fddf0ed-83d5-47e8-ae7b-a4b27fc04a9f"
			ms.Type = "iaas"
			ms.ControllerUUID = jimmtest.DefaultControllerUUID
			ms.IsController = true
			ms.ProviderType = "ec2"
			ms.DefaultSeries = "warty"
			ms.CloudTag = "cloud-aws"
			ms.CloudRegion = "eu-west-1"
			ms.OwnerTag = "user-admin"
			ms.Life = "alive"
			ms.Status = jujuparams.EntityStatus{
				Status: "available",
			}
			ms.UserAccess = "admin"
			ms.AgentVersion = &version.Current
			return nil
		},
	}

	j := &jimm.JIMM{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
		},
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	}

	ctx := context.Background()
	err := j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	u := dbmodel.User{
		Username:         "alice@external",
		ControllerAccess: "superuser",
	}

	ctl := dbmodel.Controller{
		Name:          "test-controller",
		AdminUser:     "admin",
		AdminPassword: "5ecret",
		PublicAddress: "example.com:443",
	}
	err = j.AddController(context.Background(), &u, &ctl)
	c.Assert(err, qt.IsNil)

	ctl2 := dbmodel.Controller{
		Name: "test-controller",
	}
	err = j.Database.GetController(ctx, &ctl2)
	c.Assert(err, qt.IsNil)
	c.Check(ctl2, qt.CmpEquals(cmpopts.EquateEmpty()), ctl)
}

const testEarliestControllerVersionEnv = `clouds:
- name: test
  type: test
  regions:
  - name: test-region
cloud-credentials:
- name: test-cred
  cloud: test
  owner: alice@external
  type: empty
controllers:
- name: test1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region-1
  agent-version: 3.2.1
- name: test2
  uuid: 00000001-0000-0000-0000-000000000002
  cloud: test
  region: test-region-2
  agent-version: 3.2.0
- name: test3
  uuid: 00000001-0000-0000-0000-000000000003
  cloud: test
  region: test-region-3
  agent-version: 2.1.0
`

func TestEarliestControllerVersion(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	j := &jimm.JIMM{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
		},
	}

	err := j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, testEarliestControllerVersionEnv)
	env.PopulateDB(c, j.Database)

	v, err := j.EarliestControllerVersion(ctx)
	c.Assert(err, qt.Equals, nil)
	c.Assert(v, qt.DeepEquals, semversion.MustParse("2.1.0"))
}

const modifyControllerAccessEnv = `users:
- username: alice@external
  controller-access: superuser
- username: bob@external
  controller-access: superuser
- username: eve@external
  controller-access: add-model
- username: fred@external
  controller-access: login
- username: george@external
`

func TestGrantControllerAccess(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	tests := []struct {
		about               string
		user                string
		accessUser          string
		accessLevel         string
		expectedError       string
		expectedAccessLevel string
	}{{
		about:               "grant superuser access",
		user:                "alice@external",
		accessUser:          "george@external",
		accessLevel:         "superuser",
		expectedAccessLevel: "superuser",
	}, {
		about:               "grant add-model access",
		user:                "alice@external",
		accessUser:          "george@external",
		accessLevel:         "add-model",
		expectedAccessLevel: "add-model",
	}, {
		about:               "grant login access - users have add-model by default",
		user:                "alice@external",
		accessUser:          "george@external",
		accessLevel:         "login",
		expectedAccessLevel: "add-model",
	}, {
		about:         "invalid access level",
		user:          "alice@external",
		accessUser:    "bob@external",
		accessLevel:   "no-such-level",
		expectedError: `invalid access level "no-such-level"`,
	}, {
		about:         "not superuser",
		user:          "george@external",
		accessUser:    "eve@external",
		accessLevel:   "superuser",
		expectedError: `cannot grant controller access`,
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
				},
			}

			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, modifyControllerAccessEnv)
			env.PopulateDB(c, j.Database)

			user := env.User(test.user).DBObject(c, j.Database)
			err = j.GrantControllerAccess(ctx, &user, names.NewUserTag(test.accessUser), test.accessLevel)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			} else {
				u := dbmodel.User{
					Username: test.accessUser,
				}
				err = j.Database.GetUser(ctx, &u)
				c.Assert(err, qt.Equals, nil)
				c.Assert(u.ControllerAccess, qt.Equals, test.expectedAccessLevel)
			}

		})
	}
}

func TestRevokeControllerAccess(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	tests := []struct {
		about               string
		user                string
		accessUser          string
		accessLevel         string
		expectedError       string
		expectedAccessLevel string
	}{{
		about:               "revoke superuser access",
		user:                "alice@external",
		accessUser:          "bob@external",
		accessLevel:         "superuser",
		expectedAccessLevel: "add-model",
	}, {
		about:               "revoke add-model access",
		user:                "alice@external",
		accessUser:          "bob@external",
		accessLevel:         "add-model",
		expectedAccessLevel: "login",
	}, {
		about:               "revoke login access",
		user:                "alice@external",
		accessUser:          "bob@external",
		accessLevel:         "login",
		expectedAccessLevel: "",
	}, {
		about:         "invalid access level",
		user:          "alice@external",
		accessUser:    "bob@external",
		accessLevel:   "no-such-level",
		expectedError: `invalid access level "no-such-level"`,
	}, {
		about:         "not superuser",
		user:          "george@external",
		accessUser:    "eve@external",
		accessLevel:   "superuser",
		expectedError: `cannot revoke controller access`,
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
				},
			}

			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, modifyControllerAccessEnv)
			env.PopulateDB(c, j.Database)

			user := env.User(test.user).DBObject(c, j.Database)
			err = j.RevokeControllerAccess(ctx, &user, names.NewUserTag(test.accessUser), test.accessLevel)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			} else {
				u := dbmodel.User{
					Username: test.accessUser,
				}
				err = j.Database.GetUser(ctx, &u)
				c.Assert(err, qt.Equals, nil)
				c.Assert(u.ControllerAccess, qt.Equals, test.expectedAccessLevel)
			}

		})
	}
}
