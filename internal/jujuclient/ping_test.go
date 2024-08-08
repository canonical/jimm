// Copyright 2024 Canonical.

package jujuclient_test

import (
	"context"

	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

type pingSuite struct {
	jujuclientSuite
}

var _ = gc.Suite(&pingSuite{})

func (s *pingSuite) TestPing(c *gc.C) {
	ctx := context.Background()

	info := s.APIInfo(c)
	ctl := dbmodel.Controller{
		UUID:              s.ControllerConfig.ControllerUUID(),
		Name:              s.ControllerConfig.ControllerName(),
		CACertificate:     info.CACert,
		AdminIdentityName: info.Tag.Id(),
		AdminPassword:     info.Password,
		PublicAddress:     info.Addrs[0],
	}
	api, err := s.Dialer.Dial(ctx, &ctl, names.ModelTag{}, nil)
	c.Assert(err, gc.Equals, nil)
	defer api.Close()

	err = api.Ping(ctx)
	c.Assert(err, gc.Equals, nil)
}
