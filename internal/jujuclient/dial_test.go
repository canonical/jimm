// Copyright 2020 Canonical Ltd.

package jujuclient_test

import (
	"context"

	jujuversion "github.com/juju/juju/version"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/jemtest"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jujuclient"
)

type jujuclientSuite struct {
	jemtest.JujuConnSuite

	Dialer jimm.Dialer
	API    jimm.API
}

func (s *jujuclientSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.Dialer = jujuclient.Dialer{}
	var err error
	info := s.APIInfo(c)
	ctl := dbmodel.Controller{
		Name:          s.ControllerConfig.ControllerName(),
		CACertificate: info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
		Addresses:     dbmodel.Strings(info.Addrs),
	}
	s.API, err = s.Dialer.Dial(context.Background(), &ctl, names.ModelTag{})
	c.Assert(err, gc.Equals, nil)
}

func (s *jujuclientSuite) TearDownTest(c *gc.C) {
	if s.API != nil {
		err := s.API.Close()
		s.API = nil
		c.Assert(err, gc.Equals, nil)
	}
	s.JujuConnSuite.TearDownTest(c)
}

type dialSuite struct {
	jujuclientSuite
}

var _ = gc.Suite(&dialSuite{})

func (s *dialSuite) TestDial(c *gc.C) {
	info := s.APIInfo(c)
	ctl := dbmodel.Controller{
		Name:          s.ControllerConfig.ControllerName(),
		CACertificate: info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
		PublicAddress: info.Addrs[0],
	}
	api, err := s.Dialer.Dial(context.Background(), &ctl, names.ModelTag{})
	c.Assert(err, gc.Equals, nil)
	defer api.Close()
	c.Check(ctl.UUID, gc.Equals, "deadbeef-1bad-500d-9000-4b1d0d06f00d")
	c.Check(ctl.AgentVersion, gc.Equals, jujuversion.Current.String())
	c.Check(ctl.Addresses, jc.DeepEquals, dbmodel.Strings(info.Addrs))
}
