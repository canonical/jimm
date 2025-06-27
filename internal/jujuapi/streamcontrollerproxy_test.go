// Copyright 2025 Canonical.

package jujuapi_test

import (
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller/migrationtarget"
	gc "gopkg.in/check.v1"
)

type logTransferProxySuite struct {
	websocketSuite
}

var _ = gc.Suite(&logTransferProxySuite{})

func (s *logTransferProxySuite) TestImportLogs(c *gc.C) {
	conn := s.open(c, &api.Info{}, s.AdminUser.Name)
	defer conn.Close()
	client := migrationtarget.NewClient(conn)
	_, err := client.OpenLogTransferStream(s.Model.UUID.String)
	c.Assert(err, gc.IsNil)
}

// TestImportLogsError tests that an error is returned when
// a user is not a JIMM admin.
func (s *logTransferProxySuite) TestImportLogsError(c *gc.C) {
	conn := s.open(c, &api.Info{}, s.AdminUser.Name)
	defer conn.Close()
	client := migrationtarget.NewClient(conn)
	_, err := client.OpenLogTransferStream(s.Model.UUID.String)
	c.Assert(err, gc.IsNil)
}
