// Copyright 2020 Canonical Ltd.

package jimm_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/apiserver/params"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"
	"sigs.k8s.io/yaml"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
)

func TestModelCreateArgs(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		about         string
		args          jujuparams.ModelCreateArgs
		expectedArgs  jimm.ModelCreateArgs
		expectedError string
	}{{
		about: "all ok",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           names.NewUserTag("alice@external").String(),
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1").String(),
		},
		expectedArgs: jimm.ModelCreateArgs{
			Name:            "test-model",
			Owner:           names.NewUserTag("alice@external"),
			Cloud:           names.NewCloudTag("test-cloud"),
			CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
		},
	}, {
		about: "name not specified",
		args: jujuparams.ModelCreateArgs{
			OwnerTag:           names.NewUserTag("alice@external").String(),
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
		},
		expectedError: "name not specified",
	}, {
		about: "invalid owner tag",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           "alice@external",
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
		},
		expectedError: "invalid owner tag",
	}, {
		about: "invalid cloud tag",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           names.NewUserTag("alice@external").String(),
			CloudTag:           "test-cloud",
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
		},
		expectedError: "invalid cloud tag",
	}, {
		about: "invalid cloud credential tag",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           names.NewUserTag("alice@external").String(),
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: "test-credential-1",
		},
		expectedError: "invalid cloud credential tag",
	}, {
		about: "cloud does not match cloud credential cloud",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           names.NewUserTag("alice@external").String(),
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: names.NewCloudCredentialTag("another-cloud/alice/test-credential-1").String(),
		},
		expectedError: "cloud credential cloud mismatch",
	}, {
		about: "owner tag not specified",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
		},
		expectedError: "owner tag not specified",
	}, {
		about: "cloud tag not specified",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           names.NewUserTag("alice@external").String(),
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
		},
		expectedError: "cloud tag not specified",
	}}

	opts := []cmp.Option{
		cmp.Comparer(func(t1, t2 names.UserTag) bool {
			return t1.String() == t2.String()
		}),
		cmp.Comparer(func(t1, t2 names.CloudTag) bool {
			return t1.String() == t2.String()
		}),
		cmp.Comparer(func(t1, t2 names.CloudCredentialTag) bool {
			return t1.String() == t2.String()
		}),
	}
	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			var a jimm.ModelCreateArgs
			err := a.FromJujuModelCreateArgs(&test.args)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(a, qt.CmpEquals(opts...), test.expectedArgs)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

