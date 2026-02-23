// Copyright 2026 Canonical.

package testing

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller/migrationtarget"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func TestImportLogs(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupWebsocketEnv(c)

	conn := s.Open(c, &api.Info{}, s.AdminUser.Name, nil)
	defer conn.Close()
	client := migrationtarget.NewClient(conn)
	_, err := client.OpenLogTransferStream(s.Model.UUID.String)
	c.Assert(err, qt.IsNil)
}

// TestImportLogsError tests that an error is returned when
// a user is not a JIMM admin.
func TestImportLogsError(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupWebsocketEnv(c)

	conn := s.Open(c, &api.Info{}, s.AdminUser.Name, nil)
	defer conn.Close()
	client := migrationtarget.NewClient(conn)
	_, err := client.OpenLogTransferStream(s.Model.UUID.String)
	c.Assert(err, qt.IsNil)
}
