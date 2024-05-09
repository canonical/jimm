// Copyright 2020 Canonical Ltd.

package jujuclient_test

import (
	"context"
	"fmt"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/network"
	jujuparams "github.com/juju/juju/rpc/params"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/jujuclient"
)

type jujuclientSuite struct {
	jimmtest.JujuSuite

	Dialer jimm.Dialer
	API    jimm.API
}

func parseInfoAddressToHostPorts(info *api.Info) dbmodel.HostPorts {
	hpss := make(dbmodel.HostPorts, 0, len(info.Addrs))
	for _, addr := range info.Addrs {
		hp, err := network.ParseMachineHostPort(addr)
		if err != nil {
			continue
		}
		hpss = append(hpss, []jujuparams.HostPort{{
			Address: jujuparams.FromMachineAddress(hp.MachineAddress),
			Port:    hp.Port(),
		}})
	}
	return hpss
}

func (s *jujuclientSuite) SetUpTest(c *gc.C) {
	s.JujuSuite.SetUpTest(c)

	s.Dialer = s.JIMM.Dialer
	var err error
	info := s.APIInfo(c)
	hpss := parseInfoAddressToHostPorts(info)

	ctl := dbmodel.Controller{
		UUID:          s.ControllerConfig.ControllerUUID(),
		Name:          s.ControllerConfig.ControllerName(),
		CACertificate: info.CACert,
		Addresses:     hpss,
	}
	s.API, err = s.Dialer.Dial(context.Background(), &ctl, names.ModelTag{}, nil)
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
		UUID:              s.ControllerConfig.ControllerUUID(),
		Name:              s.ControllerConfig.ControllerName(),
		CACertificate:     info.CACert,
		AdminIdentityName: info.Tag.Id(),
		AdminPassword:     info.Password,
		PublicAddress:     info.Addrs[0],
		// TODO(ale8k):
		// In the new simple connector, ADDRESS MUST BE PRESENT
		// but we often dial without addresses present, and only public address
		// figure out what's best.
		Addresses: parseInfoAddressToHostPorts(info),
	}

	api, err := s.Dialer.Dial(context.Background(), &ctl, names.ModelTag{}, nil)
	c.Assert(err, gc.Equals, nil)
	defer api.Close()
	c.Check(ctl.UUID, gc.Equals, "deadbeef-1bad-500d-9000-4b1d0d06f00d")
	c.Check(ctl.AgentVersion, gc.Equals, jujuversion.Current.String())
	addrs := make([]string, len(ctl.Addresses))
	for i, addr := range ctl.Addresses {
		addrs[i] = fmt.Sprintf("%s:%d", addr[0].Value, addr[0].Port)
	}
	c.Check(addrs, jc.DeepEquals, info.Addrs)
}

func (s *dialSuite) TestDialWithJWT(c *gc.C) {
	ctx := context.Background()

	info := s.APIInfo(c)

	ctl := dbmodel.Controller{
		UUID:          info.ControllerUUID,
		Name:          s.ControllerConfig.ControllerName(),
		CACertificate: info.CACert,
		PublicAddress: info.Addrs[0],
		Addresses:     parseInfoAddressToHostPorts(info),
	}

	dialer := &jujuclient.Dialer{
		JWTService: s.JIMM.JWTService,
	}

	// Check dial is OK
	api, err := dialer.Dial(ctx, &ctl, names.ModelTag{}, nil)
	c.Assert(err, gc.Equals, nil)
	defer api.Close()
	// Check UUID matches expected
	c.Check(ctl.UUID, gc.Equals, "deadbeef-1bad-500d-9000-4b1d0d06f00d")
	// Check agent version matches expected
	c.Check(ctl.AgentVersion, gc.Equals, jujuversion.Current.String())
	addrs := make([]string, len(ctl.Addresses))
	for i, addr := range ctl.Addresses {
		addrs[i] = fmt.Sprintf("%s:%d", addr[0].Value, addr[0].Port)
	}
	c.Check(addrs, gc.DeepEquals, info.Addrs)
}
