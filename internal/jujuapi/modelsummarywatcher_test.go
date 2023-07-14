// Copyright 2020 Canonical Ltd.

package jujuapi_test

import (
	"context"
	"sync"
	"time"

	jujuparams "github.com/juju/juju/rpc/params"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/internal/jujuapi"
)

type modelSummaryWatcherSuite struct{}

var _ = gc.Suite(&modelSummaryWatcherSuite{})

func (s *modelSummaryWatcherSuite) TestModelSummaryWatcher(c *gc.C) {
	watcher := jujuapi.NewModelSummaryWatcher()
	defer watcher.Stop()

	result, err := watcher.Next()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, jujuparams.SummaryWatcherNextResults{
		Models: []jujuparams.ModelAbstract{},
	})

	jujuapi.PublishToWatcher(watcher, "test-model", jujuparams.ModelAbstract{
		UUID: "12345",
		Name: "test-model",
	})
	jujuapi.PublishToWatcher(watcher, "test-model-2", jujuparams.ModelAbstract{
		UUID: "12346",
		Name: "test-model-2",
	})

	result, err = watcher.Next()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, jujuparams.SummaryWatcherNextResults{
		Models: []jujuparams.ModelAbstract{{
			UUID: "12345",
			Name: "test-model",
		}, {
			UUID: "12346",
			Name: "test-model-2",
		}},
	})
}

func (s *modelSummaryWatcherSuite) TestModelAccessWatcher(c *gc.C) {

	ctx, cancelFunc := context.WithCancel(context.Background())

	modelGetter := &testModelGetter{
		calledChan: make(chan bool, 1),
	}

	watcher := jujuapi.NewModelAccessWatcher(ctx, 100*time.Millisecond, modelGetter.getModels)
	jujuapi.RunModelAccessWatcher(watcher)

	select {
	case <-modelGetter.calledChan:
	case <-time.After(200 * time.Millisecond):
		c.Fatalf("timed oud")
	}

	match := jujuapi.ModelAccessWatcherMatch(watcher, "model1")
	c.Assert(match, jc.IsFalse)

	modelGetter.setModels([]string{"model1", "model2"})

	select {
	case <-modelGetter.calledChan:
	case <-time.After(200 * time.Millisecond):
		c.Fatalf("timed oud")
	}

	match = jujuapi.ModelAccessWatcherMatch(watcher, "model1")
	c.Assert(match, jc.IsTrue)

	match = jujuapi.ModelAccessWatcherMatch(watcher, "model2")
	c.Assert(match, jc.IsTrue)

	match = jujuapi.ModelAccessWatcherMatch(watcher, "model3")
	c.Assert(match, jc.IsFalse)

	cancelFunc()

	modelGetter.setModels([]string{"model1", "model3"})

	<-time.After(200 * time.Millisecond)

	match = jujuapi.ModelAccessWatcherMatch(watcher, "model2")
	c.Assert(match, jc.IsTrue)
}

type testModelGetter struct {
	mu         sync.Mutex
	models     []string
	called     bool
	calledChan chan bool
}

func (t *testModelGetter) setModels(models []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.models = models
	t.called = false
}

func (t *testModelGetter) getModels(_ context.Context) ([]string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.called == false {
		t.calledChan <- true
	}
	t.called = true
	return t.models, nil
}
