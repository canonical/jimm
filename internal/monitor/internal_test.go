// Copyright 2016 Canonical Ltd.

package monitor

import (
	"time"

	"github.com/juju/idmclient"
	corejujutesting "github.com/juju/juju/juju/testing"
	jujuwatcher "github.com/juju/juju/state/watcher"
	jujutesting "github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/tomb.v2"

	"github.com/CanonicalLtd/jem/internal/idmtest"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type internalSuite struct {
	corejujutesting.JujuConnSuite
	idmSrv *idmtest.Server
	pool   *jem.Pool
	jem    *jem.JEM

	// startTime holds the time that the testing clock is initially
	// set to.
	startTime time.Time

	// clock holds the mock clock used by the monitor package.
	clock *jujutesting.Clock
}

// We don't want to wait for the usual 5s poll interval.
func init() {
	jujuwatcher.Period = 50 * time.Millisecond
}

var _ = gc.Suite(&internalSuite{})

func (s *internalSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.idmSrv = idmtest.NewServer()
	pool, err := jem.NewPool(
		jem.ServerParams{
			DB: s.Session.DB("jem"),
		},
		bakery.NewServiceParams{
			Location: "here",
		},
		idmclient.New(idmclient.NewParams{
			BaseURL: s.idmSrv.URL.String(),
			Client:  s.idmSrv.Client("agent"),
		}),
	)
	c.Assert(err, gc.IsNil)
	s.pool = pool
	s.jem = pool.JEM()

	// Set up the clock mockery.
	s.clock = jujutesting.NewClock(epoch)
	s.PatchValue(&Clock, s.clock)
}

func (s *internalSuite) TearDownTest(c *gc.C) {
	s.jem.Close()
	s.pool.Close()
	s.JujuConnSuite.TearDownTest(c)
}

var epoch = parseTime("2016-01-01T12:00:00Z")

func (s *internalSuite) TestLeaseUpdater(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.jem.AddController(&mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	}, &mongodoc.Model{})

	// The controller monitor assumes that it already has the
	// lease when started, so acquire the lease.
	expiry, err := acquireLease(s.jem, ctlPath, time.Time{}, "", "jem1")
	c.Assert(err, gc.IsNil)

	m := &controllerMonitor{
		ctlPath:     ctlPath,
		leaseExpiry: expiry,
		jem:         s.jem,
		ownerId:     "jem1",
	}
	done := make(chan error)
	go func() {
		done <- m.leaseUpdater()
	}()

	// Advance the clock until the lease updater will need
	// to renew the lease.
	s.clock.Advance(leaseExpiryDuration * 5 / 6)

	// Wait for the lease to actually be renewed.
	var ctl *mongodoc.Controller
	for a := jujutesting.LongAttempt.Start(); a.Next(); {
		ctl, err = s.jem.Controller(ctlPath)
		c.Assert(err, gc.IsNil)
		if !ctl.MonitorLeaseExpiry.Equal(expiry) {
			break
		}
		if !a.HasNext() {
			c.Fatalf("lease never acquired")
		}
	}
	c.Assert(ctl.MonitorLeaseExpiry.UTC(), gc.DeepEquals, s.clock.Now().Add(leaseExpiryDuration))
	c.Assert(ctl.MonitorLeaseOwner, gc.Equals, "jem1")

	// Kill the monitor and wait for the updater to exit.
	m.tomb.Kill(nil)

	select {
	case err = <-done:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("lease updater never stopped")
	}
	c.Assert(err, gc.Equals, tomb.ErrDying)

	// Check that the lease has been dropped.
	s.assertLease(c, ctlPath, time.Time{}, "")
}

func (s *internalSuite) TestLeaseUpdaterWhenControllerRemoved(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}

	// Start the lease updater with no controller existing.
	m := &controllerMonitor{
		ctlPath:     ctlPath,
		leaseExpiry: epoch.Add(leaseExpiryDuration),
		jem:         s.jem,
		ownerId:     "jem1",
	}
	defer m.tomb.Kill(nil)
	done := make(chan error)
	go func() {
		done <- m.leaseUpdater()
	}()

	// Advance the clock until the lease updater will need
	// to renew the lease.
	s.clock.Advance(leaseExpiryDuration * 5 / 6)

	var err error
	select {
	case err = <-done:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("lease updater never stopped")
	}
	c.Assert(err, gc.ErrorMatches, `cannot renew lease on bob/foo: controller removed`)
	c.Assert(errgo.Cause(err), gc.Equals, errMonitoringStopped)
}

func (s *internalSuite) assertLease(c *gc.C, ctlPath params.EntityPath, t time.Time, owner string) {
	ctl, err := s.jem.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.MonitorLeaseExpiry.UTC(), jc.DeepEquals, mongodoc.Time(t).UTC())
	c.Assert(ctl.MonitorLeaseOwner, gc.Equals, owner)
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
