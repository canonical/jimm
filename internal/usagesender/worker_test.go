// Copyright 2017 Canonical Ltd.

package usagesender_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"sync"
	"time"

	"github.com/juju/clock/testclock"
	jujujujutesting "github.com/juju/juju/testing"
	"github.com/julienschmidt/httprouter"
	gc "gopkg.in/check.v1"
	"gopkg.in/httprequest.v1"

	external_jem "github.com/CanonicalLtd/jimm"
	"github.com/CanonicalLtd/jimm/internal/apitest"
	"github.com/CanonicalLtd/jimm/internal/jemerror"
	"github.com/CanonicalLtd/jimm/internal/usagesender"
	"github.com/CanonicalLtd/jimm/params"
)

var (
	epoch = mustParseTime("2016-01-01T12:00:00Z")
)

var _ = gc.Suite(&spoolDirMetricRecorderSuite{})

type spoolDirMetricRecorderSuite struct {
	usageSenderSuite
}

func (s *spoolDirMetricRecorderSuite) SetUpTest(c *gc.C) {
	s.MetricsSpoolPath = c.MkDir()
	s.ServerParams = external_jem.ServerParams{
		UsageSenderSpoolPath: s.MetricsSpoolPath,
	}
	s.usageSenderSuite.SetUpTest(c)
}

var _ = gc.Suite(&sliceMetricRecorderSuite{})

type sliceMetricRecorderSuite struct {
	usageSenderSuite
}

type usageSenderSuite struct {
	apitest.Suite

	handler *testHandler
	server  *httptest.Server
}

func (s *usageSenderSuite) SetUpTest(c *gc.C) {
	s.handler = &testHandler{receivedMetrics: make(chan receivedMetric)}

	router := httprouter.New()
	srv := httprequest.Server{
		ErrorMapper: jemerror.Mapper,
	}
	handlers := srv.Handlers(func(p httprequest.Params) (*testHandler, context.Context, error) {
		return s.handler, p.Context, nil
	})
	for _, h := range handlers {
		router.Handle(h.Method, h.Path, h.Handle)
	}

	s.server = httptest.NewServer(router)

	// Set up the clock mockery.
	s.Clock = testclock.NewClock(epoch)

	s.ServerParams.UsageSenderURL = s.server.URL

	s.Suite.SetUpTest(c)
}

func (s *usageSenderSuite) TearDownTest(c *gc.C) {
	s.server.Close()
	s.Suite.TearDownTest(c)
}

func (s *usageSenderSuite) TestUsageSenderWorker(c *gc.C) {
	ctx := context.Background()

	ctlId := s.AssertAddController(ctx, c, params.EntityPath{"bob", "foo"}, false)
	cred := s.AssertUpdateCredential(ctx, c, "bob", "dummy", "cred1", "empty")
	_, uuid := s.CreateModel(ctx, c, params.EntityPath{"bob", "foo"}, ctlId, cred)

	s.setUnitNumberAndCheckSentMetrics(ctx, c, ctlId, uuid, 0, true)
	s.setUnitNumberAndCheckSentMetrics(ctx, c, ctlId, uuid, 0, false)

	s.setUnitNumberAndCheckSentMetrics(ctx, c, ctlId, uuid, 99, true)
	s.setUnitNumberAndCheckSentMetrics(ctx, c, ctlId, uuid, 99, false)

	s.setUnitNumberAndCheckSentMetrics(ctx, c, ctlId, uuid, 42, true)
	s.setUnitNumberAndCheckSentMetrics(ctx, c, ctlId, uuid, 42, false)
}

