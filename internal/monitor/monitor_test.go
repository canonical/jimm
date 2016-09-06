package monitor_test

import (
	"time"

	jujuwatcher "github.com/juju/juju/state/watcher"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jem/internal/apitest"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/internal/monitor"
	"github.com/CanonicalLtd/jem/params"
)

type monitorSuite struct {
	apitest.Suite
}

var _ = gc.Suite(&monitorSuite{})

func (s *monitorSuite) TestMonitor(c *gc.C) {
	// Create a controller.
	ctlPath := params.EntityPath{"bob", "foo"}
	info := s.APIInfo(c)

	err := s.JEM.DB.AddController(&mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     info.Addrs,
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	})
	c.Assert(err, gc.IsNil)

	// Start a monitor.
	m := monitor.New(s.Pool, "jem1")
	defer worker.Stop(m)

	// Wait for the stats to be updated.
	var ctl *mongodoc.Controller
	for a := jujutesting.LongAttempt.Start(); a.Next(); {
		ctl, err = s.JEM.DB.Controller(ctlPath)
		c.Assert(err, gc.IsNil)
		if ctl.Stats != (mongodoc.ControllerStats{}) {
			break
		}
		if !a.HasNext() {
			c.Fatalf("controller stats never changed")
		}
	}
	c.Assert(ctl.Stats, jc.DeepEquals, mongodoc.ControllerStats{
		ModelCount: 1,
	})
}

// TODO test the monitor with a broken Juju API connection
// when we've fixed the API cache connection logic for
// broken API connections.

func (s *monitorSuite) TestMonitorWithBrokenMongoConnection(c *gc.C) {
	s.PatchValue(monitor.APIConnectRetryDuration, 10*time.Millisecond)
	pool, proxy := s.ProxiedPool(c)
	defer pool.Close()
	defer proxy.Close()

	// Create a controller.
	apiInfo := s.APIInfo(c)
	ctlPath := params.EntityPath{"bob", "foo"}
	jem := pool.JEM()
	defer jem.Close()
	err := jem.DB.AddController(&mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     apiInfo.Addrs,
		CACert:        apiInfo.CACert,
		AdminUser:     apiInfo.Tag.Id(),
		AdminPassword: apiInfo.Password,
	})
	c.Assert(err, gc.IsNil)

	// Start a monitor.
	m := monitor.New(pool, "jem1")
	defer worker.Stop(m)

	// Wait for the stats to be updated.
	stats := s.waitControllerStats(c, ctlPath, mongodoc.ControllerStats{})
	c.Assert(stats, jc.DeepEquals, mongodoc.ControllerStats{
		ModelCount: 1,
	})
	c.Logf("watcher period: %v", jujuwatcher.Period)

	// Tear down the mongo connection and check that
	// the monitoring continues.
	proxy.CloseConns()

	f := factory.NewFactory(s.State)
	f.MakeApplication(c, &factory.ApplicationParams{
		Name: "wordpress",
	})

	stats = s.waitControllerStats(c, ctlPath, stats)
	c.Assert(stats, jc.DeepEquals, mongodoc.ControllerStats{
		ModelCount:   1,
		ServiceCount: 1,
	})

	// Check that it shuts down cleanly.
	err = worker.Stop(m)
	c.Assert(err, gc.IsNil)
}

func (s *monitorSuite) TestMonitorWithBrokenJujuAPIConnection(c *gc.C) {
	s.PatchValue(monitor.APIConnectRetryDuration, 10*time.Millisecond)
	// Create a controller with API information that will cause
	// it to connect through our proxy instead of directly.
	apiInfo := s.APIInfo(c)
	proxy := testing.NewTCPProxy(c, apiInfo.Addrs[0])
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.JEM.DB.AddController(&mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     []string{proxy.Addr()},
		CACert:        apiInfo.CACert,
		AdminUser:     apiInfo.Tag.Id(),
		AdminPassword: apiInfo.Password,
	})
	c.Assert(err, gc.IsNil)

	// Start a monitor.
	m := monitor.New(s.Pool, "jem1")
	defer worker.Stop(m)

	// Wait for the stats to be updated.
	stats := s.waitControllerStats(c, ctlPath, mongodoc.ControllerStats{})
	c.Assert(stats, jc.DeepEquals, mongodoc.ControllerStats{
		ModelCount: 1,
	})

	// Tear down the mongo connection, make a new service and
	// check the monitoring continues OK.
	proxy.CloseConns()

	f := factory.NewFactory(s.State)
	f.MakeApplication(c, &factory.ApplicationParams{
		Name: "wordpress",
	})

	stats = s.waitControllerStats(c, ctlPath, stats)
	c.Assert(stats, jc.DeepEquals, mongodoc.ControllerStats{
		ModelCount:   1,
		ServiceCount: 1,
	})
}

func (s *monitorSuite) waitControllerStats(c *gc.C, ctlPath params.EntityPath, oldStats mongodoc.ControllerStats) mongodoc.ControllerStats {
	for a := jujutesting.LongAttempt.Start(); a.Next(); {
		ctl, err := s.JEM.DB.Controller(ctlPath)
		c.Assert(err, gc.IsNil)
		if ctl.Stats != oldStats {
			return ctl.Stats
		}
	}
	c.Fatalf("controller stats never changed")
	panic("unreachable")
}
