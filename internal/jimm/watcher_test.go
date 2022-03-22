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
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/core/instance"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

const testWatcherEnv = `clouds:
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
  life: dying
- name: model-3
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000003
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@external
  life: dead
`

var watcherTests = []struct {
	name    string
	initDB  func(*qt.C, db.Database)
	deltas  [][]jujuparams.Delta
	checkDB func(*qt.C, db.Database)
}{{
	name: "AddMachine",
	deltas: [][]jujuparams.Delta{
		{{
			Entity: &jujuparams.MachineInfo{
				ModelUUID:  "00000002-0000-0000-0000-000000000001",
				Id:         "2",
				InstanceId: "machine-2",
				HardwareCharacteristics: &instance.HardwareCharacteristics{
					CpuCores: newUint64(2),
				},
			},
		}},
		nil,
	},
	checkDB: func(c *qt.C, db db.Database) {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Assert(err, qt.IsNil)

		c.Check(model.Machines, qt.Equals, int64(1))
		c.Check(model.Cores, qt.Equals, int64(2))
	},
}, {
	name: "UpdateMachine",
	deltas: [][]jujuparams.Delta{
		{{
			Entity: &jujuparams.MachineInfo{
				ModelUUID:  "00000002-0000-0000-0000-000000000001",
				Id:         "0",
				InstanceId: "machine-0",
			},
		}}, {{
			Entity: &jujuparams.MachineInfo{
				ModelUUID:  "00000002-0000-0000-0000-000000000001",
				Id:         "0",
				InstanceId: "machine-0",
				HardwareCharacteristics: &instance.HardwareCharacteristics{
					CpuCores: newUint64(4),
				},
			},
		}},
		nil,
	},
	checkDB: func(c *qt.C, db db.Database) {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Assert(err, qt.IsNil)

		c.Check(model.Machines, qt.Equals, int64(1))
		c.Check(model.Cores, qt.Equals, int64(4))
	},
}, {
	name: "DeleteMachine",
	deltas: [][]jujuparams.Delta{
		{{
			Entity: &jujuparams.MachineInfo{
				ModelUUID:  "00000002-0000-0000-0000-000000000001",
				Id:         "0",
				InstanceId: "machine-0",
				HardwareCharacteristics: &instance.HardwareCharacteristics{
					CpuCores: newUint64(3),
				},
			},
		}}, {{
			Removed: true,
			Entity: &jujuparams.MachineInfo{
				ModelUUID: "00000002-0000-0000-0000-000000000001",
				Id:        "0",
			},
		}},
		nil,
	},
	checkDB: func(c *qt.C, db db.Database) {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Assert(err, qt.IsNil)

		c.Check(model.Machines, qt.Equals, int64(0))
		c.Check(model.Cores, qt.Equals, int64(0))
	},
}, {
	name: "UpdateApplication",
	initDB: func(c *qt.C, db db.Database) {
		ctx := context.Background()

		var m dbmodel.Model
		m.SetTag(names.NewModelTag("00000002-0000-0000-0000-000000000001"))
		err := db.GetModel(ctx, &m)
		c.Assert(err, qt.IsNil)

		err = db.AddApplicationOffer(ctx, &dbmodel.ApplicationOffer{
			ModelID:         m.ID,
			UUID:            "00000010-0000-0000-0000-000000000001",
			Name:            "offer-1",
			ApplicationName: "app-1",
		})
		c.Assert(err, qt.IsNil)
	},
	deltas: [][]jujuparams.Delta{
		{{
			Entity: &jujuparams.ApplicationInfo{
				ModelUUID:       "00000002-0000-0000-0000-000000000001",
				Name:            "app-1",
				Exposed:         true,
				CharmURL:        "cs:app-1",
				Life:            "alive",
				MinUnits:        1,
				WorkloadVersion: "2",
			},
		}},
		nil,
	},
	checkDB: func(c *qt.C, db db.Database) {
		ctx := context.Background()

		var m dbmodel.Model
		m.SetTag(names.NewModelTag("00000002-0000-0000-0000-000000000001"))
		err := db.GetModel(ctx, &m)
		c.Assert(err, qt.IsNil)

		c.Assert(m.Offers, qt.HasLen, 1)
		c.Assert(m.Offers[0].CharmURL, qt.Equals, "cs:app-1")
	},
}, {
	name: "AddUnit",
	deltas: [][]jujuparams.Delta{
		{{
			Entity: &jujuparams.UnitInfo{
				ModelUUID: "00000002-0000-0000-0000-000000000001",
				Name:      "app-1/2",
			},
		}},
		nil,
	},
	checkDB: func(c *qt.C, db db.Database) {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Assert(err, qt.IsNil)

		c.Check(model.Units, qt.Equals, int64(1))
	},
}, {
	name: "UpdateUnit",
	deltas: [][]jujuparams.Delta{
		{{
			Entity: &jujuparams.UnitInfo{
				ModelUUID: "00000002-0000-0000-0000-000000000001",
				Name:      "app-1/2",
			},
		}},
		{{
			Entity: &jujuparams.UnitInfo{
				ModelUUID: "00000002-0000-0000-0000-000000000001",
				Name:      "app-1/2",
			},
		}},
		nil,
	},
	checkDB: func(c *qt.C, db db.Database) {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Assert(err, qt.IsNil)

		c.Check(model.Units, qt.Equals, int64(1))
	},
}, {
	name: "DeleteUnit",
	deltas: [][]jujuparams.Delta{
		{{
			Entity: &jujuparams.UnitInfo{
				ModelUUID: "00000002-0000-0000-0000-000000000001",
				Name:      "app-1/0",
			},
		}},
		{{
			Removed: true,
			Entity: &jujuparams.UnitInfo{
				ModelUUID: "00000002-0000-0000-0000-000000000001",
				Name:      "app-1/0",
			},
		}},
		nil,
	},
	checkDB: func(c *qt.C, db db.Database) {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Assert(err, qt.IsNil)

		c.Check(model.Units, qt.Equals, int64(0))
	},
}, {
	name: "UnknownModelsIgnored",
	deltas: [][]jujuparams.Delta{
		{{
			Entity: &jujuparams.ModelUpdate{
				ModelUUID: "00000002-0000-0000-0000-000000000004",
				Name:      "new-model",
				Owner:     "charlie@external",
				Life:      "starting",
			},
		}},
		nil,
	},
	checkDB: func(c *qt.C, db db.Database) {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000004",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Check(err, qt.ErrorMatches, `model not found`)
		c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
	},
}, {
	name: "UpdateModel",
	deltas: [][]jujuparams.Delta{
		{{
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
		}},
		nil,
	},
	checkDB: func(c *qt.C, db db.Database) {
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
		model.Users = nil
		c.Check(model, jimmtest.DBObjectEquals, dbmodel.Model{
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
	deltas: [][]jujuparams.Delta{
		{{
			Removed: true,
			Entity: &jujuparams.ModelUpdate{
				ModelUUID: "00000002-0000-0000-0000-000000000002",
			},
		}},
		nil,
	},
	checkDB: func(c *qt.C, db db.Database) {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000002",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Check(err, qt.ErrorMatches, `model not found`)
		c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
	},
}, {
	name: "DeleteDeadModel",
	deltas: [][]jujuparams.Delta{
		{{
			Removed: true,
			Entity: &jujuparams.ModelUpdate{
				ModelUUID: "00000002-0000-0000-0000-000000000003",
			},
		}},
		nil,
	},
	checkDB: func(c *qt.C, db db.Database) {
		ctx := context.Background()

		model := dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000003",
				Valid:  true,
			},
		}
		err := db.GetModel(ctx, &model)
		c.Check(err, qt.ErrorMatches, `model not found`)
		c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
	},
}, {
	name: "DeleteLivingModelFails",
	deltas: [][]jujuparams.Delta{
		{{
			Removed: true,
			Entity: &jujuparams.ModelUpdate{
				ModelUUID: "00000002-0000-0000-0000-000000000001",
			},
		}},
		nil,
	},
	checkDB: func(c *qt.C, db db.Database) {
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
		model.Users = nil
		c.Check(model, jimmtest.DBObjectEquals, dbmodel.Model{
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
						ModelInfo_: func(_ context.Context, info *jujuparams.ModelInfo) error {
							switch info.UUID {
							case "00000002-0000-0000-0000-000000000002":
								return errors.E(errors.CodeNotFound)
							case "00000002-0000-0000-0000-000000000003":
								return errors.E(errors.CodeUnauthorized)
							default:
								c.Errorf("unexpected model uuid: %s", info.UUID)
								return errors.E("unexpected API call")
							}

						},
					},
				},
			}

			env := jimmtest.ParseEnvironment(c, testWatcherEnv)
			err := w.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, w.Database)

			if test.initDB != nil {
				test.initDB(c, w.Database)
			}

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

			test.checkDB(c, w.Database)
		})
	}
}

