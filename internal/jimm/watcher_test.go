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
	jujuparams "github.com/juju/juju/rpc/params"

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
- name: model-2
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000002
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: dying
- name: model-3
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000003
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: dead
`

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
		err := w.WatchAllModelSummaries(ctx, time.Millisecond)
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
				ModelSummaryWatcherNext_: func(ctx context.Context, s string) ([]jujuparams.ModelAbstract, error) {
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
				WatchAllModelSummaries_: func(ctx context.Context) (string, error) {
					return "1234", nil
				},
				SupportsModelSummaryWatcher_: true,
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
		err := w.WatchAllModelSummaries(ctx, time.Millisecond)
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
