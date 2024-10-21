// Copyright 2024 Canonical.

package jimm_test

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/juju/juju/core/life"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	"sigs.k8s.io/yaml"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
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
			OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1").String(),
		},
		expectedArgs: jimm.ModelCreateArgs{
			Name:            "test-model",
			Owner:           names.NewUserTag("alice@canonical.com"),
			Cloud:           names.NewCloudTag("test-cloud"),
			CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1"),
		},
	}, {
		about: "name not specified",
		args: jujuparams.ModelCreateArgs{
			OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
		},
		expectedError: "name not specified",
	}, {
		about: "invalid owner tag",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           "alice@canonical.com",
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
		},
		expectedError: `"alice@canonical.com" is not a valid tag`,
	}, {
		about: "invalid cloud tag",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
			CloudTag:           "test-cloud",
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
		},
		expectedError: `"test-cloud" is not a valid tag`,
	}, {
		about: "invalid cloud credential tag",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: "test-credential-1",
		},
		expectedError: "invalid cloud credential tag",
	}, {
		about: "cloud does not match cloud credential cloud",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
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
	jimmAdmin           bool
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
  users:
  - user: alice@canonical.com
    access: add-model
user-defaults:
- user: alice@canonical.com
  defaults:
    key4: value4
cloud-defaults:
- user: alice@canonical.com
  cloud: test-cloud
  region: test-region-1
  defaults:
    key1: value1
    key2: value2
- user: alice@canonical.com
  cloud: test-cloud
  defaults:
    key3: value3
cloud-credentials:
- name: test-credential-1
  owner: alice@canonical.com
  cloud: test-cloud
  auth-type: empty
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-1
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
- user: alice@canonical.com
  access: admin
- user: bob
  access: read
`[1:])),
	username:  "alice@canonical.com",
	jimmAdmin: true,
	args: jujuparams.ModelCreateArgs{
		Name:               "test-model",
		OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-region-1",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1").String(),
	},
	expectModel: dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner: dbmodel.Identity{
			Name: "alice@canonical.com",
		},
		Controller: dbmodel.Controller{
			Name:        "controller-2",
			UUID:        "00000000-0000-0000-0000-0000-0000000000002",
			CloudName:   "test-cloud",
			CloudRegion: "test-region-1",
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
		Life: state.Alive.String(),
		Status: dbmodel.Status{
			Status: "started",
			Info:   "running a test",
		},
	},
}, {
	name: "CreateModelWithoutCloudRegion",
	env: `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
  users:
  - user: alice@canonical.com
    access: add-model
user-defaults:
- user: alice@canonical.com
  defaults:
    key4: value4
cloud-defaults:
- user: alice@canonical.com
  cloud: test-cloud
  region: test-region-1
  defaults:
    key1: value1
    key2: value2
- user: alice@canonical.com
  cloud: test-cloud
  defaults:
    key3: value3
cloud-credentials:
- name: test-credential-1
  owner: alice@canonical.com
  cloud: test-cloud
  auth-type: empty
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-1
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
- user: alice@canonical.com
  access: admin
- user: bob
  access: read
`[1:])),
	username:  "alice@canonical.com",
	jimmAdmin: true,
	args: jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: names.NewUserTag("alice@canonical.com").String(),
		CloudTag: names.NewCloudTag("test-cloud").String(),
		// Creating a model without specifying the cloud region
		CloudRegion:        "",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1").String(),
	},
	expectModel: dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner: dbmodel.Identity{
			Name: "alice@canonical.com",
		},
		Controller: dbmodel.Controller{
			Name:        "controller-2",
			UUID:        "00000000-0000-0000-0000-0000-0000000000002",
			CloudName:   "test-cloud",
			CloudRegion: "test-region-1",
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
		Life: state.Alive.String(),
		Status: dbmodel.Status{
			Status: "started",
			Info:   "running a test",
		},
	},
}, {
	name: "CreateModelWithCloud",
	env: `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
  users:
  - user: alice@canonical.com
    access: add-model
cloud-credentials:
- name: test-credential-1
  owner: alice@canonical.com
  cloud: test-cloud
  auth-type: empty
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-1
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
- user: alice@canonical.com
  access: admin
- user: bob
  access: read
`[1:]),
	username:  "alice@canonical.com",
	jimmAdmin: true,
	args: jujuparams.ModelCreateArgs{
		Name:               "test-model",
		OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-region-1",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1").String(),
	},
	expectModel: dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner: dbmodel.Identity{
			Name: "alice@canonical.com",
		},
		Controller: dbmodel.Controller{
			Name:        "controller-2",
			UUID:        "00000000-0000-0000-0000-0000-0000000000002",
			CloudName:   "test-cloud",
			CloudRegion: "test-region-1",
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
		Life: state.Alive.String(),
		Status: dbmodel.Status{
			Status: "started",
			Info:   "running a test",
		},
	},
}, {
	name: "CreateModelInOtherNamespaceAsSuperUser",
	env: `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: add-model
cloud-credentials:
- name: test-credential-1
  owner: alice@canonical.com
  cloud: test-cloud
  auth-type: empty
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 2
users:
- username: alice@canonical.com
  controller-access: superuser
- username: bob@canonical.com
  controller-access: login
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
- user: alice@canonical.com
  access: admin
- user: bob
  access: read
`[1:]),
	username:  "alice@canonical.com",
	jimmAdmin: true,
	args: jujuparams.ModelCreateArgs{
		Name:               "test-model",
		OwnerTag:           names.NewUserTag("bob@canonical.com").String(),
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-region-1",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1").String(),
	},
	expectModel: dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner: dbmodel.Identity{
			Name: "bob@canonical.com",
		},
		Controller: dbmodel.Controller{
			Name:        "controller-2",
			UUID:        "00000000-0000-0000-0000-0000-0000000000002",
			CloudName:   "test-cloud",
			CloudRegion: "test-region-1",
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
		Life: state.Alive.String(),
		Status: dbmodel.Status{
			Status: "started",
			Info:   "running a test",
		},
	},
}, {
	name: "CreateModelInOtherNamespace",
	env: `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
  users:
  - user: alice@canonical.com
    access: add-model
cloud-credentials:
- name: test-credential-1
  owner: alice@canonical.com
  cloud: test-cloud
  auth-type: empty
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 2
users:
- username: alice@canonical.com
  controller-access: login
- username: bob@canonical.com
  controller-access: login
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
- user: alice@canonical.com
  access: admin
- user: bob
  access: read
`[1:]),
	username: "alice@canonical.com",
	args: jujuparams.ModelCreateArgs{
		Name:               "test-model",
		OwnerTag:           names.NewUserTag("bob@canonical.com").String(),
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-region-1",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1").String(),
	},
	expectError: "unauthorized",
}, {
	name: "CreateModelError",
	env: `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
  users:
  - user: alice@canonical.com
    access: add-model
cloud-credentials:
- name: test-credential-1
  owner: alice@canonical.com
  cloud: test-cloud
  auth-type: empty
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  regions: test-region-1
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
	username:  "alice@canonical.com",
	jimmAdmin: true,
	args: jujuparams.ModelCreateArgs{
		Name:               "test-model",
		OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-region-1",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1").String(),
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
  users:
  - user: alice@canonical.com
    access: add-model
- name: test-cloud-2
  type: test-provider
  regions:
  - name: test-region-2
cloud-credentials:
- name: test-credential-1
  owner: alice@canonical.com
  cloud: test-cloud
  auth-type: empty
- name: test-credential-2
  owner: alice@canonical.com
  cloud: test-cloud-2
  auth-type: empty
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-000000000002
  cloud: test-cloud-2
  region: test-region-2
  cloud-regions:
  - cloud: test-cloud-2
    region: test-region-2
    priority: 1
models:
- name: test-model
  owner: alice@canonical.com
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
- user: alice@canonical.com
  access: admin
- user: bob
  access: read
`[1:]),
	username:  "alice@canonical.com",
	jimmAdmin: true,
	args: jujuparams.ModelCreateArgs{
		Name:               "test-model",
		OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-region-1",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1").String(),
	},
	expectError: "model alice@canonical.com/test-model already exists",
}, {
	name: "UpdateCredentialError",
	env: `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
  users:
  - user: alice@canonical.com
    access: add-model
cloud-credentials:
- name: test-credential-1
  owner: alice@canonical.com
  cloud: test-cloud
  auth-type: empty
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-1
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
- user: alice@canonical.com
  access: admin
- user: bob
  access: read
`[1:]),
	username:  "alice@canonical.com",
	jimmAdmin: true,
	args: jujuparams.ModelCreateArgs{
		Name:               "test-model",
		OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-region-1",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1").String(),
	},
	expectError: "failed to update cloud credential: a silly error",
}, {
	name: "UserWithoutAddModelPermission",
	env: `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
cloud-credentials:
- name: test-credential-1
  owner: alice@canonical.com
  cloud: test-cloud
  auth-type: empty
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 1
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
- user: alice@canonical.com
  access: admin
`[1:]),
	username:  "alice@canonical.com",
	jimmAdmin: true,
	args: jujuparams.ModelCreateArgs{
		Name:               "test-model",
		OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-region-1",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1").String(),
	},
	expectError: "unauthorized",
}, {
	name: "CreateModelWithImplicitCloud",
	env: `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
  users:
  - user: alice@canonical.com
    access: add-model
user-defaults:
- user: alice@canonical.com
  defaults:
    key4: value4
cloud-defaults:
- user: alice@canonical.com
  cloud: test-cloud
  region: test-region-1
  defaults:
    key1: value1
    key2: value2
- user: alice@canonical.com
  cloud: test-cloud
  defaults:
    key3: value3
cloud-credentials:
- name: test-credential-1
  owner: alice@canonical.com
  cloud: test-cloud
  auth-type: empty
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-1
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
		"key4": "value4",
	}, createModel(`
uuid: 00000001-0000-0000-0000-0000-000000000001
status:
  status: started
  info: running a test
life: alive
users:
- user: alice@canonical.com
  access: admin
- user: bob
  access: read
`[1:])),
	username:  "alice@canonical.com",
	jimmAdmin: true,
	args: jujuparams.ModelCreateArgs{
		Name:               "test-model",
		OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1").String(),
	},
	expectModel: dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner: dbmodel.Identity{
			Name: "alice@canonical.com",
		},
		Controller: dbmodel.Controller{
			Name:        "controller-2",
			UUID:        "00000000-0000-0000-0000-0000-0000000000002",
			CloudName:   "test-cloud",
			CloudRegion: "test-region-1",
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
		Life: state.Alive.String(),
		Status: dbmodel.Status{
			Status: "started",
			Info:   "running a test",
		},
	},
}, {
	name: "CreateModelWithImplicitCloudAndMultipleClouds",
	env: `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
  users:
  - user: alice@canonical.com
    access: add-model
- name: test-cloud-2
  type: test-provider-2
  regions:
  - name: test-region-2
  users:
  - user: alice@canonical.com
    access: add-model
user-defaults:
- user: alice@canonical.com
  defaults:
    key4: value4
cloud-defaults:
- user: alice@canonical.com
  cloud: test-cloud
  region: test-region-1
  defaults:
    key1: value1
    key2: value2
- user: alice@canonical.com
  cloud: test-cloud
  defaults:
    key3: value3
cloud-credentials:
- name: test-credential-1
  owner: alice@canonical.com
  cloud: test-cloud
  auth-type: empty
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-1
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
		"key4": "value4",
	}, createModel(`
uuid: 00000001-0000-0000-0000-0000-000000000001
status:
  status: started
  info: running a test
life: alive
users:
- user: alice@canonical.com
  access: admin
- user: bob
  access: read
`[1:])),
	username:  "alice@canonical.com",
	jimmAdmin: true,
	args: jujuparams.ModelCreateArgs{
		Name:               "test-model",
		OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1").String(),
	},
	expectError: "no cloud specified for model; please specify one",
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

			client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name(), test.name)
			c.Assert(err, qt.IsNil)

			j := &jimm.JIMM{
				UUID: uuid.NewString(),
				Database: db.Database{
					DB: jimmtest.PostgresDB(c, nil),
				},
				Dialer: &jimmtest.Dialer{
					API: api,
				},
				OpenFGAClient: client,
			}
			ctx := context.Background()
			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, test.env)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

			dbUser := env.User(test.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, client)
			user.JimmAdmin = test.jimmAdmin

			args := jimm.ModelCreateArgs{}
			err = args.FromJujuModelCreateArgs(&test.args)
			c.Assert(err, qt.IsNil)

			_, err = j.AddModel(context.Background(), user, &args)
			if test.expectError == "" {
				c.Assert(err, qt.IsNil)

				m1 := dbmodel.Model{
					UUID: test.expectModel.UUID,
				}
				err = j.Database.GetModel(ctx, &m1)
				c.Assert(err, qt.IsNil)
				c.Assert(m1, jimmtest.DBObjectEquals, test.expectModel)

				ownerId := args.Owner.Id()
				if ownerId == "" {
					ownerId = user.Tag().Id()
				}

				ownerIdentity, err := dbmodel.NewIdentity(ownerId)
				c.Assert(err, qt.IsNil)
				isModelAdmin, err := openfga.IsAdministrator(
					context.Background(),
					openfga.NewUser(
						ownerIdentity,
						client,
					),
					m1.ResourceTag(),
				)
				c.Assert(err, qt.IsNil)
				c.Assert(isModelAdmin, qt.IsTrue)

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
		if args.CloudTag == "" {
			return errors.E("cloud not specified")
		}
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

const getModelTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: alive
  status:
    status: available
    info: "OK!"
    since: 2020-02-20T20:02:20Z
  sla:
    level: unsupported
  agent-version: 1.2.3
`

func TestGetModel(t *testing.T) {
	ctx := context.Background()
	c := qt.New(t)

	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name(), t.Name())
	c.Assert(err, qt.IsNil)

	j := &jimm.JIMM{
		UUID:          uuid.NewString(),
		OpenFGAClient: client,
		Database: db.Database{
			DB: jimmtest.PostgresDB(c, nil),
		},
	}
	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, getModelTestEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

	// Get model
	model, err := j.GetModel(ctx, env.Models[0].UUID)
	c.Assert(err, qt.IsNil)
	c.Assert(model.UUID.String, qt.Equals, env.Models[0].UUID)
	c.Assert(model.Name, qt.Equals, env.Models[0].Name)
	c.Assert(model.ControllerID, qt.Equals, env.Models[0].DBObject(c, j.Database).ControllerID)

	// Get model that doesn't exist
	_, err = j.GetModel(ctx, "fake-uuid")
	c.Assert(err, qt.ErrorMatches, "failed to get model: model not found")
}

