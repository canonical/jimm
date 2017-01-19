// Copyright 2017 Canonical Ltd.

package usagesender_test

import (
	"fmt"
	"net/http/httptest"
	"sync"
	"time"

	omniapi "github.com/CanonicalLtd/omnibus/metrics-collector/api"
	"github.com/juju/httprequest"
	jujujujutesting "github.com/juju/juju/testing"
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

type usageSenderSuite struct {
	apitest.Suite

	// clock holds the mock clock used by the monitor package.
	clock   *jujutesting.Clock
	handler *testHandler
	server  *httptest.Server
}

var _ = gc.Suite(&usageSenderSuite{})

var (
	testContext = context.Background()
	epoch       = mustParseTime("2016-01-01T12:00:00Z")
)

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
	s.ServerParams = external_jem.ServerParams{UsageSenderURL: s.server.URL}

	// Set up the clock mockery.
	s.clock = jujutesting.NewClock(epoch)
	s.PatchValue(usagesender.SenderClock, s.clock)
	s.Suite.SetUpTest(c)
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

func (s *usageSenderSuite) setUnitNumberAndCheckSentMetrics(c *gc.C, modelUUID string, unitCount int, acknowledge bool) {
	m := &testMonitor{failed: make(chan int)}
	s.PatchValue(usagesender.MonitorFailure, m.set)

	s.handler.setAcknowledge(acknowledge)

	err := s.JEM.DB.UpdateModelCounts(testContext, modelUUID, map[params.EntityCount]int{
		params.MachineCount: 0,
		params.UnitCount:    unitCount,
	}, s.clock.Now())
	c.Assert(err, jc.ErrorIsNil)

	s.clock.Advance(6 * time.Minute)

	unitCountString := fmt.Sprintf("%d", unitCount)

	select {
	case receivedUnitCount := <-s.handler.receivedMetrics:
		c.Assert(receivedUnitCount, gc.Equals, unitCountString)
	case <-time.After(jujujujutesting.LongWait):
		c.Fail()
	}
	if !acknowledge {
		select {
		case failed := <-m.failed:
			c.Assert(failed, gc.Equals, 1)
		case <-time.After(jujujujutesting.LongWait):
			c.Fail()
		}
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

func (c *testHandler) Metrics(arg *usagePost) (*omniapi.Response, error) {
	if len(arg.Body) == 1 && len(arg.Body[0].Metrics) == 1 {
		c.receivedMetrics <- arg.Body[0].Metrics[0].Value
	} else {
		c.receivedMetrics <- "-1"
	}

	uuids := make([]string, len(arg.Body))
	for i, b := range arg.Body {
		uuids[i] = b.UUID
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.acknowledge {
		return &omniapi.Response{
			UUID: utils.MustNewUUID().String(),
			UserResponses: map[string]omniapi.UserResponse{
				"bob": omniapi.UserResponse{AcknowledgedBatches: uuids},
			},
		}, nil
	}
	return &omniapi.Response{}, nil
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
