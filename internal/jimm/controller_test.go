// Copyright 2020 Canonical Ltd.

package jimm_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	semversion "github.com/juju/version"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/vault"
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
			ms.AgentVersion = newVersion("1.2.3")
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

	ctl1 := dbmodel.Controller{
		Name:          "test-controller",
		AdminUser:     "admin",
		AdminPassword: "5ecret",
		PublicAddress: "example.com:443",
	}
	err = j.AddController(context.Background(), &u, &ctl1)
	c.Assert(err, qt.IsNil)

	ctl2 := dbmodel.Controller{
		Name: "test-controller",
	}
	err = j.Database.GetController(ctx, &ctl2)
	c.Assert(err, qt.IsNil)
	c.Check(ctl2, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(dbmodel.CloudRegion{})), ctl1)

	ctl3 := dbmodel.Controller{
		Name:          "test-controller-2",
		AdminUser:     "admin",
		AdminPassword: "5ecret",
		PublicAddress: "example.com:443",
	}
	err = j.AddController(context.Background(), &u, &ctl3)
	c.Assert(err, qt.IsNil)

	ctl4 := dbmodel.Controller{
		Name: "test-controller-2",
	}
	err = j.Database.GetController(ctx, &ctl4)
	c.Assert(err, qt.IsNil)
	c.Check(ctl4, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(dbmodel.CloudRegion{})), ctl3)
}

func TestAddControllerWithVault(t *testing.T) {
	c := qt.New(t)

	jimmtest.StartVault()
	defer jimmtest.StopVault()

	client, path, creds, ok := jimmtest.VaultClient(c)
	if !ok {
		c.Skip("vault not available")
	}
	store := &vault.VaultStore{
		Client:     client,
		AuthSecret: creds,
		AuthPath:   jimmtest.VaultAuthPath,
		KVPath:     path,
	}

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
			ms.AgentVersion = newVersion("1.2.3")
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
		CredentialStore: store,
	}

	ctx := context.Background()
	err := j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	u := dbmodel.User{
		Username:         "alice@external",
		ControllerAccess: "superuser",
	}

	ctl1 := dbmodel.Controller{
		Name:          "test-controller",
		AdminUser:     "admin",
		AdminPassword: "5ecret",
		PublicAddress: "example.com:443",
	}
	err = j.AddController(context.Background(), &u, &ctl1)
	c.Assert(err, qt.IsNil)
	c.Assert(ctl1.AdminUser, qt.Equals, "")
	c.Assert(ctl1.AdminPassword, qt.Equals, "")

	ctl2 := dbmodel.Controller{
		Name: "test-controller",
	}
	err = j.Database.GetController(ctx, &ctl2)
	c.Assert(err, qt.IsNil)
	c.Check(ctl2, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(dbmodel.CloudRegion{})), ctl1)

	username, password, err := store.GetControllerCredentials(ctx, ctl1.Name)
	c.Assert(err, qt.IsNil)
	c.Assert(username, qt.Equals, "admin")
	c.Assert(password, qt.Equals, "5ecret")

	ctl3 := dbmodel.Controller{
		Name:          "test-controller-2",
		AdminUser:     "admin",
		AdminPassword: "5ecretToo",
		PublicAddress: "example.com:443",
	}
	err = j.AddController(context.Background(), &u, &ctl3)
	c.Assert(err, qt.IsNil)
	c.Assert(ctl3.AdminUser, qt.Equals, "")
	c.Assert(ctl3.AdminPassword, qt.Equals, "")

	ctl4 := dbmodel.Controller{
		Name: "test-controller-2",
	}
	err = j.Database.GetController(ctx, &ctl4)
	c.Assert(err, qt.IsNil)
	c.Check(ctl4, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(dbmodel.CloudRegion{})), ctl3)

	username, password, err = store.GetControllerCredentials(ctx, ctl4.Name)
	c.Assert(err, qt.IsNil)
	c.Assert(username, qt.Equals, "admin")
	c.Assert(password, qt.Equals, "5ecretToo")
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
  controller-access: login
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
		about:               "grant login access - users have login by default",
		user:                "alice@external",
		accessUser:          "george@external",
		accessLevel:         "login",
		expectedAccessLevel: "login",
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