// Note that this env does not give the everyone user access to the model.
const modelInfoTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: alive
  status:
    status: available
    info: "OK!"
    since: 2020-02-20T20:02:20Z
  sla:
    level: unsupported
  agent-version: 1.2.3
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: write
  - user: charlie@canonical.com
    access: read
`

// This env extends the one above to provide the everyone user with access to the model.
const modelInfoTestEnvWithEveryoneAccess = modelInfoTestEnv + `
  - user: everyone@external
    access: read
`

func modelInfoTestExpectedModelInfo(canReadMachineInfo bool, limitedExpectedUsers []jujuparams.ModelUserInfo) *jujuparams.ModelInfo {
	info := jujuparams.ModelInfo{
		Name:               "model-1",
		Type:               "iaas",
		UUID:               "00000002-0000-0000-0000-000000000001",
		ControllerUUID:     "00000001-0000-0000-0000-000000000001",
		ProviderType:       "test-provider",
		DefaultSeries:      "warty",
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-cloud-region",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/cred-1").String(),
		OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
		Life:               life.Value(state.Alive.String()),
		Status: jujuparams.EntityStatus{
			Status: "available",
			Info:   "OK!",
			Since:  newDate(2020, 2, 20, 20, 2, 20, 0, time.UTC),
		},
		Users: []jujuparams.ModelUserInfo{{
			UserName: "alice@canonical.com",
			Access:   "admin",
		}, {
			UserName: "bob@canonical.com",
			Access:   "write",
		}, {
			UserName: "charlie@canonical.com",
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
	}
	if !canReadMachineInfo {
		info.Machines = nil
	}
	if limitedExpectedUsers != nil {
		info.Users = limitedExpectedUsers
	}
	return &info
}

var modelInfoTests = []struct {
	name             string
	env              string
	username         string
	uuid             string
	originModelOwner string
	expectModelInfo  *jujuparams.ModelInfo
	expectError      string
}{{
	name:             "AdminUser",
	env:              modelInfoTestEnv,
	username:         "alice@canonical.com",
	uuid:             "00000002-0000-0000-0000-000000000001",
	originModelOwner: names.NewUserTag("alice@canonical.com").String(),
	expectModelInfo:  modelInfoTestExpectedModelInfo(true, nil),
}, {
	name:             "WriteUser",
	env:              modelInfoTestEnv,
	username:         "bob@canonical.com",
	uuid:             "00000002-0000-0000-0000-000000000001",
	originModelOwner: names.NewUserTag("alice@canonical.com").String(),
	expectModelInfo: modelInfoTestExpectedModelInfo(true, []jujuparams.ModelUserInfo{{
		UserName: "bob@canonical.com",
		Access:   "write",
	}}),
}, {
	name:             "ReadUser",
	env:              modelInfoTestEnv,
	username:         "charlie@canonical.com",
	uuid:             "00000002-0000-0000-0000-000000000001",
	originModelOwner: names.NewUserTag("alice@canonical.com").String(),
	expectModelInfo: modelInfoTestExpectedModelInfo(false, []jujuparams.ModelUserInfo{{
		UserName: "charlie@canonical.com",
		Access:   "read",
	}}),
}, {
	name:        "NoAccess",
	env:         modelInfoTestEnv,
	username:    "diane@canonical.com",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: "unauthorized",
}, {
	name:        "NotFound",
	env:         modelInfoTestEnv,
	username:    "alice@canonical.com",
	uuid:        "00000002-0000-0000-0000-000000000002",
	expectError: "model not found",
}, {
	name:             "Access through everyone user",
	env:              modelInfoTestEnvWithEveryoneAccess,
	username:         "diane@canonical.com",
	uuid:             "00000002-0000-0000-0000-000000000001",
	originModelOwner: names.NewUserTag("alice@canonical.com").String(),
	expectModelInfo: modelInfoTestExpectedModelInfo(false, []jujuparams.ModelUserInfo{{
		UserName: "everyone@external",
		Access:   "read",
	}}),
}, {
	name:             "Owner field is replaced",
	env:              modelInfoTestEnv,
	username:         "alice@canonical.com",
	uuid:             "00000002-0000-0000-0000-000000000001",
	originModelOwner: names.NewUserTag("bob").String(),
	expectModelInfo:  modelInfoTestExpectedModelInfo(true, nil),
},
}

func TestModelInfo(t *testing.T) {
	c := qt.New(t)

	for _, test := range modelInfoTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name(), test.name)
			c.Assert(err, qt.IsNil)

			j := &jimm.JIMM{
				UUID:          uuid.NewString(),
				OpenFGAClient: client,
				Database: db.Database{
					DB: jimmtest.PostgresDB(c, nil),
				},
				Dialer: &jimmtest.Dialer{
					API: &jimmtest.API{
						ModelInfo_: func(_ context.Context, mi *jujuparams.ModelInfo) error {
							mi.Name = "model-1"
							mi.Type = "iaas"
							mi.ControllerUUID = "00000001-0000-0000-0000-000000000001"
							mi.ProviderType = "test-provider"
							mi.DefaultSeries = "warty"
							mi.CloudTag = names.NewCloudTag("test-cloud").String()
							mi.CloudRegion = "test-cloud-region"
							mi.CloudCredentialTag = names.NewCloudCredentialTag("test-cloud/alice@canonical.com/cred-1").String()
							mi.OwnerTag = test.originModelOwner
							mi.Life = life.Value(state.Alive.String())
							mi.Status = jujuparams.EntityStatus{
								Status: "available",
								Info:   "OK!",
								Since:  newDate(2020, 2, 20, 20, 2, 20, 0, time.UTC),
							}
							// Note that users are populated from OpenFGA
							mi.Users = []jujuparams.ModelUserInfo{}
							mi.Machines = []jujuparams.ModelMachineInfo{{
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
							}}
							mi.SLA = &jujuparams.ModelSLAInfo{
								Level: "unsupported",
							}
							mi.AgentVersion = newVersion("1.2.3")
							return nil
						},
					},
				},
			}
			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, test.env)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

			dbUser, err := dbmodel.NewIdentity(test.username)
			c.Assert(err, qt.IsNil)

			user := openfga.NewUser(dbUser, client)

			mi, err := j.ModelInfo(context.Background(), user, names.NewModelTag(test.uuid))
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
			} else {
				c.Assert(err, qt.IsNil)
				sort.Slice(mi.Users, func(i, j int) bool {
					return mi.Users[i].UserName < mi.Users[j].UserName
				})
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
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: write
  - user: charlie@canonical.com
    access: read
users:
- username: diane@canonical.com
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
	username:    "alice@canonical.com",
	uuid:        "00000001-0000-0000-0000-000000000001",
	expectError: `model not found`,
}, {
	name:        "UnauthorizedUser",
	env:         modelStatusTestEnv,
	username:    "bob@canonical.com",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: "unauthorized",
}, {
	name: "Success",
	env:  modelStatusTestEnv,
	modelStatus: func(_ context.Context, ms *jujuparams.ModelStatus) error {
		if ms.ModelTag != names.NewModelTag("00000002-0000-0000-0000-000000000001").String() {
			return errors.E("incorrect model tag")
		}
		ms.Life = life.Value(state.Alive.String())
		ms.Type = "iaas"
		ms.HostedMachineCount = 10
		ms.ApplicationCount = 3
		ms.UnitCount = 20
		ms.OwnerTag = names.NewUserTag("alice@canonical.com").String()
		return nil
	},
	username: "alice@canonical.com",
	uuid:     "00000002-0000-0000-0000-000000000001",
	expectModelStatus: &jujuparams.ModelStatus{
		ModelTag:           names.NewModelTag("00000002-0000-0000-0000-000000000001").String(),
		Life:               life.Value(state.Alive.String()),
		Type:               "iaas",
		HostedMachineCount: 10,
		ApplicationCount:   3,
		UnitCount:          20,
		OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
	},
}, {
	name: "APIError",
	env:  modelStatusTestEnv,
	modelStatus: func(_ context.Context, ms *jujuparams.ModelStatus) error {
		return errors.E("test error")
	},
	username:    "alice@canonical.com",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: "test error",
}}

func TestModelStatus(t *testing.T) {
	c := qt.New(t)

	for _, test := range modelStatusTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					ModelStatus_: test.modelStatus,
				},
			}

			client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name(), test.name)
			c.Assert(err, qt.IsNil)

			j := &jimm.JIMM{
				UUID:          uuid.NewString(),
				OpenFGAClient: client,
				Database: db.Database{
					DB: jimmtest.PostgresDB(c, nil),
				},
				Dialer: dialer,
			}
			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, test.env)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

			dbUser := env.User(test.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, client)

			ms, err := j.ModelStatus(context.Background(), user, names.NewModelTag(test.uuid))
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
			} else {
				c.Assert(err, qt.IsNil)
				c.Check(ms, qt.CmpEquals(cmpopts.EquateEmpty()), test.expectModelStatus)
			}

			c.Check(dialer.IsClosed(), qt.IsTrue)
		})
	}
}

const forEachModelTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
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
    access: admin
  sla:
    level: unsupported
  agent-version: 1.2.3
  cores: 3
  machines: 2
  units: 4
- name: model-2
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000002
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
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
  sla:
    level: unsupported
  agent-version: 1.2.3
  cores: 3
  machines: 2
- name: model-3
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000003
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: alive
  status:
    status: available
    info: "OK!"
    since: 2020-02-20T20:02:20Z
  users:
  - user: alice@canonical.com
    access: admin
  sla:
    level: unsupported
  agent-version: 1.2.3
  cores: 3
  machines: 2
- name: model-4
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000004
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
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
    access: read
  sla:
    level: unsupported
  agent-version: 1.2.3
users:
- username: alice@canonical.com
  controller-access: superuser
`