func (s *spoolDirMetricRecorderSuite) TestSpool(c *gc.C) {
	ctx := context.Background()

	ctlId := s.AssertAddController(ctx, c, params.EntityPath{"bob", "foo"}, false)
	cred := s.AssertUpdateCredential(ctx, c, "bob", "dummy", "cred1", "empty")
	_, model := s.CreateModel(ctx, c, params.EntityPath{"bob", "test-model"}, ctlId, cred)

	m := &testMonitor{failed: make(chan int, 10)}
	s.PatchValue(usagesender.MonitorFailure, m.set)

	// on first attempt the metrics will not be acknowledged - will
	// remain stored in the spool directory and should be resent on the
	// next attempt
	s.handler.setAcknowledge(false)

	err := s.JEM.UpdateModelCounts(ctx, ctlId, model, map[params.EntityCount]int{
		params.MachineCount: 0,
		params.UnitCount:    17,
	}, s.Clock.Now())
	c.Assert(err, gc.Equals, nil)

	s.Clock.WaitAdvance(6*time.Minute, jujujujutesting.LongWait, 1)

	select {
	case receivedMetric := <-s.handler.receivedMetrics:
		c.Assert(receivedMetric.value, gc.Equals, "17")
	case <-time.After(jujujujutesting.LongWait):
		c.Fatal("timed out waiting for metrics to be received")
	}

	// on the second attempt all metrics will be acknowledged
	s.handler.setAcknowledge(true)

	err = s.JEM.UpdateModelCounts(ctx, ctlId, model, map[params.EntityCount]int{
		params.MachineCount: 0,
		params.UnitCount:    42,
	}, s.Clock.Now())
	c.Assert(err, gc.Equals, nil)

	s.Clock.WaitAdvance(6*time.Minute, jujujujutesting.LongWait, 1)

	// we expect both metrics to be sent this time
	a := "17"
	select {
	case receivedMetric := <-s.handler.receivedMetrics:
		c.Logf("received %v", receivedMetric)
		if receivedMetric.value == a {
			a = "42"
		}
	case <-time.After(jujujujutesting.LongWait):
		c.Fatal("timed out waiting for metrics to be received")
	}
	select {
	case receivedMetric := <-s.handler.receivedMetrics:
		c.Logf("received %v", receivedMetric)
		c.Assert(receivedMetric.value, gc.Equals, a)
		c.Assert(receivedMetric.modelName, gc.Equals, "bob/test-model")
	case <-time.After(jujujujutesting.LongWait):
		c.Fatal("timed out waiting for metrics to be received")
	}
}

func (s *usageSenderSuite) setUnitNumberAndCheckSentMetrics(ctx context.Context, c *gc.C, ctlPath params.EntityPath, modelUUID string, unitCount int, acknowledge bool) {
	m := &testMonitor{failed: make(chan int)}
	s.PatchValue(usagesender.MonitorFailure, m.set)

	s.handler.setAcknowledge(acknowledge)

	err := s.JEM.UpdateModelCounts(ctx, ctlPath, modelUUID, map[params.EntityCount]int{
		params.MachineCount: 0,
		params.UnitCount:    unitCount,
	}, s.Clock.Now())
	c.Assert(err, gc.Equals, nil)

	s.Clock.WaitAdvance(6*time.Minute, jujujujutesting.LongWait, 1)

	unitCountString := fmt.Sprintf("%d", unitCount)

	select {
	case receivedMetric := <-s.handler.receivedMetrics:
		c.Assert(receivedMetric.value, gc.Equals, unitCountString)
		c.Assert(receivedMetric.modelName, gc.Equals, "bob/foo")
	case <-time.After(jujujujutesting.LongWait):
		c.Fatal("timed out waiting for metrics to be received")
	}
	if !acknowledge {
		select {
		case failed := <-m.failed:
			c.Assert(failed, gc.Equals, 1)
		case <-time.After(jujujujutesting.LongWait):
			c.Fatal("timed out waiting for metrics batch to be acknowledged")
		}
		err = os.RemoveAll(s.MetricsSpoolPath)
		c.Assert(err, gc.Equals, nil)
	}
}

type testMonitor struct {
	failed chan int
}

func (m *testMonitor) set(value float64) {
	m.failed <- int(value)
}

type usagePost struct {
	httprequest.Route `httprequest:"POST /v4/jimm/metrics"`
	Body              []usagesender.MetricBatch `httprequest:",body"`
}

type testHandler struct {
	mutex           sync.Mutex
	acknowledge     bool
	receivedMetrics chan receivedMetric
}

type receivedMetric struct {
	value     string
	modelName string
}

func (c *testHandler) Metrics(arg *usagePost) (*usagesender.Response, error) {
	for _, b := range arg.Body {
		c.receivedMetrics <- receivedMetric{
			value:     b.Metrics[0].Value,
			modelName: b.Metrics[0].Tags[usagesender.ModelNameTag],
		}
	}

	uuids := make([]string, len(arg.Body))
	for i, b := range arg.Body {
		uuids[i] = b.UUID
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.acknowledge {
		return &usagesender.Response{
			UserStatus: map[string]usagesender.UserStatus{
				"bob": usagesender.UserStatus{
					Code: "GREEN",
					Info: "",
				}},
		}, nil
	}
	return nil, fmt.Errorf("error")
}

func (c *testHandler) setAcknowledge(value bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.acknowledge = value
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
