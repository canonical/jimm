// Copyright 2020 Canonical Ltd.

package jujuclient_test

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"
	gc "gopkg.in/check.v1"
)

type modelSummaryWatcherSuite struct {
	jujuclientSuite
}

var _ = gc.Suite(&modelSummaryWatcherSuite{})

func (s *modelSummaryWatcherSuite) TestSupportsWatchAllModelSummaries(c *gc.C) {
	c.Assert(s.API.SupportsModelSummaryWatcher(), gc.Equals, true)
}

func (s *modelSummaryWatcherSuite) TestWatchAllModelSummaries(c *gc.C) {
	ctx := context.Background()

	id, err := s.API.WatchAllModelSummaries(ctx)
	c.Assert(err, gc.Equals, nil)
	c.Assert(id, gc.Not(gc.Equals), "")

	err = s.API.ModelSummaryWatcherStop(ctx, id)
	c.Assert(err, gc.Equals, nil)
}

func (s *modelSummaryWatcherSuite) TestModelSummaryWatcherNext(c *gc.C) {
	ctx := context.Background()

	id, err := s.API.WatchAllModelSummaries(ctx)
	c.Assert(err, gc.Equals, nil)

	_, err = s.API.ModelSummaryWatcherNext(ctx, id)
	c.Assert(err, gc.Equals, nil)

	err = s.API.ModelSummaryWatcherStop(ctx, id)
	c.Assert(err, gc.Equals, nil)
}

func (s *modelSummaryWatcherSuite) TestModelSummaryWatcherNextError(c *gc.C) {
	_, err := s.API.ModelSummaryWatcherNext(context.Background(), "invalid-watcher")
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `watcher id: invalid-watcher: unknown watcher id \(not found\)`)
}

func (s *modelSummaryWatcherSuite) TestModelSummaryWatcherStopError(c *gc.C) {
	err := s.API.ModelSummaryWatcherStop(context.Background(), "invalid-watcher")
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `watcher id: invalid-watcher: unknown watcher id \(not found\)`)
}
