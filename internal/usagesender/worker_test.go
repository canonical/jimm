// Copyright 2017 Canonical Ltd.

package usagesender_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	jujujujutesting "github.com/juju/juju/testing"
	wireformat "github.com/juju/romulus/wireformat/metrics"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/apitest"
	"github.com/CanonicalLtd/jem/internal/usagesender"
	"github.com/CanonicalLtd/jem/params"
)

type usageSenderSuite struct {
	apitest.Suite

	client *stubHTTPClient
	// clock holds the mock clock used by the monitor package.
	clock *jujutesting.Clock
}

var _ = gc.Suite(&usageSenderSuite{})

var (
	testContext = context.Background()
	epoch       = parseTime("2016-01-01T12:00:00Z")
)

func (s *usageSenderSuite) SetUpTest(c *gc.C) {
	s.client = &stubHTTPClient{}
	s.PatchValue(usagesender.NewHTTPClient, func() usagesender.HTTPClient {
		return s.client
	})
	// Set up the clock mockery.
	s.clock = jujutesting.NewClock(epoch)
	s.PatchValue(usagesender.SenderClock, s.clock)
	s.Suite.SetUpTest(c)
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

	for a := jujujujutesting.LongAttempt.Start(); a.Next(); {
		if s.client.URL == "https://0.1.2.3/omnibus/v2" &&
			s.client.BodyType == "application/json" &&
			len(s.client.Batches) == 1 &&
			len(s.client.Batches[0].Metrics) == 1 &&
			s.client.Batches[0].Metrics[0].Value == unitCountString {
			break
		}
		c.Logf("http client %#v", s.client)
		if !a.HasNext() {
			c.Fatalf("expected metrics not sent")
		}
	}
}

type stubHTTPClient struct {
	URL      string
	BodyType string
	Batches  []wireformat.MetricBatch
}

func (c *stubHTTPClient) Post(url string, bodyType string, body io.Reader) (*http.Response, error) {
	c.URL, c.BodyType = url, bodyType
	decoder := json.NewDecoder(body)
	err := decoder.Decode(&c.Batches)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return &http.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		Body:       ioutil.NopCloser(bytes.NewBufferString("{}")),
	}, nil
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
