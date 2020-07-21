// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/jujuapi"
)

type pingerSuite struct {
	websocketSuite
}

var _ = gc.Suite(&pingerSuite{})

func (s *pingerSuite) SetUpTest(c *gc.C) {
	s.ServerParams.CharmstoreLocation = "https://api.jujucharms.com/charmstore"
	s.ServerParams.MeteringLocation = "https://api.jujucharms.com/omnibus"
	s.websocketSuite.SetUpTest(c)
	s.PatchValue(&utils.OutgoingAccessAllowed, true)
}

func (s *pingerSuite) TestUnauthenticatedPinger(c *gc.C) {
	hm := newTestHeartMonitor()
	s.PatchValue(jujuapi.NewHeartMonitor, jujuapi.InternalHeartMonitor(func(time.Duration) jujuapi.HeartMonitor {
		return hm
	}))
	conn := s.open(c, &api.Info{SkipLogin: true}, "test")
	defer conn.Close()
	err := conn.APICall("Pinger", 1, "", "Ping", nil, nil)
	c.Assert(err, gc.Equals, nil)
	c.Assert(hm.beats(), gc.Equals, 1)
}

func newBool(b bool) *bool {
	return &b
}