var addModelTests = []struct {
	name                string
	env                 string
	updateCredential    func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error)
	grantJIMMModelAdmin func(context.Context, names.ModelTag) error
	createModel         func(ctx context.Context, args *jujuparams.ModelCreateArgs, mi *jujuparams.ModelInfo) error
	username            string
	args                jujuparams.ModelCreateArgs
	expectModel         dbmodel.Model
	expectError         string
}{{
	name: "CreateModelWithCloudRegion",
	env: `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
user-defaults:
- user: alice@external
  defaults:
    key4: value4
cloud-defaults:
- user: alice@external
  cloud: test-cloud
  region: test-region-1
  defaults:
    key1: value1
    key2: value2
- user: alice@external
  cloud: test-cloud
  defaults:
    key3: value3
cloud-credentials:
- name: test-credential-1
  owner: alice@external
  cloud: test-cloud
  auth-type: empty
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 2
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
		return nil, nil
	},
	grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
		return nil
	},
	createModel: assertConfig(map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
		"key4": "value4",
	}, createModel(`
uuid: 00000001-0000-0000-0000-0000-000000000001
status:
  status: started
  info: running a test
life: alive
users:
- user: alice@external
  access: admin
- user: bob
  access: read
machines:
- id: test-machine-id
  hardware:
    arch: amd64
    mem: 8096
    cores: 8
  display-name: a test machine
  status: running
  message: a test message
  has-vote: true
  wants-vote: false
`[1:])),
	username: "alice@external",
	args: jujuparams.ModelCreateArgs{
		Name:               "test-model",
		OwnerTag:           names.NewUserTag("alice@external").String(),
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-region-1",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1").String(),
	},
	expectModel: dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner: dbmodel.User{
			Username:         "alice@external",
			ControllerAccess: "add-model",
		},
		Controller: dbmodel.Controller{
			Name: "controller-2",
			UUID: "00000000-0000-0000-0000-0000-0000000000002",
		},
		CloudRegion: dbmodel.CloudRegion{
			Cloud: dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			},
			Name: "test-region-1",
		},
		CloudCredential: dbmodel.CloudCredential{
			Name:     "test-credential-1",
			AuthType: "empty",
		},
		Life: "alive",
		Status: dbmodel.Status{
			Status: "started",
			Info:   "running a test",
		},
		Machines: []dbmodel.Machine{{
			MachineID: "test-machine-id",
			Hardware: dbmodel.Hardware{
				Arch: sql.NullString{
					String: "amd64",
					Valid:  true,
				},
				Mem: dbmodel.NullUint64{
					Uint64: 8096,
					Valid:  true,
				},
				CPUCores: dbmodel.NullUint64{
					Uint64: 8,
					Valid:  true,
				},
			},
			DisplayName: "a test machine",
			InstanceStatus: dbmodel.Status{
				Status: "running",
				Info:   "a test message",
			},
			HasVote: true,
		}},
		Users: []dbmodel.UserModelAccess{{
			User: dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "add-model",
			},
			Access: "admin",
		}},
	},
}, {
	name: "CreateModelWithCloud",
	env: `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
cloud-credentials:
- name: test-credential-1
  owner: alice@external
  cloud: test-cloud
  auth-type: empty
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 2
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
		return nil, nil
	},
	grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
		return nil
	},
	createModel: createModel(`
uuid: 00000001-0000-0000-0000-0000-000000000001
status:
  status: started
  info: running a test
life: alive
users:
- user: alice@external
  access: admin
- user: bob
  access: read
machines:
- id: test-machine-id
  hardware:
    arch: amd64
    mem: 8096
    cores: 8
  display-name: a test machine
  status: running
  message: a test message
  has-vote: true
  wants-vote: false
`[1:]),
	username: "alice@external",
	args: jujuparams.ModelCreateArgs{
		Name:               "test-model",
		OwnerTag:           names.NewUserTag("alice@external").String(),
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-region-1",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1").String(),
	},
	expectModel: dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner: dbmodel.User{
			Username:         "alice@external",
			ControllerAccess: "add-model",
		},
		Controller: dbmodel.Controller{
			Name: "controller-2",
			UUID: "00000000-0000-0000-0000-0000-0000000000002",
		},
		CloudRegion: dbmodel.CloudRegion{
			Cloud: dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			},
			Name: "test-region-1",
		},
		CloudCredential: dbmodel.CloudCredential{
			Name:     "test-credential-1",
			AuthType: "empty",
		},
		Life: "alive",
		Status: dbmodel.Status{
			Status: "started",
			Info:   "running a test",
		},
		Machines: []dbmodel.Machine{{
			MachineID: "test-machine-id",
			Hardware: dbmodel.Hardware{
				Arch: sql.NullString{
					String: "amd64",
					Valid:  true,
				},
				Mem: dbmodel.NullUint64{
					Uint64: 8096,
					Valid:  true,
				},
				CPUCores: dbmodel.NullUint64{
					Uint64: 8,
					Valid:  true,
				},
			},
			DisplayName: "a test machine",
			InstanceStatus: dbmodel.Status{
				Status: "running",
				Info:   "a test message",
			},
			HasVote: true,
		}},
		Users: []dbmodel.UserModelAccess{{
			User: dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "add-model",
			},
			Access: "admin",
		}},
	},
}, {
	name: "CreateModelInOtherNamespaceAsSuperUser",
	env: `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
cloud-credentials:
- name: test-credential-1
  owner: alice@external
  cloud: test-cloud
  auth-type: empty
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 2
users:
- username: alice@external
  controller-access: superuser
- username: bob@external
  controller-access: add-model
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
		return nil, nil
	},
	grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
		return nil
	},
	createModel: createModel(`
uuid: 00000001-0000-0000-0000-0000-000000000001
status:
  status: started
  info: running a test
life: alive
users:
- user: alice@external
  access: admin
- user: bob
  access: read
machines:
- id: test-machine-id
  hardware:
    arch: amd64
    mem: 8096
    cores: 8
  display-name: a test machine
  status: running
  message: a test message
  has-vote: true
  wants-vote: false
`[1:]),
	username: "alice@external",
	args: jujuparams.ModelCreateArgs{
		Name:               "test-model",
		OwnerTag:           names.NewUserTag("bob@external").String(),
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-region-1",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1").String(),
	},
	expectModel: dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner: dbmodel.User{
			Username:         "bob@external",
			ControllerAccess: "add-model",
		},
		Controller: dbmodel.Controller{
			Name: "controller-2",
			UUID: "00000000-0000-0000-0000-0000-0000000000002",
		},
		CloudRegion: dbmodel.CloudRegion{
			Cloud: dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			},
			Name: "test-region-1",
		},
		CloudCredential: dbmodel.CloudCredential{
			Name:     "test-credential-1",
			AuthType: "empty",
		},
		Life: "alive",
		Status: dbmodel.Status{
			Status: "started",
			Info:   "running a test",
		},
		Machines: []dbmodel.Machine{{
			MachineID: "test-machine-id",
			Hardware: dbmodel.Hardware{
				Arch: sql.NullString{
					String: "amd64",
					Valid:  true,
				},
				Mem: dbmodel.NullUint64{
					Uint64: 8096,
					Valid:  true,
				},
				CPUCores: dbmodel.NullUint64{
					Uint64: 8,
					Valid:  true,
				},
			},
			DisplayName: "a test machine",
			InstanceStatus: dbmodel.Status{
				Status: "running",
				Info:   "a test message",
			},
			HasVote: true,
		}},
		Users: []dbmodel.UserModelAccess{{
			User: dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			},
			Access: "admin",
		}},
	},
}, {
	name: "CreateModelInOtherNamespace",
	env: `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
cloud-credentials:
- name: test-credential-1
  owner: alice@external
  cloud: test-cloud
  auth-type: empty
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 2
users:
- username: alice@external
  controller-access: add-model
- username: bob@external
  controller-access: add-model
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
		return nil, nil
	},
	grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
		return nil
	},
	createModel: createModel(`
uuid: 00000001-0000-0000-0000-0000-000000000001
status:
  status: started
  info: running a test
life: alive
users:
- user: alice@external
  access: admin
- user: bob
  access: read
machines:
- id: test-machine-id
  hardware:
    arch: amd64
    mem: 8096
    cores: 8
  display-name: a test machine
  status: running
  message: a test message
  has-vote: true
  wants-vote: false
`[1:]),
	username: "alice@external",
	args: jujuparams.ModelCreateArgs{
		Name:               "test-model",
		OwnerTag:           names.NewUserTag("bob@external").String(),
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-region-1",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1").String(),
	},
	expectError: "unauthorized access",
}, {
	name: "CreateModelError",
	env: `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
cloud-credentials:
- name: test-credential-1
  owner: alice@external
  cloud: test-cloud
  auth-type: empty
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 2
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
		return nil, nil
	},
	grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
		return nil
	},
	createModel: func(ctx context.Context, args *jujuparams.ModelCreateArgs, mi *jujuparams.ModelInfo) error {
		return errors.E("a test error")
	},
	username: "alice@external",
	args: jujuparams.ModelCreateArgs{
		Name:               "test-model",
		OwnerTag:           names.NewUserTag("alice@external").String(),
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-region-1",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1").String(),
	},
	expectError: "a test error",
}, {
	name: "ModelExists",
	env: `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
cloud-credentials:
- name: test-credential-1
  owner: alice@external
  cloud: test-cloud
  auth-type: empty
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-000000000001
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-000000000002
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 2
models:
- name: test-model
  owner: alice@external
  uuid: 00000001-0000-0000-0000-0000-000000000003
  cloud: test-cloud
  region: test-region-1
  cloud-credential: test-credential-1
  controller: controller-1
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
		return nil, nil
	},
	grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
		return nil
	},
	createModel: createModel(`
uuid: 00000001-0000-0000-0000-0000-000000000001
status:
  status: started
  info: running a test
life: alive
users:
- user: alice@external
  access: admin
- user: bob
  access: read
machines:
- id: test-machine-id
  hardware:
    arch: amd64
    mem: 8096
    cores: 8
  display-name: a test machine
  status: running
  message: a test message
  has-vote: true
  wants-vote: false
`[1:]),
	username: "alice@external",
	args: jujuparams.ModelCreateArgs{
		Name:               "test-model",
		OwnerTag:           names.NewUserTag("alice@external").String(),
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-region-1",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1").String(),
	},
	expectError: "model alice@external/test-model already exists",
}, {
	name: "UpdateCredentialError",
	env: `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
cloud-credentials:
- name: test-credential-1
  owner: alice@external
  cloud: test-cloud
  auth-type: empty
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 2
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
		return nil, errors.E("a silly error")
	},
	grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
		return nil
	},
	createModel: createModel(`
uuid: 00000001-0000-0000-0000-0000-000000000001
status:
  status: started
  info: running a test
life: alive
users:
- user: alice@external
  access: admin
- user: bob
  access: read
machines:
- id: test-machine-id
  hardware:
    arch: amd64
    mem: 8096
    cores: 8
  display-name: a test machine
  status: running
  message: a test message
  has-vote: true
  wants-vote: false
`[1:]),
	username: "alice@external",
	args: jujuparams.ModelCreateArgs{
		Name:               "test-model",
		OwnerTag:           names.NewUserTag("alice@external").String(),
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-region-1",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1").String(),
	},
	expectError: "failed to update cloud credential",
}}

func TestAddModel(t *testing.T) {
	c := qt.New(t)

	for _, test := range addModelTests {
		c.Run(test.name, func(c *qt.C) {
			api := &jimmtest.API{
				UpdateCredential_:    test.updateCredential,
				GrantJIMMModelAdmin_: test.grantJIMMModelAdmin,
				CreateModel_:         test.createModel,
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

			env := jimmtest.ParseEnvironment(c, test.env)
			env.PopulateDB(c, j.Database)

			u := env.User(test.username).DBObject(c, j.Database)
			args := jimm.ModelCreateArgs{}
			err = args.FromJujuModelCreateArgs(&test.args)
			c.Assert(err, qt.IsNil)

			_, err = j.AddModel(context.Background(), &u, &args)
			if test.expectError == "" {
				c.Assert(err, qt.IsNil)

				m1 := dbmodel.Model{
					UUID: test.expectModel.UUID,
				}
				err = j.Database.GetModel(ctx, &m1)
				c.Assert(err, qt.IsNil)
				c.Assert(m1, jimmtest.DBObjectEquals, test.expectModel)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectError)
			}
		})
	}
}

func createModel(template string) func(context.Context, *jujuparams.ModelCreateArgs, *jujuparams.ModelInfo) error {
	var tmi jujuparams.ModelInfo
	err := yaml.Unmarshal([]byte(template), &tmi)
	return func(_ context.Context, args *jujuparams.ModelCreateArgs, mi *jujuparams.ModelInfo) error {
		if err != nil {
			return err
		}
		*mi = tmi
		mi.Name = args.Name
		mi.CloudTag = args.CloudTag
		mi.CloudCredentialTag = args.CloudCredentialTag
		mi.CloudRegion = args.CloudRegion
		mi.OwnerTag = args.OwnerTag
		return nil
	}
}

func assertConfig(config map[string]interface{}, fnc func(context.Context, *jujuparams.ModelCreateArgs, *jujuparams.ModelInfo) error) func(context.Context, *jujuparams.ModelCreateArgs, *jujuparams.ModelInfo) error {
	return func(ctx context.Context, args *jujuparams.ModelCreateArgs, mi *jujuparams.ModelInfo) error {
		if len(config) != len(args.Config) {
			return errors.E(fmt.Sprintf("expected %d config settings, got %d", len(config), len(args.Config)))
		}
		for k, v := range args.Config {
			if config[k] != v {
				return errors.E(fmt.Sprintf("config value mismatch for key %s", k))
			}
		}
		return fnc(ctx, args, mi)
	}

}

const modelInfoTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@external
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
models:
- name: model-1
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
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
  machines:
  - id: 0
    hardware:
      arch: amd64
      mem: 8096
      root-disk: 10240
      cores: 1
    instance-id: 00000009-0000-0000-0000-0000000000000
    display-name: Machine 0
    status: available
    message: OK!
    has-vote: true
    wants-vote: false
    ha-primary: false
  - id: 1
    hardware:
      arch: amd64
      mem: 8096
      root-disk: 10240
      cores: 2
    instance-id: 00000009-0000-0000-0000-0000000000001
    display-name: Machine 1
    status: available
    message: OK!
    has-vote: true
    wants-vote: false
    ha-primary: false
  sla:
    level: unsupported
  agent-version: 1.2.3
`

var modelInfoTests = []struct {
	name            string
	env             string
	username        string
	uuid            string
	expectModelInfo *jujuparams.ModelInfo
	expectError     string
}{{
	name:     "AdminUser",
	env:      modelInfoTestEnv,
	username: "alice@external",
	uuid:     "00000002-0000-0000-0000-000000000001",
	expectModelInfo: &jujuparams.ModelInfo{
		Name:               "model-1",
		Type:               "iaas",
		UUID:               "00000002-0000-0000-0000-000000000001",
		ControllerUUID:     "00000001-0000-0000-0000-000000000001",
		ProviderType:       "test-provider",
		DefaultSeries:      "warty",
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-cloud-region",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/cred-1").String(),
		OwnerTag:           names.NewUserTag("alice@external").String(),
		Life:               "alive",
		Status: jujuparams.EntityStatus{
			Status: "available",
			Info:   "OK!",
			Since:  newDate(2020, 2, 20, 20, 2, 20, 0, time.UTC),
		},
		Users: []jujuparams.ModelUserInfo{{
			UserName: "alice@external",
			Access:   "admin",
		}, {
			UserName: "bob@external",
			Access:   "write",
		}, {
			UserName: "charlie@external",
			Access:   "read",
		}},
		Machines: []jujuparams.ModelMachineInfo{{
			Id:          "0",
			Hardware:    jimmtest.ParseMachineHardware("arch=amd64 mem=8096 root-disk=10240 cores=1"),
			InstanceId:  "00000009-0000-0000-0000-0000000000000",
			DisplayName: "Machine 0",
			Status:      "available",
			Message:     "OK!",
			HasVote:     true,
		}, {
			Id:          "1",
			Hardware:    jimmtest.ParseMachineHardware("arch=amd64 mem=8096 root-disk=10240 cores=2"),
			InstanceId:  "00000009-0000-0000-0000-0000000000001",
			DisplayName: "Machine 1",
			Status:      "available",
			Message:     "OK!",
			HasVote:     true,
		}},
		SLA: &jujuparams.ModelSLAInfo{
			Level: "unsupported",
		},
		AgentVersion: newVersion("1.2.3"),
	},
}, {
	name:     "WriteUser",
	env:      modelInfoTestEnv,
	username: "bob@external",
	uuid:     "00000002-0000-0000-0000-000000000001",
	expectModelInfo: &jujuparams.ModelInfo{
		Name:               "model-1",
		Type:               "iaas",
		UUID:               "00000002-0000-0000-0000-000000000001",
		ControllerUUID:     "00000001-0000-0000-0000-000000000001",
		ProviderType:       "test-provider",
		DefaultSeries:      "warty",
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-cloud-region",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/cred-1").String(),
		OwnerTag:           names.NewUserTag("alice@external").String(),
		Life:               "alive",
		Status: jujuparams.EntityStatus{
			Status: "available",
			Info:   "OK!",
			Since:  newDate(2020, 2, 20, 20, 2, 20, 0, time.UTC),
		},
		Users: []jujuparams.ModelUserInfo{{
			UserName: "bob@external",
			Access:   "write",
		}},
		Machines: []jujuparams.ModelMachineInfo{{
			Id:          "0",
			Hardware:    jimmtest.ParseMachineHardware("arch=amd64 mem=8096 root-disk=10240 cores=1"),
			InstanceId:  "00000009-0000-0000-0000-0000000000000",
			DisplayName: "Machine 0",
			Status:      "available",
			Message:     "OK!",
			HasVote:     true,
		}, {
			Id:          "1",
			Hardware:    jimmtest.ParseMachineHardware("arch=amd64 mem=8096 root-disk=10240 cores=2"),
			InstanceId:  "00000009-0000-0000-0000-0000000000001",
			DisplayName: "Machine 1",
			Status:      "available",
			Message:     "OK!",
			HasVote:     true,
		}},
		SLA: &jujuparams.ModelSLAInfo{
			Level: "unsupported",
		},
		AgentVersion: newVersion("1.2.3"),
	},
}, {
	name:     "ReadUser",
	env:      modelInfoTestEnv,
	username: "charlie@external",
	uuid:     "00000002-0000-0000-0000-000000000001",
	expectModelInfo: &jujuparams.ModelInfo{
		Name:               "model-1",
		Type:               "iaas",
		UUID:               "00000002-0000-0000-0000-000000000001",
		ControllerUUID:     "00000001-0000-0000-0000-000000000001",
		ProviderType:       "test-provider",
		DefaultSeries:      "warty",
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-cloud-region",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/cred-1").String(),
		OwnerTag:           names.NewUserTag("alice@external").String(),
		Life:               "alive",
		Status: jujuparams.EntityStatus{
			Status: "available",
			Info:   "OK!",
			Since:  newDate(2020, 2, 20, 20, 2, 20, 0, time.UTC),
		},
		Users: []jujuparams.ModelUserInfo{{
			UserName: "charlie@external",
			Access:   "read",
		}},
		SLA: &jujuparams.ModelSLAInfo{
			Level: "unsupported",
		},
		AgentVersion: newVersion("1.2.3"),
	},
}, {
	name:        "NoAccess",
	env:         modelInfoTestEnv,
	username:    "diane@external",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: "unauthorized access",
}, {
	name:        "NotFound",
	env:         modelInfoTestEnv,
	username:    "alice@external",
	uuid:        "00000002-0000-0000-0000-000000000002",
	expectError: "record not found",
}}

