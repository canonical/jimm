// Copyright 2021 Canonical Ltd.

package jujuclient_test

import (
	"context"

	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jujuclient"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
)

type modelWatcherSuite struct {
	jemtest.JujuConnSuite

	Dialer jimm.Dialer
	API    jimm.API
}

func (s *modelWatcherSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.Dialer = jujuclient.Dialer{}
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
		Name:          s.ControllerConfig.ControllerName(),
		CACertificate: info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
		Addresses:     hpss,
	}

	s.API, err = s.Dialer.Dial(context.Background(), &ctl, s.Model.ModelTag())
	c.Assert(err, gc.Equals, nil)
}

func (s *modelWatcherSuite) TearDownTest(c *gc.C) {
	if s.API != nil {
		err := s.API.Close()
		s.API = nil
		c.Assert(err, gc.Equals, nil)
	}
	s.JujuConnSuite.TearDownTest(c)
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
