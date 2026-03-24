// Copyright 2025 Canonical.

package juju_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/api/base"
	jujurpc "github.com/juju/juju/rpc"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuclient"
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
  migration-mode: importing
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: write
users:
- username: alice@canonical.com
  controller-access: superuser
`

type modelPollerTest struct {
	jujuManager *juju.JujuManager
	jimmAdmin   *openfga.User
	env         *jimmtest.Environment
}

func setupModelPollerTest(c *qt.C) modelPollerTest {
	var s modelPollerTest
	s.jujuManager = newTestJujuManager(c, nil)

	i, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)
	s.jimmAdmin = openfga.NewUser(i, s.jujuManager.OpenFGAClient)
	s.jimmAdmin.JimmAdmin = true
	c.Assert(err, qt.IsNil)

	s.env = jimmtest.ParseEnvironment(c, modelPollerTestEnv)
	s.env.PopulateDBAndPermissions(c, s.jujuManager.ResourceTag(), s.jujuManager.Database, s.jujuManager.OpenFGAClient)
	return s
}

func TestModelCleanup(t *testing.T) {
	c := qt.New(t)
	s := setupModelPollerTest(c)
	ctx := context.Background()

	s.jujuManager.Dialer = &jimmtest.Dialer{
		API: &jimmtest.API{
			ModelInfo_: func(ctx context.Context, model names.ModelTag) (jujuclient.ModelInfo, error) {
				switch model.Id() {
				case s.env.Models[0].UUID:
					return jujuclient.ModelInfo{}, errors.E(errors.CodeNotFound)
				case s.env.Models[1].UUID:
					return jujuclient.ModelInfo{ModelInfo: base.ModelInfo{UUID: model.Id()}}, nil
				case s.env.Models[2].UUID:
					return jujuclient.ModelInfo{}, fmt.Errorf("unexpected call to ModelInfo_ for model %s", model.Id())
				default:
					return jujuclient.ModelInfo{}, errors.New("new error")
				}
			},
			DestroyModel_: func(ctx context.Context, tag names.ModelTag, destroyStorage, force *bool, maxWait, timeout *time.Duration) error {
				return nil
			},
		},
	}

	err := s.jujuManager.PollModels(ctx)
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

func TestInternalMigrationSuccess(t *testing.T) {
	c := qt.New(t)
	s := setupModelPollerTest(c)
	ctx := context.Background()

	model := s.env.Models[0].DBObject(c, s.jujuManager.Database)
	model.MigrationMode = dbmodel.MigrationModeMigrateInternal
	err := s.jujuManager.Database.UpdateModel(ctx, &model)
	c.Assert(err, qt.IsNil)

	sourceController := s.env.Controllers[0].DBObject(c, s.jujuManager.Database)
	targetController := s.env.Controllers[1].DBObject(c, s.jujuManager.Database)
	c.Assert(model.ControllerID, qt.Equals, sourceController.ID)

	s.jujuManager.Dialer = &jimmtest.Dialer{
		API: &jimmtest.API{
			ModelInfo_: func(ctx context.Context, model names.ModelTag) (jujuclient.ModelInfo, error) {
				return jujuclient.ModelInfo{}, &jujurpc.RequestError{
					Message: "redirect",
					Code:    jujuparams.CodeRedirect,
					Info: jujuparams.RedirectErrorInfo{
						ControllerAlias: s.env.Controllers[1].Name,
					}.AsMap(),
				}
			},
		},
	}

	err = s.jujuManager.PollModels(ctx)
	c.Assert(err, qt.IsNil)

	model = dbmodel.Model{
		UUID: sql.NullString{
			String: s.env.Models[0].UUID,
			Valid:  true,
		},
	}
	err = s.jujuManager.Database.GetModel(ctx, &model)
	c.Assert(err, qt.IsNil)
	c.Assert(model.ControllerID, qt.Equals, targetController.ID)
	c.Assert(model.MigrationMode, qt.Equals, dbmodel.MigrationModeNone)
}

func TestInternalMigrationFailure(t *testing.T) {
	c := qt.New(t)
	s := setupModelPollerTest(c)
	ctx := context.Background()

	model := s.env.Models[0].DBObject(c, s.jujuManager.Database)
	model.MigrationMode = dbmodel.MigrationModeMigrateInternal
	err := s.jujuManager.Database.UpdateModel(ctx, &model)
	c.Assert(err, qt.IsNil)

	sourceController := s.env.Controllers[0].DBObject(c, s.jujuManager.Database)
	c.Assert(model.ControllerID, qt.Equals, sourceController.ID)

	s.jujuManager.Dialer = &jimmtest.Dialer{
		API: &jimmtest.API{
			ModelInfo_: func(ctx context.Context, model names.ModelTag) (jujuclient.ModelInfo, error) {
				mi := jujuclient.ModelInfo{}
				mi.MigrationStatus = &jujuclient.ModelMigrationStatus{
					Status: "migration failed",
					Start:  &time.Time{},
					End:    &time.Time{},
				}
				return mi, nil
			},
		},
	}

	err = s.jujuManager.PollModels(ctx)
	c.Assert(err, qt.IsNil)

	model = dbmodel.Model{
		UUID: sql.NullString{
			String: s.env.Models[0].UUID,
			Valid:  true,
		},
	}
	err = s.jujuManager.Database.GetModel(ctx, &model)
	c.Assert(err, qt.IsNil)
	c.Assert(model.ControllerID, qt.Equals, sourceController.ID)
	c.Assert(model.MigrationMode, qt.Equals, dbmodel.MigrationModeNone)
}

func TestPollModelsDyingControllerErrors(t *testing.T) {
	c := qt.New(t)
	s := setupModelPollerTest(c)
	ctx := context.Background()

	s.jujuManager.Dialer = &jimmtest.Dialer{
		API: &jimmtest.API{
			ModelInfo_: func(ctx context.Context, model names.ModelTag) (jujuclient.ModelInfo, error) {
				return jujuclient.ModelInfo{}, errors.New("controller not available")
			},
			DestroyModel_: func(ctx context.Context, tag names.ModelTag, destroyStorage, force *bool, maxWait, timeout *time.Duration) error {
				return nil
			},
		},
	}

	err := s.jujuManager.PollModels(ctx)
	c.Assert(err, qt.IsNil)

	model := dbmodel.Model{
		UUID: sql.NullString{
			String: s.env.Models[0].UUID,
			Valid:  true,
		},
	}
	err = s.jujuManager.Database.GetModel(ctx, &model)
	c.Assert(err, qt.IsNil)
	c.Assert(model.Life, qt.Equals, state.Alive.String())
}