func TestForEachUserModel(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	j := &jimm.JIMM{
		UUID:          uuid.NewString(),
		OpenFGAClient: client,
		Database: db.Database{
			DB: jimmtest.PostgresDB(c, nil),
		},
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{},
		},
	}

	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, forEachModelTestEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

	dbUser := env.User("bob@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, client)

	var res []jujuparams.ModelSummaryResult
	err = j.ForEachUserModel(ctx, user, func(m *dbmodel.Model, access jujuparams.UserAccessPermission) error {
		s := m.ToJujuModelSummary()
		s.UserAccess = access
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
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/cred-1").String(),
			OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
			Life:               life.Value(state.Alive.String()),
			Status: jujuparams.EntityStatus{
				Status: "available",
				Info:   "OK!",
				Since:  newDate(2020, 02, 20, 20, 02, 20, 0, time.UTC),
			},
			UserAccess: "admin",
			Counts: []jujuparams.ModelEntityCount{{
				Entity: "machines",
				Count:  2,
			}, {
				Entity: "cores",
				Count:  3,
			}, {
				Entity: "units",
				Count:  4,
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
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/cred-1").String(),
			OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
			Life:               life.Value(state.Alive.String()),
			Status: jujuparams.EntityStatus{
				Status: "available",
				Info:   "OK!",
				Since:  newDate(2020, 02, 20, 20, 02, 20, 0, time.UTC),
			},
			UserAccess: "write",
			Counts: []jujuparams.ModelEntityCount{{
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
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/cred-1").String(),
			OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
			Life:               life.Value(state.Alive.String()),
			Status: jujuparams.EntityStatus{
				Status: "available",
				Info:   "OK!",
				Since:  newDate(2020, 02, 20, 20, 02, 20, 0, time.UTC),
			},
			UserAccess: "read",
			Counts: []jujuparams.ModelEntityCount{{
				Entity: "machines",
				Count:  0,
			}, {
				Entity: "cores",
				Count:  0,
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

	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	j := &jimm.JIMM{
		UUID:          uuid.NewString(),
		OpenFGAClient: client,
		Database: db.Database{
			DB: jimmtest.PostgresDB(c, nil),
		},
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{},
		},
	}
	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, forEachModelTestEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

	dbUser := env.User("bob@canonical.com").DBObject(c, j.Database)
	bob := openfga.NewUser(&dbUser, client)

	err = j.ForEachModel(ctx, bob, func(_ *dbmodel.Model, _ jujuparams.UserAccessPermission) error {
		return errors.E("function called unexpectedly")
	})
	c.Check(err, qt.ErrorMatches, `unauthorized`)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)

	dbUser = env.User("alice@canonical.com").DBObject(c, j.Database)
	alice := openfga.NewUser(&dbUser, client)
	alice.JimmAdmin = true

	var models []string
	err = j.ForEachModel(ctx, alice, func(m *dbmodel.Model, access jujuparams.UserAccessPermission) error {
		c.Check(access, qt.Equals, jujuparams.UserAccessPermission("admin"))
		models = append(models, m.UUID.String)
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
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  users:
  - user: alice@canonical.com
    access: admin
  - user: charlie@canonical.com
    access: write
`

var grantModelAccessTests = []struct {
	name            string
	env             string
	dialError       error
	username        string
	uuid            string
	targetUsername  string
	access          string
	expectRelations []openfga.Tuple
	expectError     string
	expectErrorCode errors.Code
}{{
	name:            "ModelNotFound",
	username:        "alice@canonical.com",
	uuid:            "00000002-0000-0000-0000-000000000001",
	targetUsername:  "bob@canonical.com",
	access:          "write",
	expectError:     `model not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name:           "Admin grants 'admin' access to a user with no access",
	env:            grantModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@canonical.com",
	access:         "admin",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin grants 'write' access to a user with no access",
	env:            grantModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@canonical.com",
	access:         "write",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin grants 'read' access to a user with no access",
	env:            grantModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@canonical.com",
	access:         "read",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin grants 'write' access to a user who already has 'write' access",
	env:            grantModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "charlie@canonical.com",
	access:         "write",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin grants 'read' access to a user who already has 'write' access",
	env:            grantModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "charlie@canonical.com",
	access:         "read",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin grants 'admin' access to themselves",
	env:            grantModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "alice@canonical.com",
	access:         "admin",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin grants 'write' access to themselves",
	env:            grantModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "alice@canonical.com",
	access:         "write",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin grants 'read' access to themselves",
	env:            grantModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "alice@canonical.com",
	access:         "read",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:            "UserNotAuthorized",
	env:             grantModelAccessTestEnv,
	username:        "charlie@canonical.com",
	uuid:            "00000002-0000-0000-0000-000000000001",
	targetUsername:  "bob@canonical.com",
	access:          "write",
	expectError:     `unauthorized`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:           "DialError",
	env:            grantModelAccessTestEnv,
	dialError:      errors.E("test dial error"),
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@canonical.com",
	access:         "write",
	expectError:    `test dial error`,
}, {
	name:           "unknown access",
	env:            grantModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@canonical.com",
	access:         "some-unknown-access",
	expectError:    `failed to recognize given access: "some-unknown-access"`,
}}

func TestGrantModelAccess(t *testing.T) {
	c := qt.New(t)

	for _, t := range grantModelAccessTests {
		tt := t
		c.Run(tt.name, func(c *qt.C) {
			ctx := context.Background()

			client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name(), tt.name)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, tt.env)
			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{},
				Err: tt.dialError,
			}
			j := &jimm.JIMM{
				UUID: jimmtest.ControllerUUID,
				Database: db.Database{
					DB: jimmtest.PostgresDB(c, nil),
				},
				Dialer:        dialer,
				OpenFGAClient: client,
			}
			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

			dbUser := env.User(tt.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, client)

			err = j.GrantModelAccess(ctx, user, names.NewModelTag(tt.uuid), names.NewUserTag(tt.targetUsername), jujuparams.UserAccessPermission(tt.access))
			c.Assert(dialer.IsClosed(), qt.IsTrue)
			if tt.expectError != "" {
				c.Check(err, qt.ErrorMatches, tt.expectError)
				if tt.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, tt.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			for _, tuple := range tt.expectRelations {
				value, err := client.CheckRelation(ctx, tuple, false)
				c.Assert(err, qt.IsNil)
				c.Assert(value, qt.IsTrue, qt.Commentf("expected the tuple to exist after granting"))
			}
		})
	}
}

const revokeModelAccessTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: admin
  - user: charlie@canonical.com
    access: write
  - user: daphne@canonical.com
    access: read
- name: model-2
  uuid: 00000002-0000-0000-0000-000000000002
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  users:
  - user: alice@canonical.com
    access: admin
  - user: earl@canonical.com
    access: admin
`

var revokeModelAccessTests = []struct {
	name                   string
	env                    string
	dialError              error
	username               string
	uuid                   string
	targetUsername         string
	access                 string
	extraInitialTuples     []openfga.Tuple
	expectRelations        []openfga.Tuple
	expectRemovedRelations []openfga.Tuple
	expectError            string
	expectErrorCode        errors.Code
}{{
	name:            "ModelNotFound",
	username:        "alice@canonical.com",
	uuid:            "00000002-0000-0000-0000-000000000001",
	targetUsername:  "bob@canonical.com",
	access:          "write",
	expectError:     `model not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name:           "Admin revokes 'admin' access from another admin",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@canonical.com",
	access:         "admin",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'write' access from another admin",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@canonical.com",
	access:         "write",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'read' access from another admin",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@canonical.com",
	access:         "read",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'admin' access from a user who has 'write' access",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "charlie@canonical.com",
	access:         "admin",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'write' access from a user who has 'write' access",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "charlie@canonical.com",
	access:         "write",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'read' access from a user who has 'write' access",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "charlie@canonical.com",
	access:         "read",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'admin' access from a user who has 'read' access",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "daphne@canonical.com",
	access:         "admin",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'write' access from a user who has 'read' access",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "daphne@canonical.com",
	access:         "write",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'read' access from a user who has 'read' access",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "daphne@canonical.com",
	access:         "read",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'admin' access from themselves",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "alice@canonical.com",
	access:         "admin",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'write' access from themselves",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "alice@canonical.com",
	access:         "write",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'read' access from themselves",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "alice@canonical.com",
	access:         "read",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Writer revokes 'admin' access from themselves",
	env:            revokeModelAccessTestEnv,
	username:       "charlie@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "charlie@canonical.com",
	access:         "admin",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Writer revokes 'write' access from themselves",
	env:            revokeModelAccessTestEnv,
	username:       "charlie@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "charlie@canonical.com",
	access:         "write",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Writer revokes 'read' access from themselves",
	env:            revokeModelAccessTestEnv,
	username:       "charlie@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "charlie@canonical.com",
	access:         "read",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Reader revokes 'admin' access from themselves",
	env:            revokeModelAccessTestEnv,
	username:       "daphne@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "daphne@canonical.com",
	access:         "admin",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Reader revokes 'write' access from themselves",
	env:            revokeModelAccessTestEnv,
	username:       "daphne@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "daphne@canonical.com",
	access:         "write",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Reader revokes 'read' access from themselves",
	env:            revokeModelAccessTestEnv,
	username:       "daphne@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "daphne@canonical.com",
	access:         "read",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'admin' access from a user who has separate tuples for all accesses (read/write/admin)",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "daphne@canonical.com",
	access:         "admin",
	extraInitialTuples: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	},
	// No need to add the 'read' relation, because it's already there due to the environment setup.
	},
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'write' access from a user who has separate tuples for all accesses (read/write/admin)",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "daphne@canonical.com",
	access:         "write",
	extraInitialTuples: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	},
	// No need to add the 'read' relation, because it's already there due to the environment setup.
	},
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'read' access from a user who has separate tuples for all accesses (read/write/admin)",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "daphne@canonical.com",
	access:         "read",
	extraInitialTuples: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	},
	// No need to add the 'read' relation, because it's already there due to the environment setup.
	},
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:            "UserNotAuthorized",
	env:             revokeModelAccessTestEnv,
	username:        "charlie@canonical.com",
	uuid:            "00000002-0000-0000-0000-000000000001",
	targetUsername:  "bob@canonical.com",
	access:          "write",
	expectError:     `unauthorized`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:           "DialError",
	env:            revokeModelAccessTestEnv,
	dialError:      errors.E("test dial error"),
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@canonical.com",
	access:         "write",
	expectError:    `test dial error`,
}, {
	name:           "unknown access",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@canonical.com",
	access:         "some-unknown-access",
	expectError:    `failed to recognize given access: "some-unknown-access"`,
}}

