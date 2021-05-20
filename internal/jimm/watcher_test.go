// Copyright 2021 Canonical Ltd.

package jimm_test

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/apiserver/params"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

const testWatcherEnv = `clouds:
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
  applications:
  - name: app-1
    exposed: true
    charm-url: cs:app-1
    life: starting
    min-units: 1
    constraints:
      arch: amd64
      cores: 2
      mem: 8096
      root-disk: 10240
    workload-version: 1    
  - name: app-2
    exposed: true
    charm-url: ch:app-2
    subordinate: true
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
  units:
  - name: app-1/0
    application: app-1
    series: warty
    charm-url: cs:app-1
    life: starting
    machine-id: 0
    agent-status:
      current: running
      message: wotcha
      version: "1.2.3"
  - name: app-1/1
    application: app-1
    series: warty
    charm-url: cs:app-1
    life: stopping
    machine-id: 1
    agent-status:
      current: stopping
      message: hiya
      version: "1.2.3"
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
  life: dying
`

var watcherTests = []struct {
	name    string
	deltas  [][]jujuparams.Delta
	checkDB func(*qt.C, db.Database) bool
}{{
	name: "AddMachine",
	deltas: [][]jujuparams.Delta{{{
		Entity: &jujuparams.MachineInfo{
			ModelUUID:  "00000002-0000-0000-0000-000000000001",
			Id:         "2",
			InstanceId: "machine-2",
			AgentStatus: jujuparams.StatusInfo{
				Current: "running",
				Message: "hello",
			},
			InstanceStatus: jujuparams.StatusInfo{
				Current: "starting",
				Message: "hi",
			},
			Life: "alive",
		},
	}}},
	checkDB: func(c *qt.C, db db.Database) bool {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Assert(err, qt.IsNil)

		m := dbmodel.Machine{
			ModelID:   model.ID,
			MachineID: "2",
		}
		err = db.GetMachine(ctx, &m)
		c.Assert(err, qt.IsNil)
		return c.Check(m, jimmtest.DBObjectEquals, dbmodel.Machine{
			ModelID:    model.ID,
			MachineID:  "2",
			InstanceID: "machine-2",
			AgentStatus: dbmodel.Status{
				Status: "running",
				Info:   "hello",
			},
			InstanceStatus: dbmodel.Status{
				Status: "starting",
				Info:   "hi",
			},
			Life: "alive",
		})
	},
}, {
	name: "UpdateMachine",
	deltas: [][]jujuparams.Delta{{{
		Entity: &jujuparams.MachineInfo{
			ModelUUID:  "00000002-0000-0000-0000-000000000001",
			Id:         "0",
			InstanceId: "machine-0",
			AgentStatus: jujuparams.StatusInfo{
				Current: "running",
				Message: "hello",
			},
			InstanceStatus: jujuparams.StatusInfo{
				Current: "starting",
				Message: "hi",
			},
			Life: "alive",
		},
	}}},
	checkDB: func(c *qt.C, db db.Database) bool {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Assert(err, qt.IsNil)

		m := dbmodel.Machine{
			ModelID:   model.ID,
			MachineID: "0",
		}
		err = db.GetMachine(ctx, &m)
		c.Assert(err, qt.IsNil)
		return c.Check(m, jimmtest.DBObjectEquals, dbmodel.Machine{
			ModelID:   model.ID,
			MachineID: "0",
			Hardware: dbmodel.Hardware{
				Arch: sql.NullString{
					String: "amd64",
					Valid:  true,
				},
				Mem: dbmodel.NullUint64{
					Uint64: 8096,
					Valid:  true,
				},
				RootDisk: dbmodel.NullUint64{
					Uint64: 10240,
					Valid:  true,
				},
				CPUCores: dbmodel.NullUint64{
					Uint64: 1,
					Valid:  true,
				},
			},
			InstanceID:  "machine-0",
			DisplayName: "Machine 0",
			AgentStatus: dbmodel.Status{
				Status: "running",
				Info:   "hello",
			},
			InstanceStatus: dbmodel.Status{
				Status: "starting",
				Info:   "hi",
			},
			Life: "alive",
		})
	},
}, {
	name: "DeleteMachine",
	deltas: [][]jujuparams.Delta{{{
		Removed: true,
		Entity: &jujuparams.MachineInfo{
			ModelUUID: "00000002-0000-0000-0000-000000000001",
			Id:        "0",
		},
	}}},
	checkDB: func(c *qt.C, db db.Database) bool {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Assert(err, qt.IsNil)

		m := dbmodel.Machine{
			ModelID:   model.ID,
			MachineID: "0",
		}
		err = db.GetMachine(ctx, &m)
		success := c.Check(err, qt.ErrorMatches, `machine not found`)
		if !success {
			return success
		}
		return c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
	},
}, {
	name: "AddApplication",
	deltas: [][]jujuparams.Delta{{{
		Entity: &jujuparams.ApplicationInfo{
			ModelUUID: "00000002-0000-0000-0000-000000000001",
			Name:      "app-3",
			CharmURL:  "ch:app-3",
			Life:      "starting",
			MinUnits:  3,
			Config:    map[string]interface{}{"a": "B"},
		},
	}}},
	checkDB: func(c *qt.C, db db.Database) bool {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Assert(err, qt.IsNil)

		app := dbmodel.Application{
			ModelID: model.ID,
			Name:    "app-3",
		}
		err = db.GetApplication(ctx, &app)
		c.Assert(err, qt.IsNil)
		return c.Check(app, jimmtest.DBObjectEquals, dbmodel.Application{
			ModelID:  model.ID,
			Name:     "app-3",
			CharmURL: "ch:app-3",
			Life:     "starting",
			MinUnits: 3,
			Config:   dbmodel.Map{"a": "B"},
		})
	},
}, {
	name: "UpdateApplication",
	deltas: [][]jujuparams.Delta{{{
		Entity: &jujuparams.ApplicationInfo{
			ModelUUID:       "00000002-0000-0000-0000-000000000001",
			Name:            "app-1",
			Exposed:         true,
			CharmURL:        "cs:app-1",
			Life:            "alive",
			MinUnits:        1,
			WorkloadVersion: "2",
		},
	}}},
	checkDB: func(c *qt.C, db db.Database) bool {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Assert(err, qt.IsNil)

		app := dbmodel.Application{
			ModelID: model.ID,
			Name:    "app-1",
		}
		err = db.GetApplication(ctx, &app)
		c.Assert(err, qt.IsNil)
		return c.Check(app, jimmtest.DBObjectEquals, dbmodel.Application{
			ModelID:         model.ID,
			Name:            "app-1",
			Exposed:         true,
			CharmURL:        "cs:app-1",
			Life:            "alive",
			MinUnits:        1,
			WorkloadVersion: "2",
		})
	},
}, {
	name: "DeleteApplication",
	deltas: [][]jujuparams.Delta{{{
		Removed: true,
		Entity: &jujuparams.ApplicationInfo{
			ModelUUID: "00000002-0000-0000-0000-000000000001",
			Name:      "app-1",
		},
	}}},
	checkDB: func(c *qt.C, db db.Database) bool {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Assert(err, qt.IsNil)

		app := dbmodel.Application{
			ModelID: model.ID,
			Name:    "app-1",
		}
		err = db.GetApplication(ctx, &app)
		success := c.Check(err, qt.ErrorMatches, `application not found`)
		if !success {
			return success
		}
		return c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
	},
}, {
	name: "AddUnit",
	deltas: [][]jujuparams.Delta{{{
		Entity: &jujuparams.UnitInfo{
			ModelUUID:   "00000002-0000-0000-0000-000000000001",
			Name:        "app-1/2",
			Application: "app-1",
			Series:      "warty",
			CharmURL:    "cs:app-1",
			Life:        "starting",
			MachineId:   "0",
		},
	}}},
	checkDB: func(c *qt.C, db db.Database) bool {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Assert(err, qt.IsNil)

		u := dbmodel.Unit{
			ModelID: model.ID,
			Name:    "app-1/2",
		}
		err = db.GetUnit(ctx, &u)
		c.Assert(err, qt.IsNil)
		return c.Check(u, jimmtest.DBObjectEquals, dbmodel.Unit{
			ModelID:         model.ID,
			Name:            "app-1/2",
			ApplicationName: "app-1",
			MachineID:       "0",
			Life:            "starting",
		})
	},
}, {
	name: "UpdateUnit",
	deltas: [][]jujuparams.Delta{{{
		Entity: &jujuparams.UnitInfo{
			ModelUUID:   "00000002-0000-0000-0000-000000000001",
			Name:        "app-1/0",
			Application: "app-1",
			Series:      "warty",
			CharmURL:    "cs:app-1",
			Life:        "alive",
			MachineId:   "0",
		},
	}}},
	checkDB: func(c *qt.C, db db.Database) bool {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Assert(err, qt.IsNil)

		u := dbmodel.Unit{
			ModelID: model.ID,
			Name:    "app-1/0",
		}
		err = db.GetUnit(ctx, &u)
		c.Assert(err, qt.IsNil)
		return c.Check(u, jimmtest.DBObjectEquals, dbmodel.Unit{
			ModelID:         model.ID,
			Name:            "app-1/0",
			ApplicationName: "app-1",
			MachineID:       "0",
			Life:            "alive",
		})
	},
}, {
	name: "DeleteUnit",
	deltas: [][]jujuparams.Delta{{{
		Removed: true,
		Entity: &jujuparams.UnitInfo{
			ModelUUID: "00000002-0000-0000-0000-000000000001",
			Name:      "app-1/0",
		},
	}}},
	checkDB: func(c *qt.C, db db.Database) bool {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Assert(err, qt.IsNil)

		u := dbmodel.Unit{
			ModelID: model.ID,
			Name:    "app-1/0",
		}
		err = db.GetUnit(ctx, &u)
		success := c.Check(err, qt.ErrorMatches, `unit not found`)
		if !success {
			return success
		}
		return c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
	},
}, {
	name: "UnknownModelsIgnored",
	deltas: [][]jujuparams.Delta{{{
		Entity: &jujuparams.ModelUpdate{
			ModelUUID: "00000002-0000-0000-0000-000000000003",
			Name:      "new-model",
			Owner:     "charlie@external",
			Life:      "starting",
		},
	}}},
	checkDB: func(c *qt.C, db db.Database) bool {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000003",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		success := c.Check(err, qt.ErrorMatches, `record not found`)
		if !success {
			return success
		}
		return c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
	},
}, {
	name: "UpdateModel",
	deltas: [][]jujuparams.Delta{{{
		Entity: &jujuparams.ModelUpdate{
			ModelUUID:      "00000002-0000-0000-0000-000000000001",
			Name:           "model-1",
			Owner:          "alice@external",
			Life:           "alive",
			ControllerUUID: "00000001-0000-0000-0000-000000000001",
			Status: jujuparams.StatusInfo{
				Current: "available",
				Message: "updated status message",
				Version: "1.2.3",
			},
			SLA: jujuparams.ModelSLAInfo{
				Level: "1",
				Owner: "me",
			},
		},
	}}},
	checkDB: func(c *qt.C, db db.Database) bool {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Assert(err, qt.IsNil)
		// zero any uninteresting associations
		// TODO(mhilton) don't fetch these in the first place.
		model.Owner = dbmodel.User{}
		model.CloudCredential = dbmodel.CloudCredential{}
		model.CloudRegion = dbmodel.CloudRegion{}
		model.Controller = dbmodel.Controller{}
		model.Applications = nil
		model.Machines = nil
		model.Users = nil
		return c.Check(model, jimmtest.DBObjectEquals, dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
			Name:          "model-1",
			Type:          "iaas",
			DefaultSeries: "warty",
			Life:          "alive",
			Status: dbmodel.Status{
				Status:  "available",
				Info:    "updated status message",
				Version: "1.2.3",
			},
			SLA: dbmodel.SLA{
				Level: "1",
				Owner: "me",
			},
		})
	},
}, {
	name: "DeleteDyingModel",
	deltas: [][]jujuparams.Delta{{{
		Removed: true,
		Entity: &jujuparams.ModelUpdate{
			ModelUUID: "00000002-0000-0000-0000-000000000002",
		},
	}}},
	checkDB: func(c *qt.C, db db.Database) bool {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000002",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		success := c.Check(err, qt.ErrorMatches, `record not found`)
		if !success {
			return success
		}
		return c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
	},
}, {
	name: "DeleteLivingModelFails",
	deltas: [][]jujuparams.Delta{{{
		Removed: true,
		Entity: &jujuparams.ModelUpdate{
			ModelUUID: "00000002-0000-0000-0000-000000000001",
		},
	}}},
	checkDB: func(c *qt.C, db db.Database) bool {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Assert(err, qt.IsNil)
		// zero any uninteresting associations
		// TODO(mhilton) don't fetch these in the first place.
		model.Owner = dbmodel.User{}
		model.CloudCredential = dbmodel.CloudCredential{}
		model.CloudRegion = dbmodel.CloudRegion{}
		model.Controller = dbmodel.Controller{}
		model.Applications = nil
		model.Machines = nil
		model.Users = nil
		return c.Check(model, jimmtest.DBObjectEquals, dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
			Name:          "model-1",
			Type:          "iaas",
			DefaultSeries: "warty",
			Life:          "alive",
			Status: dbmodel.Status{
				Status: "available",
				Info:   "OK!",
				Since: sql.NullTime{
					Time:  time.Date(2020, 2, 20, 20, 2, 20, 0, time.UTC),
					Valid: true,
				},
				Version: "1.2.3",
			},
			SLA: dbmodel.SLA{
				Level: "unsupported",
			},
		})
	},
}}