const testImportModelEnv = `
users:
- username: alice@external
  display-name: Alice
  controller-access: superuser
- username: bob@external
  display-name: Bob
  controller-access: login
clouds:
- name: test-cloud
  type: test
  regions:
  - name: test-region
cloud-credentials:
- name: test-credential
  cloud: test-cloud
  owner: alice@external
  type: empty
controllers:
- name: test-controller
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-region-1
  agent-version: 3.2.1
models:
- name: model-1
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000002
  controller: test-controller
  default-series: warty
  cloud: test-cloud
  region: test-region
  cloud-credential: test-credential
  owner: alice@external
  life: alive
  status:
    status: available
    info: "OK!"
    since: 2020-02-20T20:02:20Z
  users:
  - user: alice@external
    access: admin
  - user: bob@external
    access: write
  - user: charlie@external
    access: read
  sla:
    level: unsupported
  agent-version: 1.2.3
`

func TestImportModel(t *testing.T) {
	c := qt.New(t)
	trueValue := true

	now := time.Now().UTC().Truncate(time.Millisecond)

	tests := []struct {
		about          string
		user           string
		controllerName string
		modelUUID      string
		modelInfo      func(context.Context, *jujuparams.ModelInfo) error
		expectedModel  dbmodel.Model
		expectedError  string
		deltas         []jujuparams.Delta
	}{{
		about:          "model imported",
		user:           "alice@external",
		controllerName: "test-controller",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		modelInfo: func(_ context.Context, info *jujuparams.ModelInfo) error {
			info.Name = "test-model"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.CloudTag = names.NewCloudTag("test-cloud").String()
			info.CloudRegion = "test-region"
			info.CloudCredentialTag = names.NewCloudCredentialTag("test-cloud/alice@external/test-credential").String()
			info.CloudCredentialValidity = &trueValue
			info.OwnerTag = names.NewUserTag("alice@external").String()
			info.Life = life.Alive
			info.Status = jujuparams.EntityStatus{
				Status: status.Status("ok"),
				Info:   "test-info",
				Since:  &now,
			}
			info.Users = []jujuparams.ModelUserInfo{{
				UserName: "alice@external",
				Access:   jujuparams.ModelAdminAccess,
			}, {
				UserName: "bob@external",
				Access:   jujuparams.ModelReadAccess,
			}}
			info.Machines = []jujuparams.ModelMachineInfo{{
				Id:          "test-machine",
				DisplayName: "Test machine",
				Status:      "test-status",
				Message:     "test-message",
			}}
			info.SLA = &jujuparams.ModelSLAInfo{
				Level: "essential",
				Owner: "alice@external",
			}
			info.AgentVersion = newVersion("2.1.0")
			return nil
		},
		deltas: []jujuparams.Delta{{
			Entity: &jujuparams.ModelUpdate{
				ModelUUID:      "00000002-0000-0000-0000-000000000001",
				Name:           "test-model",
				Owner:          "alice@external",
				Life:           "alive",
				ControllerUUID: "00000001-0000-0000-0000-000000000001",
				Status: jujuparams.StatusInfo{
					Current: "available",
					Message: "updated status message",
					Version: "1.2.3",
					Since:   &now,
				},
				SLA: jujuparams.ModelSLAInfo{
					Level: "1",
					Owner: "me",
				},
			},
		}, {
			Entity: &jujuparams.ApplicationInfo{
				ModelUUID:       "00000002-0000-0000-0000-000000000001",
				Name:            "app-1",
				Exposed:         true,
				CharmURL:        "cs:app-1",
				Life:            "alive",
				MinUnits:        1,
				WorkloadVersion: "2",
			},
		}, {
			Entity: &jujuparams.MachineInfo{
				ModelUUID: "00000002-0000-0000-0000-000000000001",
				Id:        "machine-1",
				Life:      "alive",
				Hostname:  "test-machine-1",
			},
		}, {
			Entity: &jujuparams.UnitInfo{
				ModelUUID:   "00000002-0000-0000-0000-000000000001",
				Name:        "app-1/1",
				Application: "app-1",
				CharmURL:    "cs:app-1",
				Life:        "starting",
				MachineId:   "machine-1",
			},
		}, {
			// TODO (ashipika) ApplicationOfferInfo is currently ignored. Consider
			// fetching application offer details from the controller.
			Entity: &jujuparams.ApplicationOfferInfo{
				ModelUUID:            "00000002-0000-0000-0000-000000000001",
				OfferName:            "test-offer-1",
				OfferUUID:            "00000003-0000-0000-0000-000000000001",
				ApplicationName:      "app-1",
				CharmName:            "cs:~test-charmers/test-charm",
				TotalConnectedCount:  17,
				ActiveConnectedCount: 7,
			},
		}},
		expectedModel: dbmodel.Model{
			Name: "test-model",
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
			Owner: dbmodel.User{
				Username:         "alice@external",
				DisplayName:      "Alice",
				ControllerAccess: "superuser",
			},
			Controller: dbmodel.Controller{
				Name:         "test-controller",
				UUID:         "00000001-0000-0000-0000-000000000001",
				CloudName:    "test-cloud",
				CloudRegion:  "test-region-1",
				AgentVersion: "3.2.1",
			},
			CloudRegion: dbmodel.CloudRegion{
				Cloud: dbmodel.Cloud{
					Name: "test-cloud",
					Type: "test",
				},
				Name: "test-region",
			},
			CloudCredential: dbmodel.CloudCredential{
				Name: "test-credential",
			},
			Type:          "test-type",
			DefaultSeries: "test-series",
			Life:          "alive",
			Status: dbmodel.Status{
				Status: "available",
				Info:   "updated status message",
				Since: sql.NullTime{
					Valid: true,
					Time:  now,
				},
				Version: "1.2.3",
			},
			SLA: dbmodel.SLA{
				Level: "1",
				Owner: "me",
			},
			Users: []dbmodel.UserModelAccess{{
				User: dbmodel.User{
					Username:         "alice@external",
					DisplayName:      "Alice",
					ControllerAccess: "superuser",
				},
				Access: "admin",
			}, {
				User: dbmodel.User{
					Username:         "bob@external",
					DisplayName:      "Bob",
					ControllerAccess: "login",
				},
				Access: "read",
			}},
		},
	}, {
		about:          "model from local user imported",
		user:           "alice@external",
		controllerName: "test-controller",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		modelInfo: func(_ context.Context, info *jujuparams.ModelInfo) error {
			info.Name = "test-model"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.CloudTag = names.NewCloudTag("test-cloud").String()
			info.CloudRegion = "test-region"
			info.CloudCredentialTag = names.NewCloudCredentialTag("test-cloud/local-user/test-credential").String()
			info.CloudCredentialValidity = &trueValue
			info.OwnerTag = names.NewUserTag("local-user").String()
			info.Life = life.Alive
			info.Status = jujuparams.EntityStatus{
				Status: status.Status("available"),
				Info:   "test-info",
				Since:  &now,
			}
			info.Users = []jujuparams.ModelUserInfo{{
				UserName: "local-user",
				Access:   jujuparams.ModelAdminAccess,
			}, {
				UserName: "another-user",
				Access:   jujuparams.ModelReadAccess,
			}}
			info.Machines = []jujuparams.ModelMachineInfo{{
				Id:          "test-machine",
				DisplayName: "Test machine",
				Status:      "test-status",
				Message:     "test-message",
			}}
			info.SLA = &jujuparams.ModelSLAInfo{
				Level: "essential",
				Owner: "local-user",
			}
			info.AgentVersion = newVersion("2.1.0")
			return nil
		},
		expectedModel: dbmodel.Model{
			Name: "test-model",
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
			Owner: dbmodel.User{
				Username:         "alice@external", // Owner will switch to the user doing the import
				DisplayName:      "Alice",
				ControllerAccess: "superuser",
			},
			Controller: dbmodel.Controller{
				Name:         "test-controller",
				UUID:         "00000001-0000-0000-0000-000000000001",
				CloudName:    "test-cloud",
				CloudRegion:  "test-region-1",
				AgentVersion: "3.2.1",
			},
			CloudRegion: dbmodel.CloudRegion{
				Cloud: dbmodel.Cloud{
					Name: "test-cloud",
					Type: "test",
				},
				Name: "test-region",
			},
			CloudCredential: dbmodel.CloudCredential{
				Name: "test-credential",
			},
			Type:          "test-type",
			DefaultSeries: "test-series",
			Life:          "alive",
			Status: dbmodel.Status{
				Status: "available",
				Info:   "test-info",
				Since: sql.NullTime{
					Valid: true,
					Time:  now,
				},
				Version: "2.1.0",
			},
			SLA: dbmodel.SLA{
				Level: "essential",
				Owner: "local-user",
			},
			Users: []dbmodel.UserModelAccess{{
				User: dbmodel.User{
					Username:         "alice@external",
					DisplayName:      "Alice",
					ControllerAccess: "superuser",
				},
				Access: "admin",
			}},
		},
	}, {
		about:          "model not found",
		user:           "alice@external",
		controllerName: "test-controller",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		modelInfo: func(_ context.Context, info *jujuparams.ModelInfo) error {
			return errors.E(errors.CodeNotFound, "model not found")
		},
		expectedError: "model not found",
	}, {
		about:          "cloud credentials not found",
		user:           "alice@external",
		controllerName: "test-controller",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		modelInfo: func(_ context.Context, info *jujuparams.ModelInfo) error {
			info.Name = "test-model"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.CloudTag = names.NewCloudTag("test-cloud").String()
			info.CloudRegion = "test-region"
			info.CloudCredentialTag = names.NewCloudCredentialTag("test-cloud/alice@external/unknown-credential").String()
			info.CloudCredentialValidity = &trueValue
			info.OwnerTag = names.NewUserTag("alice@external").String()
			return nil
		},
		expectedError: `cloudcredential "test-cloud/alice@external/unknown-credential" not found`,
	}, {
		about:          "cloud region not found",
		user:           "alice@external",
		controllerName: "test-controller",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		modelInfo: func(_ context.Context, info *jujuparams.ModelInfo) error {
			info.Name = "test-model"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.CloudTag = names.NewCloudTag("test-cloud").String()
			info.CloudRegion = "unknown-region"
			info.CloudCredentialTag = names.NewCloudCredentialTag("test-cloud/alice@external/test-credential").String()
			info.CloudCredentialValidity = &trueValue
			info.OwnerTag = names.NewUserTag("alice@external").String()
			return nil
		},
		expectedError: `cloud region not found`,
	}, {
		about:          "not allowed if not superuser",
		user:           "bob@external",
		controllerName: "test-controller",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		modelInfo: func(_ context.Context, info *jujuparams.ModelInfo) error {
			info.Name = "test-model"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.CloudTag = names.NewCloudTag("test-cloud").String()
			info.CloudRegion = "test-region"
			info.CloudCredentialTag = names.NewCloudCredentialTag("test-cloud/alice@external/test-credential").String()
			info.CloudCredentialValidity = &trueValue
			info.OwnerTag = names.NewUserTag("alice@external").String()
			return nil
		},
		expectedError: `unauthorized`,
	}, {
		about:          "model already exists",
		user:           "alice@external",
		controllerName: "test-controller",
		modelUUID:      "00000002-0000-0000-0000-000000000002",
		modelInfo: func(_ context.Context, info *jujuparams.ModelInfo) error {
			info.Name = "model-1"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.CloudTag = names.NewCloudTag("test-cloud").String()
			info.CloudRegion = "test-region"
			info.CloudCredentialTag = names.NewCloudCredentialTag("test-cloud/alice@external/test-credential").String()
			info.CloudCredentialValidity = &trueValue
			info.OwnerTag = names.NewUserTag("alice@external").String()
			return nil
		},
		expectedError: `model already exists`,
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			api := &jimmtest.API{
				ModelInfo_: test.modelInfo,
				ModelWatcherNext_: func(ctx context.Context, id string) ([]jujuparams.Delta, error) {
					if id != test.about {
						return nil, errors.E("incorrect id")
					}
					return test.deltas, nil
				},
				ModelWatcherStop_: func(ctx context.Context, id string) error {
					if id != test.about {
						return errors.E("incorrect id")
					}
					return nil
				},
				WatchAll_: func(context.Context) (string, error) {
					return test.about, nil
				},
			}

			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: &jimmtest.Dialer{
					API: api,
				},
			}
			ctx := context.Background()
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, testImportModelEnv)
			env.PopulateDB(c, j.Database)

			user := env.User(test.user).DBObject(c, j.Database)
			err = j.ImportModel(ctx, &user, test.controllerName, names.NewModelTag(test.modelUUID))
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)

				m1 := dbmodel.Model{
					UUID: test.expectedModel.UUID,
				}
				err = j.Database.GetModel(ctx, &m1)
				c.Assert(err, qt.IsNil)
				c.Assert(m1, jimmtest.DBObjectEquals, test.expectedModel)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