func TestModelInfo(t *testing.T) {
	c := qt.New(t)

	for _, test := range modelInfoTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, test.env)
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: &jimmtest.Dialer{
					API: &jimmtest.API{},
				},
			}
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, j.Database)

			u := &dbmodel.User{
				Username: test.username,
			}
			mi, err := j.ModelInfo(context.Background(), u, names.NewModelTag(test.uuid))
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
			} else {
				c.Assert(err, qt.IsNil)
				c.Check(mi, qt.CmpEquals(cmpopts.EquateEmpty()), test.expectModelInfo)
			}
		})
	}
}

const modelStatusTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@external
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
models:
- name: model-1
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@external
  users:
  - user: alice@external
    access: admin
  - user: bob@external
    access: write
  - user: charlie@external
    access: read
users:
- username: diane@external
  controller-access: superuser
`

var modelStatusTests = []struct {
	name              string
	env               string
	modelStatus       func(context.Context, *jujuparams.ModelStatus) error
	username          string
	uuid              string
	expectModelStatus *jujuparams.ModelStatus
	expectError       string
}{{
	name:        "ModelNotFound",
	username:    "alice@external",
	uuid:        "00000001-0000-0000-0000-000000000001",
	expectError: `record not found`,
}, {
	name:        "UnauthorizedUser",
	env:         modelStatusTestEnv,
	username:    "bob@external",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: "unauthorized access",
}, {
	name: "Success",
	env:  modelStatusTestEnv,
	modelStatus: func(_ context.Context, ms *jujuparams.ModelStatus) error {
		if ms.ModelTag != names.NewModelTag("00000002-0000-0000-0000-000000000001").String() {
			return errors.E("incorrect model tag")
		}
		ms.Life = "alive"
		ms.Type = "iaas"
		ms.HostedMachineCount = 10
		ms.ApplicationCount = 3
		ms.UnitCount = 20
		ms.OwnerTag = names.NewUserTag("alice@external").String()
		return nil
	},
	username: "alice@external",
	uuid:     "00000002-0000-0000-0000-000000000001",
	expectModelStatus: &jujuparams.ModelStatus{
		ModelTag:           names.NewModelTag("00000002-0000-0000-0000-000000000001").String(),
		Life:               "alive",
		Type:               "iaas",
		HostedMachineCount: 10,
		ApplicationCount:   3,
		UnitCount:          20,
		OwnerTag:           names.NewUserTag("alice@external").String(),
	},
}, {
	name: "APIError",
	env:  modelStatusTestEnv,
	modelStatus: func(_ context.Context, ms *jujuparams.ModelStatus) error {
		return errors.E("test error")
	},
	username:    "alice@external",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: "test error",
}}

func TestModelStatus(t *testing.T) {
	c := qt.New(t)

	for _, test := range modelStatusTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, test.env)
			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					ModelStatus_: test.modelStatus,
				},
			}
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: dialer,
			}
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, j.Database)
			u := env.User(test.username).DBObject(c, j.Database)
			ms, err := j.ModelStatus(context.Background(), &u, names.NewModelTag(test.uuid))
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
			} else {
				c.Assert(err, qt.IsNil)
				c.Check(ms, qt.CmpEquals(cmpopts.EquateEmpty()), test.expectModelStatus)
			}

			c.Check(dialer.IsClosed(), qt.Equals, true)
		})
	}
}

const forEachModelTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@external
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
models:
- name: model-1
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
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
    access: admin
  machines:
  - id: 0
    hardware:
      arch: amd64
      mem: 8096
      root-disk: 10240
      cores: 1
    instance-id: 00000009-0000-0000-0000-0000000000000
    display-name: Machine 0
    status: available
    message: OK!
    has-vote: true
    wants-vote: false
    ha-primary: false
  - id: 1
    hardware:
      arch: amd64
      mem: 8096
      root-disk: 10240
      cores: 2
    instance-id: 00000009-0000-0000-0000-0000000000001
    display-name: Machine 1
    status: available
    message: OK!
    has-vote: true
    wants-vote: false
    ha-primary: false
  sla:
    level: unsupported
  agent-version: 1.2.3
- name: model-2
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000002
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
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
  machines:
  - id: 0
    hardware:
      arch: amd64
      mem: 8096
      root-disk: 10240
      cores: 1
    instance-id: 0000000a-0000-0000-0000-0000000000000
    display-name: Machine 0
    status: available
    message: OK!
    has-vote: true
    wants-vote: false
    ha-primary: false
  - id: 1
    hardware:
      arch: amd64
      mem: 8096
      root-disk: 10240
      cores: 2
    instance-id: 0000000a-0000-0000-0000-0000000000001
    display-name: Machine 1
    status: available
    message: OK!
    has-vote: true
    wants-vote: false
    ha-primary: false
  sla:
    level: unsupported
  agent-version: 1.2.3
- name: model-3
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000003
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
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
    access: ""
  machines:
  - id: 0
    hardware:
      arch: amd64
      mem: 8096
      root-disk: 10240
      cores: 1
    instance-id: 0000000b-0000-0000-0000-0000000000000
    display-name: Machine 0
    status: available
    message: OK!
    has-vote: true
    wants-vote: false
    ha-primary: false
  - id: 1
    hardware:
      arch: amd64
      mem: 8096
      root-disk: 10240
      cores: 2
    instance-id: 0000000b-0000-0000-0000-0000000000001
    display-name: Machine 1
    status: available
    message: OK!
    has-vote: true
    wants-vote: false
    ha-primary: false
  sla:
    level: unsupported
  agent-version: 1.2.3
- name: model-4
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000004
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
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
    access: read
  machines:
  - id: 0
    hardware:
      arch: amd64
      mem: 8096
      root-disk: 10240
      cores: 1
    instance-id: 0000000c-0000-0000-0000-0000000000000
    display-name: Machine 0
    status: available
    message: OK!
    has-vote: true
    wants-vote: false
    ha-primary: false
  - id: 1
    hardware:
      arch: amd64
      mem: 8096
      root-disk: 10240
      cores: 2
    instance-id: 0000000c-0000-0000-0000-0000000000001
    display-name: Machine 1
    status: available
    message: OK!
    has-vote: true
    wants-vote: false
    ha-primary: false
  sla:
    level: unsupported
  agent-version: 1.2.3
users:
- username: alice@external
  controller-access: superuser
`

