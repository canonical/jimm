// Copyright 2026 Canonical.

package juju_test

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"sort"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	jujurpc "github.com/juju/juju/rpc"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	"sigs.k8s.io/yaml"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuclient"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

var addModelTests = []struct {
	name                string
	env                 string
	updateCredential    func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error)
	grantJIMMModelAdmin func(context.Context, names.ModelTag) error
	createModel         func(context.Context, *jujuclient.CreateModelArgs) (base.ModelInfo, error)
	username            string
	jimmAdmin           bool
	// This cloudCredTag is used to manually populate a dummy cloud credential
	// into JIMM's credential store and then applied onto args before adding a model.
	cloudCredTag names.CloudCredentialTag
	args         juju.ModelCreateArgs
	expectModel  dbmodel.Model
	expectError  string
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
  users:
  - user: alice@canonical.com
    access: add-model
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 2
  users:
  - user: alice@canonical.com
    access: add-model
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		return []jujuparams.UpdateCredentialResult{{}}, nil
	},
	grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
		return nil
	},
	createModel: assertConfig(map[string]any{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
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
	username:     "alice@canonical.com",
	jimmAdmin:    true,
	cloudCredTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1"),
	args: juju.ModelCreateArgs{
		Name:        "test-model",
		Owner:       names.NewUserTag("alice@canonical.com"),
		Cloud:       names.NewCloudTag("test-cloud"),
		CloudRegion: "test-region-1",
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
  users:
  - user: alice@canonical.com
    access: add-model
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 2
  users:
  - user: alice@canonical.com
    access: add-model
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		return []jujuparams.UpdateCredentialResult{{}}, nil
	},
	grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
		return nil
	},
	createModel: assertConfig(map[string]any{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
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
	username:     "alice@canonical.com",
	jimmAdmin:    true,
	cloudCredTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1"),
	args: juju.ModelCreateArgs{
		Name:  "test-model",
		Owner: names.NewUserTag("alice@canonical.com"),
		Cloud: names.NewCloudTag("test-cloud"),
		// Creating a model without specifying the cloud region
		CloudRegion: "",
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
controllers:
- name: controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
  users:
  - user: alice@canonical.com
    access: add-model
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 2
  users:
  - user: alice@canonical.com
    access: add-model
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		return []jujuparams.UpdateCredentialResult{{}}, nil
	},
	grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
		return nil
	},
	createModel: assertConfig(map[string]any{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
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
	username:     "alice@canonical.com",
	jimmAdmin:    true,
	cloudCredTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1"),
	args: juju.ModelCreateArgs{
		Name:        "test-model",
		Owner:       names.NewUserTag("alice@canonical.com"),
		Cloud:       names.NewCloudTag("test-cloud"),
		CloudRegion: "test-region-1",
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
  owner: bob@canonical.com
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
  users:
  - user: alice@canonical.com
    access: add-model
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 2
  users:
  - user: alice@canonical.com
    access: add-model
users:
- username: alice@canonical.com
  controller-access: superuser
- username: bob@canonical.com
  controller-access: login
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		return []jujuparams.UpdateCredentialResult{{}}, nil
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
	username:     "alice@canonical.com",
	jimmAdmin:    true,
	cloudCredTag: names.NewCloudCredentialTag("test-cloud/bob@canonical.com/test-credential-1"),
	args: juju.ModelCreateArgs{
		Name:        "test-model",
		Owner:       names.NewUserTag("bob@canonical.com"),
		Cloud:       names.NewCloudTag("test-cloud"),
		CloudRegion: "test-region-1",
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
  users:
  - user: alice@canonical.com
    access: add-model
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 2
  users:
  - user: alice@canonical.com
    access: add-model
users:
- username: alice@canonical.com
  controller-access: login
- username: bob@canonical.com
  controller-access: login
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		return []jujuparams.UpdateCredentialResult{{}}, nil
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
	username:     "alice@canonical.com",
	cloudCredTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1"),
	args: juju.ModelCreateArgs{
		Name:        "test-model",
		Owner:       names.NewUserTag("bob@canonical.com"),
		Cloud:       names.NewCloudTag("test-cloud"),
		CloudRegion: "test-region-1",
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
  users:
  - user: alice@canonical.com
    access: add-model
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  regions: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 2
  users:
  - user: alice@canonical.com
    access: add-model
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		return []jujuparams.UpdateCredentialResult{{}}, nil
	},
	grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
		return nil
	},
	createModel: func(ctx context.Context, args *jujuclient.CreateModelArgs) (base.ModelInfo, error) {
		return base.ModelInfo{}, errors.New("a test error")
	},
	username:     "alice@canonical.com",
	jimmAdmin:    true,
	cloudCredTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1"),
	args: juju.ModelCreateArgs{
		Name:        "test-model",
		Owner:       names.NewUserTag("alice@canonical.com"),
		Cloud:       names.NewCloudTag("test-cloud"),
		CloudRegion: "test-region-1",
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
  users:
  - user: alice@canonical.com
    access: add-model
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-000000000002
  cloud: test-cloud-2
  region: test-region-2
  cloud-regions:
  - cloud: test-cloud-2
    region: test-region-2
    priority: 1
  users:
  - user: alice@canonical.com
    access: add-model
models:
- name: test-model
  owner: alice@canonical.com
  uuid: 00000001-0000-0000-0000-0000-000000000003
  cloud: test-cloud
  region: test-region-1
  cloud-credential: test-credential-1
  controller: controller-1
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		return []jujuparams.UpdateCredentialResult{{}}, nil
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
	username:     "alice@canonical.com",
	jimmAdmin:    true,
	cloudCredTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1"),
	args: juju.ModelCreateArgs{
		Name:        "test-model",
		Owner:       names.NewUserTag("alice@canonical.com"),
		Cloud:       names.NewCloudTag("test-cloud"),
		CloudRegion: "test-region-1",
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
  users:
  - user: alice@canonical.com
    access: add-model
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 2
  users:
  - user: alice@canonical.com
    access: add-model
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		return nil, errors.New("a silly error")
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
	username:     "alice@canonical.com",
	jimmAdmin:    true,
	cloudCredTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1"),
	args: juju.ModelCreateArgs{
		Name:        "test-model",
		Owner:       names.NewUserTag("alice@canonical.com"),
		Cloud:       names.NewCloudTag("test-cloud"),
		CloudRegion: "test-region-1",
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
  users:
  - user: alice@canonical.com
    access: add-model
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 1
  users:
  - user: alice@canonical.com
    access: add-model
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		return []jujuparams.UpdateCredentialResult{{}}, nil
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
	username:     "alice@canonical.com",
	jimmAdmin:    true,
	cloudCredTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1"),
	args: juju.ModelCreateArgs{
		Name:        "test-model",
		Owner:       names.NewUserTag("alice@canonical.com"),
		Cloud:       names.NewCloudTag("test-cloud"),
		CloudRegion: "test-region-1",
	},
	expectError: "not authorized.*",
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
cloud-defaults:
- user: alice@canonical.com
  cloud: test-cloud
  region: test-region-1
  defaults:
    key1: value1
    key2: value2
    key4: value4
- user: alice@canonical.com
  cloud: test-cloud
  defaults:
    key3: value3
    key4: val5
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
  users:
  - user: alice@canonical.com
    access: add-model
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 2
  users:
  - user: alice@canonical.com
    access: add-model
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		return []jujuparams.UpdateCredentialResult{{}}, nil
	},
	grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
		return nil
	},
	createModel: assertConfig(map[string]any{
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
	username:     "alice@canonical.com",
	jimmAdmin:    true,
	cloudCredTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1"),
	args: juju.ModelCreateArgs{
		Name:  "test-model",
		Owner: names.NewUserTag("alice@canonical.com"),
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
  users:
  - user: alice@canonical.com
    access: add-model
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 2
  users:
  - user: alice@canonical.com
    access: add-model
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		return []jujuparams.UpdateCredentialResult{{}}, nil
	},
	grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
		return nil
	},
	createModel: assertConfig(map[string]any{
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
	username:     "alice@canonical.com",
	jimmAdmin:    true,
	cloudCredTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1"),
	args: juju.ModelCreateArgs{
		Name:  "test-model",
		Owner: names.NewUserTag("alice@canonical.com"),
	},
	expectError: "no cloud specified for model; please specify one",
}, {
	name: "CreateModelOnACloudWithNoRegions",
	// test-cloud has one virtual cloud region
	env: `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: default
    virtual: true
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
  region: default
  cloud-regions:
  - cloud: test-cloud
    region: default
    priority: 0
  users:
  - user: alice@canonical.com
    access: add-model
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: default
  cloud-regions:
  - cloud: test-cloud
    region: default
    priority: 2
  users:
  - user: alice@canonical.com
    access: add-model
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		return []jujuparams.UpdateCredentialResult{{}}, nil
	},
	grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
		return nil
	},
	createModel: assertCreateModelArgs(&jujuclient.CreateModelArgs{
		Name:  "test-model",
		Owner: "alice@canonical.com",
		Cloud: "test-cloud",
		// we expect cloud region to be empty, because it is a virtual "default" region
		CloudRegion:        "",
		CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1"),
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
	username:     "alice@canonical.com",
	jimmAdmin:    true,
	cloudCredTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1"),
	args: juju.ModelCreateArgs{
		Name:  "test-model",
		Owner: names.NewUserTag("alice@canonical.com"),
		Cloud: names.NewCloudTag("test-cloud"),
		// Creating a model without specifying the cloud region
		CloudRegion: "",
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
			CloudRegion: "default",
		},
		CloudRegion: dbmodel.CloudRegion{
			Cloud: dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			},
			Name:    "default",
			Virtual: true,
		},
		CloudCredential: dbmodel.CloudCredential{
			Name:     "test-credential-1",
			AuthType: "empty",
		},
		Life: state.Alive.String(),
	},
}, {
	name: "CreateModelWithDeprecatedControllers",
	env: `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
  users:
  - user: alice@canonical.com
    access: add-model
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
  deprecated: true
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 0
  users:
  - user: alice@canonical.com
    access: add-model
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		return []jujuparams.UpdateCredentialResult{{}}, nil
	},
	grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
		return nil
	},
	createModel: assertConfig(map[string]any{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
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
	username:     "alice@canonical.com",
	jimmAdmin:    true,
	cloudCredTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1"),
	args: juju.ModelCreateArgs{
		Name:        "test-model",
		Owner:       names.NewUserTag("alice@canonical.com"),
		Cloud:       names.NewCloudTag("test-cloud"),
		CloudRegion: "test-region-1",
	},
	expectError: "no available controllers - check permissions to controllers and list of available controllers",
}, {
	name: "CreateModelWithAnotherUsersCredential",
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
  owner: bob@canonical.com
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
  users:
  - user: alice@canonical.com
    access: add-model
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		return []jujuparams.UpdateCredentialResult{{}}, nil
	},
	grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
		return nil
	},
	createModel:  nil,
	username:     "alice@canonical.com",
	jimmAdmin:    true,
	cloudCredTag: names.NewCloudCredentialTag("test-cloud/bob@canonical.com/test-credential-1"),
	args: juju.ModelCreateArgs{
		Name:        "test-model",
		Owner:       names.NewUserTag("alice@canonical.com"),
		Cloud:       names.NewCloudTag("test-cloud"),
		CloudRegion: "test-region-1",
	},
	expectError: "model owner doesn't match cloud-credential owner",
}, {
	name: "CreateModelWithoutPermissionOnController",
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
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		return []jujuparams.UpdateCredentialResult{{}}, nil
	},
	grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
		return nil
	},
	createModel:  nil,
	username:     "alice@canonical.com",
	jimmAdmin:    false,
	cloudCredTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1"),
	args: juju.ModelCreateArgs{
		Name:        "test-model",
		Owner:       names.NewUserTag("alice@canonical.com"),
		Cloud:       names.NewCloudTag("test-cloud"),
		CloudRegion: "test-region-1",
	},
	expectError: "no available controllers - check permissions to controllers and list of available controllers",
}, {
	// Controller-2 has higher priority than controller-1
	// but we are specifying controller-1 as the target controller
	// so the model should be created on controller-1.
	name: "CreateModelWithTargetController",
	env: `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
  users:
  - user: alice@canonical.com
    access: add-model
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
  users:
  - user: alice@canonical.com
    access: add-model
- name: controller-2
  uuid: 00000000-0000-0000-0000-0000-0000000000002
  cloud: test-cloud
  region: test-region-1
  cloud-regions:
  - cloud: test-cloud
    region: test-region-1
    priority: 2
  users:
  - user: alice@canonical.com
    access: add-model
`[1:],
	updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		return []jujuparams.UpdateCredentialResult{{}}, nil
	},
	grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
		return nil
	},
	createModel: assertConfig(map[string]any{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
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
	username:     "alice@canonical.com",
	jimmAdmin:    false,
	cloudCredTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1"),
	args: juju.ModelCreateArgs{
		Name:           "test-model",
		Owner:          names.NewUserTag("alice@canonical.com"),
		Cloud:          names.NewCloudTag("test-cloud"),
		CloudRegion:    "test-region-1",
		ControllerName: "controller-1",
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
			Name:        "controller-1",
			UUID:        "00000000-0000-0000-0000-0000-0000000000001",
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
	},
}}

func TestAddModel(t *testing.T) {
	c := qt.New(t)

	for _, test := range addModelTests {
		c.Run(test.name, func(c *qt.C) {
			j := newTestJujuManager(c, &parameters{
				Dialer: &jimmtest.Dialer{
					API: &jimmtest.API{
						UpdateCloudsCredentialForce_: test.updateCredential,
						GrantJIMMModelAdmin_:         test.grantJIMMModelAdmin,
						CreateModel_:                 test.createModel,
					},
				},
			})

			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, test.env)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

			err := j.CredentialStore.Put(ctx, test.cloudCredTag, map[string]string{"key": "value"})
			c.Assert(err, qt.IsNil)

			dbUser := env.User(test.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, j.OpenFGAClient)
			user.JimmAdmin = test.jimmAdmin

			test.args.CloudCredential = test.cloudCredTag

			_, err = j.AddModel(context.Background(), user, &test.args)
			if test.expectError == "" {
				c.Assert(err, qt.IsNil)

				m1 := dbmodel.Model{
					UUID: test.expectModel.UUID,
				}
				err = j.Database.GetModel(ctx, &m1)
				c.Assert(err, qt.IsNil)
				c.Assert(m1, jimmtest.DBObjectEquals, test.expectModel)

				ownerId := test.args.Owner.Id()
				if ownerId == "" {
					ownerId = user.Tag().Id()
				}

				ownerIdentity, err := dbmodel.NewIdentity(ownerId)
				c.Assert(err, qt.IsNil)
				isModelAdmin, err := openfga.IsAdministrator(
					context.Background(),
					openfga.NewUser(
						ownerIdentity,
						j.OpenFGAClient,
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

// convertParamsModelInfo converts a params.ModelInfo to a base.ModelInfo.
// It is copy/pasted from the juju params package and adjusted to fit the needs
// of this test where not all values need to be filled in.
// we only need it for these existing tests that use a yaml template to create
// a base.ModelInfo object. Now that we are using Juju's client and in turn,
// using base.ModelInfo in most places, we need to convert the params.ModelInfo
// to base.ModelInfo to keep these tests working until they get a larger refactor.
func convertParamsModelInfo(modelInfo jujuparams.ModelInfo) (base.ModelInfo, error) {
	var cloudTag names.CloudTag
	var err error
	if modelInfo.CloudTag != "" {
		cloudTag, err = names.ParseCloudTag(modelInfo.CloudTag)
		if err != nil {
			return base.ModelInfo{}, err
		}
	}
	var credential string
	if modelInfo.CloudCredentialTag != "" {
		credTag, err := names.ParseCloudCredentialTag(modelInfo.CloudCredentialTag)
		if err != nil {
			return base.ModelInfo{}, err
		}
		credential = credTag.Id()
	}
	var ownerTag names.UserTag
	if modelInfo.OwnerTag != "" {
		ownerTag, err = names.ParseUserTag(modelInfo.OwnerTag)
		if err != nil {
			return base.ModelInfo{}, err
		}
	}
	result := base.ModelInfo{
		Name:            modelInfo.Name,
		UUID:            modelInfo.UUID,
		ControllerUUID:  modelInfo.ControllerUUID,
		IsController:    modelInfo.IsController,
		ProviderType:    modelInfo.ProviderType,
		DefaultSeries:   modelInfo.DefaultSeries,
		Cloud:           cloudTag.Id(),
		CloudRegion:     modelInfo.CloudRegion,
		CloudCredential: credential,
		Owner:           ownerTag.Id(),
		Life:            modelInfo.Life,
		AgentVersion:    modelInfo.AgentVersion,
	}
	modelType := modelInfo.Type
	if modelType == "" {
		modelType = model.IAAS.String()
	}
	result.Type = model.ModelType(modelType)
	result.Status = base.Status{
		Status: modelInfo.Status.Status,
		Info:   modelInfo.Status.Info,
		Data:   make(map[string]any),
		Since:  modelInfo.Status.Since,
	}
	maps.Copy(result.Status.Data, modelInfo.Status.Data)
	result.Users = make([]base.UserInfo, len(modelInfo.Users))
	for i, u := range modelInfo.Users {
		result.Users[i] = base.UserInfo{
			UserName:       u.UserName,
			DisplayName:    u.DisplayName,
			Access:         string(u.Access),
			LastConnection: u.LastConnection,
		}
	}
	result.Machines = make([]base.Machine, len(modelInfo.Machines))
	for i, m := range modelInfo.Machines {
		machine := base.Machine{
			Id:          m.Id,
			InstanceId:  m.InstanceId,
			DisplayName: m.DisplayName,
			HasVote:     m.HasVote,
			WantsVote:   m.WantsVote,
			Status:      m.Status,
			HAPrimary:   m.HAPrimary,
		}
		if m.Hardware != nil {
			machine.Hardware = &instance.HardwareCharacteristics{
				Arch:             m.Hardware.Arch,
				Mem:              m.Hardware.Mem,
				RootDisk:         m.Hardware.RootDisk,
				CpuCores:         m.Hardware.Cores,
				CpuPower:         m.Hardware.CpuPower,
				Tags:             m.Hardware.Tags,
				AvailabilityZone: m.Hardware.AvailabilityZone,
			}
		}
		result.Machines[i] = machine
	}
	return result, nil
}

func createModel(template string) func(context.Context, *jujuclient.CreateModelArgs) (base.ModelInfo, error) {
	var tmi jujuparams.ModelInfo
	err := yaml.Unmarshal([]byte(template), &tmi)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal template: %v", err))
	}
	mi, err := convertParamsModelInfo(tmi)
	if err != nil {
		panic(fmt.Sprintf("failed to convert params.ModelInfo to base.ModelInfo: %v", err))
	}

	return func(ctx context.Context, args *jujuclient.CreateModelArgs) (base.ModelInfo, error) {
		if err != nil {
			return base.ModelInfo{}, err
		}
		mi.Name = args.Name
		mi.Cloud = args.Cloud
		mi.CloudCredential = args.CloudCredentialTag.Id()
		mi.CloudRegion = args.CloudRegion
		mi.Owner = args.Owner
		return mi, nil
	}
}

func assertConfig(config map[string]any, fnc func(context.Context, *jujuclient.CreateModelArgs) (base.ModelInfo, error)) func(context.Context, *jujuclient.CreateModelArgs) (base.ModelInfo, error) {
	return func(ctx context.Context, args *jujuclient.CreateModelArgs) (base.ModelInfo, error) {
		if args.Cloud == "" {
			return base.ModelInfo{}, errors.New("cloud not specified")
		}
		if len(config) != len(args.Config) {
			return base.ModelInfo{}, errors.New(fmt.Sprintf("expected %d config settings, got %d", len(config), len(args.Config)))
		}
		for k, v := range args.Config {
			if config[k] != v {
				return base.ModelInfo{}, errors.New(fmt.Sprintf("config value mismatch for key %s: %s -> %s", k, config[k], v))
			}
		}
		return fnc(ctx, args)
	}
}

func assertCreateModelArgs(expectedArgs *jujuclient.CreateModelArgs, fnc func(context.Context, *jujuclient.CreateModelArgs) (base.ModelInfo, error)) func(context.Context, *jujuclient.CreateModelArgs) (base.ModelInfo, error) {
	return func(ctx context.Context, args *jujuclient.CreateModelArgs) (base.ModelInfo, error) {
		if expectedArgs.Name != args.Name {
			return base.ModelInfo{}, fmt.Errorf("name mismatch: expected %q, got %q", expectedArgs.Name, args.Name)
		}
		if expectedArgs.Owner != args.Owner {
			return base.ModelInfo{}, fmt.Errorf("owner mismatch: expected %q, got %q", expectedArgs.Name, args.Name)
		}
		if expectedArgs.Cloud != args.Cloud {
			return base.ModelInfo{}, fmt.Errorf("cloud mismatch: expected %q, got %q", expectedArgs.Cloud, args.Cloud)
		}
		if expectedArgs.CloudRegion != args.CloudRegion {
			return base.ModelInfo{}, fmt.Errorf("cloud region mismatch: expected %q, got %q", expectedArgs.CloudRegion, args.CloudRegion)
		}
		if expectedArgs.CloudCredentialTag.String() != args.CloudCredentialTag.String() {
			return base.ModelInfo{}, fmt.Errorf("credential mismatch: expected %q, got %q", expectedArgs.CloudCredentialTag, args.CloudCredentialTag)
		}
		return fnc(ctx, args)
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
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: alive
`

func TestGetModel(t *testing.T) {
	ctx := context.Background()
	c := qt.New(t)

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, getModelTestEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

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
- name: controller-2
  uuid: 00000001-0000-0000-0000-000000000002
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
  - user: charlie@canonical.com
    access: read
`

// This env extends the one above to provide the everyone user with access to the model.
const modelInfoTestEnvWithEveryoneAccess = modelInfoTestEnv + `
  - user: everyone@external
    access: read
`

func modelInfoTestExpectedModelInfo(canReadMachineInfo bool, limitedExpectedUsers []base.UserInfo) jujuclient.ModelInfo {
	info := base.ModelInfo{
		Name:            "model-1",
		Type:            "iaas",
		UUID:            "00000002-0000-0000-0000-000000000001",
		ControllerUUID:  "00000001-0000-0000-0000-000000000001",
		ProviderType:    "test-provider",
		DefaultSeries:   "warty",
		Cloud:           "test-cloud",
		CloudRegion:     "test-cloud-region",
		CloudCredential: "test-cloud/alice@canonical.com/cred-1",
		Owner:           "alice@canonical.com",
		Life:            life.Value(state.Alive.String()),
		Status: base.Status{
			Status: "available",
			Info:   "OK!",
			Since:  newDate(2020, 2, 20, 20, 2, 20, 0, time.UTC),
		},
		Users: []base.UserInfo{{
			UserName: "alice@canonical.com",
			Access:   "admin",
		}, {
			UserName: "bob@canonical.com",
			Access:   "write",
		}, {
			UserName: "charlie@canonical.com",
			Access:   "read",
		}},
		Machines: []base.Machine{{
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
		AgentVersion: newVersion("1.2.3"),
	}
	if !canReadMachineInfo {
		info.Machines = nil
	}
	if limitedExpectedUsers != nil {
		info.Users = limitedExpectedUsers
	}
	return jujuclient.ModelInfo{ModelInfo: info}
}

var modelInfoTests = []struct {
	name             string
	env              string
	username         string
	uuid             string
	originModelOwner string
	expectModelInfo  jujuclient.ModelInfo
	expectError      string
}{{
	name:             "AdminUser",
	env:              modelInfoTestEnv,
	username:         "alice@canonical.com",
	uuid:             "00000002-0000-0000-0000-000000000001",
	originModelOwner: "alice@canonical.com",
	expectModelInfo:  modelInfoTestExpectedModelInfo(true, nil),
}, {
	name:             "WriteUser",
	env:              modelInfoTestEnv,
	username:         "bob@canonical.com",
	uuid:             "00000002-0000-0000-0000-000000000001",
	originModelOwner: "alice@canonical.com",
	expectModelInfo: modelInfoTestExpectedModelInfo(true, []base.UserInfo{{
		UserName: "bob@canonical.com",
		Access:   "write",
	}}),
}, {
	name:             "ReadUser",
	env:              modelInfoTestEnv,
	username:         "charlie@canonical.com",
	uuid:             "00000002-0000-0000-0000-000000000001",
	originModelOwner: "alice@canonical.com",
	expectModelInfo: modelInfoTestExpectedModelInfo(false, []base.UserInfo{{
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
	originModelOwner: "alice@canonical.com",
	expectModelInfo: modelInfoTestExpectedModelInfo(false, []base.UserInfo{{
		UserName: "everyone@external",
		Access:   "read",
	}}),
}, {
	name:             "Owner field is replaced",
	env:              modelInfoTestEnv,
	username:         "alice@canonical.com",
	uuid:             "00000002-0000-0000-0000-000000000001",
	originModelOwner: "bob",
	expectModelInfo:  modelInfoTestExpectedModelInfo(true, nil),
},
}

func TestModelInfo(t *testing.T) {
	c := qt.New(t)

	for _, test := range modelInfoTests {
		c.Run(test.name, func(c *qt.C) {
			j := newTestJujuManager(c, &parameters{
				Dialer: &jimmtest.Dialer{
					API: &jimmtest.API{
						ModelInfo_: func(ctx context.Context, model names.ModelTag) (jujuclient.ModelInfo, error) {
							mi := jujuclient.ModelInfo{}
							mi.Name = "model-1"
							mi.Type = "iaas"
							mi.ControllerUUID = "00000001-0000-0000-0000-000000000001"
							mi.UUID = "00000002-0000-0000-0000-000000000001"
							mi.ProviderType = "test-provider"
							mi.DefaultSeries = "warty"
							mi.Cloud = "test-cloud"
							mi.CloudRegion = "test-cloud-region"
							mi.CloudCredential = "test-cloud/alice@canonical.com/cred-1"
							mi.Owner = test.originModelOwner
							mi.Life = life.Value(state.Alive.String())
							mi.Status = base.Status{
								Status: "available",
								Info:   "OK!",
								Since:  newDate(2020, 2, 20, 20, 2, 20, 0, time.UTC),
							}
							// Note that users are populated from OpenFGA
							mi.Users = []base.UserInfo{}
							mi.Machines = []base.Machine{{
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
							mi.AgentVersion = newVersion("1.2.3")
							return mi, nil
						},
					},
				},
			})

			env := jimmtest.ParseEnvironment(c, test.env)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

			dbUser, err := dbmodel.NewIdentity(test.username)
			c.Assert(err, qt.IsNil)

			user := openfga.NewUser(dbUser, j.OpenFGAClient)

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

func TestModelInfoNotFound(t *testing.T) {
	c := qt.New(t)

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{
				ModelInfo_: func(ctx context.Context, model names.ModelTag) (jujuclient.ModelInfo, error) {
					return jujuclient.ModelInfo{}, errors.Codef(errors.CodeNotFound, "model not found")
				},
			},
		},
	})

	env := jimmtest.ParseEnvironment(c, modelInfoTestEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)
	dbUser, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)

	user := openfga.NewUser(dbUser, j.OpenFGAClient)
	mt := names.NewModelTag("00000002-0000-0000-0000-000000000001")

	ok, err := user.IsModelReader(c.Context(), mt)
	c.Assert(err, qt.IsNil)
	c.Assert(ok, qt.IsTrue)

	_, err = j.ModelInfo(context.Background(), user, mt)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	// Check the model is deleted as a consequence of the error
	model := env.Models[0].DBObject(c, j.Database)
	err = j.Database.GetModel(context.Background(), &model)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	// Check the openfga tuple is deleted as a consequence of the error
	ok, err = user.IsModelReader(c.Context(), mt)
	c.Assert(err, qt.IsNil)
	c.Assert(ok, qt.IsFalse)

	_, err = j.ModelInfo(context.Background(), user, mt)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

}

func TestModelInfoRedirect(t *testing.T) {
	c := qt.New(t)

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{
				ModelInfo_: func(ctx context.Context, model names.ModelTag) (jujuclient.ModelInfo, error) {
					return jujuclient.ModelInfo{}, errors.Codef(errors.CodeNotFound, "model not found")
				},
			},
		},
	})

	env := jimmtest.ParseEnvironment(c, modelInfoTestEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)
	dbUser, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)

	user := openfga.NewUser(dbUser, j.OpenFGAClient)
	mt := names.NewModelTag("00000002-0000-0000-0000-000000000001")

	ok, err := user.IsModelReader(c.Context(), mt)
	c.Assert(err, qt.IsNil)
	c.Assert(ok, qt.IsTrue)

	model := env.Models[0].DBObject(c, j.Database)
	model.MigrationMode = dbmodel.MigrationModeMigrateInternal
	err = j.Database.UpdateModel(t.Context(), &model)
	c.Assert(err, qt.IsNil)

	sourceController := env.Controllers[0].DBObject(c, j.Database)
	targetController := env.Controllers[1].DBObject(c, j.Database)
	c.Assert(model.ControllerID, qt.Equals, sourceController.ID)
	numCalls := 0
	j.Dialer = &jimmtest.Dialer{
		API: &jimmtest.API{
			ModelInfo_: func(ctx context.Context, model names.ModelTag) (jujuclient.ModelInfo, error) {
				if numCalls == 0 {
					numCalls++
					return jujuclient.ModelInfo{}, &jujurpc.RequestError{
						Message: "redirect",
						Code:    jujuparams.CodeRedirect,
						Info: jujuparams.RedirectErrorInfo{
							ControllerAlias: env.Controllers[1].Name,
						}.AsMap(),
					}
				} else {
					return jujuclient.ModelInfo{}, nil
				}
			},
		},
	}

	modelInfo, err := j.ModelInfo(t.Context(), user, names.NewModelTag(model.UUID.String))
	c.Assert(err, qt.IsNil)
	c.Assert(modelInfo.ControllerUUID, qt.Equals, targetController.UUID)

	_, err = j.ModelInfo(t.Context(), user, names.NewModelTag(model.UUID.String))
	c.Assert(err, qt.IsNil)
	c.Assert(modelInfo.ControllerUUID, qt.Equals, targetController.UUID)
}
func TestModelStatusNotFound(t *testing.T) {
	c := qt.New(t)

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{
				ModelStatus_: func(ctx context.Context, modelTag names.ModelTag) (base.ModelStatus, error) {
					return base.ModelStatus{}, errors.Codef(errors.CodeNotFound, "model not found")
				},
			},
		},
	})

	env := jimmtest.ParseEnvironment(c, modelInfoTestEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)
	dbUser, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)

	user := openfga.NewUser(dbUser, j.OpenFGAClient)

	mt := names.NewModelTag("00000002-0000-0000-0000-000000000001")

	ok, err := user.IsModelReader(c.Context(), mt)
	c.Assert(err, qt.IsNil)
	c.Assert(ok, qt.IsTrue)

	_, err = j.ModelStatus(context.Background(), user, mt)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	// Check the model is deleted as a consequence of the error
	model := env.Models[0].DBObject(c, j.Database)
	err = j.Database.GetModel(context.Background(), &model)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	// Check the openfga tuple is deleted as a consequence of the error
	ok, err = user.IsModelReader(c.Context(), mt)
	c.Assert(err, qt.IsNil)
	c.Assert(ok, qt.IsFalse)

	_, err = j.ModelStatus(context.Background(), user, mt)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
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
	modelStatus       func(ctx context.Context, modelTag names.ModelTag) (base.ModelStatus, error)
	username          string
	uuid              string
	expectModelStatus base.ModelStatus
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
	modelStatus: func(ctx context.Context, modelTag names.ModelTag) (base.ModelStatus, error) {
		if modelTag.Id() != "00000002-0000-0000-0000-000000000001" {
			return base.ModelStatus{}, errors.New("incorrect model tag")
		}
		ms := base.ModelStatus{}
		ms.UUID = modelTag.Id()
		ms.Life = life.Value(state.Alive.String())
		ms.ModelType = "iaas"
		ms.HostedMachineCount = 10
		ms.ApplicationCount = 3
		ms.UnitCount = 20
		ms.Owner = "alice@canonical.com"
		return ms, nil
	},
	username: "alice@canonical.com",
	uuid:     "00000002-0000-0000-0000-000000000001",
	expectModelStatus: base.ModelStatus{
		UUID:               "00000002-0000-0000-0000-000000000001",
		Life:               life.Value(state.Alive.String()),
		ModelType:          "iaas",
		HostedMachineCount: 10,
		ApplicationCount:   3,
		UnitCount:          20,
		Owner:              "alice@canonical.com",
	},
}, {
	name: "APIError",
	env:  modelStatusTestEnv,
	modelStatus: func(ctx context.Context, modelTag names.ModelTag) (base.ModelStatus, error) {
		return base.ModelStatus{}, errors.New("test error")
	},
	username:    "alice@canonical.com",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: "test error",
}}

func TestModelStatus(t *testing.T) {
	c := qt.New(t)

	for _, test := range modelStatusTests {
		c.Run(test.name, func(c *qt.C) {
			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					ModelStatus_: test.modelStatus,
				},
			}
			j := newTestJujuManager(c, &parameters{
				Dialer: dialer,
			})

			env := jimmtest.ParseEnvironment(c, test.env)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

			dbUser := env.User(test.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, j.OpenFGAClient)

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
    access: admin
- name: model-2
  uuid: 00000002-0000-0000-0000-000000000002
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
  sla:
    level: unsupported
- name: model-3
  uuid: 00000002-0000-0000-0000-000000000003
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: alive
  users:
  - user: alice@canonical.com
    access: admin
- name: model-4
  uuid: 00000002-0000-0000-0000-000000000004
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
    access: read
users:
- username: alice@canonical.com
  controller-access: superuser
`

func TestForEachUserModel(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, forEachModelTestEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("bob@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, j.OpenFGAClient)

	var res []base.UserModelSummary
	err := j.ForEachUserModel(ctx, user, func(m *dbmodel.Model, access string) error {
		s := m.MergeModelSummaryFromController(base.UserModelSummary{}, "", access)
		res = append(res, s)
		return nil
	})
	c.Assert(err, qt.IsNil)
	c.Check(res, qt.DeepEquals, []base.UserModelSummary{{
		Name:            "model-1",
		UUID:            "00000002-0000-0000-0000-000000000001",
		ControllerUUID:  "00000001-0000-0000-0000-000000000001",
		ProviderType:    "test-provider",
		Cloud:           "test-cloud",
		CloudRegion:     "test-cloud-region",
		CloudCredential: "test-cloud/alice@canonical.com/cred-1",
		Owner:           "alice@canonical.com",
		Life:            life.Value(state.Alive.String()),
		ModelUserAccess: "admin",
	}, {
		Name:            "model-2",
		UUID:            "00000002-0000-0000-0000-000000000002",
		ControllerUUID:  "00000001-0000-0000-0000-000000000001",
		ProviderType:    "test-provider",
		Cloud:           "test-cloud",
		CloudRegion:     "test-cloud-region",
		CloudCredential: "test-cloud/alice@canonical.com/cred-1",
		Owner:           "alice@canonical.com",
		Life:            life.Value(state.Alive.String()),
		ModelUserAccess: "write",
	}, {
		Name:            "model-4",
		UUID:            "00000002-0000-0000-0000-000000000004",
		ControllerUUID:  "00000001-0000-0000-0000-000000000001",
		ProviderType:    "test-provider",
		Cloud:           "test-cloud",
		CloudRegion:     "test-cloud-region",
		CloudCredential: "test-cloud/alice@canonical.com/cred-1",
		Owner:           "alice@canonical.com",
		Life:            life.Value(state.Alive.String()),
		ModelUserAccess: "read",
	}})
}

func TestForEachModel(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	env := jimmtest.ParseEnvironment(c, forEachModelTestEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("bob@canonical.com").DBObject(c, j.Database)
	bob := openfga.NewUser(&dbUser, j.OpenFGAClient)

	err := j.ForEachModel(ctx, bob, func(_ *dbmodel.Model, _ jujuparams.UserAccessPermission) error {
		return errors.New("function called unexpectedly")
	})
	c.Check(err, qt.ErrorMatches, `unauthorized`)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)

	dbUser = env.User("alice@canonical.com").DBObject(c, j.Database)
	alice := openfga.NewUser(&dbUser, j.OpenFGAClient)
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

const modelSummariesTestEnv = `clouds:
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
    access: admin
- name: model-2
  uuid: 00000002-0000-0000-0000-000000000002
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
- username: alice@canonical.com
  controller-access: superuser
`

func TestModelSummaries(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := newTestJujuManager(c, nil)

	err := j.Database.Migrate(ctx)
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, modelSummariesTestEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	alice := openfga.NewUser(&dbUser, j.OpenFGAClient)

	tests := []struct {
		description            string
		controllerAPISummaries []base.UserModelSummary
		expectedSummaries      []base.UserModelSummary
		expectedSummariesSize  int
	}{
		{
			description: "info from controller, so all models available",
			controllerAPISummaries: []base.UserModelSummary{
				{
					Name:           "model-1",
					UUID:           "00000002-0000-0000-0000-000000000001",
					Type:           "iaas",
					ControllerUUID: "00000002-0000-0000-0000-000000000001",
					IsController:   false,
					DefaultSeries:  "series-1",
					Life:           "alive",
					Status: base.Status{
						Status: "available",
					},
					ModelUserAccess: "testtest",
				},
				{
					Name:           "model-2",
					UUID:           "00000002-0000-0000-0000-000000000002",
					Type:           "iaas",
					ControllerUUID: "00000001-0000-0000-0000-000000000001",
					IsController:   false,
					DefaultSeries:  "series-2",
					Life:           "alive",
					Status: base.Status{
						Status: "available",
					},
					ModelUserAccess: "admin",
				},
			},
			expectedSummaries: []base.UserModelSummary{
				{

					Name:            "model-1",
					UUID:            "00000002-0000-0000-0000-000000000001",
					Type:            "iaas",
					ControllerUUID:  "00000001-0000-0000-0000-000000000001",
					IsController:    false,
					ProviderType:    "test-provider",
					DefaultSeries:   "series-1",
					Cloud:           "test-cloud",
					CloudRegion:     "test-cloud-region",
					CloudCredential: "test-cloud/alice@canonical.com/cred-1",
					Owner:           "alice@canonical.com",
					Life:            "alive",
					Status: base.Status{
						Status: "available",
					},
					ModelUserAccess: "admin",
				},
				{
					Name:            "model-2",
					UUID:            "00000002-0000-0000-0000-000000000002",
					Type:            "iaas",
					ControllerUUID:  "00000001-0000-0000-0000-000000000001",
					IsController:    false,
					ProviderType:    "test-provider",
					DefaultSeries:   "series-2",
					Cloud:           "test-cloud",
					CloudRegion:     "test-cloud-region",
					CloudCredential: "test-cloud/alice@canonical.com/cred-1",
					Owner:           "alice@canonical.com",
					Life:            "alive",
					Status: base.Status{
						Status: "available",
					},
					ModelUserAccess: "admin",
				},
			},
			expectedSummariesSize: 2,
		},
		{
			description: "partial info from controller, so one model is not available and info are not filled in.",
			controllerAPISummaries: []base.UserModelSummary{
				{
					Name:           "model-1",
					UUID:           "00000002-0000-0000-0000-000000000001",
					Type:           "iaas",
					ControllerUUID: "00000002-0000-0000-0000-000000000001",
					IsController:   false,
					Life:           "alive",
					Status: base.Status{
						Status: "available",
					},
				},
			},
			expectedSummaries: []base.UserModelSummary{
				{
					Name:            "model-1",
					UUID:            "00000002-0000-0000-0000-000000000001",
					Type:            "iaas",
					ControllerUUID:  "00000001-0000-0000-0000-000000000001",
					IsController:    false,
					ProviderType:    "test-provider",
					DefaultSeries:   "",
					Cloud:           "test-cloud",
					CloudRegion:     "test-cloud-region",
					CloudCredential: "test-cloud/alice@canonical.com/cred-1",
					Owner:           "alice@canonical.com",
					Life:            "alive",
					Status: base.Status{
						Status: "available",
					},
					ModelUserAccess: "admin",
				},
				{
					Name:            "model-2",
					UUID:            "00000002-0000-0000-0000-000000000002",
					ControllerUUID:  "00000001-0000-0000-0000-000000000001",
					IsController:    false,
					ProviderType:    "test-provider",
					Cloud:           "test-cloud",
					CloudRegion:     "test-cloud-region",
					CloudCredential: "test-cloud/alice@canonical.com/cred-1",
					Owner:           "alice@canonical.com",
					Life:            "alive",
					Status: base.Status{
						Status: "unavailable",
					},
					ModelUserAccess: "admin",
				},
			},
			expectedSummariesSize: 2,
		},
		{
			description: "no info from controller, so all models unavailable",
			expectedSummaries: []base.UserModelSummary{
				{
					Name:            "model-1",
					UUID:            "00000002-0000-0000-0000-000000000001",
					Type:            "",
					ControllerUUID:  "00000001-0000-0000-0000-000000000001",
					IsController:    false,
					ProviderType:    "test-provider",
					DefaultSeries:   "",
					Cloud:           "test-cloud",
					CloudRegion:     "test-cloud-region",
					CloudCredential: "test-cloud/alice@canonical.com/cred-1",
					Owner:           "alice@canonical.com",
					Life:            "alive",
					Status: base.Status{
						Status: "unavailable",
					},
					ModelUserAccess: "admin",
				},
				{
					Name:            "model-2",
					UUID:            "00000002-0000-0000-0000-000000000002",
					Type:            "",
					ControllerUUID:  "00000001-0000-0000-0000-000000000001",
					IsController:    false,
					ProviderType:    "test-provider",
					DefaultSeries:   "",
					Cloud:           "test-cloud",
					CloudRegion:     "test-cloud-region",
					CloudCredential: "test-cloud/alice@canonical.com/cred-1",
					Owner:           "alice@canonical.com",
					Life:            "alive",
					Status: base.Status{
						Status: "unavailable",
					},
					ModelUserAccess: "admin",
				},
			},
			expectedSummariesSize: 2,
		},
	}
	for _, t := range tests {
		c.Run(t.description, func(c *qt.C) {
			j.Dialer = &jimmtest.Dialer{
				API: &jimmtest.API{
					ListModelSummaries_: func(ctx context.Context, ms jujuparams.ModelSummariesRequest) ([]base.UserModelSummary, error) {
						return t.controllerAPISummaries, nil
					},
				},
			}
			summaries, err := j.ListModelSummaries(ctx, alice, "")
			c.Check(err, qt.IsNil)
			c.Check(summaries, qt.HasLen, t.expectedSummariesSize)
			c.Check(summaries, qt.DeepEquals, t.expectedSummaries)
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
	destroyModel    func(ctx context.Context, tag names.ModelTag, destroyStorage, force *bool, maxWait, timeout *time.Duration) error
	dialError       error
	username        string
	uuid            string
	destroyStorage  *bool
	force           *bool
	maxWait         *time.Duration
	timeout         *time.Duration
	expectError     string
	expectErrorCode errors.Code
	expectedLife    string
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
	destroyModel: func(ctx context.Context, tag names.ModelTag, destroyStorage, force *bool, maxWait, timeout *time.Duration) error {
		if tag.Id() != "00000002-0000-0000-0000-000000000001" {
			return errors.New("incorrect model uuid")
		}
		if destroyStorage == nil || *destroyStorage != true {
			return errors.New("invalid destroyStorage")
		}
		if force == nil || *force != false {
			return errors.New("invalid force")
		}
		if maxWait == nil || *maxWait != time.Second {
			return errors.New("invalid maxWait")
		}
		if timeout == nil || *timeout != time.Second {
			return errors.New("invalid timeout")
		}
		return nil
	},
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	destroyStorage: new(true),
	force:          new(false),
	maxWait:        new(time.Second),
	timeout:        new(time.Second),
	expectedLife:   "dying",
}, {
	name: "SuperuserSuccess",
	env:  destroyModelTestEnv,
	destroyModel: func(ctx context.Context, tag names.ModelTag, destroyStorage, force *bool, maxWait, timeout *time.Duration) error {
		return nil
	},
	username:     "charlie@canonical.com",
	uuid:         "00000002-0000-0000-0000-000000000001",
	expectedLife: "dying",
}, {
	name:         "DialError",
	env:          destroyModelTestEnv,
	dialError:    errors.New("dial error"),
	username:     "alice@canonical.com",
	uuid:         "00000002-0000-0000-0000-000000000001",
	expectError:  `dial error`,
	expectedLife: "alive",
}, {
	name: "APIError",
	env:  destroyModelTestEnv,
	destroyModel: func(ctx context.Context, tag names.ModelTag, destroyStorage, force *bool, maxWait, timeout *time.Duration) error {
		return errors.New("api error")
	},
	username:     "charlie@canonical.com",
	uuid:         "00000002-0000-0000-0000-000000000001",
	expectError:  `api error`,
	expectedLife: "alive",
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

			j := newTestJujuManager(c, &parameters{
				Dialer: dialer,
			})

			env := jimmtest.ParseEnvironment(c, test.env)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

			dbUser := env.User(test.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, j.OpenFGAClient)

			err := j.DestroyModel(ctx, user, names.NewModelTag(test.uuid), test.destroyStorage, test.force, test.maxWait, test.timeout)
			c.Assert(dialer.IsClosed(), qt.IsTrue)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
			} else {
				c.Assert(err, qt.IsNil)
			}
			if test.expectedLife != "" {
				m := dbmodel.Model{
					UUID: sql.NullString{
						String: test.uuid,
						Valid:  true,
					},
				}
				err = j.Database.GetModel(ctx, &m)
				c.Assert(err, qt.IsNil)
				c.Assert(m.Life, qt.Equals, test.expectedLife)
			}
		})
	}
}

var dumpModelTests = []struct {
	name            string
	env             string
	dumpModel       func(ctx context.Context, tag names.ModelTag, simplified bool) (map[string]any, error)
	dialError       error
	username        string
	uuid            string
	simplified      bool
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
	dumpModel: func(ctx context.Context, tag names.ModelTag, simplified bool) (map[string]any, error) {
		if tag.Id() != "00000002-0000-0000-0000-000000000001" {
			return nil, errors.New("incorrect model uuid")
		}
		if simplified != true {
			return nil, errors.New("invalid simplified")
		}
		return map[string]any{}, nil
	},
	username:   "alice@canonical.com",
	uuid:       "00000002-0000-0000-0000-000000000001",
	simplified: true,
}, {
	name: "SuperuserSuccess",
	env:  destroyModelTestEnv,
	dumpModel: func(ctx context.Context, tag names.ModelTag, simplified bool) (map[string]any, error) {
		return map[string]any{}, nil
	},
	username: "charlie@canonical.com",
	uuid:     "00000002-0000-0000-0000-000000000001",
}, {
	name:        "DialError",
	env:         destroyModelTestEnv,
	dialError:   errors.New("dial error"),
	username:    "alice@canonical.com",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: `dial error`,
}, {
	name: "APIError",
	env:  destroyModelTestEnv,
	dumpModel: func(ctx context.Context, tag names.ModelTag, simplified bool) (map[string]any, error) {
		return map[string]any{}, errors.New("api error")
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

			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					DumpModel_: test.dumpModel,
				},
				Err: test.dialError,
			}
			j := newTestJujuManager(c, &parameters{
				Dialer: dialer,
			})

			env := jimmtest.ParseEnvironment(c, test.env)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

			dbUser := env.User(test.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, j.OpenFGAClient)

			_, err := j.DumpModel(ctx, user, names.NewModelTag(test.uuid), test.simplified)
			c.Assert(dialer.IsClosed(), qt.IsTrue)
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

var dumpModelDBTests = []struct {
	name            string
	env             string
	dumpModelDB     func(ctx context.Context, tag names.ModelTag) (map[string]any, error)
	dialError       error
	username        string
	uuid            string
	expectDump      map[string]any
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
	dumpModelDB: func(ctx context.Context, tag names.ModelTag) (map[string]any, error) {
		if tag.Id() != "00000002-0000-0000-0000-000000000001" {
			return nil, errors.New("incorrect model uuid")
		}
		return map[string]any{"model": "dump"}, nil
	},
	username:   "alice@canonical.com",
	uuid:       "00000002-0000-0000-0000-000000000001",
	expectDump: map[string]any{"model": "dump"},
}, {
	name: "SuperuserSuccess",
	env:  destroyModelTestEnv,
	dumpModelDB: func(ctx context.Context, tag names.ModelTag) (map[string]any, error) {
		return map[string]any{"model": "dump 2"}, nil
	},
	username:   "charlie@canonical.com",
	uuid:       "00000002-0000-0000-0000-000000000001",
	expectDump: map[string]any{"model": "dump 2"},
}, {
	name:        "DialError",
	env:         destroyModelTestEnv,
	dialError:   errors.New("dial error"),
	username:    "alice@canonical.com",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: `dial error`,
}, {
	name: "APIError",
	env:  destroyModelTestEnv,
	dumpModelDB: func(ctx context.Context, tag names.ModelTag) (map[string]any, error) {
		return nil, errors.New("api error")
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

			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					DumpModelDB_: test.dumpModelDB,
				},
				Err: test.dialError,
			}
			j := newTestJujuManager(c, &parameters{
				Dialer: dialer,
			})

			env := jimmtest.ParseEnvironment(c, test.env)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

			dbUser := env.User(test.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, j.OpenFGAClient)

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
	validateModelUpgrade func(ctx context.Context, model names.ModelTag, force bool) error
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
	validateModelUpgrade: func(ctx context.Context, model names.ModelTag, force bool) error {
		if model.Id() != "00000002-0000-0000-0000-000000000001" {
			return errors.New("incorrect model uuid")
		}
		if force != true {
			return errors.New("incorrect force")
		}
		return nil
	},
	username: "alice@canonical.com",
	uuid:     "00000002-0000-0000-0000-000000000001",
	force:    true,
}, {
	name: "SuperuserSuccess",
	env:  destroyModelTestEnv,
	validateModelUpgrade: func(ctx context.Context, model names.ModelTag, force bool) error {
		if force != false {
			return errors.New("incorrect force")
		}
		return nil
	},
	username: "charlie@canonical.com",
	uuid:     "00000002-0000-0000-0000-000000000001",
}, {
	name:        "DialError",
	env:         destroyModelTestEnv,
	dialError:   errors.New("dial error"),
	username:    "alice@canonical.com",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: `dial error`,
}, {
	name: "APIError",
	env:  destroyModelTestEnv,
	validateModelUpgrade: func(ctx context.Context, model names.ModelTag, force bool) error {
		return errors.New("api error")
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

			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					ValidateModelUpgrade_: test.validateModelUpgrade,
				},
				Err: test.dialError,
			}

			j := newTestJujuManager(c, &parameters{
				Dialer: dialer,
			})

			env := jimmtest.ParseEnvironment(c, test.env)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

			dbUser := env.User(test.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, j.OpenFGAClient)

			err := j.ValidateModelUpgrade(ctx, user, names.NewModelTag(test.uuid), test.force)
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
	updateCredential      func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error)
	changeModelCredential func(ctx context.Context, model names.ModelTag, credential names.CloudCredentialTag) error
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
	updateCredential: func(_ context.Context, taggedCredential jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		if taggedCredential.Tag != "cloudcred-test-cloud_alice@canonical.com_cred-2" {
			return nil, errors.New("bad cloud credential tag")
		}
		return []jujuparams.UpdateCredentialResult{{}}, nil
	},
	changeModelCredential: func(ctx context.Context, model names.ModelTag, credential names.CloudCredentialTag) error {
		if model.Id() != "00000002-0000-0000-0000-000000000001" {
			return errors.New("bad model tag")
		}
		if credential.Id() != "test-cloud/alice@canonical.com/cred-2" {
			return errors.New("bad cloud credential tag")
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
	updateCredential: func(_ context.Context, taggedCredential jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		if taggedCredential.Tag != "cloudcred-test-cloud_alice@canonical.com_cred-2" {
			return nil, errors.New("bad cloud credential tag")
		}
		return []jujuparams.UpdateCredentialResult{{}}, nil
	},
	changeModelCredential: func(ctx context.Context, model names.ModelTag, credential names.CloudCredentialTag) error {
		if model.Id() != "00000002-0000-0000-0000-000000000001" {
			return errors.New("bad model tag")
		}
		if credential.Id() != "test-cloud/alice@canonical.com/cred-2" {
			return errors.New("bad cloud credential tag")
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
	updateCredential: func(_ context.Context, taggedCredential jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		if taggedCredential.Tag != "cloudcred-test-cloud_alice@canonical.com_cred-2" {
			return nil, errors.New("bad cloud credential tag")
		}
		return []jujuparams.UpdateCredentialResult{{}}, nil
	},
	changeModelCredential: func(ctx context.Context, model names.ModelTag, credential names.CloudCredentialTag) error {
		if model.Id() != "00000002-0000-0000-0000-000000000001" {
			return errors.New("bad model tag")
		}
		if credential.Id() != "test-cloud/alice@canonical.com/cred-2" {
			return errors.New("bad cloud credential tag")
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
	updateCredential: func(_ context.Context, taggedCredential jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		return nil, errors.New("an error")
	},
	username:    "alice@canonical.com",
	credential:  "test-cloud/alice@canonical.com/cred-2",
	uuid:        "00000002-0000-0000-0000-000000000001",
	expectError: "an error",
}, {
	name: "change model credential returns an error",
	env:  updateModelCredentialTestEnv,
	updateCredential: func(_ context.Context, taggedCredential jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
		if taggedCredential.Tag != "cloudcred-test-cloud_alice@canonical.com_cred-2" {
			return nil, errors.New("bad cloud credential tag")
		}
		return []jujuparams.UpdateCredentialResult{{}}, nil
	},
	changeModelCredential: func(ctx context.Context, model names.ModelTag, credential names.CloudCredentialTag) error {
		return errors.New("an error")
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

			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					UpdateCloudsCredentialForce_: test.updateCredential,
					ChangeModelCredential_:       test.changeModelCredential,
				},
				Err: test.dialError,
			}
			j := newTestJujuManager(c, &parameters{
				Dialer: dialer,
			})

			env := jimmtest.ParseEnvironment(c, test.env)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

			dbUser := env.User(test.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, j.OpenFGAClient)

			testAttributes := map[string]string{"key": "value"}
			err := j.CredentialStore.Put(ctx, names.NewCloudCredentialTag(test.credential), testAttributes)
			c.Assert(err, qt.IsNil)

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
		UpdateCloudsCredentialForce_: func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
			return []jujuparams.UpdateCredentialResult{{}}, nil
		},
		GrantJIMMModelAdmin_: func(ctx context.Context, mt names.ModelTag) error {
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

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	ctx := context.Background()

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
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	dbUser := env.User("alice@canonical.com").DBObject(c, j.Database)
	user := openfga.NewUser(&dbUser, j.OpenFGAClient)

	controller := dbmodel.Controller{
		Name: "controller-1",
	}
	err := j.Database.GetController(ctx, &controller)
	c.Assert(err, qt.IsNil)

	err = j.Database.DeleteController(ctx, &controller)
	c.Assert(err, qt.IsNil)

	cloudCredTag := names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1")
	err = j.CredentialStore.Put(ctx, cloudCredTag, map[string]string{"key": "value"})
	c.Assert(err, qt.IsNil)

	args := juju.ModelCreateArgs{
		Name:            "test-model",
		Owner:           names.NewUserTag("alice@canonical.com"),
		Cloud:           names.NewCloudTag("test-cloud"),
		CloudRegion:     "test-region-1",
		CloudCredential: cloudCredTag,
	}

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

const listModelsTestEnv = `clouds:
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

- name: controller-2
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
    access: admin

- name: model-2
  uuid: 00000002-0000-0000-0000-000000000002
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
  sla:
    level: unsupported

- name: model-3
  uuid: 00000002-0000-0000-0000-000000000003
  controller: controller-2
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: alive
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: read

- name: model-4
  uuid: 00000002-0000-0000-0000-000000000004
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: alive
  users:
  - user: alice@canonical.com
    access: admin

users:
- username: alice@canonical.com
  controller-access: superuser
`

var modelListTests = []struct {
	name                           string
	env                            string
	username                       string
	expectedUserModels             []base.UserModel
	expectedError                  string
	listModelsMockByControllerName map[string]func(context.Context) ([]base.UserModel, error)
}{
	{
		name:     "Bob lists models across controllers 1 and 2",
		env:      listModelsTestEnv,
		username: "bob@canonical.com",
		expectedUserModels: []base.UserModel{
			{UUID: "00000002-0000-0000-0000-000000000001", Owner: "alice@canonical.com"},
			{UUID: "00000002-0000-0000-0000-000000000002", Owner: "alice@canonical.com"},
			{UUID: "00000002-0000-0000-0000-000000000003", Owner: "alice@canonical.com"},
		},
		listModelsMockByControllerName: map[string]func(context.Context) ([]base.UserModel, error){
			"controller-1": func(ctx context.Context) ([]base.UserModel, error) {
				return []base.UserModel{
					{UUID: "00000002-0000-0000-0000-000000000001"},
					{UUID: "00000002-0000-0000-0000-000000000002"},
				}, nil
			},
			"controller-2": func(ctx context.Context) ([]base.UserModel, error) {
				return []base.UserModel{
					{UUID: "00000002-0000-0000-0000-000000000003"},
				}, nil
			},
		},
	},
	{
		name:     "Alice lists models across controllers 1 and 2",
		env:      listModelsTestEnv,
		username: "alice@canonical.com",
		expectedUserModels: []base.UserModel{
			{UUID: "00000002-0000-0000-0000-000000000001", Owner: "alice@canonical.com"},
			{UUID: "00000002-0000-0000-0000-000000000002", Owner: "alice@canonical.com"},
			{UUID: "00000002-0000-0000-0000-000000000003", Owner: "alice@canonical.com"},
			{UUID: "00000002-0000-0000-0000-000000000004", Owner: "alice@canonical.com"},
		},
		listModelsMockByControllerName: map[string]func(context.Context) ([]base.UserModel, error){
			"controller-1": func(ctx context.Context) ([]base.UserModel, error) {
				return []base.UserModel{
					{UUID: "00000002-0000-0000-0000-000000000001"},
					{UUID: "00000002-0000-0000-0000-000000000002"},
					{UUID: "00000002-0000-0000-0000-000000000004"},
				}, nil
			},
			"controller-2": func(ctx context.Context) ([]base.UserModel, error) {
				return []base.UserModel{
					{UUID: "00000002-0000-0000-0000-000000000003"},
				}, nil
			},
		},
	},
	{
		name:               "Alice lists models across controllers 1 and 2",
		env:                listModelsTestEnv,
		username:           "alice@canonical.com",
		expectedUserModels: []base.UserModel{},
		expectedError:      "failed to list models.*",
		listModelsMockByControllerName: map[string]func(context.Context) ([]base.UserModel, error){
			"controller-1": func(ctx context.Context) ([]base.UserModel, error) {
				return []base.UserModel{}, errors.New("test error")
			},
		},
	},
}

func TestListModels(t *testing.T) {
	c := qt.New(t)

	for _, test := range modelListTests {
		c.Run(
			test.name,
			func(c *qt.C) {
				j := newTestJujuManager(c, &parameters{
					Dialer: jimmtest.DialerMap{
						"controller-1": &jimmtest.Dialer{
							API: &jimmtest.API{
								ListModels_: test.listModelsMockByControllerName["controller-1"],
							},
						},
						"controller-2": &jimmtest.Dialer{
							API: &jimmtest.API{
								ListModels_: test.listModelsMockByControllerName["controller-2"],
							},
						},
					},
				})

				env := jimmtest.ParseEnvironment(c, test.env)
				env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

				dbUser, err := dbmodel.NewIdentity(test.username)
				c.Assert(err, qt.IsNil)
				user := openfga.NewUser(dbUser, j.OpenFGAClient)

				models, err := j.ListModels(context.Background(), user)
				if test.expectedError != "" {
					c.Assert(err, qt.ErrorMatches, test.expectedError)
				} else {
					c.Assert(models, qt.ContentEquals, test.expectedUserModels)
				}
			},
		)
	}
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