const testControllerConfigEnv = `
users:
- username: alice@external
  controller-access: superuser
- username: eve@external
  controller-access: login
- username: fred@external
  controller-access: login
`

func TestSetControllerConfig(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		about          string
		user           string
		args           jujuparams.ControllerConfigSet
		expectedError  string
		expectedConfig dbmodel.ControllerConfig
	}{{
		about: "admin allowed to set config",
		user:  "alice@external",
		args: jujuparams.ControllerConfigSet{
			Config: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
				"key3": "value3",
			},
		},
		expectedConfig: dbmodel.ControllerConfig{
			Name: "jimm",
			Config: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
				"key3": "value3",
			},
		},
	}, {
		about: "add-model user - unauthorized",
		user:  "eve@external",
		args: jujuparams.ControllerConfigSet{
			Config: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
				"key3": "value3",
			},
		},
		expectedError: "unauthorized",
	}, {
		about: "login user - unauthorized",
		user:  "fred@external",
		args: jujuparams.ControllerConfigSet{
			Config: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
				"key3": "value3",
			},
		},
		expectedError: "unauthorized",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
			}
			ctx := context.Background()
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, testControllerConfigEnv)
			env.PopulateDB(c, j.Database)

			user := env.User(test.user).DBObject(c, j.Database)
			err = j.SetControllerConfig(ctx, &user, test.args)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)

				cfg := dbmodel.ControllerConfig{
					Name: "jimm",
				}
				err = j.Database.GetControllerConfig(ctx, &cfg)
				c.Assert(err, qt.IsNil)
				c.Assert(cfg, jimmtest.DBObjectEquals, test.expectedConfig)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