var modelSummaryWatcherTests = []struct {
	name           string
	summaries      [][]jujuparams.ModelAbstract
	checkPublisher func(*qt.C, *testPublisher)
}{{
	name: "ModelSummaries",
	summaries: [][]jujuparams.ModelAbstract{
		{{
			UUID:   "00000002-0000-0000-0000-000000000001",
			Status: "test status",
			Size: jujuparams.ModelSummarySize{
				Applications: 1,
				Machines:     2,
				Containers:   3,
				Units:        4,
				Relations:    12,
			},
			Admins: []string{"alice@external", "bob"},
		}, {
			// this is a summary for an model unknown to jimm
			// meaning its summary will not be published
			// to the pubsub hub.
			UUID:   "00000002-0000-0000-0000-000000000004",
			Status: "test status 2",
			Size: jujuparams.ModelSummarySize{
				Applications: 5,
				Machines:     4,
				Containers:   3,
				Units:        2,
				Relations:    1,
			},
			Admins: []string{"bob@external"},
		}},
		nil,
	},
	checkPublisher: func(c *qt.C, publisher *testPublisher) {
		c.Assert(publisher.messages, qt.DeepEquals, []interface{}{
			jujuparams.ModelAbstract{
				UUID:   "00000002-0000-0000-0000-000000000001",
				Status: "test status",
				Size: jujuparams.ModelSummarySize{
					Applications: 1,
					Machines:     2,
					Containers:   3,
					Units:        4,
					Relations:    12,
				},
				Admins: []string{"alice@external"},
			},
		})
	},
}}

