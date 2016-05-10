package monitor_test

import (
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker"
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

	err := s.JEM.AddController(&mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     info.Addrs,
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	}, &mongodoc.Model{
		UUID: info.ModelTag.Id(),
	})
	c.Assert(err, gc.IsNil)

	// Start a monitor.
	m := monitor.New(s.Pool, "jem1")
	defer worker.Stop(m)

	// Wait for the stats to be updated.
	var ctl *mongodoc.Controller
	for a := jujutesting.LongAttempt.Start(); a.Next(); {
		ctl, err = s.JEM.Controller(ctlPath)
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
	pool, proxy := s.ProxiedPool(c)
	defer pool.Close()
	defer proxy.Close()

	// Create a controller.
	apiInfo := s.APIInfo(c)
	ctlPath := params.EntityPath{"bob", "foo"}
	jem := pool.JEM()
	defer jem.Close()
	err := jem.AddController(&mongodoc.Controller{
		Path:          ctlPath,
		HostPorts:     apiInfo.Addrs,
		CACert:        apiInfo.CACert,
		AdminUser:     apiInfo.Tag.Id(),
		AdminPassword: apiInfo.Password,
	}, &mongodoc.Model{
		UUID: apiInfo.ModelTag.Id(),
	})
	c.Assert(err, gc.IsNil)

	// Start a monitor.
	m := monitor.New(pool, "jem1")
	defer worker.Stop(m)

	// Wait for the stats to be updated.
	var ctl *mongodoc.Controller
	for a := jujutesting.LongAttempt.Start(); a.Next(); {
		ctl, err = jem.Controller(ctlPath)
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

	// Tear down the mongo connection and check that
	// the monitoring continues.
	proxy.CloseConns()

	f := factory.NewFactory(s.State)
	f.MakeService(c, &factory.ServiceParams{
		Name: "wordpress",
	})

	oldStats := ctl.Stats
	for a := jujutesting.LongAttempt.Start(); a.Next(); {
		ctl, err = s.JEM.Controller(ctlPath)
		c.Assert(err, gc.IsNil)
		if ctl.Stats != oldStats {
			break
		}
		if !a.HasNext() {
			c.Fatalf("controller stats never changed")
		}
	}
	c.Assert(ctl.Stats, jc.DeepEquals, mongodoc.ControllerStats{
		ModelCount:   1,
		ServiceCount: 1,
	})

	// Check that it shuts down cleanly.
	err = worker.Stop(m)
	c.Assert(err, gc.IsNil)

}