func TestGetControllerConfig(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		about          string
		user           string
		expectedError  string
		expectedConfig dbmodel.ControllerConfig
	}{{
		about: "admin allowed to set config",
		user:  "alice@external",
		expectedConfig: dbmodel.ControllerConfig{
			Name: "jimm",
			Config: map[string]interface{}{
				"key1": "value1",
			},
		},
	}, {
		about: "add-model user - unauthorized",
		user:  "eve@external",
		expectedConfig: dbmodel.ControllerConfig{
			Name: "jimm",
			Config: map[string]interface{}{
				"key1": "value1",
			},
		},
	}, {
		about: "login user - unauthorized",
		user:  "fred@external",
		expectedConfig: dbmodel.ControllerConfig{
			Name: "jimm",
			Config: map[string]interface{}{
				"key1": "value1",
			},
		},
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
			}
			ctx := context.Background()
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, testImportModelEnv)
			env.PopulateDB(c, j.Database)

			superuser := env.User("alice@external").DBObject(c, j.Database)
			user := env.User(test.user).DBObject(c, j.Database)
			err = j.SetControllerConfig(ctx, &superuser, jujuparams.ControllerConfigSet{
				Config: map[string]interface{}{
					"key1": "value1",
				},
			})
			c.Assert(err, qt.Equals, nil)

			cfg, err := j.GetControllerConfig(ctx, &user)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(cfg, jimmtest.DBObjectEquals, &test.expectedConfig)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