//nolint:gocognit
func TestRevokeModelAccess(t *testing.T) {
	c := qt.New(t)

	for _, t := range revokeModelAccessTests {
		tt := t
		c.Run(tt.name, func(c *qt.C) {
			ctx := context.Background()

			client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name(), tt.name)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, tt.env)
			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{},
				Err: tt.dialError,
			}
			j := &jimm.JIMM{
				UUID: jimmtest.ControllerUUID,
				Database: db.Database{
					DB: jimmtest.PostgresDB(c, nil),
				},
				Dialer:        dialer,
				OpenFGAClient: client,
			}
			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

			if len(tt.extraInitialTuples) > 0 {
				err = client.AddRelation(ctx, tt.extraInitialTuples...)
				c.Assert(err, qt.IsNil)
			}

			if tt.expectRemovedRelations != nil {
				for _, tuple := range tt.expectRemovedRelations {
					value, err := client.CheckRelation(ctx, tuple, false)
					c.Assert(err, qt.IsNil)
					c.Assert(value, qt.IsTrue, qt.Commentf("expected the tuple to exist before revoking"))
				}
			}

			dbUser := env.User(tt.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, client)

			err = j.RevokeModelAccess(ctx, user, names.NewModelTag(tt.uuid), names.NewUserTag(tt.targetUsername), jujuparams.UserAccessPermission(tt.access))
			c.Assert(dialer.IsClosed(), qt.IsTrue)
			if tt.expectError != "" {
				c.Check(err, qt.ErrorMatches, tt.expectError)
				if tt.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, tt.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			if tt.expectRemovedRelations != nil {
				for _, tuple := range tt.expectRemovedRelations {
					value, err := client.CheckRelation(ctx, tuple, false)
					c.Assert(err, qt.IsNil)
					c.Assert(value, qt.IsFalse, qt.Commentf("expected the tuple to be removed after revoking"))
				}
			}
			if tt.expectRelations != nil {
				for _, tuple := range tt.expectRelations {
					value, err := client.CheckRelation(ctx, tuple, false)
					c.Assert(err, qt.IsNil)
					c.Assert(value, qt.IsTrue, qt.Commentf("expected the tuple to exist after revoking"))
				}
			}
		})
	}
}

const destroyModelTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: alive
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: write
users:
- username: charlie@canonical.com
  controller-access: superuser
`

var destroyModelTests = []struct {
	name            string
	env             string
	destroyModel    func(context.Context, names.ModelTag, *bool, *bool, *time.Duration, *time.Duration) error
	dialError       error
	username        string
	uuid            string
	destroyStorage  *bool
	force           *bool
	maxWait         *time.Duration
	timeout         *time.Duration
	expectError     string
	expectErrorCode errors.Code
}{{
	name:            "NotFound",
	env:             destroyModelTestEnv,
	username:        "alice@canonical.com",
	uuid:            "00000002-0000-0000-0000-000000000002",
	expectError:     `model not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name:            "Unauthorized",
	env:             destroyModelTestEnv,
	username:        "bob@canonical.com",
	uuid:            "00000002-0000-0000-0000-000000000001",
	expectError:     `unauthorized`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name: "Success",
	env:  destroyModelTestEnv,
	destroyModel: func(_ context.Context, mt names.ModelTag, destroyStorage, force *bool, maxWait, timeout *time.Duration) error {
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
		if timeout == nil || *timeout != time.Second {
			return errors.E("invalid timeout")
		}
		return nil
	},
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	destroyStorage: newBool(true),
	force:          newBool(false),
	maxWait:        newDuration(time.Second),
	timeout:        newDuration(time.Second),
}, {
	name: "SuperuserSuccess",
	env:  destroyModelTestEnv,
	destroyModel: func(_ context.Context, _ names.ModelTag, _, _ *bool, _, _ *time.Duration) error {
		return nil
	},
	username: "charlie@canonical.com",
	uuid:     "00000002-0000-0000-0000-000000000001",
}, {
	name:        "DialError",
	env:         destroyModelTestEnv,
	dialError:   errors.E("dial error"),
	username:    "alice@canonical.com",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: `dial error`,
}, {
	name: "APIError",
	env:  destroyModelTestEnv,
	destroyModel: func(_ context.Context, _ names.ModelTag, _, _ *bool, _, _ *time.Duration) error {
		return errors.E("api error")
	},
	username:    "charlie@canonical.com",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: `api error`,
}}

