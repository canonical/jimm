// Copyright 2026 Canonical.

package testing

import (
	"context"
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/common"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type streamProxySuite struct {
	jimmtest.WebsocketE2ESuite
}

var _ = gc.Suite(&streamProxySuite{})

func (s *streamProxySuite) TestDebugLogs(c *gc.C) {
	conn := s.Open(c, &api.Info{ModelTag: s.Model.ResourceTag()}, "bob", nil)
	defer conn.Close()
	logs, err := common.StreamDebugLog(context.TODO(), conn, common.DebugLogParams{})
	c.Assert(err, gc.IsNil)
	select {
	case _, ok := <-logs:
		c.Assert(ok, gc.Equals, true)
	case <-time.After(5 * time.Second):
		c.Fatal("expected to receive log message, but did not receive any after timeout")
	}
}

func (s *streamProxySuite) TestDebugLogsWithParams(c *gc.C) {
	conn := s.Open(c, &api.Info{ModelTag: s.Model.ResourceTag()}, "bob", nil)
	defer conn.Close()

	logChan, err := common.StreamDebugLog(context.TODO(), conn, common.DebugLogParams{
		NoTail: true,
		Limit:  1,
	})
	c.Assert(err, gc.IsNil)
	messages := 0
	for {
		select {
		case _, ok := <-logChan:
			if !ok {
				c.Assert(messages, gc.Equals, 1)
				return
			}
			messages++
		case <-time.After(5 * time.Second):
			c.Fatal("expected log channel to be closed, but it is still open after timeout")
		}
	}
}

// TestDebugLogsError tests that an error is returned from JIMM
// when a user doesn't have model access but tries to access model logs.
// A user could craft a connection to immediately fetch logs, but using the Go client,
// we must first establish a connection to the Juju API.
// To test this we give the user model access so that the initial connection
// can be established without the Juju controller returning an unauthorized error.
// Then, before we call the log stream, we remove the user's model access.
func (s *streamProxySuite) TestDebugLogsError(c *gc.C) {
	fooUser, err := dbmodel.NewIdentity("foo@canonical.com")
	c.Assert(err, gc.IsNil)
	ctx := context.Background()
	err = s.JIMM.Database.GetIdentity(ctx, fooUser)
	c.Assert(err, gc.IsNil)
	// Give foo access to the model
	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(fooUser.ResourceTag()),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(s.Model.ResourceTag()),
	}
	err = s.JIMM.OpenFGAClient.AddRelation(ctx, tuple)
	c.Assert(err, gc.IsNil)
	conn := s.Open(c, &api.Info{ModelTag: s.Model.ResourceTag()}, "foo", nil)
	defer conn.Close()
	err = s.JIMM.OpenFGAClient.RemoveRelation(ctx, tuple)
	c.Assert(err, gc.IsNil)
	_, err = common.StreamDebugLog(context.TODO(), conn, common.DebugLogParams{})
	c.Assert(err, gc.ErrorMatches, "unauthorized access to endpoint: log")
}
