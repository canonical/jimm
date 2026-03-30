// Copyright 2026 Canonical.

package juju_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"

	"github.com/canonical/ofga"
	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/controller"
	jujucloud "github.com/juju/juju/cloud"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/cloudspec"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"
	semversion "github.com/juju/version/v2"
	"gopkg.in/macaroon.v2"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuclient"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/vault"
)

func TestAddController(t *testing.T) {
	c := qt.New(t)

	api := &jimmtest.API{
		Clouds_: func() (map[names.CloudTag]jujucloud.Cloud, error) {
			clouds := map[names.CloudTag]jujucloud.Cloud{
				names.NewCloudTag("aws"): {
					Type:             "ec2",
					AuthTypes:        jujucloud.AuthTypes{jujucloud.UserPassAuthType},
					Endpoint:         "https://example.com",
					IdentityEndpoint: "https://identity.example.com",
					StorageEndpoint:  "https://storage.example.com",
					Regions: []jujucloud.Region{{
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
					RegionConfig: jujucloud.RegionConfig{
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
					AuthTypes: jujucloud.AuthTypes{jujucloud.UserPassAuthType},
					Endpoint:  "https://k8s.example.com",
					Regions: []jujucloud.Region{{
						Name: "default",
					}},
				},
			}
			return clouds, nil
		},
		CloudSpec_: func(ctx context.Context) (cloudspec.CloudSpec, error) {
			cs := cloudspec.CloudSpec{}
			cs.Name = "aws"
			cs.Type = "iaas"
			cs.Region = "eu-west-1"
			return cs, nil
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	ctx := context.Background()

	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)

	alice := openfga.NewUser(u, j.OpenFGAClient)
	err = alice.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	ctl1 := dbmodel.Controller{
		Name:          "test-controller",
		PublicAddress: "example.com:443",
	}
	ctlCreds := juju.ControllerCreds{
		AdminIdentityName: "user",
		AdminPassword:     "secret",
	}
	err = j.AddController(context.Background(), alice, &ctl1, ctlCreds)
	c.Assert(err, qt.IsNil)

	ctl2 := dbmodel.Controller{
		Name: "test-controller",
	}
	err = j.Database.GetController(ctx, &ctl2)
	c.Assert(err, qt.IsNil)
	c.Check(ctl2, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(dbmodel.CloudRegion{})), ctl1)

	ctl3 := dbmodel.Controller{
		Name:          "test-controller-2",
		PublicAddress: "example.com:443",
	}
	err = j.AddController(context.Background(), alice, &ctl3, ctlCreds)
	c.Assert(err, qt.IsNil)

	ctl4 := dbmodel.Controller{
		Name: "test-controller-2",
	}
	err = j.Database.GetController(ctx, &ctl4)
	c.Assert(err, qt.IsNil)
	c.Check(ctl4, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(dbmodel.CloudRegion{})), ctl3)
}

func TestAddControllerWithCloudWithoutRegions(t *testing.T) {
	c := qt.New(t)

	api := &jimmtest.API{
		Clouds_: func() (map[names.CloudTag]jujucloud.Cloud, error) {
			clouds := map[names.CloudTag]jujucloud.Cloud{
				names.NewCloudTag("k8s"): {
					Type:      "kubernetes",
					AuthTypes: jujucloud.AuthTypes{jujucloud.UserPassAuthType},
					Endpoint:  "https://k8s.example.com",
				},
			}
			return clouds, nil
		},
		CloudSpec_: func(ctx context.Context) (cloudspec.CloudSpec, error) {
			cs := cloudspec.CloudSpec{}
			cs.Name = "k8s"
			cs.Type = "iaas"
			return cs, nil
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	ctx := context.Background()

	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)

	alice := openfga.NewUser(u, j.OpenFGAClient)
	alice.JimmAdmin = true
	err = alice.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	ctl1 := dbmodel.Controller{
		Name:          "test-controller",
		PublicAddress: "example.com:443",
	}
	ctlCreds := juju.ControllerCreds{
		AdminIdentityName: "user",
		AdminPassword:     "secret",
	}
	err = j.AddController(context.Background(), alice, &ctl1, ctlCreds)
	c.Assert(err, qt.IsNil)

	ctl2 := dbmodel.Controller{
		Name: "test-controller",
	}
	err = j.Database.GetController(ctx, &ctl2)
	c.Assert(err, qt.IsNil)
	c.Check(ctl2, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(dbmodel.CloudRegion{})), ctl1)
	c.Check(ctl2.CloudRegion, qt.Equals, "default")

	cloud := dbmodel.Cloud{
		Name: "k8s",
	}
	err = j.Database.GetCloud(ctx, &cloud)
	c.Assert(err, qt.IsNil)
	c.Assert(cloud.Regions, qt.HasLen, 1)
	c.Assert(cloud.Regions[0].Name, qt.Equals, "default")
	c.Assert(cloud.Regions[0].Controllers, qt.HasLen, 1)
	c.Assert(cloud.Regions[0].Controllers[0].Controller.Name, qt.Equals, ctl1.Name)
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

	api := &jimmtest.API{
		Clouds_: func() (map[names.CloudTag]jujucloud.Cloud, error) {
			clouds := map[names.CloudTag]jujucloud.Cloud{
				names.NewCloudTag("aws"): {
					Type:             "ec2",
					AuthTypes:        jujucloud.AuthTypes{jujucloud.UserPassAuthType},
					Endpoint:         "https://example.com",
					IdentityEndpoint: "https://identity.example.com",
					StorageEndpoint:  "https://storage.example.com",
					Regions: []jujucloud.Region{{
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
					RegionConfig: jujucloud.RegionConfig{
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
					AuthTypes: jujucloud.AuthTypes{jujucloud.UserPassAuthType},
					Endpoint:  "https://k8s.example.com",
					Regions: []jujucloud.Region{{
						Name: "default",
					}},
				},
			}
			return clouds, nil
		},
		CloudSpec_: func(ctx context.Context) (cloudspec.CloudSpec, error) {
			cs := cloudspec.CloudSpec{}
			cs.Name = "aws"
			cs.Type = "iaas"
			cs.Region = "eu-west-1"
			return cs, nil
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
		CredentialStore: store,
	})

	ctx := context.Background()

	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)

	alice := openfga.NewUser(u, j.OpenFGAClient)
	alice.JimmAdmin = true

	err = alice.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	ctl1 := dbmodel.Controller{
		Name:          "test-controller",
		PublicAddress: "example.com:443",
	}
	ctlCreds := juju.ControllerCreds{
		AdminIdentityName: "admin",
		AdminPassword:     "secret",
	}
	err = j.AddController(context.Background(), alice, &ctl1, ctlCreds)
	c.Assert(err, qt.IsNil)

	ctl2 := dbmodel.Controller{
		Name: "test-controller",
	}
	err = j.Database.GetController(ctx, &ctl2)
	c.Assert(err, qt.IsNil)
	c.Check(ctl2, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(dbmodel.CloudRegion{})), ctl1)

	username, password, err := store.GetControllerCredentials(ctx, ctl1.Name)
	c.Assert(err, qt.IsNil)
	c.Assert(username, qt.Equals, "admin")
	c.Assert(password, qt.Equals, "secret")

	ctl3 := dbmodel.Controller{
		Name:          "test-controller-2",
		PublicAddress: "example.com:443",
	}
	err = j.AddController(context.Background(), alice, &ctl3, ctlCreds)
	c.Assert(err, qt.IsNil)

	ctl4 := dbmodel.Controller{
		Name: "test-controller-2",
	}
	err = j.Database.GetController(ctx, &ctl4)
	c.Assert(err, qt.IsNil)
	c.Check(ctl4, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(dbmodel.CloudRegion{})), ctl3)

	username, password, err = store.GetControllerCredentials(ctx, ctl4.Name)
	c.Assert(err, qt.IsNil)
	c.Assert(username, qt.Equals, "admin")
	c.Assert(password, qt.Equals, "secret")
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

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testEarliestControllerVersionEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	v, err := j.EarliestControllerVersion(ctx)
	c.Assert(err, qt.Equals, nil)
	c.Assert(v, qt.DeepEquals, semversion.MustParse("2.1.0"))
}

const testControllerConfigEnv = `clouds:
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
`

func TestControllerConfig(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	api := &jimmtest.API{
		ControllerConfig_: func(_ context.Context) (jujucontroller.Config, error) {
			return jujucontroller.Config(map[string]interface{}{
				"controller-uuid": "00000001-0000-0000-0000-000000000001",
			},
			), nil
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	env := jimmtest.ParseEnvironment(c, testControllerConfigEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)

	alice := openfga.NewUser(u, j.OpenFGAClient)
	alice.JimmAdmin = true

	err = alice.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	config, err := j.ControllerConfig(ctx, alice, env.Controllers[0].Name)
	c.Assert(err, qt.Equals, nil)
	c.Assert(config, qt.Not(qt.IsNil))
	c.Assert(config.SSHServerPort(), qt.Equals, 17022)

	_, err = j.ControllerConfig(ctx, alice, "not-found")
	c.Assert(err, qt.ErrorMatches, "controller not found")
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

	tests := []struct {
		about          string
		user           string
		controllerName string
		modelUUID      string
		modelInfo      func(model names.ModelTag) (jujuclient.ModelInfo, error)
		newOwner       string
		jimmAdmin      bool
		expectedModel  dbmodel.Model
		expectedError  string
		offers         []jujuparams.ApplicationOfferAdminDetailsV5
	}{{
		about:          "model imported",
		user:           "alice@canonical.com",
		controllerName: "test-controller",
		newOwner:       "",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		jimmAdmin:      true,
		modelInfo: func(model names.ModelTag) (jujuclient.ModelInfo, error) {
			info := jujuclient.ModelInfo{}
			info.Name = "test-model"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.Cloud = "test-cloud"
			info.CloudRegion = "test-region"
			info.CloudCredential = "test-cloud/alice@canonical.com/test-credential"
			info.Owner = "alice@canonical.com"
			info.Life = life.Alive
			info.Status = base.Status{
				Status: status.Status("ok"),
				Info:   "test-info",
				Since:  &now,
			}
			info.Users = []base.UserInfo{{
				UserName: "alice@canonical.com",
				Access:   string(string(jujuparams.ModelAdminAccess)),
			}, {
				UserName: "bob@canonical.com",
				Access:   string(string(jujuparams.ModelReadAccess)),
			}}
			info.Machines = []base.Machine{{
				Id:          "test-machine",
				DisplayName: "Test machine",
				Status:      "test-status",
				Message:     "test-message",
			}}
			info.AgentVersion = newVersion("2.1.0")
			return info, nil
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
			Life: state.Alive.String(),
		},
	}, {
		about:          "model with default region imported",
		user:           "alice@canonical.com",
		controllerName: "test-controller",
		newOwner:       "",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		jimmAdmin:      true,
		modelInfo: func(model names.ModelTag) (jujuclient.ModelInfo, error) {
			info := jujuclient.ModelInfo{}
			info.Name = "test-model"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.Cloud = "test-cloud"
			// info.CloudRegion is not set to test default region handling
			info.CloudCredential = "test-cloud/alice@canonical.com/test-credential"
			info.Owner = "alice@canonical.com"
			info.Life = life.Alive
			info.Status = base.Status{
				Status: status.Status("ok"),
				Info:   "test-info",
				Since:  &now,
			}
			info.Users = []base.UserInfo{{
				UserName: "alice@canonical.com",
				Access:   string(jujuparams.ModelAdminAccess),
			}, {
				UserName: "bob@canonical.com",
				Access:   string(jujuparams.ModelReadAccess),
			}}
			info.Machines = []base.Machine{{
				Id:          "test-machine",
				DisplayName: "Test machine",
				Status:      "test-status",
				Message:     "test-message",
			}}
			info.AgentVersion = newVersion("2.1.0")
			return info, nil
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
			Life: state.Alive.String(),
		},
	}, {
		about:          "model from local user imported",
		user:           "alice@canonical.com",
		controllerName: "test-controller",
		newOwner:       "alice@canonical.com",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		jimmAdmin:      true,
		modelInfo: func(model names.ModelTag) (jujuclient.ModelInfo, error) {
			info := jujuclient.ModelInfo{}
			info.Name = "test-model"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.Cloud = "test-cloud"
			info.CloudRegion = "test-region"
			info.CloudCredential = "test-cloud/local-user/test-credential"
			info.Owner = "local-user"
			info.Life = life.Alive
			info.Status = base.Status{
				Status: status.Status("available"),
				Info:   "test-info",
				Since:  &now,
			}
			info.Users = []base.UserInfo{{
				UserName: "local-user",
				Access:   string(jujuparams.ModelAdminAccess),
			}, {
				UserName: "another-user",
				Access:   string(jujuparams.ModelReadAccess),
			}}
			info.Machines = []base.Machine{{
				Id:          "test-machine",
				DisplayName: "Test machine",
				Status:      "test-status",
				Message:     "test-message",
			}}
			info.AgentVersion = newVersion("2.1.0")
			return info, nil
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
			Life: state.Alive.String(),
		},
	}, {
		about:          "new model owner is local user",
		user:           "alice@canonical.com",
		controllerName: "test-controller",
		newOwner:       "bob",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		expectedError:  "cannot import model from local user, try --owner to switch the model owner",
		jimmAdmin:      true,
		modelInfo: func(model names.ModelTag) (jujuclient.ModelInfo, error) {
			info := jujuclient.ModelInfo{}
			info.Name = "test-model"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.Cloud = "test-cloud"
			info.CloudRegion = "test-region"
			info.CloudCredential = "test-cloud/local-user/test-credential"
			info.Owner = "local-user"
			info.Life = life.Alive
			info.Status = base.Status{
				Status: status.Status("available"),
				Info:   "test-info",
				Since:  &now,
			}
			info.Users = []base.UserInfo{{
				UserName: "local-user",
				Access:   string(jujuparams.ModelAdminAccess),
			}, {
				UserName: "another-user",
				Access:   string(jujuparams.ModelReadAccess),
			}}
			info.Machines = []base.Machine{{
				Id:          "test-machine",
				DisplayName: "Test machine",
				Status:      "test-status",
				Message:     "test-message",
			}}
			info.AgentVersion = newVersion("2.1.0")
			return info, nil
		},
	}, {
		about:          "model not found",
		user:           "alice@canonical.com",
		controllerName: "test-controller",
		newOwner:       "",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		jimmAdmin:      true,
		modelInfo: func(model names.ModelTag) (jujuclient.ModelInfo, error) {
			return jujuclient.ModelInfo{}, errors.Codef(errors.CodeNotFound, "model not found")
		},
		expectedError: "model not found",
	}, {
		about:          "fail import from local user without newOwner flag",
		user:           "alice@canonical.com",
		controllerName: "test-controller",
		newOwner:       "",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		jimmAdmin:      true,
		modelInfo: func(model names.ModelTag) (jujuclient.ModelInfo, error) {
			info := jujuclient.ModelInfo{}
			info.Name = "test-model"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.Cloud = "test-cloud"
			info.CloudRegion = "test-region"
			info.CloudCredential = "test-cloud/alice@canonical.com/unknown-credential"
			info.Owner = "local-user"
			return info, nil
		},
		expectedError: `cannot import model from local user, try --owner to switch the model owner`,
	}, {
		about:          "cloud credentials not found",
		user:           "alice@canonical.com",
		controllerName: "test-controller",
		newOwner:       "",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		jimmAdmin:      true,
		modelInfo: func(model names.ModelTag) (jujuclient.ModelInfo, error) {
			info := jujuclient.ModelInfo{}
			info.Name = "test-model"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.Cloud = "invalid-cloud"
			info.CloudRegion = "test-region"
			info.CloudCredential = "invalid-cloud/alice@canonical.com/unknown-credential"
			info.Owner = "alice@canonical.com"
			return info, nil
		},
		expectedError: `Failed to find cloud credential for user alice@canonical.com on cloud invalid-cloud`,
	}, {
		about:          "cloud region not found",
		user:           "alice@canonical.com",
		controllerName: "test-controller",
		newOwner:       "",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		jimmAdmin:      true,
		modelInfo: func(model names.ModelTag) (jujuclient.ModelInfo, error) {
			info := jujuclient.ModelInfo{}
			info.Name = "test-model"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.Cloud = "test-cloud"
			info.CloudRegion = "unknown-region"
			info.CloudCredential = "test-cloud/alice@canonical.com/test-credential"
			info.Owner = "alice@canonical.com"
			return info, nil
		},
		expectedError: `cloud region not found`,
	}, {
		about:          "not allowed if not superuser",
		user:           "bob@canonical.com",
		controllerName: "test-controller",
		newOwner:       "",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		jimmAdmin:      false,
		modelInfo: func(model names.ModelTag) (jujuclient.ModelInfo, error) {
			info := jujuclient.ModelInfo{}
			info.Name = "test-model"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.Cloud = "test-cloud"
			info.CloudRegion = "test-region"
			info.CloudCredential = "test-cloud/alice@canonical.com/test-credential"
			info.Owner = "alice@canonical.com"
			return info, nil
		},
		expectedError: `unauthorized`,
	}, {
		about:          "model already exists",
		user:           "alice@canonical.com",
		controllerName: "test-controller",
		newOwner:       "",
		modelUUID:      "00000002-0000-0000-0000-000000000002",
		jimmAdmin:      true,
		modelInfo: func(model names.ModelTag) (jujuclient.ModelInfo, error) {
			info := jujuclient.ModelInfo{}
			info.Name = "model-1"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.Cloud = "test-cloud"
			info.CloudRegion = "test-region"
			info.CloudCredential = "test-cloud/alice@canonical.com/test-credential"
			info.Owner = "alice@canonical.com"
			return info, nil
		},
		expectedError: `model (.*) already exists`,
	}, {
		about:          "import model with offers",
		user:           "alice@canonical.com",
		controllerName: "test-controller",
		newOwner:       "",
		modelUUID:      "00000002-0000-0000-0000-000000000001",
		jimmAdmin:      true,
		modelInfo: func(model names.ModelTag) (jujuclient.ModelInfo, error) {
			info := jujuclient.ModelInfo{}
			info.Name = "test-model"
			info.Type = "test-type"
			info.UUID = "00000002-0000-0000-0000-000000000001"
			info.ControllerUUID = "00000001-0000-0000-0000-000000000001"
			info.DefaultSeries = "test-series"
			info.Cloud = "test-cloud"
			info.CloudRegion = "test-region"
			info.CloudCredential = "test-cloud/alice@canonical.com/test-credential"
			info.Owner = "alice@canonical.com"
			info.Life = life.Alive
			info.Status = base.Status{
				Status: status.Status("ok"),
				Info:   "test-info",
				Since:  &now,
			}
			info.AgentVersion = newVersion("2.1.0")
			return info, nil
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
			Life: state.Alive.String(),
			Offers: []dbmodel.ApplicationOffer{
				{
					URL:  "url1",
					UUID: "00000001-0000-0000-0000-000000000001",
					Name: "offer1",
				},
				{
					URL:  "url2",
					UUID: "00000001-0000-0000-0000-000000000002",
					Name: "offer2",
				},
			},
		},
		offers: []jujuparams.ApplicationOfferAdminDetailsV5{{
			ApplicationOfferDetailsV5: jujuparams.ApplicationOfferDetailsV5{
				OfferUUID: "00000001-0000-0000-0000-000000000001",
				OfferName: "offer1",
				OfferURL:  "url1",
			},
			ApplicationName: "app1",
		}, {
			ApplicationOfferDetailsV5: jujuparams.ApplicationOfferDetailsV5{
				OfferUUID: "00000001-0000-0000-0000-000000000002",
				OfferName: "offer2",
				OfferURL:  "url2",
			},
			ApplicationName: "app2",
		}},
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			api := &jimmtest.API{
				ModelInfo_: func(ctx context.Context, model names.ModelTag) (jujuclient.ModelInfo, error) {
					return test.modelInfo(model)
				},
				ListApplicationOffers_: func(ctx context.Context, of []crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error) {
					offers := make([]*crossmodel.ApplicationOfferDetails, len(test.offers))
					for i, offer := range test.offers {
						offers[i] = &crossmodel.ApplicationOfferDetails{
							OfferUUID:              offer.OfferUUID,
							OfferName:              offer.OfferName,
							OfferURL:               offer.OfferURL,
							ApplicationName:        offer.ApplicationName,
							ApplicationDescription: offer.ApplicationDescription,
						}
					}
					return offers, nil
				},
				GetApplicationOfferConsumeDetails_: func(ctx context.Context, url string) (jujuparams.ConsumeOfferDetails, error) {
					for _, offer := range test.offers {
						if offer.OfferURL == url {
							return jujuparams.ConsumeOfferDetails{
								Offer: &jujuparams.ApplicationOfferDetailsV5{
									OfferUUID: offer.OfferUUID,
									OfferURL:  offer.OfferURL,
									OfferName: offer.OfferName,
								},
							}, nil
						}
					}
					return jujuparams.ConsumeOfferDetails{}, errors.Codef(errors.CodeNotFound, "not found")
				},
			}

			j := newTestJujuManager(c, &parameters{
				Dialer: &jimmtest.Dialer{
					API:  api,
					UUID: test.expectedModel.Controller.UUID,
				},
			})

			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, testImportModelEnv)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

			dbUser := env.User(test.user).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, j.OpenFGAClient)
			user.JimmAdmin = test.jimmAdmin

			err := j.ImportModel(ctx, user, test.controllerName, names.NewModelTag(test.modelUUID), test.newOwner)
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
				ok, err := j.OpenFGAClient.CheckRelation(ctx, controllerPermissionCheck, false)
				c.Assert(err, qt.IsNil)
				c.Assert(ok, qt.IsTrue)

				for _, offer := range test.expectedModel.Offers {
					offerPermissionCheck := ofga.Tuple{
						Object:   ofganames.ConvertTag(names.NewUserTag(test.user)),
						Relation: ofganames.AdministratorRelation,
						Target:   ofganames.ConvertTag(names.NewApplicationOfferTag(offer.UUID)),
					}
					ok, err := j.OpenFGAClient.CheckRelation(ctx, offerPermissionCheck, false)
					c.Assert(err, qt.IsNil)
					c.Assert(ok, qt.IsTrue)
				}
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
		modelInfo        func(model names.ModelTag) (jujuclient.ModelInfo, error)
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
		modelInfo: func(model names.ModelTag) (jujuclient.ModelInfo, error) {
			return jujuclient.ModelInfo{}, errors.New("an error")
		},
		expectedError: "an error",
		jimmAdmin:     true,
	}, {
		about:            "all ok",
		user:             "alice@canonical.com",
		model:            names.NewModelTag("00000002-0000-0000-0000-000000000002"),
		targetController: "controller-2",
		modelInfo: func(model names.ModelTag) (jujuclient.ModelInfo, error) {
			return jujuclient.ModelInfo{}, nil
		},
		jimmAdmin: true,
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			j := newTestJujuManager(c, &parameters{
				Dialer: &jimmtest.Dialer{
					API: &jimmtest.API{
						ModelInfo_: func(ctx context.Context, model names.ModelTag) (jujuclient.ModelInfo, error) {
							return test.modelInfo(model)
						},
					},
				},
			})

			env := jimmtest.ParseEnvironment(c, testUpdateMigratedModelEnv)
			env.PopulateDB(c, j.Database)

			dbUser := env.User(test.user).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, nil)
			user.JimmAdmin = test.jimmAdmin

			ctx := context.Background()

			err := j.UpdateMigratedModel(ctx, user, test.model, test.targetController)
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
  uuid: 00000002-0000-0000-0000-000000000003
  controller: controller-1
  cloud: test-cloud
  region: test-region-1
  cloud-credential: test-cred
  owner: alice@canonical.com
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: write
  - user: charlie@canonical.com
    access: read
- name: model-2
  uuid: 00000002-0000-0000-0000-000000000004
  controller: controller-2
  cloud: test-cloud
  region: test-region-1
  cloud-credential: test-cred
  owner: alice@canonical.com
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
`

func TestInitiateMigration(t *testing.T) {
	c := qt.New(t)

	mt1 := names.NewModelTag("00000002-0000-0000-0000-000000000003")

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
		expectedError:            "unauthorized",
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
			err: errors.New("mocked error"),
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
		expectedError:            "unauthorized",
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
			j := newTestJujuManager(c, nil)

			env := jimmtest.ParseEnvironment(c, testInitiateMigrationEnv)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

			c.Patch(juju.NewControllerClient, func(api base.APICallCloser) juju.ControllerClient {
				return &testControllerClient{
					initiateMigrationResults: test.initiateMigrationResults,
				}
			})

			user := test.user(j.OpenFGAClient)

			result, err := j.InitiateMigration(context.Background(), user, test.spec)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(result, qt.DeepEquals, test.expectedResult)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
			model := dbmodel.Model{
				UUID: sql.NullString{String: mt1.Id(), Valid: true},
			}
			err = j.Database.GetModel(context.Background(), &model)
			c.Assert(err, qt.IsNil)
			if test.expectedError == "" {
				c.Assert(model.MigrationMode, qt.Equals, dbmodel.MigrationModeExporting)
			} else {
				c.Assert(model.MigrationMode, qt.Equals, dbmodel.MigrationModeNone)
			}
		})
	}
}

func TestInitiateMigration_InProgress(t *testing.T) {
	c := qt.New(t)

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testInitiateMigrationEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	c.Patch(juju.NewControllerClient, func(api base.APICallCloser) juju.ControllerClient {
		return &testControllerClient{
			initiateMigrationResults: []result{{
				result: "migration-result-id",
			}},
		}
	})

	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)
	user := openfga.NewUser(u, j.OpenFGAClient)
	user.JimmAdmin = true

	migrationSpec := jujuparams.MigrationSpec{
		ModelTag: names.NewModelTag("00000002-0000-0000-0000-000000000003").String(),
		TargetInfo: jujuparams.MigrationTargetInfo{
			ControllerTag: names.NewControllerTag(uuid.NewString()).String(),
			AuthTag:       names.NewUserTag("target-user@canonical.com").String(),
		},
	}
	_, err = j.InitiateMigration(context.Background(), user, migrationSpec)
	c.Assert(err, qt.IsNil)

	_, err = j.InitiateMigration(context.Background(), user, migrationSpec)
	c.Assert(err, qt.ErrorMatches, `failed to update the model's migration mode: model is already in migration mode "exporting"`)
}

type result struct {
	err    error
	result any
}

type testControllerClient struct {
	mu                       sync.Mutex
	initiateMigrationResults []result
}

func (c *testControllerClient) InitiateMigration(spec controller.MigrationSpec, dryRun bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.initiateMigrationResults) == 0 {
		return "", errors.Codef(errors.CodeNotImplemented, "not implemented")
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

const testControllerDetailsForModelEnv = `clouds:
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
  public-address: test-address.com
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000003
  controller: controller-1
  cloud: test-cloud
  region: test-region-1
  cloud-credential: test-cred
  owner: alice@canonical.com
`

func TestControllerDetailsForModel(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	invalidUUID := "invalid-uuid"
	validUUID := "00000002-0000-0000-0000-000000000003"

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, testControllerDetailsForModelEnv)
	env.PopulateDB(c, j.Database)

	// Expect a failure with an invalid UUID
	_, err := j.ControllerDetailsForModel(ctx, invalidUUID)
	c.Assert(err, qt.IsNotNil)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	// Expect a failure with a valid UUID but no credentials
	_, err = j.ControllerDetailsForModel(ctx, validUUID)
	c.Assert(err, qt.IsNotNil)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	// Set up credentials for the controller
	err = j.CredentialStore.PutControllerCredentials(ctx, "controller-1", "test-user", "test-password")
	c.Assert(err, qt.IsNil)

	// Expect a successful retrieval of controller details with valid UUID and credentials
	controllerDetails, err := j.ControllerDetailsForModel(ctx, validUUID)
	c.Assert(err, qt.IsNil)
	c.Assert(controllerDetails.PublicAddress, qt.Equals, "test-address.com")
	c.Assert(controllerDetails.Credentials.AdminIdentityName, qt.Equals, "test-user")
	c.Assert(controllerDetails.Credentials.AdminPassword, qt.Equals, "test-password")
}
