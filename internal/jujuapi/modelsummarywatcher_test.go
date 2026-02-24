// Copyright 2025 Canonical.

package jujuapi_test

import (
	"context"
	"sync"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/v3/internal/jujuapi"
)

func TestModelSummaryWatcher(t *testing.T) {
	c := qt.New(t)

	watcher := jujuapi.NewModelSummaryWatcher()
	defer func() {
		err := watcher.Stop()
		c.Assert(err, qt.IsNil)
	}()
	result, err := watcher.Next()
	c.Assert(err, qt.IsNil)
	c.Assert(result, qt.DeepEquals, jujuparams.SummaryWatcherNextResults{
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
	c.Assert(err, qt.IsNil)
	c.Assert(result, qt.DeepEquals, jujuparams.SummaryWatcherNextResults{
		Models: []jujuparams.ModelAbstract{{
			UUID: "12345",
			Name: "test-model",
		}, {
			UUID: "12346",
			Name: "test-model-2",
		}},
	})
}

func TestModelAccessWatcher(t *testing.T) {
	c := qt.New(t)

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	modelGetter := &testModelGetter{
		calledChan: make(chan bool, 1),
	}

	watcher := jujuapi.NewModelAccessWatcher(ctx, 100*time.Millisecond, modelGetter.getModels)
	wg := sync.WaitGroup{}
	jujuapi.RunModelAccessWatcher(watcher, &wg)

	select {
	case <-modelGetter.calledChan:
	case <-time.After(200 * time.Millisecond):
		c.Fatalf("timed out")
	}

	match := jujuapi.ModelAccessWatcherMatch(watcher, "model1")
	c.Assert(match, qt.IsFalse)

	modelGetter.setModels([]string{"model1", "model2"})

	select {
	case <-modelGetter.calledChan:
	case <-time.After(200 * time.Millisecond):
		c.Fatalf("timed out")
	}

	// Once the modelGetter has been called, the watcher should have the models.
	// We then cancel the watcher and call Wait() as way of synchronising the test
	// to ensure the watcher has processed the models.
	cancelFunc()
	wg.Wait()

	match = jujuapi.ModelAccessWatcherMatch(watcher, "model1")
	c.Assert(match, qt.IsTrue)

	match = jujuapi.ModelAccessWatcherMatch(watcher, "model2")
	c.Assert(match, qt.IsTrue)

	match = jujuapi.ModelAccessWatcherMatch(watcher, "model3")
	c.Assert(match, qt.IsFalse)

	// Now with the watcher stopped, we set new models and
	// check that the previous models are still matched.
	modelGetter.setModels([]string{"model1", "model3"})

	<-time.After(200 * time.Millisecond)

	match = jujuapi.ModelAccessWatcherMatch(watcher, "model2")
	c.Assert(match, qt.IsTrue)
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