func TestForEachUserModel(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	env := jimmtest.ParseEnvironment(c, forEachModelTestEnv)
	j := &jimm.JIMM{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, nil),
		},
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{},
		},
	}
	err := j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)
	env.PopulateDB(c, j.Database)

	u := env.User("bob@external").DBObject(c, j.Database)
	var res []jujuparams.ModelSummaryResult
	err = j.ForEachUserModel(ctx, &u, func(uma *dbmodel.UserModelAccess) error {
		s := uma.Model_.ToJujuModelSummary()
		s.UserAccess = jujuparams.UserAccessPermission(uma.Access)
		res = append(res, jujuparams.ModelSummaryResult{Result: &s})
		return nil
	})
	c.Assert(err, qt.IsNil)
	c.Check(res, qt.DeepEquals, []jujuparams.ModelSummaryResult{{
		Result: &jujuparams.ModelSummary{
			Name:               "model-1",
			UUID:               "00000002-0000-0000-0000-000000000001",
			Type:               "iaas",
			ControllerUUID:     "00000001-0000-0000-0000-000000000001",
			ProviderType:       "test-provider",
			DefaultSeries:      "warty",
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudRegion:        "test-cloud-region",
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/cred-1").String(),
			OwnerTag:           names.NewUserTag("alice@external").String(),
			Life:               "alive",
			Status: jujuparams.EntityStatus{
				Status: "available",
				Info:   "OK!",
				Since:  newDate(2020, 02, 20, 20, 02, 20, 0, time.UTC),
			},
			UserAccess: "admin",
			Counts: []params.ModelEntityCount{{
				Entity: "machines",
				Count:  2,
			}, {
				Entity: "cores",
				Count:  3,
			}, {
				Entity: "units",
				Count:  0,
			}},
			SLA: &jujuparams.ModelSLAInfo{
				Level: "unsupported",
			},
			AgentVersion: newVersion("1.2.3"),
		},
	}, {
		Result: &jujuparams.ModelSummary{
			Name:               "model-2",
			UUID:               "00000002-0000-0000-0000-000000000002",
			Type:               "iaas",
			ControllerUUID:     "00000001-0000-0000-0000-000000000001",
			ProviderType:       "test-provider",
			DefaultSeries:      "warty",
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudRegion:        "test-cloud-region",
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/cred-1").String(),
			OwnerTag:           names.NewUserTag("alice@external").String(),
			Life:               "alive",
			Status: jujuparams.EntityStatus{
				Status: "available",
				Info:   "OK!",
				Since:  newDate(2020, 02, 20, 20, 02, 20, 0, time.UTC),
			},
			UserAccess: "write",
			Counts: []params.ModelEntityCount{{
				Entity: "machines",
				Count:  2,
			}, {
				Entity: "cores",
				Count:  3,
			}, {
				Entity: "units",
				Count:  0,
			}},
			SLA: &jujuparams.ModelSLAInfo{
				Level: "unsupported",
			},
			AgentVersion: newVersion("1.2.3"),
		},
	}, {
		Result: &jujuparams.ModelSummary{
			Name:               "model-4",
			UUID:               "00000002-0000-0000-0000-000000000004",
			Type:               "iaas",
			ControllerUUID:     "00000001-0000-0000-0000-000000000001",
			ProviderType:       "test-provider",
			DefaultSeries:      "warty",
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudRegion:        "test-cloud-region",
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/cred-1").String(),
			OwnerTag:           names.NewUserTag("alice@external").String(),
			Life:               "alive",
			Status: jujuparams.EntityStatus{
				Status: "available",
				Info:   "OK!",
				Since:  newDate(2020, 02, 20, 20, 02, 20, 0, time.UTC),
			},
			UserAccess: "read",
			Counts: []params.ModelEntityCount{{
				Entity: "machines",
				Count:  2,
			}, {
				Entity: "cores",
				Count:  3,
			}, {
				Entity: "units",
				Count:  0,
			}},
			SLA: &jujuparams.ModelSLAInfo{
				Level: "unsupported",
			},
			AgentVersion: newVersion("1.2.3"),
		},
	}})
}

