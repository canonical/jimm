// Copyright 2024 Canonical.

package jimm_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/canonical/ofga"
	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"
	semversion "github.com/juju/version"
	"gopkg.in/macaroon.v2"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/vault"
)

func TestAddController(t *testing.T) {
	c := qt.New(t)

	now := time.Now().UTC().Round(time.Millisecond)
	api := &jimmtest.API{
		Clouds_: func(context.Context) (map[names.CloudTag]jujuparams.Cloud, error) {
			clouds := map[names.CloudTag]jujuparams.Cloud{
				names.NewCloudTag("aws"): {
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
						"eu-west-1": {
							"B": 0xb0,
							"C": "C",
						},
						"eu-west-2": {
							"B": 0xb1,
							"D": "D",
						},
					},
				},
				names.NewCloudTag("k8s"): {
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
			// TODO(Kian) We can remove these returned users, we ignore them when importing a
			// controller into JIMM.
			ci.Users = []jujuparams.CloudUserInfo{{
				UserName:    "alice@canonical.com",
				DisplayName: "Alice",
				Access:      "admin",
			}, {
				UserName:    "bob@canonical.com",
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
			ms.Life = life.Value(state.Alive.String())
			ms.Status = jujuparams.EntityStatus{
				Status: "available",
			}
			ms.UserAccess = "admin"
			ms.AgentVersion = newVersion("1.2.3")
			return nil
		},
	}

	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	j := &jimm.JIMM{
		UUID: uuid.NewString(),
		Database: db.Database{
			DB: jimmtest.PostgresDB(c, func() time.Time { return now }),
		},
		Dialer: &jimmtest.Dialer{
			API: api,
		},
		OpenFGAClient: client,
	}

	ctx := context.Background()
	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)

	alice := openfga.NewUser(u, client)
	alice.JimmAdmin = true
	err = alice.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	ctl1 := dbmodel.Controller{
		Name:              "test-controller",
		AdminIdentityName: "admin",
		AdminPassword:     "5ecret",
		PublicAddress:     "example.com:443",
	}
	err = j.AddController(context.Background(), alice, &ctl1)
	c.Assert(err, qt.IsNil)

	ctl2 := dbmodel.Controller{
		Name: "test-controller",
	}
	err = j.Database.GetController(ctx, &ctl2)
	c.Assert(err, qt.IsNil)
	c.Check(ctl2, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(dbmodel.CloudRegion{})), ctl1)

	ctl3 := dbmodel.Controller{
		Name:              "test-controller-2",
		AdminIdentityName: "admin",
		AdminPassword:     "5ecret",
		PublicAddress:     "example.com:443",
	}
	err = j.AddController(context.Background(), alice, &ctl3)
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

	client, path, roleID, roleSecretID, ok := jimmtest.VaultClient(c)
	if !ok {
		c.Skip("vault not available")
	}
	store := &vault.VaultStore{
		Client:       client,
		RoleID:       roleID,
		RoleSecretID: roleSecretID,
		KVPath:       path,
	}

	now := time.Now().UTC().Round(time.Millisecond)
	api := &jimmtest.API{
		Clouds_: func(context.Context) (map[names.CloudTag]jujuparams.Cloud, error) {
			clouds := map[names.CloudTag]jujuparams.Cloud{
				names.NewCloudTag("aws"): {
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
						"eu-west-1": {
							"B": 0xb0,
							"C": "C",
						},
						"eu-west-2": {
							"B": 0xb1,
							"D": "D",
						},
					},
				},
				names.NewCloudTag("k8s"): {
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
			// TODO(Kian) We can remove these returned users, we ignore them when importing a
			// controller into JIMM.
			ci.Users = []jujuparams.CloudUserInfo{{
				UserName:    "alice@canonical.com",
				DisplayName: "Alice",
				Access:      "admin",
			}, {
				UserName:    "bob@canonical.com",
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
			ms.Life = life.Value(state.Alive.String())
			ms.Status = jujuparams.EntityStatus{
				Status: "available",
			}
			ms.UserAccess = "admin"
			ms.AgentVersion = newVersion("1.2.3")
			return nil
		},
	}

	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	j := &jimm.JIMM{
		UUID: uuid.NewString(),
		Database: db.Database{
			DB: jimmtest.PostgresDB(c, func() time.Time { return now }),
		},
		Dialer: &jimmtest.Dialer{
			API: api,
		},
		CredentialStore: store,
		OpenFGAClient:   ofgaClient,
	}

	ctx := context.Background()
	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)

	alice := openfga.NewUser(u, ofgaClient)
	alice.JimmAdmin = true

	err = alice.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	ctl1 := dbmodel.Controller{
		Name:              "test-controller",
		AdminIdentityName: "admin",
		AdminPassword:     "5ecret",
		PublicAddress:     "example.com:443",
	}
	err = j.AddController(context.Background(), alice, &ctl1)
	c.Assert(err, qt.IsNil)
	c.Assert(ctl1.AdminIdentityName, qt.Equals, "")
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
		Name:              "test-controller-2",
		AdminIdentityName: "admin",
		AdminPassword:     "5ecretToo",
		PublicAddress:     "example.com:443",
	}
	err = j.AddController(context.Background(), alice, &ctl3)
	c.Assert(err, qt.IsNil)
	c.Assert(ctl3.AdminIdentityName, qt.Equals, "")
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
  owner: alice@canonical.com
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

	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	j := &jimm.JIMM{
		UUID: uuid.NewString(),
		Database: db.Database{
			DB: jimmtest.PostgresDB(c, func() time.Time { return now }),
		},
		OpenFGAClient: client,
	}

	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, testEarliestControllerVersionEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

	v, err := j.EarliestControllerVersion(ctx)
	c.Assert(err, qt.Equals, nil)
	c.Assert(v, qt.DeepEquals, semversion.MustParse("2.1.0"))
}

const testImportModelEnv = `
users:
- username: alice@canonical.com
  display-name: Alice
  controller-access: superuser
- username: bob@canonical.com
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
  owner: alice@canonical.com
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
  owner: alice@canonical.com
  life: alive
  status:
    status: available
    info: "OK!"
    since: 2020-02-20T20:02:20Z
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: write
  - user: charlie@canonical.com
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
		newOwner       string
		jimmAdmin      bool
		expectedModel  dbmodel.Model
		expectedError  string
		deltas         []jujuparams.Delta
	}{{
		about:          "model imported",
		user:           "alice@canonical.com",
		controllerName: "test-controller",
		newOwner:       "",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		jimmAdmin:      true,
		modelInfo: func(_ context.Context, info *jujuparams.ModelInfo) error {
			info.Name = "test-model"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.CloudTag = names.NewCloudTag("test-cloud").String()
			info.CloudRegion = "test-region"
			info.CloudCredentialTag = names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential").String()
			info.CloudCredentialValidity = &trueValue
			info.OwnerTag = names.NewUserTag("alice@canonical.com").String()
			info.Life = life.Alive
			info.Status = jujuparams.EntityStatus{
				Status: status.Status("ok"),
				Info:   "test-info",
				Since:  &now,
			}
			info.Users = []jujuparams.ModelUserInfo{{
				UserName: "alice@canonical.com",
				Access:   jujuparams.ModelAdminAccess,
			}, {
				UserName: "bob@canonical.com",
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
				Owner: "alice@canonical.com",
			}
			info.AgentVersion = newVersion("2.1.0")
			return nil
		},
		deltas: []jujuparams.Delta{{
			Entity: &jujuparams.ModelUpdate{
				ModelUUID:      "00000002-0000-0000-0000-000000000001",
				Name:           "test-model",
				Owner:          "alice@canonical.com",
				Life:           life.Value(state.Alive.String()),
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
				Life:            life.Value(state.Alive.String()),
				MinUnits:        1,
				WorkloadVersion: "2",
			},
		}, {
			Entity: &jujuparams.MachineInfo{
				ModelUUID: "00000002-0000-0000-0000-000000000001",
				Id:        "machine-1",
				Life:      life.Value(state.Alive.String()),
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
			Owner: dbmodel.Identity{
				Name:        "alice@canonical.com",
				DisplayName: "Alice",
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
			Life:          state.Alive.String(),
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
		},
	}, {
		about:          "model from local user imported",
		user:           "alice@canonical.com",
		controllerName: "test-controller",
		newOwner:       "alice@canonical.com",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		jimmAdmin:      true,
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
			Owner: dbmodel.Identity{
				Name:        "alice@canonical.com",
				DisplayName: "Alice",
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
			Life:          state.Alive.String(),
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
		},
	}, {
		about:          "new model owner is local user",
		user:           "alice@canonical.com",
		controllerName: "test-controller",
		newOwner:       "bob",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		expectedError:  "cannot import model from local user, try --owner to switch the model owner",
		jimmAdmin:      true,
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
	}, {
		about:          "model not found",
		user:           "alice@canonical.com",
		controllerName: "test-controller",
		newOwner:       "",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		jimmAdmin:      true,
		modelInfo: func(_ context.Context, info *jujuparams.ModelInfo) error {
			return errors.E(errors.CodeNotFound, "model not found")
		},
		expectedError: "model not found",
	}, {
		about:          "fail import from local user without newOwner flag",
		user:           "alice@canonical.com",
		controllerName: "test-controller",
		newOwner:       "",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		jimmAdmin:      true,
		modelInfo: func(_ context.Context, info *jujuparams.ModelInfo) error {
			info.Name = "test-model"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.CloudTag = names.NewCloudTag("test-cloud").String()
			info.CloudRegion = "test-region"
			info.CloudCredentialTag = names.NewCloudCredentialTag("test-cloud/alice@canonical.com/unknown-credential").String()
			info.CloudCredentialValidity = &trueValue
			info.OwnerTag = names.NewUserTag("local-user").String()
			return nil
		},
		expectedError: `cannot import model from local user, try --owner to switch the model owner`,
	}, {
		about:          "cloud credentials not found",
		user:           "alice@canonical.com",
		controllerName: "test-controller",
		newOwner:       "",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		jimmAdmin:      true,
		modelInfo: func(_ context.Context, info *jujuparams.ModelInfo) error {
			info.Name = "test-model"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.CloudTag = names.NewCloudTag("invalid-cloud").String()
			info.CloudRegion = "test-region"
			info.CloudCredentialTag = names.NewCloudCredentialTag("invalid-cloud/alice@canonical.com/unknown-credential").String()
			info.CloudCredentialValidity = &trueValue
			info.OwnerTag = names.NewUserTag("alice@canonical.com").String()
			return nil
		},
		expectedError: `Failed to find cloud credential for user alice@canonical.com on cloud invalid-cloud`,
	}, {
		about:          "cloud region not found",
		user:           "alice@canonical.com",
		controllerName: "test-controller",
		newOwner:       "",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		jimmAdmin:      true,
		modelInfo: func(_ context.Context, info *jujuparams.ModelInfo) error {
			info.Name = "test-model"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.CloudTag = names.NewCloudTag("test-cloud").String()
			info.CloudRegion = "unknown-region"
			info.CloudCredentialTag = names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential").String()
			info.CloudCredentialValidity = &trueValue
			info.OwnerTag = names.NewUserTag("alice@canonical.com").String()
			return nil
		},
		expectedError: `cloud region not found`,
	}, {
		about:          "not allowed if not superuser",
		user:           "bob@canonical.com",
		controllerName: "test-controller",
		newOwner:       "",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		jimmAdmin:      false,
		modelInfo: func(_ context.Context, info *jujuparams.ModelInfo) error {
			info.Name = "test-model"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.CloudTag = names.NewCloudTag("test-cloud").String()
			info.CloudRegion = "test-region"
			info.CloudCredentialTag = names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential").String()
			info.CloudCredentialValidity = &trueValue
			info.OwnerTag = names.NewUserTag("alice@canonical.com").String()
			return nil
		},
		expectedError: `unauthorized`,
	}, {
		about:          "model already exists",
		user:           "alice@canonical.com",
		controllerName: "test-controller",
		newOwner:       "",
		modelUUID:      "00000002-0000-0000-0000-000000000002",
		jimmAdmin:      true,
		modelInfo: func(_ context.Context, info *jujuparams.ModelInfo) error {
			info.Name = "model-1"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.CloudTag = names.NewCloudTag("test-cloud").String()
			info.CloudRegion = "test-region"
			info.CloudCredentialTag = names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential").String()
			info.CloudCredentialValidity = &trueValue
			info.OwnerTag = names.NewUserTag("alice@canonical.com").String()
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

			client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name(), test.about)
			c.Assert(err, qt.IsNil)

			j := &jimm.JIMM{
				UUID: uuid.NewString(),
				Database: db.Database{
					DB: jimmtest.PostgresDB(c, nil),
				},
				Dialer: &jimmtest.Dialer{
					API:  api,
					UUID: test.expectedModel.Controller.UUID,
				},
				OpenFGAClient: client,
			}
			ctx := context.Background()
			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, testImportModelEnv)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

			dbUser := env.User(test.user).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, client)
			user.JimmAdmin = test.jimmAdmin

			err = j.ImportModel(ctx, user, test.controllerName, names.NewModelTag(test.modelUUID), test.newOwner)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)

				m1 := dbmodel.Model{
					UUID: test.expectedModel.UUID,
				}
				err = j.Database.GetModel(ctx, &m1)
				c.Assert(err, qt.IsNil)
				c.Assert(m1, jimmtest.DBObjectEquals, test.expectedModel)
				c.Assert(user.GetModelAccess(ctx, names.NewModelTag(test.modelUUID)), qt.Equals, ofganames.AdministratorRelation)
				controllerPermissionCheck := ofga.Tuple{
					Object:   ofganames.ConvertTag(names.NewControllerTag(test.expectedModel.Controller.UUID)),
					Relation: ofganames.ControllerRelation,
					Target:   ofganames.ConvertTag(names.NewModelTag(test.modelUUID)),
				}
				ok, err := client.CheckRelation(ctx, controllerPermissionCheck, false)
				c.Assert(err, qt.IsNil)
				c.Assert(ok, qt.IsTrue)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

const testControllerConfigEnv = `
users:
- username: alice@canonical.com
- username: eve@canonical.com
- username: fred@canonical.com
`

func TestSetControllerConfig(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		about          string
		user           string
		args           jujuparams.ControllerConfigSet
		jimmAdmin      bool
		expectedError  string
		expectedConfig dbmodel.ControllerConfig
	}{{
		about: "admin allowed to set config",
		user:  "alice@canonical.com",
		args: jujuparams.ControllerConfigSet{
			Config: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
				"key3": "value3",
			},
		},
		jimmAdmin: true,
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
		user:  "eve@canonical.com",
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
		user:  "fred@canonical.com",
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
				UUID: uuid.NewString(),
				Database: db.Database{
					DB: jimmtest.PostgresDB(c, nil),
				},
			}
			ctx := context.Background()
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, testControllerConfigEnv)
			env.PopulateDB(c, j.Database)

			dbUser := env.User(test.user).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, nil)
			user.JimmAdmin = test.jimmAdmin

			err = j.SetControllerConfig(ctx, user, test.args)
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
		jimmAdmin      bool
		expectedError  string
		expectedConfig dbmodel.ControllerConfig
	}{{
		about:     "admin allowed to set config",
		user:      "alice@canonical.com",
		jimmAdmin: true,
		expectedConfig: dbmodel.ControllerConfig{
			Name: "jimm",
			Config: map[string]interface{}{
				"key1": "value1",
			},
		},
	}, {
		about:     "add-model user - unauthorized",
		user:      "eve@canonical.com",
		jimmAdmin: false,
		expectedConfig: dbmodel.ControllerConfig{
			Name: "jimm",
			Config: map[string]interface{}{
				"key1": "value1",
			},
		},
	}, {
		about:     "login user - unauthorized",
		user:      "fred@canonical.com",
		jimmAdmin: false,
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
				UUID: uuid.NewString(),
				Database: db.Database{
					DB: jimmtest.PostgresDB(c, nil),
				},
			}
			ctx := context.Background()
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, testImportModelEnv)
			env.PopulateDB(c, j.Database)

			dbSuperuser := env.User("alice@canonical.com").DBObject(c, j.Database)
			superuser := openfga.NewUser(&dbSuperuser, nil)
			superuser.JimmAdmin = true

			dbUser := env.User(test.user).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, nil)
			user.JimmAdmin = test.jimmAdmin

			err = j.SetControllerConfig(ctx, superuser, jujuparams.ControllerConfigSet{
				Config: map[string]interface{}{
					"key1": "value1",
				},
			})
			c.Assert(err, qt.Equals, nil)

			cfg, err := j.GetControllerConfig(ctx, user.Identity)
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
- username: alice@canonical.com
  display-name: Alice
  controller-access: superuser
- username: bob@canonical.com
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
  owner: alice@canonical.com
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
  admin-user: alice@canonical.com
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
  owner: alice@canonical.com
  life: alive
  status:
    status: available
    info: "OK!"
    since: 2020-02-20T20:02:20Z
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: write
  - user: charlie@canonical.com
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
		jimmAdmin        bool
		expectedError    string
	}{{
		about:         "add-model user not allowed to update migrated model",
		user:          "bob@canonical.com",
		expectedError: "unauthorized",
	}, {
		about:         "model not found",
		user:          "alice@canonical.com",
		model:         names.NewModelTag("unknown-model"),
		expectedError: "model not found",
		jimmAdmin:     true,
	}, {
		about:            "controller not found",
		user:             "alice@canonical.com",
		model:            names.NewModelTag("00000002-0000-0000-0000-000000000002"),
		targetController: "no-such-controller",
		expectedError:    "controller not found",
		jimmAdmin:        true,
	}, {
		about:            "api returns an error",
		user:             "alice@canonical.com",
		model:            names.NewModelTag("00000002-0000-0000-0000-000000000002"),
		targetController: "controller-2",
		modelInfo: func(context.Context, *jujuparams.ModelInfo) error {
			return errors.E("an error")
		},
		expectedError: "an error",
		jimmAdmin:     true,
	}, {
		about:            "all ok",
		user:             "alice@canonical.com",
		model:            names.NewModelTag("00000002-0000-0000-0000-000000000002"),
		targetController: "controller-2",
		modelInfo: func(context.Context, *jujuparams.ModelInfo) error {
			return nil
		},
		jimmAdmin: true,
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			j := &jimm.JIMM{
				UUID: uuid.NewString(),
				Database: db.Database{
					DB: jimmtest.PostgresDB(c, nil),
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

			dbUser := env.User(test.user).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, nil)
			user.JimmAdmin = test.jimmAdmin

			err = j.UpdateMigratedModel(ctx, user, test.model, test.targetController)
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
- username: alice@canonical.com
  display-name: Alice
  controller-access: superuser
- username: bob@canonical.com
  display-name: Bob
  controller-access: login
`

func TestGetControllerAccess(t *testing.T) {
	c := qt.New(t)

	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	j := &jimm.JIMM{
		UUID: uuid.NewString(),
		Database: db.Database{
			DB: jimmtest.PostgresDB(c, nil),
		},
		OpenFGAClient: client,
	}
	ctx := context.Background()
	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, testGetControllerAccessEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	alice := openfga.NewUser(&dbUser, client)
	alice.JimmAdmin = true

	access, err := j.GetJimmControllerAccess(ctx, alice, names.NewUserTag("alice@canonical.com"))
	c.Assert(err, qt.IsNil)
	c.Check(access, qt.Equals, "superuser")

	access, err = j.GetJimmControllerAccess(ctx, alice, names.NewUserTag("bob@canonical.com"))
	c.Assert(err, qt.IsNil)
	c.Check(access, qt.Equals, "login")

	access, err = j.GetJimmControllerAccess(ctx, alice, names.NewUserTag("charlie@canonical.com"))
	c.Assert(err, qt.IsNil)
	c.Check(access, qt.Equals, "login")

	dbUser = env.User("bob@canonical.com").DBObject(c, j.Database)
	alice = openfga.NewUser(&dbUser, client)
	access, err = j.GetJimmControllerAccess(ctx, alice, names.NewUserTag("bob@canonical.com"))
	c.Assert(err, qt.IsNil)
	c.Check(access, qt.Equals, "login")

	_, err = j.GetJimmControllerAccess(ctx, alice, names.NewUserTag("alice@canonical.com"))
	c.Assert(err, qt.ErrorMatches, "unauthorized")
}

const testInitiateMigrationEnv = `clouds:
- name: test-cloud
  type: test
  regions:
  - name: test-region-1
cloud-credentials:
- name: test-cred
  cloud: test-cloud
  owner: alice@canonical.com
  type: empty
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-region-1
  agent-version: 3.3
- name: controller-2
  uuid: 00000001-0000-0000-0000-000000000002
  cloud: test-cloud
  region: test-region-2
  agent-version: 3.3
models:
- name: model-1
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000003
  controller: controller-1
  default-series: mantic
  cloud: test-cloud
  region: test-region-1
  cloud-credential: test-cred
  owner: alice@canonical.com
  life: alive
  status:
    status: available
    info: "OK!"
    since: 2020-02-20T20:02:20Z
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: write
  - user: charlie@canonical.com
    access: read
  sla:
    level: unsupported
  agent-version: 3.3
- name: model-2
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000004
  controller: controller-2
  default-series: mantic
  cloud: test-cloud
  region: test-region-1
  cloud-credential: test-cred
  owner: alice@canonical.com
  life: alive
  status:
    status: available
    info: "OK!"
    since: 2020-02-20T20:02:20Z
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: write
  - user: charlie@canonical.com
    access: read
  sla:
    level: unsupported
  agent-version: 3.3
`

func TestInitiateMigration(t *testing.T) {
	c := qt.New(t)

	mt1 := names.NewModelTag("00000002-0000-0000-0000-000000000003")
	// mt2 := names.NewModelTag("00000002-0000-0000-0000-000000000004")

	migrationId1 := uuid.New().String()

	m, err := macaroon.New([]byte("root key"), []byte("id"), "", macaroon.V2)
	c.Assert(err, qt.IsNil)

	macaroonData, err := json.Marshal([]macaroon.Slice{[]*macaroon.Macaroon{m}})
	c.Assert(err, qt.IsNil)

	tests := []struct {
		about                    string
		initiateMigrationResults []result
		user                     func(*openfga.OFGAClient) *openfga.User
		spec                     jujuparams.MigrationSpec
		expectedError            string
		expectedResult           jujuparams.InitiateMigrationResult
	}{{
		about: "model migration initiated successfully",
		user: func(client *openfga.OFGAClient) *openfga.User {
			u, err := dbmodel.NewIdentity("alice@canonical.com")
			c.Assert(err, qt.IsNil)
			return openfga.NewUser(
				u,
				client,
			)
		},
		spec: jujuparams.MigrationSpec{
			ModelTag: mt1.String(),
			TargetInfo: jujuparams.MigrationTargetInfo{
				ControllerTag: names.NewControllerTag(uuid.NewString()).String(),
				AuthTag:       names.NewUserTag("target-user@canonical.com").String(),
				Macaroons:     string(macaroonData),
			},
		},
		initiateMigrationResults: []result{{
			result: migrationId1,
		}},
		expectedResult: jujuparams.InitiateMigrationResult{
			ModelTag:    mt1.String(),
			MigrationId: migrationId1,
		},
	}, {
		about: "model not found",
		user: func(client *openfga.OFGAClient) *openfga.User {
			u, err := dbmodel.NewIdentity("alice@canonical.com")
			c.Assert(err, qt.IsNil)
			return openfga.NewUser(
				u,
				client,
			)
		},
		spec: jujuparams.MigrationSpec{
			ModelTag: names.NewModelTag(uuid.NewString()).String(),
			TargetInfo: jujuparams.MigrationTargetInfo{
				ControllerTag: names.NewControllerTag(uuid.NewString()).String(),
				AuthTag:       names.NewUserTag("target-user@canonical.com").String(),
				Macaroons:     string(macaroonData),
			},
		},
		initiateMigrationResults: []result{{}},
		expectedError:            "unauthorized access",
	}, {
		about: "InitiateMigration call fails",
		user: func(client *openfga.OFGAClient) *openfga.User {
			u, err := dbmodel.NewIdentity("alice@canonical.com")
			c.Assert(err, qt.IsNil)
			return openfga.NewUser(
				u,
				client,
			)
		},
		spec: jujuparams.MigrationSpec{
			ModelTag: mt1.String(),
			TargetInfo: jujuparams.MigrationTargetInfo{
				ControllerTag: names.NewControllerTag(uuid.NewString()).String(),
				AuthTag:       names.NewUserTag("target-user@canonical.com").String(),
			},
		},
		initiateMigrationResults: []result{{
			err: errors.E("mocked error"),
		}},
		expectedError: "mocked error",
	}, {
		about: "non-admin-user gets unauthorized error",
		user: func(client *openfga.OFGAClient) *openfga.User {
			u, err := dbmodel.NewIdentity("bob@canonical.com")
			c.Assert(err, qt.IsNil)
			return openfga.NewUser(
				u,
				client,
			)
		},
		spec: jujuparams.MigrationSpec{
			ModelTag: mt1.String(),
			TargetInfo: jujuparams.MigrationTargetInfo{
				ControllerTag: names.NewControllerTag(uuid.NewString()).String(),
				AuthTag:       names.NewUserTag("target-user@canonical.com").String(),
			},
		},
		initiateMigrationResults: []result{{}},
		expectedError:            "unauthorized access",
	}, {
		about: "invalid model tag",
		user: func(client *openfga.OFGAClient) *openfga.User {
			u, err := dbmodel.NewIdentity("alice@canonical.com")
			c.Assert(err, qt.IsNil)
			return openfga.NewUser(
				u,
				client,
			)
		},
		spec: jujuparams.MigrationSpec{
			ModelTag: "invalid-model-tag",
			TargetInfo: jujuparams.MigrationTargetInfo{
				ControllerTag: names.NewControllerTag(uuid.NewString()).String(),
				AuthTag:       names.NewUserTag("target-user@canonical.com").String(),
			},
		},
		initiateMigrationResults: []result{{}},
		expectedError:            `"invalid-model-tag" is not a valid tag`,
	}, {
		about: "invalid target controller tag",
		user: func(client *openfga.OFGAClient) *openfga.User {
			u, err := dbmodel.NewIdentity("alice@canonical.com")
			c.Assert(err, qt.IsNil)
			return openfga.NewUser(
				u,
				client,
			)
		},
		spec: jujuparams.MigrationSpec{
			ModelTag: mt1.String(),
			TargetInfo: jujuparams.MigrationTargetInfo{
				ControllerTag: "invalid-controller-tag",
				AuthTag:       names.NewUserTag("target-user@canonical.com").String(),
			},
		},
		initiateMigrationResults: []result{{}},
		expectedError:            `"invalid-controller-tag" is not a valid tag`,
	}, {
		about: "invalid target user tag",
		user: func(client *openfga.OFGAClient) *openfga.User {
			u, err := dbmodel.NewIdentity("alice@canonical.com")
			c.Assert(err, qt.IsNil)
			return openfga.NewUser(
				u,
				client,
			)
		},
		spec: jujuparams.MigrationSpec{
			ModelTag: mt1.String(),
			TargetInfo: jujuparams.MigrationTargetInfo{
				ControllerTag: names.NewControllerTag(uuid.NewString()).String(),
				AuthTag:       "invalid-user-tag",
			},
		},
		initiateMigrationResults: []result{{}},
		expectedError:            `"invalid-user-tag" is not a valid tag`,
	}, {
		about: "invalid macaroon data",
		user: func(client *openfga.OFGAClient) *openfga.User {
			u, err := dbmodel.NewIdentity("alice@canonical.com")
			c.Assert(err, qt.IsNil)
			return openfga.NewUser(
				u,
				client,
			)
		},
		spec: jujuparams.MigrationSpec{
			ModelTag: mt1.String(),
			TargetInfo: jujuparams.MigrationTargetInfo{
				ControllerTag: names.NewControllerTag(uuid.NewString()).String(),
				AuthTag:       names.NewUserTag("target-user@canonical.com").String(),
				Macaroons:     "invalid-macaroon-data",
			},
		},
		initiateMigrationResults: []result{{}},
		expectedError:            "failed to unmarshal macaroons",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
			c.Assert(err, qt.IsNil)

			j := &jimm.JIMM{
				UUID: uuid.NewString(),
				Database: db.Database{
					DB: jimmtest.PostgresDB(c, nil),
				},
				OpenFGAClient: client,
				Dialer:        &testDialer{},
			}

			ctx := context.Background()
			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, testInitiateMigrationEnv)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

			c.Patch(jimm.NewControllerClient, func(api base.APICallCloser) jimm.ControllerClient {
				return &testControllerClient{
					initiateMigrationResults: test.initiateMigrationResults,
				}
			})

			user := test.user(client)

			result, err := j.InitiateMigration(context.Background(), user, test.spec)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(result, qt.DeepEquals, test.expectedResult)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

type testDialer struct{}

func (d *testDialer) Dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag, requiredPermissions map[string]string) (jimm.API, error) {
	return (jimm.API)(nil), nil
}

type result struct {
	err    error
	result any
}

type testControllerClient struct {
	mu                       sync.Mutex
	initiateMigrationResults []result
}

func (c *testControllerClient) InitiateMigration(spec controller.MigrationSpec) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.initiateMigrationResults) == 0 {
		return "", errors.E(errors.CodeNotImplemented)
	}
	var result result
	result, c.initiateMigrationResults = c.initiateMigrationResults[0], c.initiateMigrationResults[1:]
	if result.err != nil {
		return "", result.err
	}
	return result.result.(string), nil
}

func (c *testControllerClient) Close() error {
	return nil
}
