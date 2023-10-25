package monitor_test

import (
	"context"
	"time"

	jujuwatcher "github.com/juju/juju/state/watcher"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"

	"github.com/canonical/jimm/internal/jem"
	"github.com/canonical/jimm/internal/jemtest"
	"github.com/canonical/jimm/internal/mgosession"
	"github.com/canonical/jimm/internal/mongodoc"
	"github.com/canonical/jimm/internal/monitor"
	"github.com/canonical/jimm/internal/pubsub"
	"github.com/canonical/jimm/params"
)

var testContext = context.Background()

type monitorSuite struct {
	jemtest.BootstrapSuite
}

var _ = gc.Suite(&monitorSuite{})

func (s *monitorSuite) SetUpTest(c *gc.C) {
	s.Params.Pubsub = new(pubsub.Hub)
	s.BootstrapSuite.SetUpTest(c)
}

func (s *monitorSuite) TestMonitor(c *gc.C) {
	// Start a monitor.
	m := monitor.New(testContext, s.Pool, "jem1")
	defer worker.Stop(m)

	// Wait for the stats to be updated.
	for a := jujutesting.LongAttempt.Start(); a.Next(); {
		err := s.JEM.DB.GetController(testContext, &s.Controller)
		c.Assert(err, gc.Equals, nil)
		if s.Controller.Stats != (mongodoc.ControllerStats{}) {
			break
		}
		if !a.HasNext() {
			c.Fatalf("controller stats never changed")
		}
	}
	c.Assert(s.Controller.Stats, jc.DeepEquals, mongodoc.ControllerStats{
		ModelCount: 2,
	})
}

// TODO test the monitor with a broken Juju API connection
// when we've fixed the API cache connection logic for
// broken API connections.

func (s *monitorSuite) TestMonitorWithBrokenMongoConnection(c *gc.C) {
	s.PatchValue(monitor.APIConnectRetryDuration, 10*time.Millisecond)
	session := testing.NewProxiedSession(c)
	defer session.Close()

	sessionPool := mgosession.NewPool(context.TODO(), session.Session, 2)
	defer sessionPool.Close()
	jemparams := s.Params
	jemparams.SessionPool = sessionPool

	pool, err := jem.NewPool(testContext, jemparams)
	c.Assert(err, gc.Equals, nil)
	defer pool.Close()

	jem := pool.JEM(context.TODO())
	defer jem.Close()

	// Create a controller.
	apiInfo := s.APIInfo(c)
	ctlPath := params.EntityPath{"bob", "foo"}

	hps, err := mongodoc.ParseAddresses(apiInfo.Addrs)
	c.Assert(err, gc.Equals, nil)

	err = jem.DB.InsertController(testContext, &mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     [][]mongodoc.HostPort{hps},
		CACert:        apiInfo.CACert,
		AdminUser:     apiInfo.Tag.Id(),
		AdminPassword: apiInfo.Password,
	})

	c.Assert(err, gc.Equals, nil)

	// Start a monitor.
	m := monitor.New(context.TODO(), pool, "jem1")
	defer worker.Stop(m)

	// Wait for the stats to be updated.
	stats := s.waitControllerStats(c, ctlPath, mongodoc.ControllerStats{})
	c.Assert(stats, jc.DeepEquals, mongodoc.ControllerStats{
		ModelCount: 2,
	})
	c.Logf("watcher period: %v", jujuwatcher.Period)

	// Tear down the mongo connection and check that
	// the monitoring continues.
	session.CloseConns()

	f := factory.NewFactory(s.State, s.StatePool)
	f.MakeApplication(c, &factory.ApplicationParams{
		Name: "wordpress",
	})

	stats = s.waitControllerStats(c, ctlPath, stats)
	c.Assert(stats, jc.DeepEquals, mongodoc.ControllerStats{
		ModelCount:   2,
		ServiceCount: 1,
	})

	// Check that it shuts down cleanly.
	err = worker.Stop(m)
	c.Assert(err, gc.Equals, nil)
}

func (s *monitorSuite) TestMonitorWithBrokenJujuAPIConnection(c *gc.C) {
	s.PatchValue(monitor.APIConnectRetryDuration, 10*time.Millisecond)
	// Create a controller with API information that will cause
	// it to connect through our proxy instead of directly.
	apiInfo := s.APIInfo(c)
	proxy := testing.NewTCPProxy(c, apiInfo.Addrs[0])
	ctlPath := params.EntityPath{"bob", "foo"}

	hps, err := mongodoc.ParseAddresses([]string{proxy.Addr()})
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.DB.InsertController(testContext, &mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     [][]mongodoc.HostPort{hps},
		CACert:        apiInfo.CACert,
		AdminUser:     apiInfo.Tag.Id(),
		AdminPassword: apiInfo.Password,
	})

	c.Assert(err, gc.Equals, nil)

	// Start a monitor.
	m := monitor.New(context.TODO(), s.Pool, "jem1")
	defer worker.Stop(m)

	// Wait for the stats to be updated.
	stats := s.waitControllerStats(c, ctlPath, mongodoc.ControllerStats{})
	c.Assert(stats, jc.DeepEquals, mongodoc.ControllerStats{
		ModelCount: 2,
	})

	// Tear down the mongo connection, make a new service and
	// check the monitoring continues OK.
	proxy.CloseConns()

	f := factory.NewFactory(s.State, s.StatePool)
	f.MakeApplication(c, &factory.ApplicationParams{
		Name: "wordpress",
	})

	stats = s.waitControllerStats(c, ctlPath, stats)
	c.Assert(stats, jc.DeepEquals, mongodoc.ControllerStats{
		ModelCount:   2,
		ServiceCount: 1,
	})
}

func (s *monitorSuite) waitControllerStats(c *gc.C, ctlPath params.EntityPath, oldStats mongodoc.ControllerStats) mongodoc.ControllerStats {
	for a := jujutesting.LongAttempt.Start(); a.Next(); {
		ctl := &mongodoc.Controller{Path: ctlPath}
		err := s.JEM.DB.GetController(testContext, ctl)
		c.Assert(err, gc.Equals, nil)
		if ctl.Stats != oldStats {
			return ctl.Stats
		}
	}
	c.Fatalf("controller stats never changed")
	panic("unreachable")
}
