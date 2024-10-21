// Copyright 2024 Canonical.

package jimm_test

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

const testWatcherEnv = `clouds:
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
    access: write
  - user: charlie@canonical.com
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
  owner: alice@canonical.com
  life: dying
- name: model-3
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000003
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
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
				Life:            life.Value(state.Alive.String()),
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
				Owner:     "charlie@canonical.com",
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
				Owner:          "alice@canonical.com",
				Life:           life.Value(state.Alive.String()),
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
		model.Owner = dbmodel.Identity{}
		model.CloudCredential = dbmodel.CloudCredential{}
		model.CloudRegion = dbmodel.CloudRegion{}
		model.Controller = dbmodel.Controller{}
		c.Check(model, jimmtest.DBObjectEquals, dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
			Name:          "model-1",
			Type:          "iaas",
			DefaultSeries: "warty",
			Life:          state.Alive.String(),
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
		model.Owner = dbmodel.Identity{}
		model.CloudCredential = dbmodel.CloudCredential{}
		model.CloudRegion = dbmodel.CloudRegion{}
		model.Controller = dbmodel.Controller{}
		c.Check(model, jimmtest.DBObjectEquals, dbmodel.Model{
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
			Name:          "model-1",
			Type:          "iaas",
			DefaultSeries: "warty",
			Life:          state.Alive.String(),
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

//nolint:gocognit
func TestWatcher(t *testing.T) {
	c := qt.New(t)

	for _, test := range watcherTests {
		c.Run(test.name, func(c *qt.C) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			nextC := make(chan []jujuparams.Delta, len(test.deltas))
			var stopped uint32

			deltaProcessedChannel := make(chan bool, len(test.deltas))

			w := jimm.NewWatcherWithDeltaProcessedChannel(
				db.Database{
					DB: jimmtest.PostgresDB(c, nil),
				},
				&jimmtest.Dialer{
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
				nil,
				deltaProcessedChannel,
			)

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
				checkIfContextCanceled(c, ctx, err)
			}()

			validDeltas := 0
			for _, d := range test.deltas {
				select {
				case nextC <- d:
					if d != nil {
						validDeltas++
					}
				case <-ctx.Done():
					c.Fatal("context closed prematurely")
				}
			}

			for i := 0; i < validDeltas; i++ {
				select {
				case <-deltaProcessedChannel:
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
			Admins: []string{"alice@canonical.com", "bob"},
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
			Admins: []string{"bob@canonical.com"},
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
				Admins: []string{"alice@canonical.com"},
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
					DB: jimmtest.PostgresDB(c, nil),
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
				checkIfContextCanceled(c, ctx, err)
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

	controllerUnavailableChannel := make(chan error, 1)
	w := jimm.NewWatcherWithControllerUnavailableChan(
		db.Database{
			DB: jimmtest.PostgresDB(c, nil),
		},
		&jimmtest.Dialer{
			Err: errors.E("test error"),
		},
		&testPublisher{},
		controllerUnavailableChannel,
	)

	env := jimmtest.ParseEnvironment(c, testWatcherEnv)
	err := w.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)
	env.PopulateDB(c, w.Database)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := w.Watch(ctx, time.Millisecond)
		checkIfContextCanceled(c, ctx, err)
	}()

	// it appears that the jimm code does not treat failing to
	// set a controller as unavailable as an error - so
	// the test will not treat it as one either.
	cerr := <-controllerUnavailableChannel
	if cerr != nil {
		ctl := dbmodel.Controller{
			Name: "controller-1",
		}
		err = w.Database.GetController(ctx, &ctl)
		c.Assert(err, qt.IsNil)
		c.Check(ctl.UnavailableSince.Valid, qt.Equals, true)
	}
	cancel()
	wg.Wait()
}

func TestWatcherClearsControllerUnavailable(t *testing.T) {
	c := qt.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := jimm.Watcher{
		Database: db.Database{
			DB: jimmtest.PostgresDB(c, nil),
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
		Pubsub: &testPublisher{},
	}

	env := jimmtest.ParseEnvironment(c, testWatcherEnv)
	err := w.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)
	env.PopulateDB(c, w.Database)

	// update controller's UnavailableSince field
	ctl := dbmodel.Controller{
		Name: "controller-1",
	}
	err = w.Database.GetController(ctx, &ctl)
	c.Assert(err, qt.IsNil)
	ctl.UnavailableSince = sql.NullTime{
		Time:  time.Now(),
		Valid: true,
	}
	err = w.Database.UpdateController(ctx, &ctl)
	c.Assert(err, qt.IsNil)

	// start the watcher
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := w.Watch(ctx, time.Millisecond)
		checkIfContextCanceled(c, ctx, err)
	}()
	wg.Wait()

	// check that the unavailable since time has been cleared
	ctl = dbmodel.Controller{
		Name: "controller-1",
	}
	err = w.Database.GetController(context.Background(), &ctl)
	c.Assert(err, qt.IsNil)
	c.Assert(ctl.UnavailableSince.Valid, qt.IsFalse)
}

func TestWatcherRemoveDyingModelsOnStartup(t *testing.T) {
	c := qt.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := &jimm.Watcher{
		Pubsub: &testPublisher{},
		Database: db.Database{
			DB: jimmtest.PostgresDB(c, nil),
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
		checkIfContextCanceled(c, ctx, err)
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
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
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
			DB: jimmtest.PostgresDB(c, nil),
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
		checkIfContextCanceled(c, ctx, err)
	}()

	nextC <- []jujuparams.Delta{{
		Entity: &jujuparams.ModelUpdate{
			ModelUUID: "00000002-0000-0000-0000-000000000001",
			Name:      "model-1",
			Owner:     "alice@canonical.com",
			Life:      life.Value(state.Alive.String()),
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

func checkIfContextCanceled(c *qt.C, ctx context.Context, err error) {
	errorToCheck := err
	if ctx.Err() != nil {
		errorToCheck = ctx.Err()
	}
	c.Check(
		errorToCheck,
		qt.ErrorMatches,
		`.*(context canceled|operation was canceled).*`, qt.Commentf("unexpected error %s (%#v)", err, err),
	)
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
