// Copyright 2020 Canonical Ltd.

package jimm_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/apiserver/params"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"github.com/juju/version"
	"sigs.k8s.io/yaml"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
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
			Hardware: dbmodel.MachineHardware{
				Arch: sql.NullString{
					String: "amd64",
					Valid:  true,
				},
				Mem: dbmodel.NullUint64{
					Uint64: 8096,
					Valid:  true,
				},
				Cores: dbmodel.NullUint64{
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
			Hardware: dbmodel.MachineHardware{
				Arch: sql.NullString{
					String: "amd64",
					Valid:  true,
				},
				Mem: dbmodel.NullUint64{
					Uint64: 8096,
					Valid:  true,
				},
				Cores: dbmodel.NullUint64{
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
			Hardware: dbmodel.MachineHardware{
				Arch: sql.NullString{
					String: "amd64",
					Valid:  true,
				},
				Mem: dbmodel.NullUint64{
					Uint64: 8096,
					Valid:  true,
				},
				Cores: dbmodel.NullUint64{
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

const modelInfoTestEnv = `clouds:
- name: dummy
  type: dummy
  regions:
  - name: dummy-region
cloud-credentials:
- owner: alice@external
  name: cred-1
  cloud: dummy
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
models:
- name: model-1
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  default-series: warty
  cloud: dummy
  region: dummy-region
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
		ProviderType:       "dummy",
		DefaultSeries:      "warty",
		CloudTag:           names.NewCloudTag("dummy").String(),
		CloudRegion:        "dummy-region",
		CloudCredentialTag: names.NewCloudCredentialTag("dummy/alice@external/cred-1").String(),
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
		ProviderType:       "dummy",
		DefaultSeries:      "warty",
		CloudTag:           names.NewCloudTag("dummy").String(),
		CloudRegion:        "dummy-region",
		CloudCredentialTag: names.NewCloudCredentialTag("dummy/alice@external/cred-1").String(),
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
		ProviderType:       "dummy",
		DefaultSeries:      "warty",
		CloudTag:           names.NewCloudTag("dummy").String(),
		CloudRegion:        "dummy-region",
		CloudCredentialTag: names.NewCloudCredentialTag("dummy/alice@external/cred-1").String(),
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
- name: dummy
  type: dummy
  regions:
  - name: dummy-region
cloud-credentials:
- owner: alice@external
  name: cred-1
  cloud: dummy
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
models:
- name: model-1
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  default-series: warty
  cloud: dummy
  region: dummy-region
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
  controller-access: admin
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

const listModelSummariesTestEnv = `clouds:
- name: dummy
  type: dummy
  regions:
  - name: dummy-region
cloud-credentials:
- owner: alice@external
  name: cred-1
  cloud: dummy
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
models:
- name: model-1
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  default-series: warty
  cloud: dummy
  region: dummy-region
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
  cloud: dummy
  region: dummy-region
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
  cloud: dummy
  region: dummy-region
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
  cloud: dummy
  region: dummy-region
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
`

func TestListModelSummaries(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	env := jimmtest.ParseEnvironment(c, listModelSummariesTestEnv)
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
	res, err := j.ListModelSummaries(ctx, &u)
	c.Assert(err, qt.IsNil)
	c.Check(res, qt.DeepEquals, []jujuparams.ModelSummaryResult{{
		Result: &jujuparams.ModelSummary{
			Name:               "model-1",
			UUID:               "00000002-0000-0000-0000-000000000001",
			Type:               "iaas",
			ControllerUUID:     "00000001-0000-0000-0000-000000000001",
			ProviderType:       "dummy",
			DefaultSeries:      "warty",
			CloudTag:           names.NewCloudTag("dummy").String(),
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/alice@external/cred-1").String(),
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
			ProviderType:       "dummy",
			DefaultSeries:      "warty",
			CloudTag:           names.NewCloudTag("dummy").String(),
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/alice@external/cred-1").String(),
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
			ProviderType:       "dummy",
			DefaultSeries:      "warty",
			CloudTag:           names.NewCloudTag("dummy").String(),
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/alice@external/cred-1").String(),
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
