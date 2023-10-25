// Copyright 2020 Canonical Ltd.

package apiconn_test

import (
	"context"

	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/apiserver/params"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/internal/apiconn"
	"github.com/canonical/jimm/internal/jemtest"
)

type modelSummaryWatcherSuite struct {
	jemtest.JujuConnSuite

	cache *apiconn.Cache
	conn  *apiconn.Conn
}

var _ = gc.Suite(&modelSummaryWatcherSuite{})

func (s *modelSummaryWatcherSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.cache = apiconn.NewCache(apiconn.CacheParams{})

	var err error
	s.conn, err = s.cache.OpenAPI(context.Background(), s.ControllerConfig.ControllerUUID(), func() (api.Connection, *api.Info, error) {
		apiInfo := s.APIInfo(c)
		return apiOpen(
			&api.Info{
				Addrs:    apiInfo.Addrs,
				CACert:   apiInfo.CACert,
				Tag:      apiInfo.Tag,
				Password: apiInfo.Password,
			},
			api.DialOpts{},
		)
	})
	c.Assert(err, gc.Equals, nil)
}

func (s *modelSummaryWatcherSuite) TearDownTest(c *gc.C) {
	if s.conn != nil {
		s.conn.Close()
	}
	if s.cache != nil {
		s.cache.Close()
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *modelSummaryWatcherSuite) TestSupportsWatchAllModelSummaries(c *gc.C) {
	c.Assert(s.conn.SupportsModelSummaryWatcher(), gc.Equals, true)
}

func (s *modelSummaryWatcherSuite) TestWatchAllModelSummaries(c *gc.C) {
	ctx := context.Background()

	id, err := s.conn.WatchAllModelSummaries(ctx)
	c.Assert(err, gc.Equals, nil)
	c.Assert(id, gc.Not(gc.Equals), "")

	err = s.conn.ModelSummaryWatcherStop(ctx, id)
	c.Assert(err, gc.Equals, nil)
}

func (s *modelSummaryWatcherSuite) TestModelSummaryWatcherNext(c *gc.C) {
	ctx := context.Background()

	id, err := s.conn.WatchAllModelSummaries(ctx)
	c.Assert(err, gc.Equals, nil)

	_, err = s.conn.ModelSummaryWatcherNext(ctx, id)
	c.Assert(err, gc.Equals, nil)

	err = s.conn.ModelSummaryWatcherStop(ctx, id)
	c.Assert(err, gc.Equals, nil)
}

func (s *modelSummaryWatcherSuite) TestModelSummaryWatcherNextError(c *gc.C) {
	_, err := s.conn.ModelSummaryWatcherNext(context.Background(), "invalid-watcher")
	c.Assert(apiconn.IsAPIError(err), gc.Equals, true)
	c.Assert(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `api error: watcher id: invalid-watcher: unknown watcher id \(not found\)`)
}

func (s *modelSummaryWatcherSuite) TestModelSummaryWatcherStopError(c *gc.C) {
	err := s.conn.ModelSummaryWatcherStop(context.Background(), "invalid-watcher")
	c.Assert(apiconn.IsAPIError(err), gc.Equals, true)
	c.Assert(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `api error: watcher id: invalid-watcher: unknown watcher id \(not found\)`)
}
