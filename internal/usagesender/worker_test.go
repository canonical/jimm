// Copyright 2017 Canonical Ltd.

package usagesender_test

import (
	"fmt"
	"net/http/httptest"
	"time"

	"github.com/juju/httprequest"
	jujujujutesting "github.com/juju/juju/testing"
	wireformat "github.com/juju/romulus/wireformat/metrics"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
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

	s.setUnitNumberAndCheckSentMetrics(c, uuid, 0)

	s.setUnitNumberAndCheckSentMetrics(c, uuid, 99)

	s.setUnitNumberAndCheckSentMetrics(c, uuid, 42)
}

func (s *usageSenderSuite) setUnitNumberAndCheckSentMetrics(c *gc.C, modelUUID string, unitCount int) {
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
}

type usagePost struct {
	httprequest.Route `httprequest:"POST /metrics"`
	Body              []wireformat.MetricBatch `httprequest:",body"`
}

type testHandler struct {
	receivedMetrics chan string
}

func (c *testHandler) Metrics(arg *usagePost) error {
	if len(arg.Body) == 1 && len(arg.Body[0].Metrics) == 1 {
		c.receivedMetrics <- arg.Body[0].Metrics[0].Value
	} else {
		c.receivedMetrics <- "-1"
	}
	return nil
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
