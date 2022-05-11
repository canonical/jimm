// Copyright 2021 Canonical Ltd.

package jimm_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	"github.com/CanonicalLtd/jimm/internal/pubsub"
)

func TestWatchAllModelSummaries(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now()
	controllerUUID := "00000000-0000-0000-0000-0000-0000000000001"
	modelUUID := "00000000-0000-0000-0000-0000-0000000000002"
	controllerName := "test-controller-1"

	errorChannel := make(chan error, 1)
	api := &jimmtest.API{
		WatchAllModelSummaries_: func(context.Context) (string, error) {
			select {
			case err := <-errorChannel:
				return "test-id", err
			default:
				return "test-id", nil
			}
		},
		SupportsModelSummaryWatcher_: true,
		ModelSummaryWatcherNext_: func(_ context.Context, id string) ([]jujuparams.ModelAbstract, error) {
			if id != "test-id" {
				return nil, errors.E("watcher not found", errors.CodeNotFound)
			}
			return []jujuparams.ModelAbstract{{
				UUID:       modelUUID,
				Controller: controllerName,
				Name:       "test-model",
				Admins:     []string{"alice@external", "bob@external"},
				Cloud:      "test-cloud",
				Region:     "test-cloud-region",
				Size: jujuparams.ModelSummarySize{
					Machines:     0,
					Containers:   0,
					Applications: 0,
					Units:        0,
					Relations:    0,
				},
				Status: "green",
			}}, nil
		},
		ModelSummaryWatcherStop_: func(context.Context, string) error {
			return nil
		},
	}
	pubsub := &pubsub.Hub{
		MaxConcurrency: 10,
	}

	j := &jimm.JIMM{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
		},
		Dialer: &jimmtest.Dialer{
			API: api,
		},
		Pubsub: pubsub,
	}
	err := j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
	}
	err = j.Database.AddCloud(context.Background(), &cloud)
	c.Assert(err, qt.Equals, nil)

	controller := dbmodel.Controller{
		Name:      controllerName,
		UUID:      controllerUUID,
		CloudName: "test-cloud",
	}
	err = j.Database.AddController(context.Background(), &controller)
	c.Assert(err, qt.Equals, nil)

	summaryChannel := make(chan interface{}, 1)
	handlerFunction := func(_ string, summary interface{}) {
		select {
		case summaryChannel <- summary:
		default:
		}
	}

	cleanup, err := pubsub.Subscribe(modelUUID, handlerFunction)
	c.Assert(err, qt.Equals, nil)
	defer cleanup()

	watcherCleanup, err := j.WatchAllModelSummaries(context.Background(), &controller)
	c.Assert(err, qt.Equals, nil)
	defer func() {
		err := watcherCleanup()
		if err != nil {
			c.Logf("failed to stop all model summaries watcher: %v", err)
		}
	}()

	select {
	case summary := <-summaryChannel:
		c.Check(summary, qt.DeepEquals,
			jujuparams.ModelAbstract{
				UUID:       modelUUID,
				Removed:    false,
				Controller: controllerName,
				Name:       "test-model",
				Cloud:      "test-cloud",
				Region:     "test-cloud-region",
				Admins:     []string{"alice@external", "bob@external"},
				Size: jujuparams.ModelSummarySize{
					Machines:     0,
					Containers:   0,
					Applications: 0,
					Units:        0,
					Relations:    0,
				},
				Status: "green",
			})
	case <-time.After(time.Second):
		c.Fatal("timed out")
	}

	select {
	case errorChannel <- errors.E("an error"):
	default:
	}

	_, err = j.WatchAllModelSummaries(context.Background(), &controller)
	c.Assert(err, qt.ErrorMatches, "an error")
}
