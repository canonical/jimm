// Copyright 2025 Canonical.

package juju_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

const modelPollerTestEnv = `clouds:
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
  - user: bob@canonical.com
    access: write
users:
- username: alice@canonical.com
  controller-access: superuser
`

type modelCleanupSuite struct {
	jujuManager *juju.JujuManager
	jimmAdmin   *openfga.User
	env         *jimmtest.Environment
}

func (s *modelCleanupSuite) Init(c *qt.C) {
	s.jujuManager = newTestJujuManager(c, nil)

	i, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)
	s.jimmAdmin = openfga.NewUser(i, s.jujuManager.OpenFGAClient)
	s.jimmAdmin.JimmAdmin = true
	c.Assert(err, qt.IsNil)

	s.env = jimmtest.ParseEnvironment(c, modelPollerTestEnv)
	s.env.PopulateDBAndPermissions(c, s.jujuManager.ResourceTag(), s.jujuManager.Database, s.jujuManager.OpenFGAClient)
}

func (s *modelCleanupSuite) TestPollModelsDying(c *qt.C) {
	ctx := context.Background()

	s.jujuManager.Dialer = &jimmtest.Dialer{
		API: &jimmtest.API{
			ModelInfo_: func(ctx context.Context, mi *jujuparams.ModelInfo) error {
				switch mi.UUID {
				case s.env.Models[0].UUID:
					return errors.E(errors.CodeNotFound)
				case s.env.Models[1].UUID:
					return nil
				default:
					return errors.E("new error")
				}
			},
			DestroyModel_: func(ctx context.Context, mt names.ModelTag, b1, b2 *bool, d1, d2 *time.Duration) error {
				return nil
			},
		},
	}
	err := s.jujuManager.DestroyModel(ctx, s.jimmAdmin, names.NewModelTag(s.env.Models[0].UUID), nil, nil, nil, nil)
	c.Assert(err, qt.IsNil)

	// test
	err = s.jujuManager.CleanupDyingModels(ctx)
	c.Assert(err, qt.IsNil)

	model := dbmodel.Model{
		UUID: sql.NullString{
			String: s.env.Models[0].UUID,
			Valid:  true,
		},
	}
	err = s.jujuManager.Database.GetModel(ctx, &model)
	c.Assert(err, qt.ErrorMatches, "model not found")

	model = dbmodel.Model{
		UUID: sql.NullString{
			String: s.env.Models[1].UUID,
			Valid:  true,
		},
	}
	err = s.jujuManager.Database.GetModel(ctx, &model)
	c.Assert(err, qt.IsNil)
}

func (s *modelCleanupSuite) TestPollModelsDyingControllerErrors(c *qt.C) {
	ctx := context.Background()

	s.jujuManager.Dialer = &jimmtest.Dialer{
		API: &jimmtest.API{
			ModelInfo_: func(ctx context.Context, mi *jujuparams.ModelInfo) error {
				return errors.E("controller not available")
			},
			DestroyModel_: func(ctx context.Context, mt names.ModelTag, b1, b2 *bool, d1, d2 *time.Duration) error {
				return nil
			},
		},
	}
	err := s.jujuManager.DestroyModel(ctx, s.jimmAdmin, names.NewModelTag(s.env.Models[0].UUID), nil, nil, nil, nil)
	c.Assert(err, qt.IsNil)

	// test
	err = s.jujuManager.CleanupDyingModels(ctx)
	c.Assert(err, qt.IsNil)

	model := dbmodel.Model{
		UUID: sql.NullString{
			String: s.env.Models[0].UUID,
			Valid:  true,
		},
	}
	err = s.jujuManager.Database.GetModel(ctx, &model)
	c.Assert(err, qt.IsNil)
	c.Assert(model.Life, qt.Equals, state.Dying.String())
}

func TestDyingModelsCleanup(t *testing.T) {
	qtsuite.Run(qt.New(t), &modelCleanupSuite{})
}