func TestModelSummaryWatcher(t *testing.T) {
	c := qt.New(t)

	for _, test := range modelSummaryWatcherTests {
		c.Run(test.name, func(c *qt.C) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			nextC := make(chan []jujuparams.ModelAbstract)
			var stopped uint32

			publisher := &testPublisher{}

			w := &jimm.Watcher{
				Pubsub: publisher,
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: &jimmtest.Dialer{
					API: &jimmtest.API{
						WatchAllModelSummaries_: func(_ context.Context) (string, error) {
							return test.name, nil
						},
						ModelSummaryWatcherNext_: func(ctx context.Context, id string) ([]jujuparams.ModelAbstract, error) {
							if id != test.name {
								return nil, errors.E("incorrect id")
							}

							select {
							case <-ctx.Done():
								return nil, ctx.Err()
							case summaries, ok := <-nextC:
								c.Logf("ModelSummaryWatcherNext received %#v, %v", summaries, ok)
								if ok {
									return summaries, nil
								}
								cancel()
								<-ctx.Done()
								return nil, ctx.Err()
							}
						},
						ModelSummaryWatcherStop_: func(_ context.Context, id string) error {
							if id != test.name {
								return errors.E("incorrect id")
							}
							atomic.StoreUint32(&stopped, 1)
							return nil
						},
						SupportsModelSummaryWatcher_: true,
						ModelInfo_: func(_ context.Context, info *jujuparams.ModelInfo) error {
							switch info.UUID {
							default:
								c.Errorf("unexpected model uuid: %s", info.UUID)
							case "00000002-0000-0000-0000-000000000002":
							case "00000002-0000-0000-0000-000000000003":
							}
							return errors.E(errors.CodeNotFound)
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
				err := w.WatchAllModelSummaries(ctx, time.Millisecond)
				c.Check(err, qt.ErrorMatches, `context canceled`, qt.Commentf("unexpected error %s (%#v)", err, err))
			}()

			for _, summary := range test.summaries {
				select {
				case nextC <- summary:
				case <-ctx.Done():
					c.Fatal("context closed prematurely")
				}
			}
			close(nextC)
			wg.Wait()

			test.checkPublisher(c, publisher)
		})
	}
}

func TestWatcherSetsControllerUnavailable(t *testing.T) {
	c := qt.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := &jimm.Watcher{
		Pubsub: &testPublisher{},
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

	<-time.After(5 * time.Millisecond)
	ctl := dbmodel.Controller{
		Name: "controller-1",
	}
	err = w.Database.GetController(ctx, &ctl)
	c.Assert(err, qt.IsNil)
	c.Check(ctl.UnavailableSince.Valid, qt.Equals, true)
	c.Check(ctl.UnavailableSince.Time.After(time.Now().Add(-10*time.Millisecond)), qt.Equals, true)
	cancel()
	wg.Wait()
}

func TestWatcherRemoveDyingModelsOnStartup(t *testing.T) {
	c := qt.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := &jimm.Watcher{
		Pubsub: &testPublisher{},
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
					switch info.UUID {
					default:
						c.Errorf("unexpected model uuid: %s", info.UUID)
					case "00000002-0000-0000-0000-000000000002":
					case "00000002-0000-0000-0000-000000000003":
					}
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
  cloud: test-cloud
  region: test-cloud-region
- name: controller-2
  uuid: 00000001-0000-0000-0000-000000000002
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
  owner: alice@external
  life: alive
`

func TestWatcherIgnoreDeltasForModelsFromIncorrectController(t *testing.T) {
	c := qt.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nextC := make(chan []jujuparams.Delta)
	w := &jimm.Watcher{
		Pubsub: &testPublisher{},
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

const pollControllerModelEnv = `clouds:
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
  - user: dawn@external
    access: read
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
    last-connection: 2020-02-20T01:02:03Z
  - user: bob@external
    access: write
    last-connection: 2020-02-20T01:02:03Z
  - user: charlie@external
    access: read
    last-connection: 2020-02-20T01:02:03Z
  - user: dawn@external
    access: read
`

func TestPollControllerModel(t *testing.T) {
	c := qt.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t0 := time.Date(2020, time.February, 20, 1, 2, 3, 0, time.UTC)
	t1 := time.Date(2020, time.February, 20, 2, 2, 3, 0, time.UTC)
	t2 := time.Date(2020, time.February, 20, 0, 2, 3, 0, time.UTC)

	w := &jimm.Watcher{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, nil),
		},
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{
				ModelInfo_: func(_ context.Context, mi *jujuparams.ModelInfo) error {
					mi.Users = []jujuparams.ModelUserInfo{{
						UserName: "alice@external",
						Access:   "admin",
					}, {
						UserName:       "bob@external",
						Access:         "write",
						LastConnection: &t1,
					}, {
						UserName:       "charlie@external",
						Access:         "read",
						LastConnection: &t2,
					}, {
						UserName: "dawn@external",
						Access:   "read",
					}}
					return nil
				},
			},
		},
	}

	env := jimmtest.ParseEnvironment(c, pollControllerModelEnv)
	err := w.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)
	env.PopulateDB(c, w.Database)

	ctl := env.Controller("controller-1").DBObject(c, w.Database)
	w.PollControllerModels(ctx, &ctl)

	m1 := dbmodel.Model{
		UUID: sql.NullString{
			String: "00000002-0000-0000-0000-000000000001",
			Valid:  true,
		},
	}
	m2 := dbmodel.Model{
		UUID: sql.NullString{
			String: "00000002-0000-0000-0000-000000000002",
			Valid:  true,
		},
	}
	err = w.Database.GetModel(ctx, &m1)
	c.Assert(err, qt.IsNil)
	err = w.Database.GetModel(ctx, &m2)
	c.Assert(err, qt.IsNil)

	c.Check(m1.Users[0].LastConnection.Valid, qt.Equals, false)
	c.Check(m1.Users[1].LastConnection.Valid, qt.Equals, true)
	c.Check(m1.Users[1].LastConnection.Time, qt.Equals, t1)
	c.Check(m1.Users[2].LastConnection.Valid, qt.Equals, true)
	c.Check(m1.Users[2].LastConnection.Time, qt.Equals, t2)
	c.Check(m1.Users[3].LastConnection.Valid, qt.Equals, false)
	c.Check(m2.Users[0].LastConnection.Valid, qt.Equals, true)
	c.Check(m2.Users[0].LastConnection.Time, qt.Equals, t0)
	c.Check(m2.Users[1].LastConnection.Valid, qt.Equals, true)
	c.Check(m2.Users[1].LastConnection.Time, qt.Equals, t1)
	c.Check(m2.Users[2].LastConnection.Valid, qt.Equals, true)
	c.Check(m2.Users[2].LastConnection.Time, qt.Equals, t0)
	c.Check(m2.Users[3].LastConnection.Valid, qt.Equals, false)
}

func TestPollModelsStops(t *testing.T) {
	c := qt.New(t)

	w := &jimm.Watcher{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, nil),
		},
	}

	err := w.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	now := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err = w.PollModels(ctx, time.Minute)
	c.Check(time.Since(now), qt.Satisfies, func(d time.Duration) bool {
		return d < time.Second
	})
	c.Check(err, qt.ErrorMatches, `context deadline exceeded`)
}

type testPublisher struct {
	mu       sync.Mutex
	messages []interface{}
}

func (p *testPublisher) Publish(model string, content interface{}) <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messages = append(p.messages, content)

	done := make(chan struct{})
	close(done)
	return done
}

func newUint64(i uint64) *uint64 {
	return &i
}