func TestForEachModel(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	env := jimmtest.ParseEnvironment(c, forEachModelTestEnv)
	j := &jimm.JIMM{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, nil),
		},
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{},
		},
	}
	err := j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)
	env.PopulateDB(c, j.Database)

	u := env.User("bob@external").DBObject(c, j.Database)
	err = j.ForEachModel(ctx, &u, func(uma *dbmodel.UserModelAccess) error {
		return errors.E("function called unexpectedly")
	})
	c.Check(err, qt.ErrorMatches, `unauthorized access`)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)

	u = env.User("alice@external").DBObject(c, j.Database)
	var models []string
	err = j.ForEachModel(ctx, &u, func(uma *dbmodel.UserModelAccess) error {
		c.Check(uma.Access, qt.Equals, "admin")
		models = append(models, uma.Model_.UUID.String)
		return nil
	})
	c.Assert(err, qt.IsNil)
	c.Check(models, qt.DeepEquals, []string{
		"00000002-0000-0000-0000-000000000001",
		"00000002-0000-0000-0000-000000000002",
		"00000002-0000-0000-0000-000000000003",
		"00000002-0000-0000-0000-000000000004",
	})
}

const grantModelAccessTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@external
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@external
  users:
  - user: alice@external
    access: admin
  - user: charlie@external
    access: write
`

var grantModelAccessTests = []struct {
	name             string
	env              string
	grantModelAccess func(context.Context, names.ModelTag, names.UserTag, jujuparams.UserAccessPermission) error
	dialError        error
	username         string
	uuid             string
	targetUsername   string
	access           string
	expectModel      dbmodel.Model
	expectError      string
	expectErrorCode  errors.Code
}{{
	name:            "ModelNotFound",
	username:        "alice@external",
	uuid:            "00000002-0000-0000-0000-000000000001",
	targetUsername:  "bob@external",
	access:          "write",
	expectError:     `record not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name: "Success",
	env:  grantModelAccessTestEnv,
	grantModelAccess: func(_ context.Context, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error {
		if mt.Id() != "00000002-0000-0000-0000-000000000001" {
			return errors.E("bad model tag")
		}
		if ut.Id() != "bob@external" {
			return errors.E("bad user tag")
		}
		if access != "write" {
			return errors.E("bad permission")
		}
		return nil
	},
	username:       "alice@external",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@external",
	access:         "write",
	expectModel: dbmodel.Model{
		Name: "model-1",
		UUID: sql.NullString{
			String: "00000002-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner: dbmodel.User{
			Username:         "alice@external",
			ControllerAccess: "add-model",
		},
		Controller: dbmodel.Controller{
			Name: "controller-1",
			UUID: "00000001-0000-0000-0000-000000000001",
		},
		CloudRegion: dbmodel.CloudRegion{
			Cloud: dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			},
			Name: "test-cloud-region",
		},
		CloudCredential: dbmodel.CloudCredential{
			Name: "cred-1",
		},
		Users: []dbmodel.UserModelAccess{{
			User: dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "add-model",
			},
			Access: "admin",
		}, {
			User: dbmodel.User{
				Username:         "charlie@external",
				ControllerAccess: "add-model",
			},
			Access: "write",
		}, {
			User: dbmodel.User{
				Username:         "bob@external",
				ControllerAccess: "add-model",
			},
			Access: "write",
		}},
	},
}, {
	name:            "UserNotAuthorized",
	env:             grantModelAccessTestEnv,
	username:        "charlie@external",
	uuid:            "00000002-0000-0000-0000-000000000001",
	targetUsername:  "bob@external",
	access:          "write",
	expectError:     `unauthorized access`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:           "DialError",
	env:            grantModelAccessTestEnv,
	dialError:      errors.E("test dial error"),
	username:       "alice@external",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@external",
	access:         "write",
	expectError:    `test dial error`,
}, {
	name: "APIError",
	env:  grantModelAccessTestEnv,
	grantModelAccess: func(_ context.Context, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error {
		return errors.E("test error")
	},
	username:       "alice@external",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@external",
	access:         "write",
	expectError:    `test error`,
}}

func TestGrantModelAccess(t *testing.T) {
	c := qt.New(t)

	for _, test := range grantModelAccessTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, test.env)
			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					GrantModelAccess_: test.grantModelAccess,
				},
				Err: test.dialError,
			}
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: dialer,
			}
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, j.Database)

			u := env.User(test.username).DBObject(c, j.Database)

			err = j.GrantModelAccess(ctx, &u, names.NewModelTag(test.uuid), names.NewUserTag(test.targetUsername), jujuparams.UserAccessPermission(test.access))
			c.Assert(dialer.IsClosed(), qt.Equals, true)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			m := dbmodel.Model{
				UUID: sql.NullString{
					String: test.uuid,
					Valid:  true,
				},
			}
			err = j.Database.GetModel(ctx, &m)
			c.Assert(err, qt.IsNil)
			c.Check(m, jimmtest.DBObjectEquals, test.expectModel)
		})
	}
}

const revokeModelAccessTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@external
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@external
  users:
  - user: alice@external
    access: admin
  - user: bob@external
    access: admin
  - user: charlie@external
    access: write
