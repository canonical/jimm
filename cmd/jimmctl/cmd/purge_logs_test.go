// Copyright 2024 Canonical.
package cmd_test

import (
	"bytes"
	"context"
	"time"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/testutils/cmdtest"
)

type purgeLogsSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&purgeLogsSuite{})

func (s *purgeLogsSuite) TestPurgeLogsSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")
	datastring := "2021-01-01T00:00:00Z"
	cmdCtx, err := cmdtesting.RunCommand(c, cmd.NewPurgeLogsCommandForTesting(s.ClientStore(), bClient), datastring)
	c.Assert(err, gc.IsNil)
	expected := "deleted-count: 0\n"
	actual := cmdCtx.Stdout.(*bytes.Buffer).String()
	c.Assert(actual, gc.Equals, expected)
}

func (s *purgeLogsSuite) TestInvalidISO8601Date(c *gc.C) {
	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")
	datastring := "13/01/2021"
	_, err := cmdtesting.RunCommand(c, cmd.NewPurgeLogsCommandForTesting(s.ClientStore(), bClient), datastring)
	c.Assert(err, gc.ErrorMatches, `invalid date. Expected ISO8601 date`)

}

func (s *purgeLogsSuite) TestPurgeLogs(c *gc.C) {
	// bob is not superuser
	bClient := s.SetupCLIAccess(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewPurgeLogsCommandForTesting(s.ClientStore(), bClient), "2021-01-01T00:00:00Z")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *purgeLogsSuite) TestPurgeLogsFromDb(c *gc.C) {
	// create logs
	layouts := []string{
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04Z",
		"2006-01-02",
	}
	for _, layout := range layouts {

		ctx := context.Background()
		relativeNow := time.Now().AddDate(-1, 0, 0)
		ale := dbmodel.AuditLogEntry{
			Time:        relativeNow.UTC().Round(time.Millisecond),
			IdentityTag: names.NewUserTag("alice@canonical.com").String(),
		}
		ale_past := dbmodel.AuditLogEntry{
			Time:        relativeNow.AddDate(0, 0, -1).UTC().Round(time.Millisecond),
			IdentityTag: names.NewUserTag("alice@canonical.com").String(),
		}
		ale_future := dbmodel.AuditLogEntry{
			Time:        relativeNow.AddDate(0, 0, 5).UTC().Round(time.Millisecond),
			IdentityTag: names.NewUserTag("alice@canonical.com").String(),
		}

		err := s.JIMM.Database.Migrate(context.Background(), false)
		c.Assert(err, gc.IsNil)
		err = s.JIMM.Database.AddAuditLogEntry(ctx, &ale)
		c.Assert(err, gc.IsNil)
		err = s.JIMM.Database.AddAuditLogEntry(ctx, &ale_past)
		c.Assert(err, gc.IsNil)
		err = s.JIMM.Database.AddAuditLogEntry(ctx, &ale_future)
		c.Assert(err, gc.IsNil)

		tomorrow := relativeNow.AddDate(0, 0, 1).Format(layout)
		// alice is superuser
		bClient := s.SetupCLIAccess(c, "alice")
		cmdCtx, err := cmdtesting.RunCommand(c, cmd.NewPurgeLogsCommandForTesting(s.ClientStore(), bClient), tomorrow)
		c.Assert(err, gc.IsNil)
		// check that logs have been deleted
		expectedOutput := "deleted-count: 2\n"
		actual := cmdCtx.Stdout.(*bytes.Buffer).String()
		c.Assert(actual, gc.Equals, expectedOutput)
	}
}
