// Copyright 2025 Canonical.

package juju_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
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

// perControllerDialer is a test juju.Dialer that records how many times each
// controller was dialed and how many of those connections were subsequently
// closed.  This lets tests assert that no connections are leaked.
type perControllerDialer struct {
	api juju.API

	mu     sync.Mutex
	opened map[string]int // controller name → dial count
	closed map[string]int // controller name → close count
}

func newPerControllerDialer(api juju.API) *perControllerDialer {
	return &perControllerDialer{
		api:    api,
		opened: make(map[string]int),
		closed: make(map[string]int),
	}
}

func (d *perControllerDialer) Dial(_ context.Context, ctl *dbmodel.Controller, _ names.ModelTag, _ *openfga.User) (juju.API, error) {
	name := ctl.Name
	d.mu.Lock()
	defer d.mu.Unlock()
	d.opened[name]++
	return &perControllerAPI{
		API: d.api,
		onClose: func() {
			d.mu.Lock()
			defer d.mu.Unlock()
			d.closed[name]++
		},
	}, nil
}

// openedCounts returns a snapshot of the dial counts per controller.
func (d *perControllerDialer) openedCounts() map[string]int {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[string]int, len(d.opened))
	for k, v := range d.opened {
		out[k] = v
	}
	return out
}

// closedCounts returns a snapshot of the close counts per controller.
func (d *perControllerDialer) closedCounts() map[string]int {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[string]int, len(d.closed))
	for k, v := range d.closed {
		out[k] = v
	}
	return out
}

// perControllerAPI wraps a juju.API and calls onClose when the connection is
// closed, allowing perControllerDialer to track the close event.
type perControllerAPI struct {
	juju.API
	onClose func()
}

func (a *perControllerAPI) Close() error {
	a.onClose()
	return a.API.Close()
}

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

	model := s.env.Models[1].DBObject(c, s.jujuManager.Database)
	model.Life = state.Dying.String()
	err := s.jujuManager.Database.UpdateModel(ctx, &model)
	c.Assert(err, qt.IsNil)

	s.jujuManager.Dialer = &jimmtest.Dialer{
		API: &jimmtest.API{
			ModelInfo_: func(ctx context.Context, model names.ModelTag) (jujuclient.ModelInfo, error) {
				switch model.Id() {
				case s.env.Models[0].UUID:
					return jujuclient.ModelInfo{}, errors.Codef(errors.CodeNotFound, "not found")
				case s.env.Models[1].UUID:
					return jujuclient.ModelInfo{}, errors.Codef(errors.CodeNotFound, "not found")
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

	model = dbmodel.Model{
		UUID: sql.NullString{
			String: s.env.Models[1].UUID,
			Valid:  true,
		},
	}
	err = s.jujuManager.Database.GetModel(ctx, &model)
	c.Assert(err, qt.ErrorMatches, "model not found")

	model = dbmodel.Model{
		UUID: sql.NullString{
			String: s.env.Models[2].UUID,
			Valid:  true,
		},
	}
	err = s.jujuManager.Database.GetModel(ctx, &model)
	c.Assert(err, qt.IsNil)
}

func TestModelCleanupUnauthorizedDoesNotDelete(t *testing.T) {
	c := qt.New(t)
	s := setupModelPollerTest(c)
	ctx := context.Background()

	s.jujuManager.Dialer = &jimmtest.Dialer{
		API: &jimmtest.API{
			ModelInfo_: func(ctx context.Context, model names.ModelTag) (jujuclient.ModelInfo, error) {
				if model.Id() == s.env.Models[0].UUID {
					return jujuclient.ModelInfo{}, errors.Codef(errors.CodeUnauthorized, "unauthorized")
				}
				return jujuclient.ModelInfo{ModelInfo: base.ModelInfo{UUID: model.Id()}}, nil
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

// TestPollModelsClosesControllerConnections ensures that the connection
// dialled to each controller is closed once its models have been processed.
// Otherwise every poll cycle leaks one connection per controller.
func TestPollModelsClosesControllerConnections(t *testing.T) {
	c := qt.New(t)
	s := setupModelPollerTest(c)
	ctx := context.Background()

	dialer := newPerControllerDialer(&jimmtest.API{
		ModelInfo_: func(ctx context.Context, model names.ModelTag) (jujuclient.ModelInfo, error) {
			return jujuclient.ModelInfo{ModelInfo: base.ModelInfo{UUID: model.Id()}}, nil
		},
	})
	s.jujuManager.Dialer = dialer

	err := s.jujuManager.PollModels(ctx)
	c.Assert(err, qt.IsNil)

	// controller-1 owns model-1, model-2, and model-3 so it must be dialed
	// exactly once.  controller-2 has no models and must not be dialed at all.
	c.Assert(dialer.openedCounts(), qt.DeepEquals, map[string]int{
		"controller-1": 1,
	})
	// Every opened connection must have been closed – no leaks.
	c.Assert(dialer.closedCounts(), qt.DeepEquals, dialer.openedCounts())
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