`

var revokeModelAccessTests = []struct {
	name              string
	env               string
	revokeModelAccess func(context.Context, names.ModelTag, names.UserTag, jujuparams.UserAccessPermission) error
	dialError         error
	username          string
	uuid              string
	targetUsername    string
	access            string
	expectModel       dbmodel.Model
	expectError       string
	expectErrorCode   errors.Code
}{{
	name:            "ModelNotFound",
	username:        "alice@external",
	uuid:            "00000002-0000-0000-0000-000000000001",
	targetUsername:  "bob@external",
	access:          "write",
	expectError:     `record not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name: "SuccessAdmin",
	env:  revokeModelAccessTestEnv,
	revokeModelAccess: func(_ context.Context, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error {
		if mt.Id() != "00000002-0000-0000-0000-000000000001" {
			return errors.E("bad model tag")
		}
		if ut.Id() != "bob@external" {
			return errors.E("bad user tag")
		}
		if access != "admin" {
			return errors.E("bad permission")
		}
		return nil
	},
	username:       "alice@external",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@external",
	access:         "admin",
	expectModel: dbmodel.Model{
		Name: "model-1",
		UUID: sql.NullString{
			String: "00000002-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner: dbmodel.User{
			Username:         "alice@external",
			ControllerAccess: "add-model",
		},
		Controller: dbmodel.Controller{
			Name: "controller-1",
			UUID: "00000001-0000-0000-0000-000000000001",
		},
		CloudRegion: dbmodel.CloudRegion{
			Cloud: dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			},
			Name: "test-cloud-region",
		},
		CloudCredential: dbmodel.CloudCredential{
			Name: "cred-1",
		},
		Users: []dbmodel.UserModelAccess{{
			User: dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "add-model",
			},
			Access: "admin",
		}, {
			User: dbmodel.User{
				Username:         "bob@external",
				ControllerAccess: "add-model",
			},
			Access: "write",
		}, {
			User: dbmodel.User{
				Username:         "charlie@external",
				ControllerAccess: "add-model",
			},
			Access: "write",
		}},
	},
}, {
	name: "SuccessWrite",
	env:  revokeModelAccessTestEnv,
	revokeModelAccess: func(_ context.Context, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error {
		if mt.Id() != "00000002-0000-0000-0000-000000000001" {
			return errors.E("bad model tag")
		}
		if ut.Id() != "bob@external" {
			return errors.E("bad user tag")
		}
		if access != "write" {
			return errors.E("bad permission")
		}
		return nil
	},
	username:       "alice@external",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@external",
	access:         "write",
	expectModel: dbmodel.Model{
		Name: "model-1",
		UUID: sql.NullString{
			String: "00000002-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner: dbmodel.User{
			Username:         "alice@external",
			ControllerAccess: "add-model",
		},
		Controller: dbmodel.Controller{
			Name: "controller-1",
			UUID: "00000001-0000-0000-0000-000000000001",
		},
		CloudRegion: dbmodel.CloudRegion{
			Cloud: dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			},
			Name: "test-cloud-region",
		},
		CloudCredential: dbmodel.CloudCredential{
			Name: "cred-1",
		},
		Users: []dbmodel.UserModelAccess{{
			User: dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "add-model",
			},
			Access: "admin",
		}, {
			User: dbmodel.User{
				Username:         "bob@external",
				ControllerAccess: "add-model",
			},
			Access: "read",
		}, {
			User: dbmodel.User{
				Username:         "charlie@external",
				ControllerAccess: "add-model",
			},
			Access: "write",
		}},
	},
}, {
	name: "SuccessRead",
	env:  revokeModelAccessTestEnv,
	revokeModelAccess: func(_ context.Context, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error {
		if mt.Id() != "00000002-0000-0000-0000-000000000001" {
			return errors.E("bad model tag")
		}
		if ut.Id() != "bob@external" {
			return errors.E("bad user tag")
		}
		if access != "read" {
			return errors.E("bad permission")
		}
		return nil
	},
	username:       "alice@external",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@external",
	access:         "read",
	expectModel: dbmodel.Model{
		Name: "model-1",
		UUID: sql.NullString{
			String: "00000002-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner: dbmodel.User{
			Username:         "alice@external",
			ControllerAccess: "add-model",
		},
		Controller: dbmodel.Controller{
			Name: "controller-1",
			UUID: "00000001-0000-0000-0000-000000000001",
		},
		CloudRegion: dbmodel.CloudRegion{
			Cloud: dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			},
			Name: "test-cloud-region",
		},
		CloudCredential: dbmodel.CloudCredential{
			Name: "cred-1",
		},
		Users: []dbmodel.UserModelAccess{{
			User: dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "add-model",
			},
			Access: "admin",
		}, {
			User: dbmodel.User{
				Username:         "charlie@external",
				ControllerAccess: "add-model",
			},
			Access: "write",
		}},
	},
}, {
	name:            "UserNotAuthorized",
	env:             revokeModelAccessTestEnv,
	username:        "charlie@external",
	uuid:            "00000002-0000-0000-0000-000000000001",
	targetUsername:  "bob@external",
	access:          "write",
	expectError:     `unauthorized access`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:           "DialError",
	env:            revokeModelAccessTestEnv,
	dialError:      errors.E("test dial error"),
	username:       "alice@external",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@external",
	access:         "write",
	expectError:    `test dial error`,
}, {
	name: "APIError",
	env:  revokeModelAccessTestEnv,
	revokeModelAccess: func(_ context.Context, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error {
		return errors.E("test error")
	},
	username:       "alice@external",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@external",
	access:         "write",
	expectError:    `test error`,
}}

func TestRevokeModelAccess(t *testing.T) {
	c := qt.New(t)

	for _, test := range revokeModelAccessTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, test.env)
			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					RevokeModelAccess_: test.revokeModelAccess,
				},
				Err: test.dialError,
			}
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: dialer,
			}
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, j.Database)

			u := env.User(test.username).DBObject(c, j.Database)

			err = j.RevokeModelAccess(ctx, &u, names.NewModelTag(test.uuid), names.NewUserTag(test.targetUsername), jujuparams.UserAccessPermission(test.access))
			c.Assert(dialer.IsClosed(), qt.Equals, true)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			m := dbmodel.Model{
				UUID: sql.NullString{
					String: test.uuid,
					Valid:  true,
				},
			}
			err = j.Database.GetModel(ctx, &m)
			c.Assert(err, qt.IsNil)
			c.Check(m, jimmtest.DBObjectEquals, test.expectModel)
		})
	}
}

const destroyModelTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@external
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@external
  life: alive
  users:
  - user: alice@external
    access: admin
  - user: bob@external
    access: write
users:
- username: charlie@external
  controller-access: superuser
`

var destroyModelTests = []struct {
	name            string
	env             string
	destroyModel    func(context.Context, names.ModelTag, *bool, *bool, *time.Duration) error
	dialError       error
	username        string
	uuid            string
	destroyStorage  *bool
	force           *bool
	maxWait         *time.Duration
	expectError     string
	expectErrorCode errors.Code
}{{
	name:            "NotFound",
	env:             destroyModelTestEnv,
	username:        "alice@external",
	uuid:            "00000002-0000-0000-0000-000000000002",
	expectError:     `record not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name:            "Unauthorized",
	env:             destroyModelTestEnv,
	username:        "bob@external",
	uuid:            "00000002-0000-0000-0000-000000000001",
	expectError:     `unauthorized access`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name: "Success",
	env:  destroyModelTestEnv,
	destroyModel: func(_ context.Context, mt names.ModelTag, destroyStorage, force *bool, maxWait *time.Duration) error {
		if mt.Id() != "00000002-0000-0000-0000-000000000001" {
			return errors.E("incorrect model uuid")
		}
		if destroyStorage == nil || *destroyStorage != true {
			return errors.E("invalid destroyStorage")
		}
		if force == nil || *force != false {
			return errors.E("invalid force")
		}
		if maxWait == nil || *maxWait != time.Second {
			return errors.E("invalid maxWait")
		}
		return nil
	},
	username:       "alice@external",
	uuid:           "00000002-0000-0000-0000-000000000001",
	destroyStorage: newBool(true),
	force:          newBool(false),
	maxWait:        newDuration(time.Second),
}, {
	name: "SuperuserSuccess",
	env:  destroyModelTestEnv,
	destroyModel: func(_ context.Context, _ names.ModelTag, _, _ *bool, _ *time.Duration) error {
		return nil
	},
	username: "charlie@external",
	uuid:     "00000002-0000-0000-0000-000000000001",
}, {
	name:        "DialError",
	env:         destroyModelTestEnv,
	dialError:   errors.E("dial error"),
	username:    "alice@external",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: `dial error`,
}, {
	name: "APIError",
	env:  destroyModelTestEnv,
	destroyModel: func(_ context.Context, _ names.ModelTag, _, _ *bool, _ *time.Duration) error {
		return errors.E("api error")
	},
	username:    "charlie@external",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: `api error`,
}}

