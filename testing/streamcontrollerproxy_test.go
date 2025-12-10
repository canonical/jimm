// Copyright 2025 Canonical.

package testing

import (
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller/migrationtarget"
	gc "gopkg.in/check.v1"
)

type logTransferProxySuite struct {
	jimmtest.WebsocketE2ESuite
}

var _ = gc.Suite(&logTransferProxySuite{})

func (s *logTransferProxySuite) TestImportLogs(c *gc.C) {
	conn := s.Open(c, &api.Info{}, s.AdminUser.Name, nil)
	defer conn.Close()
	client := migrationtarget.NewClient(conn)
	_, err := client.OpenLogTransferStream(s.Model.UUID.String)
	c.Assert(err, gc.IsNil)
}

// TestImportLogsError tests that an error is returned when
// a user is not a JIMM admin.
func (s *logTransferProxySuite) TestImportLogsError(c *gc.C) {
	conn := s.Open(c, &api.Info{}, s.AdminUser.Name, nil)
	defer conn.Close()
	client := migrationtarget.NewClient(conn)
	_, err := client.OpenLogTransferStream(s.Model.UUID.String)
	c.Assert(err, gc.IsNil)
}