const testUpdateMigratedModelEnv = `
users:
- username: alice@external
  display-name: Alice
  controller-access: superuser
- username: bob@external
  display-name: Bob
  controller-access: login
clouds:
- name: test-cloud
  type: test
  regions:
  - name: test-region
cloud-credentials:
- name: test-credential
  cloud: test-cloud
  owner: alice@external
  type: empty
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-region-1
  agent-version: 3.2.1
- name: controller-2
  uuid: 00000001-0000-0000-0000-000000000002
  cloud: test-cloud
  region: test-region-1
  agent-version: 3.2.1
  admin-user: alice@external
  admin-password: c0ntr0113rs3cre7
models:
- name: model-1
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000002
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-region
  cloud-credential: test-credential
  owner: alice@external
  life: alive
  status:
    status: available
    info: "OK!"
    since: 2020-02-20T20:02:20Z
  users:
  - user: alice@external
    access: admin
  - user: bob@external
    access: write
  - user: charlie@external
    access: read
  sla:
    level: unsupported
  agent-version: 1.2.3
`

func TestUpdateMigratedModel(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		about            string
		user             string
		modelInfo        func(context.Context, *jujuparams.ModelInfo) error
		model            names.ModelTag
		targetController string
		expectedError    string
	}{{
		about:         "add-model user not allowed to update migrated model",
		user:          "bob@external",
		expectedError: "unauthorized",
	}, {
		about:         "model not found",
		user:          "alice@external",
		model:         names.NewModelTag("unknown-model"),
		expectedError: "model not found",
	}, {
		about:            "controller not found",
		user:             "alice@external",
		model:            names.NewModelTag("00000002-0000-0000-0000-000000000002"),
		targetController: "no-such-controller",
		expectedError:    "controller not found",
	}, {
		about:            "api returns an error",
		user:             "alice@external",
		model:            names.NewModelTag("00000002-0000-0000-0000-000000000002"),
		targetController: "controller-2",
		modelInfo: func(context.Context, *jujuparams.ModelInfo) error {
			return errors.E("an error")
		},
		expectedError: "an error",
	}, {
		about:            "all ok",
		user:             "alice@external",
		model:            names.NewModelTag("00000002-0000-0000-0000-000000000002"),
		targetController: "controller-2",
		modelInfo: func(context.Context, *jujuparams.ModelInfo) error {
			return nil
		},
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: &jimmtest.Dialer{
					API: &jimmtest.API{
						ModelInfo_: test.modelInfo,
					},
				},
			}
			ctx := context.Background()
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, testUpdateMigratedModelEnv)
			env.PopulateDB(c, j.Database)

			user := env.User(test.user).DBObject(c, j.Database)
			err = j.UpdateMigratedModel(ctx, &user, test.model, test.targetController)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			} else {
				c.Assert(err, qt.Equals, nil)

				model := dbmodel.Model{
					UUID: sql.NullString{
						String: test.model.Id(),
						Valid:  true,
					},
				}
				err = j.Database.GetModel(ctx, &model)
				c.Assert(err, qt.Equals, nil)
				c.Assert(model.Controller.Name, qt.Equals, test.targetController)
			}
		})
	}
}

