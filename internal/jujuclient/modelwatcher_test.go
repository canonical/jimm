// Copyright 2024 Canonical.

package jujuclient_test

import (
	"context"

	"github.com/juju/juju/core/network"
	jujuparams "github.com/juju/juju/rpc/params"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jujuclient"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type modelWatcherSuite struct {
	jimmtest.JujuSuite

	Dialer jimm.Dialer
	API    jimm.API
}

func (s *modelWatcherSuite) SetUpTest(c *gc.C) {
	s.JujuSuite.SetUpTest(c)

	s.Dialer = &jujuclient.Dialer{
		JWTService: s.JIMM.JWTService,
	}
	var err error
	info := s.APIInfo(c)
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
	ctl := dbmodel.Controller{
		UUID:              s.ControllerConfig.ControllerUUID(),
		Name:              s.ControllerConfig.ControllerName(),
		CACertificate:     info.CACert,
		AdminIdentityName: info.Tag.Id(),
		AdminPassword:     info.Password,
		Addresses:         hpss,
	}

	s.API, err = s.Dialer.Dial(context.Background(), &ctl, s.Model.ModelTag(), nil)
	c.Assert(err, gc.Equals, nil)
}

func (s *modelWatcherSuite) TearDownTest(c *gc.C) {
	if s.API != nil {
		err := s.API.Close()
		s.API = nil
		c.Assert(err, gc.Equals, nil)
	}
	s.JujuSuite.TearDownTest(c)
}

var _ = gc.Suite(&modelWatcherSuite{})

func (s *modelWatcherSuite) TestWatchAll(c *gc.C) {
	ctx := context.Background()

	id, err := s.API.WatchAll(ctx)
	c.Assert(err, gc.Equals, nil)
	c.Assert(id, gc.Not(gc.Equals), "")

	err = s.API.ModelWatcherStop(ctx, id)
	c.Assert(err, gc.Equals, nil)
}

func (s *modelWatcherSuite) TestModelWatcherNext(c *gc.C) {
	ctx := context.Background()

	id, err := s.API.WatchAll(ctx)
	c.Assert(err, gc.Equals, nil)

	_, err = s.API.ModelWatcherNext(ctx, id)
	c.Assert(err, gc.Equals, nil)

	err = s.API.ModelWatcherStop(ctx, id)
	c.Assert(err, gc.Equals, nil)
}

func (s *modelWatcherSuite) TestModelWatcherNextError(c *gc.C) {
	_, err := s.API.ModelWatcherNext(context.Background(), "invalid-watcher")
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `unknown watcher id \(not found\)`)
}

func (s *modelWatcherSuite) TestModelWatcherStopError(c *gc.C) {
	err := s.API.ModelWatcherStop(context.Background(), "invalid-watcher")
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `unknown watcher id \(not found\)`)
}
