// Copyright 2021 Canonical Ltd.

package jujuclient_test

import (
	"context"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"
)

type pingSuite struct {
	jujuclientSuite
}

var _ = gc.Suite(&pingSuite{})

func (s *pingSuite) TestPing(c *gc.C) {
	ctx := context.Background()

	info := s.APIInfo(c)
	ctl := dbmodel.Controller{
		Name:          s.ControllerConfig.ControllerName(),
		CACertificate: info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
		PublicAddress: info.Addrs[0],
	}
	api, err := s.Dialer.Dial(ctx, &ctl, names.ModelTag{})
	c.Assert(err, gc.Equals, nil)
	defer api.Close()

	err = api.Ping(ctx)
	c.Assert(err, gc.Equals, nil)
}
