// Copyright 2017 Canonical Ltd.

package usagesender_test

import (
	"fmt"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/juju/httprequest"
	jujujujutesting "github.com/juju/juju/testing"
	romulus "github.com/juju/romulus/wireformat/metrics"
	wireformat "github.com/juju/romulus/wireformat/metrics"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"

	external_jem "github.com/CanonicalLtd/jem"
	"github.com/CanonicalLtd/jem/internal/apitest"
	"github.com/CanonicalLtd/jem/internal/jemerror"
	"github.com/CanonicalLtd/jem/internal/usagesender"
	"github.com/CanonicalLtd/jem/params"
)

var (
	testContext = context.Background()
	epoch       = mustParseTime("2016-01-01T12:00:00Z")
)

var _ = gc.Suite(&mgoMetricRecorderSuite{})

type mgoMetricRecorderSuite struct {
	usageSenderSuite
}

func (s *mgoMetricRecorderSuite) SetUpTest(c *gc.C) {
	s.ServerParams = external_jem.ServerParams{
		UsageSenderCollection: "usagetest",
	}
	s.usageSenderSuite.SetUpTest(c)
}

type usageSenderSuite struct {
	apitest.Suite

	handler *testHandler
	server  *httptest.Server
}

func (s *usageSenderSuite) SetUpTest(c *gc.C) {
	s.handler = &testHandler{receivedMetrics: make(chan string, 1)}

	router := httprouter.New()
	handlers := jemerror.Mapper.Handlers(func(_ httprequest.Params) (*testHandler, error) {
		return s.handler, nil
	})
	for _, h := range handlers {
		router.Handle(h.Method, h.Path, h.Handle)
	}

	s.server = httptest.NewServer(router)

	// Set up the clock mockery.
	s.Clock = jujutesting.NewClock(epoch)

	s.ServerParams.UsageSenderURL = s.server.URL

	s.Suite.SetUpTest(c)

	err := s.Session.DB("jem").DropDatabase()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *usageSenderSuite) TearDownTest(c *gc.C) {
	s.server.Close()
	s.Suite.TearDownTest(c)
}

func (s *usageSenderSuite) TestUsageSenderWorker(c *gc.C) {
	ctlId := s.AssertAddController(c, params.EntityPath{"bob", "foo"}, false)
	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")
	_, uuid := s.CreateModel(c, params.EntityPath{"bob", "foo"}, ctlId, cred)

	s.setUnitNumberAndCheckSentMetrics(c, uuid, 0, true)
	s.setUnitNumberAndCheckSentMetrics(c, uuid, 0, false)

	s.setUnitNumberAndCheckSentMetrics(c, uuid, 99, true)
	s.setUnitNumberAndCheckSentMetrics(c, uuid, 99, false)

	s.setUnitNumberAndCheckSentMetrics(c, uuid, 42, true)
	s.setUnitNumberAndCheckSentMetrics(c, uuid, 42, false)
}

func (s *mgoMetricRecorderSuite) TestSpool(c *gc.C) {
	ctlId := s.AssertAddController(c, params.EntityPath{"bob", "foo"}, false)
	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")
	_, model := s.CreateModel(c, params.EntityPath{"bob", "foo"}, ctlId, cred)

	m := &testMonitor{failed: make(chan int, 10)}
	s.PatchValue(usagesender.MonitorFailure, m.set)

	// on first attempt the metrics will not be acknowledged - will
	// remain stored in the spool directory and should be resent on the
	// next attempt
	s.handler.setAcknowledge(false)

	err := s.JEM.DB.UpdateModelCounts(testContext, model, map[params.EntityCount]int{
		params.MachineCount: 0,
		params.UnitCount:    17,
	}, s.Clock.Now())
	c.Assert(err, jc.ErrorIsNil)

	s.Clock.WaitAdvance(6*time.Minute, jujujujutesting.LongWait, 1)

	select {
	case receivedUnitCount := <-s.handler.receivedMetrics:
		c.Assert(receivedUnitCount, gc.Equals, "17")
	case <-time.After(jujujujutesting.LongWait):
		c.Fatal("timed out waiting for metrics to be received")
	}

	// on the second attempt all metrics will be acknowledged
	s.handler.setAcknowledge(true)

	err = s.JEM.DB.UpdateModelCounts(testContext, model, map[params.EntityCount]int{
		params.MachineCount: 0,
		params.UnitCount:    42,
	}, s.Clock.Now())
	c.Assert(err, jc.ErrorIsNil)

	s.Clock.WaitAdvance(6*time.Minute, jujujujutesting.LongWait, 1)

	// we expect both metrics to be sent this time
	a := "17"
	select {
	case receivedUnitCount := <-s.handler.receivedMetrics:
		c.Logf("received %v", receivedUnitCount)
		if receivedUnitCount == a {
			a = "42"
		}
	case <-time.After(jujujujutesting.LongWait):
		c.Fatal("timed out waiting for metrics to be received")
	}
	select {
	case receivedUnitCount := <-s.handler.receivedMetrics:
		c.Logf("received %v", receivedUnitCount)
		c.Assert(receivedUnitCount, gc.Equals, a)
	case <-time.After(jujujujutesting.LongWait):
		c.Fatal("timed out waiting for metrics to be received")
	}
}

func (s *usageSenderSuite) setUnitNumberAndCheckSentMetrics(c *gc.C, modelUUID string, unitCount int, acknowledge bool) {
	m := &testMonitor{failed: make(chan int)}
	s.PatchValue(usagesender.MonitorFailure, m.set)

	s.handler.setAcknowledge(acknowledge)

	err := s.JEM.DB.UpdateModelCounts(testContext, modelUUID, map[params.EntityCount]int{
		params.MachineCount: 0,
		params.UnitCount:    unitCount,
	}, s.Clock.Now())
	c.Assert(err, jc.ErrorIsNil)

	s.Clock.WaitAdvance(6*time.Minute, jujujujutesting.LongWait, 1)

	unitCountString := fmt.Sprintf("%d", unitCount)

	select {
	case receivedUnitCount := <-s.handler.receivedMetrics:
		c.Assert(receivedUnitCount, gc.Equals, unitCountString)
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

		_, err = s.Session.DB("jem").C("usagetest").RemoveAll(nil)
		c.Assert(err, jc.ErrorIsNil)
	}
}

type testMonitor struct {
	failed chan int
}

func (m *testMonitor) set(value float64) {
	m.failed <- int(value)
}

type usagePost struct {
	httprequest.Route `httprequest:"POST /metrics"`
	Body              []wireformat.MetricBatch `httprequest:",body"`
}

type testHandler struct {
	mutex           sync.Mutex
	acknowledge     bool
	receivedMetrics chan string
}

func (c *testHandler) Metrics(arg *usagePost) (*romulus.UserStatusResponse, error) {
	uuids := make([]string, len(arg.Body))
	for i, b := range arg.Body {
		uuids[i] = b.UUID
		c.receivedMetrics <- b.Metrics[0].Value
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.acknowledge {
		return &romulus.UserStatusResponse{
			UUID: utils.MustNewUUID().String(),
			UserResponses: map[string]romulus.UserResponse{
				"bob": romulus.UserResponse{AcknowledgedBatches: uuids},
			},
		}, nil
	}
	return &romulus.UserStatusResponse{}, nil
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