func TestDestroyModel(t *testing.T) {
	c := qt.New(t)

	for _, test := range destroyModelTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					DestroyModel_: test.destroyModel,
				},
				Err: test.dialError,
			}

			client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name(), test.name)
			c.Assert(err, qt.IsNil)

			j := &jimm.JIMM{
				UUID:          uuid.NewString(),
				OpenFGAClient: client,
				Database: db.Database{
					DB: jimmtest.PostgresDB(c, nil),
				},
				Dialer: dialer,
			}
			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, test.env)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

			dbUser := env.User(test.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, client)

			err = j.DestroyModel(ctx, user, names.NewModelTag(test.uuid), test.destroyStorage, test.force, test.maxWait, test.timeout)
			c.Assert(dialer.IsClosed(), qt.IsTrue)
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
			c.Check(m.Life, qt.Equals, state.Dying.String())
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
	username:        "alice@canonical.com",
	uuid:            "00000002-0000-0000-0000-000000000002",
	expectError:     `model not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name:            "Unauthorized",
	env:             destroyModelTestEnv,
	username:        "bob@canonical.com",
	uuid:            "00000002-0000-0000-0000-000000000001",
	expectError:     `unauthorized`,
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
	username:     "alice@canonical.com",
	uuid:         "00000002-0000-0000-0000-000000000001",
	simplified:   true,
	expectString: "model dump",
}, {
	name: "SuperuserSuccess",
	env:  destroyModelTestEnv,
	dumpModel: func(_ context.Context, _ names.ModelTag, _ bool) (string, error) {
		return "model dump2", nil
	},
	username:     "charlie@canonical.com",
	uuid:         "00000002-0000-0000-0000-000000000001",
	expectString: "model dump2",
}, {
	name:        "DialError",
	env:         destroyModelTestEnv,
	dialError:   errors.E("dial error"),
	username:    "alice@canonical.com",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: `dial error`,
}, {
	name: "APIError",
	env:  destroyModelTestEnv,
	dumpModel: func(_ context.Context, _ names.ModelTag, _ bool) (string, error) {
		return "", errors.E("api error")
	},
	username:    "charlie@canonical.com",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: `api error`,
}}

func TestDumpModel(t *testing.T) {
	c := qt.New(t)

	for _, test := range dumpModelTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name(), test.name)
			c.Assert(err, qt.IsNil)

			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					DumpModel_: test.dumpModel,
				},
				Err: test.dialError,
			}
			j := &jimm.JIMM{
				UUID:          uuid.NewString(),
				OpenFGAClient: client,
				Database: db.Database{
					DB: jimmtest.PostgresDB(c, nil),
				},
				Dialer: dialer,
			}

			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, test.env)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

			dbUser := env.User(test.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, client)

			s, err := j.DumpModel(ctx, user, names.NewModelTag(test.uuid), test.simplified)
			c.Assert(dialer.IsClosed(), qt.IsTrue)
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
	username:        "alice@canonical.com",
	uuid:            "00000002-0000-0000-0000-000000000002",
	expectError:     `model not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name:            "Unauthorized",
	env:             destroyModelTestEnv,
	username:        "bob@canonical.com",
	uuid:            "00000002-0000-0000-0000-000000000001",
	expectError:     `unauthorized`,
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
	username:   "alice@canonical.com",
	uuid:       "00000002-0000-0000-0000-000000000001",
	expectDump: map[string]interface{}{"model": "dump"},
}, {
	name: "SuperuserSuccess",
	env:  destroyModelTestEnv,
	dumpModelDB: func(_ context.Context, _ names.ModelTag) (map[string]interface{}, error) {
		return map[string]interface{}{"model": "dump 2"}, nil
	},
	username:   "charlie@canonical.com",
	uuid:       "00000002-0000-0000-0000-000000000001",
	expectDump: map[string]interface{}{"model": "dump 2"},
}, {
	name:        "DialError",
	env:         destroyModelTestEnv,
	dialError:   errors.E("dial error"),
	username:    "alice@canonical.com",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: `dial error`,
}, {
	name: "APIError",
	env:  destroyModelTestEnv,
	dumpModelDB: func(_ context.Context, _ names.ModelTag) (map[string]interface{}, error) {
		return nil, errors.E("api error")
	},
	username:    "charlie@canonical.com",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: `api error`,
}}