func TestWatcher(t *testing.T) {
	c := qt.New(t)

	for _, test := range watcherTests {
		c.Run(test.name, func(c *qt.C) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			nextC := make(chan []jujuparams.Delta)
			var stopped uint32

			w := &jimm.Watcher{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: &jimmtest.Dialer{
					API: &jimmtest.API{
						AllModelWatcherNext_: func(ctx context.Context, id string) ([]jujuparams.Delta, error) {
							if id != test.name {
								return nil, errors.E("incorrect id")
							}

							select {
							case <-ctx.Done():
								return nil, ctx.Err()
							case d, ok := <-nextC:
								c.Logf("AllModelWatcherNext received %#v, %v", d, ok)
								if ok {
									return d, nil
								}
								cancel()
								<-ctx.Done()
								return nil, ctx.Err()
							}
						},
						AllModelWatcherStop_: func(ctx context.Context, id string) error {
							if id != test.name {
								return errors.E("incorrect id")
							}
							atomic.StoreUint32(&stopped, 1)
							return nil
						},
						WatchAllModels_: func(context.Context) (string, error) {
							return test.name, nil
						},
					},
				},
			}

			env := jimmtest.ParseEnvironment(c, testWatcherEnv)
			err := w.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, w.Database)

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := w.Watch(ctx, time.Millisecond)
				c.Check(err, qt.ErrorMatches, `context canceled`, qt.Commentf("unexpected error %s (%#v)", err, err))
			}()

			for _, d := range test.deltas {
				select {
				case nextC <- d:
				case <-ctx.Done():
					c.Fatal("context closed prematurely")
				}
			}
			close(nextC)
			wg.Wait()

			err = retryCheck(func() bool {
				return test.checkDB(c, w.Database)
			}, 10, 5*time.Millisecond)
			c.Assert(err, qt.Equals, nil)
		})
	}
}

