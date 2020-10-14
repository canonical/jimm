// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"context"
	"sync"
	"time"

	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/jujuapi"
	"github.com/CanonicalLtd/jimm/params"
)

type controllerrootSuite struct {
	websocketSuite
}

var _ = gc.Suite(&controllerrootSuite{})

func (s *controllerrootSuite) SetUpTest(c *gc.C) {
	s.ServerParams.CharmstoreLocation = "https://api.jujucharms.com/charmstore"
	s.ServerParams.MeteringLocation = "https://api.jujucharms.com/omnibus"
	s.websocketSuite.SetUpTest(c)
	s.PatchValue(&utils.OutgoingAccessAllowed, true)
}

func (s *controllerrootSuite) TestServerVersion(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{"test", "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	testVersion := version.MustParse("5.4.3")
	err := s.JEM.SetControllerVersion(ctx, ctlPath, testVersion)
	c.Assert(err, gc.Equals, nil)

	conn := s.open(c, nil, "test")
	defer conn.Close()

	v, ok := conn.ServerVersion()
	c.Assert(ok, gc.Equals, true)
	c.Assert(v, jc.DeepEquals, testVersion)
}

func (s *controllerrootSuite) TestUnimplementedMethodFails(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	modelUUID := mi.UUID
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 3, "", "Logout", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `no such request - method Admin\(3\).Logout is not implemented \(not implemented\)`)
}

func (s *controllerrootSuite) TestUnimplementedRootFails(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("NoSuch", 1, "", "Method", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `no such request - method NoSuch\(1\).Method is not implemented \(not implemented\)`)
}

type testHeartMonitor struct {
	c         chan time.Time
	firstBeat chan struct{}

	// mu protects the fields below.
	mu              sync.Mutex
	_beats          int
	dead            bool
	firstBeatClosed bool
}

func newTestHeartMonitor() *testHeartMonitor {
	return &testHeartMonitor{
		c:         make(chan time.Time),
		firstBeat: make(chan struct{}),
	}
}

func (m *testHeartMonitor) Heartbeat() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m._beats++
	if !m.firstBeatClosed {
		close(m.firstBeat)
		m.firstBeatClosed = true
	}

}

func (m *testHeartMonitor) Dead() <-chan time.Time {
	return m.c
}

func (m *testHeartMonitor) Stop() bool {
	return m.dead
}

func (m *testHeartMonitor) kill(t time.Time) {
	m.mu.Lock()
	m.dead = true
	m.mu.Unlock()
	m.c <- t
}

func (m *testHeartMonitor) beats() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m._beats
}

func (m *testHeartMonitor) waitForFirstPing(c *gc.C, d time.Duration) {
	select {
	case <-m.firstBeat:
	case <-time.After(d):
		c.Fatalf("timeout waiting for first ping")
	}
}

func (s *controllerrootSuite) TestConnectionClosesWhenHeartMonitorDies(c *gc.C) {
	hm := newTestHeartMonitor()
	s.PatchValue(jujuapi.NewHeartMonitor, jujuapi.InternalHeartMonitor(func(time.Duration) jujuapi.HeartMonitor {
		return hm
	}))
	conn := s.open(c, nil, "test")
	defer conn.Close()
	hm.kill(time.Now())
	beats := hm.beats()
	var err error
	for beats < 10 {
		time.Sleep(10 * time.Millisecond)
		err = conn.APICall("Pinger", 1, "", "Ping", nil, nil)
		if err != nil {
			break
		}
		beats++
	}
	c.Assert(err, gc.ErrorMatches, `connection is shut down`)
	c.Assert(hm.beats(), gc.Equals, beats)
}

func (s *controllerrootSuite) TestPingerUpdatesHeartMonitor(c *gc.C) {
	hm := newTestHeartMonitor()
	s.PatchValue(jujuapi.NewHeartMonitor, jujuapi.InternalHeartMonitor(func(time.Duration) jujuapi.HeartMonitor {
		return hm
	}))
	conn := s.open(c, nil, "test")
	defer conn.Close()
	beats := hm.beats()
	err := conn.APICall("Pinger", 1, "", "Ping", nil, nil)
	c.Assert(err, gc.Equals, nil)
	c.Assert(hm.beats(), gc.Equals, beats+1)
}