func TestDumpModelDB(t *testing.T) {
	c := qt.New(t)

	for _, test := range dumpModelDBTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name(), test.name)
			c.Assert(err, qt.IsNil)

			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					DumpModelDB_: test.dumpModelDB,
				},
				Err: test.dialError,
			}
			j := &jimm.JIMM{
				UUID:          uuid.NewString(),
				OpenFGAClient: client,
				Database: db.Database{
					DB: jimmtest.PostgresDB(c, nil),
				},
				Dialer: dialer,
			}

			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, test.env)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

			dbUser := env.User(test.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, client)

			dump, err := j.DumpModelDB(ctx, user, names.NewModelTag(test.uuid))
			c.Assert(dialer.IsClosed(), qt.IsTrue)
			if test.expectError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Assert(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			c.Assert(dump, qt.DeepEquals, test.expectDump)
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
	username:        "alice@canonical.com",
	uuid:            "00000002-0000-0000-0000-000000000002",
	expectError:     `model not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name:            "Unauthorized",
	env:             destroyModelTestEnv,
	username:        "bob@canonical.com",
	uuid:            "00000002-0000-0000-0000-000000000001",
	expectError:     `unauthorized`,
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
	username: "alice@canonical.com",
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
	username: "charlie@canonical.com",
	uuid:     "00000002-0000-0000-0000-000000000001",
}, {
	name:        "DialError",
	env:         destroyModelTestEnv,
	dialError:   errors.E("dial error"),
	username:    "alice@canonical.com",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: `dial error`,
}, {
	name: "APIError",
	env:  destroyModelTestEnv,
	validateModelUpgrade: func(_ context.Context, _ names.ModelTag, _ bool) error {
		return errors.E("api error")
	},
	username:    "charlie@canonical.com",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: `api error`,
}}

func TestValidateModelUpgrade(t *testing.T) {
	c := qt.New(t)

	for _, test := range validateModelUpgradeTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name(), test.name)
			c.Assert(err, qt.IsNil)

			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					ValidateModelUpgrade_: test.validateModelUpgrade,
				},
				Err: test.dialError,
			}

			j := &jimm.JIMM{
				UUID:          uuid.NewString(),
				OpenFGAClient: client,
				Database: db.Database{
					DB: jimmtest.PostgresDB(c, nil),
				},
				Dialer: dialer,
			}

			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, test.env)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

			dbUser := env.User(test.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, client)

			err = j.ValidateModelUpgrade(ctx, user, names.NewModelTag(test.uuid), test.force)
			c.Assert(dialer.IsClosed(), qt.IsTrue)
			if test.expectError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Assert(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
		})
	}
}

//nolint:gosec // Thinks credentials hardcoded.
const updateModelCredentialTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@canonical.com
  name: cred-2
  cloud: test-cloud
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  users:
  - user: alice@canonical.com
    access: admin
  - user: charlie@canonical.com
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
		if taggedCredential.Tag != "cloudcred-test-cloud_alice@canonical.com_cred-2" {
			return nil, errors.E("bad cloud credential tag")
		}
		return nil, nil
	},
	changeModelCredential: func(_ context.Context, modelTag names.ModelTag, credentialTag names.CloudCredentialTag) error {
		if modelTag.Id() != "00000002-0000-0000-0000-000000000001" {
			return errors.E("bad model tag")
		}
		if credentialTag.Id() != "test-cloud/alice@canonical.com/cred-2" {
			return errors.E("bad cloud credential tag")
		}
		return nil
	},
	username:   "alice@canonical.com",
	credential: "test-cloud/alice@canonical.com/cred-2",
	uuid:       "00000002-0000-0000-0000-000000000001",
	expectModel: dbmodel.Model{
		Name: "model-1",
		UUID: sql.NullString{
			String: "00000002-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner: dbmodel.Identity{
			Name: "alice@canonical.com",
		},
		Controller: dbmodel.Controller{
			Name:        "controller-1",
			UUID:        "00000001-0000-0000-0000-000000000001",
			CloudName:   "test-cloud",
			CloudRegion: "test-cloud-region",
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
	},
}, {
	name: "user not admin",
	env:  updateModelCredentialTestEnv,
	updateCredential: func(_ context.Context, taggedCredential jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
		if taggedCredential.Tag != "cloudcred-test-cloud_alice@canonical.com_cred-2" {
			return nil, errors.E("bad cloud credential tag")
		}
		return nil, nil
	},
	changeModelCredential: func(_ context.Context, modelTag names.ModelTag, credentialTag names.CloudCredentialTag) error {
		if modelTag.Id() != "00000002-0000-0000-0000-000000000001" {
			return errors.E("bad model tag")
		}
		if credentialTag.Id() != "test-cloud/alice@canonical.com/cred-2" {
			return errors.E("bad cloud credential tag")
		}
		return nil
	},
	username:        "charlie@canonical.com",
	credential:      "test-cloud/alice@canonical.com/cred-2",
	uuid:            "00000002-0000-0000-0000-000000000001",
	expectError:     "unauthorized",
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:            "model not found",
	env:             updateModelCredentialTestEnv,
	username:        "charlie@canonical.com",
	credential:      "test-cloud/alice@canonical.com/cred-2",
	uuid:            "00000002-0000-0000-0000-000000000002",
	expectError:     "unauthorized",
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name: "credential not found",
	env:  updateModelCredentialTestEnv,
	updateCredential: func(_ context.Context, taggedCredential jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
		if taggedCredential.Tag != "cloudcred-test-cloud_alice@canonical.com_cred-2" {
			return nil, errors.E("bad cloud credential tag")
		}
		return nil, nil
	},
	changeModelCredential: func(_ context.Context, modelTag names.ModelTag, credentialTag names.CloudCredentialTag) error {
		if modelTag.Id() != "00000002-0000-0000-0000-000000000001" {
			return errors.E("bad model tag")
		}
		if credentialTag.Id() != "test-cloud/alice@canonical.com/cred-2" {
			return errors.E("bad cloud credential tag")
		}
		return nil
	},
	username:        "alice@canonical.com",
	credential:      "test-cloud/alice@canonical.com/cred-3",
	uuid:            "00000002-0000-0000-0000-000000000001",
	expectError:     `cloudcredential "test-cloud/alice@canonical.com/cred-3" not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name: "update credential returns an error",
	env:  updateModelCredentialTestEnv,
	updateCredential: func(_ context.Context, taggedCredential jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
		return nil, errors.E("an error")
	},
	username:    "alice@canonical.com",
	credential:  "test-cloud/alice@canonical.com/cred-2",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: "an error",
}, {
	name: "change model credential returns an error",
	env:  updateModelCredentialTestEnv,
	updateCredential: func(_ context.Context, taggedCredential jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
		if taggedCredential.Tag != "cloudcred-test-cloud_alice@canonical.com_cred-2" {
			return nil, errors.E("bad cloud credential tag")
		}
		return nil, nil
	},
	changeModelCredential: func(_ context.Context, modelTag names.ModelTag, credentialTag names.CloudCredentialTag) error {
		return errors.E("an error")
	},
	username:    "alice@canonical.com",
	credential:  "test-cloud/alice@canonical.com/cred-2",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: "an error",
}}