func TestDestroyModel(t *testing.T) {
	c := qt.New(t)

	for _, test := range destroyModelTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, test.env)
			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					DestroyModel_: test.destroyModel,
				},
				Err: test.dialError,
			}
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: dialer,
			}
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, j.Database)

			u := env.User(test.username).DBObject(c, j.Database)

			err = j.DestroyModel(ctx, &u, names.NewModelTag(test.uuid), test.destroyStorage, test.force, test.maxWait)
			c.Assert(dialer.IsClosed(), qt.Equals, true)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			m := dbmodel.Model{
				UUID: sql.NullString{
					String: test.uuid,
					Valid:  true,
				},
			}
			err = j.Database.GetModel(ctx, &m)
			c.Assert(err, qt.IsNil)
			c.Check(m.Life, qt.Equals, "dying")
		})
	}
}

var dumpModelTests = []struct {
	name            string
	env             string
	dumpModel       func(context.Context, names.ModelTag, bool) (string, error)
	dialError       error
	username        string
	uuid            string
	simplified      bool
	expectString    string
	expectError     string
	expectErrorCode errors.Code
}{{
	name: "NotFound",
	// reuse the destroyModelTestEnv for these tests.
	env:             destroyModelTestEnv,
	username:        "alice@external",
	uuid:            "00000002-0000-0000-0000-000000000002",
	expectError:     `record not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name:            "Unauthorized",
	env:             destroyModelTestEnv,
	username:        "bob@external",
	uuid:            "00000002-0000-0000-0000-000000000001",
	expectError:     `unauthorized access`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name: "Success",
	env:  destroyModelTestEnv,
	dumpModel: func(_ context.Context, mt names.ModelTag, simplified bool) (string, error) {
		if mt.Id() != "00000002-0000-0000-0000-000000000001" {
			return "", errors.E("incorrect model uuid")
		}
		if simplified != true {
			return "", errors.E("invalid simplified")
		}
		return "model dump", nil
	},
	username:     "alice@external",
	uuid:         "00000002-0000-0000-0000-000000000001",
	simplified:   true,
	expectString: "model dump",
}, {
	name: "SuperuserSuccess",
	env:  destroyModelTestEnv,
	dumpModel: func(_ context.Context, _ names.ModelTag, _ bool) (string, error) {
		return "model dump2", nil
	},
	username:     "charlie@external",
	uuid:         "00000002-0000-0000-0000-000000000001",
	expectString: "model dump2",
}, {
	name:        "DialError",
	env:         destroyModelTestEnv,
	dialError:   errors.E("dial error"),
	username:    "alice@external",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: `dial error`,
}, {
	name: "APIError",
	env:  destroyModelTestEnv,
	dumpModel: func(_ context.Context, _ names.ModelTag, _ bool) (string, error) {
		return "", errors.E("api error")
	},
	username:    "charlie@external",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: `api error`,
}}

func TestDumpModel(t *testing.T) {
	c := qt.New(t)

	for _, test := range dumpModelTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, test.env)
			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					DumpModel_: test.dumpModel,
				},
				Err: test.dialError,
			}
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: dialer,
			}
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, j.Database)

			u := env.User(test.username).DBObject(c, j.Database)

			s, err := j.DumpModel(ctx, &u, names.NewModelTag(test.uuid), test.simplified)
			c.Assert(dialer.IsClosed(), qt.Equals, true)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			c.Check(s, qt.Equals, test.expectString)
		})
	}
}

var dumpModelDBTests = []struct {
	name            string
	env             string
	dumpModelDB     func(context.Context, names.ModelTag) (map[string]interface{}, error)
	dialError       error
	username        string
	uuid            string
	expectDump      map[string]interface{}
	expectError     string
	expectErrorCode errors.Code
}{{
	name: "NotFound",
	// reuse the destroyModelTestEnv for these tests.
	env:             destroyModelTestEnv,
	username:        "alice@external",
	uuid:            "00000002-0000-0000-0000-000000000002",
	expectError:     `record not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name:            "Unauthorized",
	env:             destroyModelTestEnv,
	username:        "bob@external",
	uuid:            "00000002-0000-0000-0000-000000000001",
	expectError:     `unauthorized access`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name: "Success",
	env:  destroyModelTestEnv,
	dumpModelDB: func(_ context.Context, mt names.ModelTag) (map[string]interface{}, error) {
		if mt.Id() != "00000002-0000-0000-0000-000000000001" {
			return nil, errors.E("incorrect model uuid")
		}
		return map[string]interface{}{"model": "dump"}, nil
	},
	username:   "alice@external",
	uuid:       "00000002-0000-0000-0000-000000000001",
	expectDump: map[string]interface{}{"model": "dump"},
}, {
	name: "SuperuserSuccess",
	env:  destroyModelTestEnv,
	dumpModelDB: func(_ context.Context, _ names.ModelTag) (map[string]interface{}, error) {
		return map[string]interface{}{"model": "dump 2"}, nil
	},
	username:   "charlie@external",
	uuid:       "00000002-0000-0000-0000-000000000001",
	expectDump: map[string]interface{}{"model": "dump 2"},
}, {
	name:        "DialError",
	env:         destroyModelTestEnv,
	dialError:   errors.E("dial error"),
	username:    "alice@external",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: `dial error`,
}, {
	name: "APIError",
	env:  destroyModelTestEnv,
	dumpModelDB: func(_ context.Context, _ names.ModelTag) (map[string]interface{}, error) {
		return nil, errors.E("api error")
	},
	username:    "charlie@external",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: `api error`,
}}

func TestDumpModelDB(t *testing.T) {
	c := qt.New(t)

	for _, test := range dumpModelDBTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, test.env)
			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					DumpModelDB_: test.dumpModelDB,
				},
				Err: test.dialError,
			}
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: dialer,
			}
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, j.Database)

			u := env.User(test.username).DBObject(c, j.Database)

			dump, err := j.DumpModelDB(ctx, &u, names.NewModelTag(test.uuid))
			c.Assert(dialer.IsClosed(), qt.Equals, true)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			c.Check(dump, qt.DeepEquals, test.expectDump)
		})
	}
}

