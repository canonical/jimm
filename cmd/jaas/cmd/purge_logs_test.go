// Copyright 2026 Canonical.

package cmd

import (
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/cmd/v3/cmdtesting"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func TestPurgeLogs(t *testing.T) {
	c := qt.New(t)
	cmdMocks := setupCmdMocks(c)

	datastring := "2021-01-01T00:00:00Z"

	cmdMocks.client.EXPECT().PurgeLogs(&apiparams.PurgeLogsRequest{
		Date: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
	}).Return(&apiparams.PurgeLogsResponse{DeletedCount: 2}, nil)
	cmdMocks.client.EXPECT().Close().Return(nil)

	command := &purgeLogsCommand{}
	command.setJIMMAPI(cmdMocks.client)
	command.SetClientStore(cmdMocks.store)

	initCommand(c, command, datastring)

	ctx := newTestContext(c)

	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	expected := "deleted-count: 2\n"
	actual := cmdtesting.Stdout(ctx)
	c.Assert(actual, qt.Equals, expected)
}

func TestInvalidISO8601Date(t *testing.T) {
	c := qt.New(t)
	cmdMocks := setupCmdMocks(c)

	datastring := "13/01/2021"

	command := &purgeLogsCommand{}
	command.setJIMMAPI(cmdMocks.client)
	command.SetClientStore(cmdMocks.store)

	err := initCommandWithError(command, datastring)
	c.Assert(err, qt.ErrorMatches, `invalid date. Expected ISO8601 date`)
}

func TestPurgeLogs_ValidFormats(t *testing.T) {
	c := qt.New(t)
	cmdMocks := setupCmdMocks(c)

	// create logs
	layouts := []string{
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04Z",
		"2006-01-02",
	}
	for _, layout := range layouts {
		command := &purgeLogsCommand{}
		command.setJIMMAPI(cmdMocks.client)
		command.SetClientStore(cmdMocks.store)

		err := initCommandWithError(command, layout)
		c.Assert(err, qt.IsNil)
	}
}