const testGetControllerAccessEnv = `
users:
- username: alice@external
  display-name: Alice
  controller-access: superuser
- username: bob@external
  display-name: Bob
  controller-access: login
`

func TestGetControllerAccess(t *testing.T) {
	c := qt.New(t)

	j := &jimm.JIMM{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, nil),
		},
	}
	ctx := context.Background()
	err := j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, testGetControllerAccessEnv)
	env.PopulateDB(c, j.Database)
	user := env.User("alice@external").DBObject(c, j.Database)

	access, err := j.GetControllerAccess(ctx, &user, names.NewUserTag("alice@external"))
	c.Assert(err, qt.IsNil)
	c.Check(access, qt.Equals, "superuser")

	access, err = j.GetControllerAccess(ctx, &user, names.NewUserTag("bob@external"))
	c.Assert(err, qt.IsNil)
	c.Check(access, qt.Equals, "login")

	access, err = j.GetControllerAccess(ctx, &user, names.NewUserTag("charlie@external"))
	c.Assert(err, qt.IsNil)
	c.Check(access, qt.Equals, "login")

	user = env.User("bob@external").DBObject(c, j.Database)
	access, err = j.GetControllerAccess(ctx, &user, names.NewUserTag("bob@external"))
	c.Assert(err, qt.IsNil)
	c.Check(access, qt.Equals, "login")

	_, err = j.GetControllerAccess(ctx, &user, names.NewUserTag("alice@external"))
	c.Assert(err, qt.ErrorMatches, "unauthorized")
}
