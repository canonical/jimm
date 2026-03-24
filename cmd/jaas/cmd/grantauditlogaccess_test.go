// Copyright 2025 Canonical.

package cmd

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/errors"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func TestGrantAuditLogAccessRun_Success(t *testing.T) {
	c := qt.New(t)

	cmdMocks := setupCmdMocks(c)

	cmdMocks.client.EXPECT().
		GrantAuditLogAccess(&apiparams.AuditLogAccessRequest{UserTag: "user-bob@canonical.com"}).
		Return(nil).
		Times(1)

	cmdMocks.client.EXPECT().Close()

	command := &grantAuditLogAccessCommand{
		username: "bob@canonical.com",
	}
	command.setJIMMAPI(cmdMocks.client)

	err := command.Run(newTestContext(c))
	c.Assert(err, qt.IsNil)
}

func TestGrantAuditLogAccessRun_APIError(t *testing.T) {
	c := qt.New(t)

	cmdMocks := setupCmdMocks(c)

	cmdMocks.client.EXPECT().
		GrantAuditLogAccess(gomock.Any()).
		Return(errors.New("unauthorised access")).
		Times(1)

	cmdMocks.client.EXPECT().Close()

	command := &grantAuditLogAccessCommand{
		username: "bob@canonical.com",
	}
	command.setJIMMAPI(cmdMocks.client)

	err := command.Run(newTestContext(c))
	c.Assert(err, qt.Not(qt.IsNil))
}

func TestGrantAuditLogAccessInit(t *testing.T) {
	c := qt.New(t)

	var command grantAuditLogAccessCommand

	c.Assert(command.Init(nil), qt.ErrorMatches, "missing username")
	c.Assert(command.Init([]string{"@@"}), qt.ErrorMatches, `invalid username "@@"`)
	c.Assert(command.Init([]string{"bob@canonical.com", "extra"}), qt.ErrorMatches, "unknown arguments")

	c.Assert(command.Init([]string{"bob@canonical.com"}), qt.IsNil)
	c.Assert(command.username, qt.Equals, "bob@canonical.com")
}