var validateModelUpgradeTests = []struct {
	name                 string
	env                  string
	validateModelUpgrade func(context.Context, names.ModelTag, bool) error
	dialError            error
	username             string
	uuid                 string
	force                bool
	expectError          string
	expectErrorCode      errors.Code
}{{
	name: "NotFound",
	// reuse the destroyModelTestEnv for these tests.
	env:             destroyModelTestEnv,
	username:        "alice@external",
	uuid:            "00000002-0000-0000-0000-000000000002",
	expectError:     `record not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name:            "Unauthorized",
	env:             destroyModelTestEnv,
	username:        "bob@external",
	uuid:            "00000002-0000-0000-0000-000000000001",
	expectError:     `unauthorized access`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name: "Success",
	env:  destroyModelTestEnv,
	validateModelUpgrade: func(_ context.Context, mt names.ModelTag, force bool) error {
		if mt.Id() != "00000002-0000-0000-0000-000000000001" {
			return errors.E("incorrect model uuid")
		}
		if force != true {
			return errors.E("incorrect force")
		}
		return nil
	},
	username: "alice@external",
	uuid:     "00000002-0000-0000-0000-000000000001",
	force:    true,
}, {
	name: "SuperuserSuccess",
	env:  destroyModelTestEnv,
	validateModelUpgrade: func(_ context.Context, _ names.ModelTag, force bool) error {
		if force != false {
			return errors.E("incorrect force")
		}
		return nil
	},
	username: "charlie@external",
	uuid:     "00000002-0000-0000-0000-000000000001",
}, {
	name:        "DialError",
	env:         destroyModelTestEnv,
	dialError:   errors.E("dial error"),
	username:    "alice@external",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: `dial error`,
}, {
	name: "APIError",
	env:  destroyModelTestEnv,
	validateModelUpgrade: func(_ context.Context, _ names.ModelTag, _ bool) error {
		return errors.E("api error")
	},
	username:    "charlie@external",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: `api error`,
}}

func TestValidateModelUpgrade(t *testing.T) {
	c := qt.New(t)

	for _, test := range validateModelUpgradeTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, test.env)
			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					ValidateModelUpgrade_: test.validateModelUpgrade,
				},
				Err: test.dialError,
			}
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: dialer,
			}
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, j.Database)

			u := env.User(test.username).DBObject(c, j.Database)

			err = j.ValidateModelUpgrade(ctx, &u, names.NewModelTag(test.uuid), test.force)
			c.Assert(dialer.IsClosed(), qt.Equals, true)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
		})
	}
}

const updateModelCredentialTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@external
  name: cred-2
  cloud: test-cloud
- owner: alice@external
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@external
  users:
  - user: alice@external
    access: admin
  - user: charlie@external
    access: write
`

var updateModelCredentialTests = []struct {
	name                  string
	env                   string
	updateCredential      func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error)
	changeModelCredential func(context.Context, names.ModelTag, names.CloudCredentialTag) error
	dialError             error
	username              string
	credential            string
	uuid                  string
	expectModel           dbmodel.Model
	expectError           string
	expectErrorCode       errors.Code
}{{
	name: "success",
	env:  updateModelCredentialTestEnv,
	updateCredential: func(_ context.Context, taggedCredential jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
		if taggedCredential.Tag != "cloudcred-test-cloud_alice@external_cred-2" {
			return nil, errors.E("bad cloud credential tag")
		}
		return nil, nil
	},
	changeModelCredential: func(_ context.Context, modelTag names.ModelTag, credentialTag names.CloudCredentialTag) error {
		if modelTag.Id() != "00000002-0000-0000-0000-000000000001" {
			return errors.E("bad model tag")
		}
		if credentialTag.Id() != "test-cloud/alice@external/cred-2" {
			return errors.E("bad cloud credential tag")
		}
		return nil
	},
	username:   "alice@external",
	credential: "test-cloud/alice@external/cred-2",
	uuid:       "00000002-0000-0000-0000-000000000001",
	expectModel: dbmodel.Model{
		Name: "model-1",
		UUID: sql.NullString{
			String: "00000002-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner: dbmodel.User{
			Username:         "alice@external",
			ControllerAccess: "add-model",
		},
		Controller: dbmodel.Controller{
			Name: "controller-1",
			UUID: "00000001-0000-0000-0000-000000000001",
		},
		CloudRegion: dbmodel.CloudRegion{
			Cloud: dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			},
			Name: "test-cloud-region",
		},
		CloudCredential: dbmodel.CloudCredential{
			Name: "cred-2",
		},
		Users: []dbmodel.UserModelAccess{{
			User: dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "add-model",
			},
			Access: "admin",
		}, {
			User: dbmodel.User{
				Username:         "charlie@external",
				ControllerAccess: "add-model",
			},
			Access: "write",
		}},
	},
}, {
	name: "user not admin",
	env:  updateModelCredentialTestEnv,
	updateCredential: func(_ context.Context, taggedCredential jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
		if taggedCredential.Tag != "cloudcred-test-cloud_alice@external_cred-2" {
			return nil, errors.E("bad cloud credential tag")
		}
		return nil, nil
	},
	changeModelCredential: func(_ context.Context, modelTag names.ModelTag, credentialTag names.CloudCredentialTag) error {
		if modelTag.Id() != "00000002-0000-0000-0000-000000000001" {
			return errors.E("bad model tag")
		}
		if credentialTag.Id() != "test-cloud/alice@external/cred-2" {
			return errors.E("bad cloud credential tag")
		}
		return nil
	},
	username:        "charlie@external",
	credential:      "test-cloud/alice@external/cred-2",
	uuid:            "00000002-0000-0000-0000-000000000001",
	expectError:     "unauthorized access",
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:            "model not found",
	env:             updateModelCredentialTestEnv,
	username:        "charlie@external",
	credential:      "test-cloud/alice@external/cred-2",
	uuid:            "00000002-0000-0000-0000-000000000002",
	expectError:     "record not found",
	expectErrorCode: errors.CodeNotFound,
}, {
	name: "credential not found",
	env:  updateModelCredentialTestEnv,
	updateCredential: func(_ context.Context, taggedCredential jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
		if taggedCredential.Tag != "cloudcred-test-cloud_alice@external_cred-2" {
			return nil, errors.E("bad cloud credential tag")
		}
		return nil, nil
	},
	changeModelCredential: func(_ context.Context, modelTag names.ModelTag, credentialTag names.CloudCredentialTag) error {
		if modelTag.Id() != "00000002-0000-0000-0000-000000000001" {
			return errors.E("bad model tag")
		}
		if credentialTag.Id() != "test-cloud/alice@external/cred-2" {
			return errors.E("bad cloud credential tag")
		}
		return nil
	},
	username:        "alice@external",
	credential:      "test-cloud/alice@external/cred-3",
	uuid:            "00000002-0000-0000-0000-000000000001",
	expectError:     `cloudcredential "test-cloud/alice@external/cred-3" not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name: "update credential returns an error",
	env:  updateModelCredentialTestEnv,
	updateCredential: func(_ context.Context, taggedCredential jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
		return nil, errors.E("an error")
	},
	username:    "alice@external",
	credential:  "test-cloud/alice@external/cred-2",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: "an error",
}, {
	name: "change model credential returns an error",
	env:  updateModelCredentialTestEnv,
	updateCredential: func(_ context.Context, taggedCredential jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
		if taggedCredential.Tag != "cloudcred-test-cloud_alice@external_cred-2" {
			return nil, errors.E("bad cloud credential tag")
		}
		return nil, nil
	},
	changeModelCredential: func(_ context.Context, modelTag names.ModelTag, credentialTag names.CloudCredentialTag) error {
		return errors.E("an error")
	},
	username:    "alice@external",
	credential:  "test-cloud/alice@external/cred-2",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: "an error",
}}

func TestUpdateModelCredential(t *testing.T) {
	c := qt.New(t)

	for _, test := range updateModelCredentialTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, test.env)
			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					UpdateCredential_:      test.updateCredential,
					ChangeModelCredential_: test.changeModelCredential,
				},
				Err: test.dialError,
			}
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: dialer,
			}
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, j.Database)

			u := env.User(test.username).DBObject(c, j.Database)

			err = j.ChangeModelCredential(
				ctx,
				&u,
				names.NewModelTag(test.uuid),
				names.NewCloudCredentialTag(test.credential),
			)
			c.Assert(dialer.IsClosed(), qt.Equals, true)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			m := dbmodel.Model{
				UUID: sql.NullString{
					String: test.uuid,
					Valid:  true,
				},
			}
			err = j.Database.GetModel(ctx, &m)
			c.Assert(err, qt.IsNil)
			c.Check(m, jimmtest.DBObjectEquals, test.expectModel)
		})
	}
}

func newBool(b bool) *bool {
	return &b
}

func newDuration(d time.Duration) *time.Duration {
	return &d
}

// newDate wraps time.Date to return a *time.Time.
func newDate(year int, month time.Month, day, hour, min, sec, nsec int, loc *time.Location) *time.Time {
	t := time.Date(year, month, day, hour, min, sec, nsec, loc)
	return &t
}

// newVersion wraps version.MustParse to return a *version.Number
func newVersion(s string) *version.Number {
	n := version.MustParse(s)
	return &n
}