func TestWatcherSetsControllerUnavailable(t *testing.T) {
	c := qt.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := &jimm.Watcher{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, nil),
		},
		Dialer: &jimmtest.Dialer{
			Err: errors.E("test error"),
		},
	}
	env := jimmtest.ParseEnvironment(c, testWatcherEnv)
	err := w.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)
	env.PopulateDB(c, w.Database)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := w.Watch(ctx, time.Millisecond)
		c.Check(err, qt.ErrorMatches, `context canceled`, qt.Commentf("unexpected error %s (%#v)", err, err))
	}()

	now := time.Now().Add(-10 * time.Millisecond)
	ctl := dbmodel.Controller{
		Name: "controller-1",
	}
	err = retryCheck(func() bool {
		err := w.Database.GetController(ctx, &ctl)
		c.Assert(err, qt.Equals, nil)
		success := c.Check(ctl.UnavailableSince.Valid, qt.Equals, true)
		if !success {
			return success
		}
		return c.Check(ctl.UnavailableSince.Time.After(now), qt.Equals, true)
	}, 10, 20*time.Millisecond)
	c.Assert(err, qt.Equals, nil)

	cancel()
	wg.Wait()
}

func TestWatcherRemoveDyingModelsOnStartup(t *testing.T) {
	c := qt.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := &jimm.Watcher{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, nil),
		},
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{
				AllModelWatcherNext_: func(_ context.Context, _ string) ([]jujuparams.Delta, error) {
					cancel()
					<-ctx.Done()
					return nil, ctx.Err()
				},
				ModelInfo_: func(_ context.Context, info *jujuparams.ModelInfo) error {
					c.Check(info.UUID, qt.Equals, "00000002-0000-0000-0000-000000000002")
					return errors.E(errors.CodeNotFound)
				},
				WatchAllModels_: func(ctx context.Context) (string, error) {
					return "1234", nil
				},
			},
		},
	}
	env := jimmtest.ParseEnvironment(c, testWatcherEnv)
	err := w.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)
	env.PopulateDB(c, w.Database)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := w.Watch(ctx, time.Millisecond)
		c.Check(err, qt.ErrorMatches, `context canceled`, qt.Commentf("unexpected error %s (%#v)", err, err))
	}()
	wg.Wait()

	m := dbmodel.Model{
		UUID: sql.NullString{
			String: "00000002-0000-0000-0000-000000000002",
			Valid:  true,
		},
	}
	err = w.Database.GetModel(context.Background(), &m)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

