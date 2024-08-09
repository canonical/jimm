// Copyright 2024 Canonical.

package jujuclient_test

import (
	"context"

	jujuparams "github.com/juju/juju/rpc/params"
	gc "gopkg.in/check.v1"
)

type allModelWatcherSuite struct {
	jujuclientSuite
}

var _ = gc.Suite(&allModelWatcherSuite{})

func (s *allModelWatcherSuite) TestWatchAllModels(c *gc.C) {
	ctx := context.Background()

	id, err := s.API.WatchAllModels(ctx)
	c.Assert(err, gc.Equals, nil)
	c.Assert(id, gc.Not(gc.Equals), "")

	err = s.API.AllModelWatcherStop(ctx, id)
	c.Assert(err, gc.Equals, nil)
}

func (s *allModelWatcherSuite) TestAllModelWatcherNext(c *gc.C) {
	ctx := context.Background()

	id, err := s.API.WatchAllModels(ctx)
	c.Assert(err, gc.Equals, nil)

	_, err = s.API.AllModelWatcherNext(ctx, id)
	c.Assert(err, gc.Equals, nil)

	err = s.API.AllModelWatcherStop(ctx, id)
	c.Assert(err, gc.Equals, nil)
}

func (s *allModelWatcherSuite) TestAllModelWatcherNextError(c *gc.C) {
	_, err := s.API.AllModelWatcherNext(context.Background(), "invalid-watcher")
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `unknown watcher id \(not found\)`)
}

func (s *allModelWatcherSuite) TestAllModelWatcherStopError(c *gc.C) {
	err := s.API.AllModelWatcherStop(context.Background(), "invalid-watcher")
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `unknown watcher id \(not found\)`)
}