func TestUpdateModelCredential(t *testing.T) {
	c := qt.New(t)

	for _, test := range updateModelCredentialTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name(), test.name)
			c.Assert(err, qt.IsNil)

			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					UpdateCredential_:      test.updateCredential,
					ChangeModelCredential_: test.changeModelCredential,
				},
				Err: test.dialError,
			}
			j := &jimm.JIMM{
				UUID:          uuid.NewString(),
				OpenFGAClient: client,
				Database: db.Database{
					DB: jimmtest.PostgresDB(c, nil),
				},
				Dialer: dialer,
			}
			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, test.env)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

			dbUser := env.User(test.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, client)

			err = j.ChangeModelCredential(
				ctx,
				user,
				names.NewModelTag(test.uuid),
				names.NewCloudCredentialTag(test.credential),
			)
			c.Assert(dialer.IsClosed(), qt.IsTrue)
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

func TestAddModelDeletedController(t *testing.T) {
	c := qt.New(t)

	api := &jimmtest.API{
		UpdateCredential_: func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
			return nil, nil
		},
		GrantJIMMModelAdmin_: func(context.Context, names.ModelTag) error {
			return nil
		},
		CreateModel_: createModel(`
uuid: 00000001-0000-0000-0000-0000-000000000004
status:
  status: started
  info: running a test
life: alive
users:
- user: alice@canonical.com
  access: admin
- user: bob
  access: read
`[1:]),
	}

	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	j := &jimm.JIMM{
		UUID:          uuid.NewString(),
		OpenFGAClient: client,
		Database: db.Database{
			DB: jimmtest.PostgresDB(c, nil),
		},
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	}
	ctx := context.Background()
	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	envDefinition := `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
  - name: test-region-2
users:
- username: alice@canonical.com
  controller-access: superuser
cloud-credentials:
- name: test-credential-1
  owner: alice@canonical.com
  cloud: test-cloud
  auth-type: empty
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 10
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-2
  cloud-regions:
  - cloud: test-cloud
    region: test-region-2
    priority: 2
- name: controller-3
  uuid: 00000000-0000-0000-0000-0000-0000000000003
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 1
`
	env := jimmtest.ParseEnvironment(c, envDefinition)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, client)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, client)

	controller := dbmodel.Controller{
		Name: "controller-1",
	}
	err = j.Database.GetController(ctx, &controller)
	c.Assert(err, qt.IsNil)

	err = j.Database.DeleteController(ctx, &controller)
	c.Assert(err, qt.IsNil)

	args := jimm.ModelCreateArgs{}
	err = args.FromJujuModelCreateArgs(&jujuparams.ModelCreateArgs{
		Name:               "test-model",
		OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
		CloudTag:           names.NewCloudTag("test-cloud").String(),
		CloudRegion:        "test-region-1",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1").String(),
	})
	c.Assert(err, qt.IsNil)

	// According to controller priority for test-region-1, we would
	// expect JIMM to use controller-1, but since it was deleted
	// we expect it to use controller-3.
	// Before the fix for the soft-delete cascade, this would error
	// out failing to store the model information. The
	// cloud region controller priority entry associated
	// with controller-1 would not be deleted, so JIMM
	// tried to use controller-1 and failed because
	// cloud region controller priority entry returned
	// an empty controller.
	m, err := j.AddModel(context.Background(), user, &args)
	c.Assert(err, qt.IsNil)

	// fetch model from storage
	model := dbmodel.Model{
		UUID: sql.NullString{
			String: m.UUID,
			Valid:  true,
		},
	}
	err = j.Database.GetModel(context.Background(), &model)
	c.Assert(err, qt.IsNil)
	// and assert that controller-3 was used.
	c.Assert(model.Controller.Name, qt.Equals, "controller-3")
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