const testWatcherIgnoreDeltasForModelsFromIncorrectControllerEnv = `clouds:
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
- name: controller-2
  uuid: 00000001-0000-0000-0000-000000000002
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
`

func TestWatcherIgnoreDeltasForModelsFromIncorrectController(t *testing.T) {
	c := qt.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nextC := make(chan []jujuparams.Delta)
	w := &jimm.Watcher{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, nil),
		},
		Dialer: jimmtest.DialerMap{
			"controller-1": &jimmtest.Dialer{
				API: &jimmtest.API{
					AllModelWatcherNext_: func(_ context.Context, _ string) ([]jujuparams.Delta, error) {
						<-ctx.Done()
						return nil, ctx.Err()
					},
					WatchAllModels_: func(ctx context.Context) (string, error) {
						return "1234", nil
					},
				},
			},
			"controller-2": &jimmtest.Dialer{
				API: &jimmtest.API{
					AllModelWatcherNext_: func(_ context.Context, _ string) ([]jujuparams.Delta, error) {
						select {
						case <-ctx.Done():
							return nil, ctx.Err()
						case d, ok := <-nextC:
							if ok {
								return d, nil
							}
							cancel()
							<-ctx.Done()
							return nil, ctx.Err()
						}

					},
					WatchAllModels_: func(ctx context.Context) (string, error) {
						return "1234", nil
					},
				},
			},
		},
	}
	env := jimmtest.ParseEnvironment(c, testWatcherIgnoreDeltasForModelsFromIncorrectControllerEnv)
	err := w.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)
	env.PopulateDB(c, w.Database)

	m1 := dbmodel.Model{
		UUID: sql.NullString{
			String: "00000002-0000-0000-0000-000000000001",
			Valid:  true,
		},
	}
	err = w.Database.GetModel(context.Background(), &m1)
	c.Assert(err, qt.IsNil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := w.Watch(ctx, time.Millisecond)
		c.Check(err, qt.ErrorMatches, `context canceled`, qt.Commentf("unexpected error %s (%#v)", err, err))
	}()

	nextC <- []jujuparams.Delta{{
		Entity: &jujuparams.ModelUpdate{
			ModelUUID: "00000002-0000-0000-0000-000000000001",
			Name:      "model-1",
			Owner:     "alice@external",
			Life:      "alive",
			Status: jujuparams.StatusInfo{
				Current: "busy",
			},
		},
	}}
	nextC <- []jujuparams.Delta{{
		Entity: &jujuparams.MachineInfo{
			ModelUUID: "00000002-0000-0000-0000-000000000001",
			Id:        "0",
		},
	}}
	nextC <- []jujuparams.Delta{{
		Entity: &jujuparams.ApplicationInfo{
			ModelUUID: "00000002-0000-0000-0000-000000000001",
			Name:      "app-1",
		},
	}}
	nextC <- []jujuparams.Delta{{
		Entity: &jujuparams.UnitInfo{
			ModelUUID: "00000002-0000-0000-0000-000000000001",
			Name:      "app-1/0",
		},
	}}
	close(nextC)

	wg.Wait()
	m2 := dbmodel.Model{
		UUID: sql.NullString{
			String: "00000002-0000-0000-0000-000000000001",
			Valid:  true,
		},
	}
	err = w.Database.GetModel(context.Background(), &m2)
	c.Assert(err, qt.IsNil)
	c.Check(m2, qt.DeepEquals, m1)
}

func retryCheck(check func() bool, n int, delay time.Duration) error {
	var success bool
	for i := 0; i < n; i++ {
		time.Sleep(delay)
		success = check()
		if success {
			return nil
		}
	}
	return errors.E("timed out")
}
