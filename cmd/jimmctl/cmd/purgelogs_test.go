package cmd_test

import (
	"bytes"
	"context"
	"time"

	"github.com/canonical/jimm/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"
)

type purgeLogsSuite struct {
	jimmSuite
}

var _ = gc.Suite(&purgeLogsSuite{})

func (s *purgeLogsSuite) TestPurgeLogsSuperuser(c *gc.C) {
	// alice is superuser
	bClient := s.userBakeryClient("alice")
	datastring := "2021-01-01T00:00:00Z"
	cmdCtx, err := cmdtesting.RunCommand(c, cmd.NewPurgeLogsCommandForTesting(s.ClientStore(), bClient), datastring)
	c.Assert(err, gc.IsNil)
	expected := "deleted-count: 0\n"
	actual := cmdCtx.Stdout.(*bytes.Buffer).String()
	c.Assert(actual, gc.Equals, expected)
}

func (s *purgeLogsSuite) TestInvalidISO8601Date(c *gc.C) {
	// alice is superuser
	bClient := s.userBakeryClient("alice")
	datastring := "13/01/2021"
	_, err := cmdtesting.RunCommand(c, cmd.NewPurgeLogsCommandForTesting(s.ClientStore(), bClient), datastring)
	c.Assert(err, gc.ErrorMatches, `invalid date. Expected ISO8601 date`)

}

func (s *purgeLogsSuite) TestPurgeLogs(c *gc.C) {
	// bob is not superuser
	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewPurgeLogsCommandForTesting(s.ClientStore(), bClient), "2021-01-01T00:00:00Z")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *purgeLogsSuite) TestPurgeLogsFromDb(c *gc.C) {
	// create logs
	ctx := context.Background()
	relativeNow := time.Now().AddDate(-1, 0, 0)
	ale := dbmodel.AuditLogEntry{
		Time:    relativeNow.UTC().Round(time.Millisecond),
		UserTag: names.NewUserTag("alice@external").String(),
	}
	ale_past := dbmodel.AuditLogEntry{
		Time:    relativeNow.AddDate(0, 0, -1).UTC().Round(time.Millisecond),
		UserTag: names.NewUserTag("alice@external").String(),
	}
	ale_future := dbmodel.AuditLogEntry{
		Time:    relativeNow.AddDate(0, 0, 5).UTC().Round(time.Millisecond),
		UserTag: names.NewUserTag("alice@external").String(),
	}

	err := s.JIMM.Database.Migrate(context.Background(), false)
	c.Assert(err, gc.IsNil)
	err = s.JIMM.Database.AddAuditLogEntry(ctx, &ale)
	c.Assert(err, gc.IsNil)
	err = s.JIMM.Database.AddAuditLogEntry(ctx, &ale_past)
	c.Assert(err, gc.IsNil)
	err = s.JIMM.Database.AddAuditLogEntry(ctx, &ale_future)
	c.Assert(err, gc.IsNil)

	tomorrow := relativeNow.AddDate(0, 0, 1)
	//alice is superuser
	bClient := s.userBakeryClient("alice")
	cmdCtx, err := cmdtesting.RunCommand(c, cmd.NewPurgeLogsCommandForTesting(s.ClientStore(), bClient), tomorrow.Format(time.RFC3339))
	c.Assert(err, gc.IsNil)
	// check that logs have been deleted
	expectedOutput := "deleted-count: 2\n"
	actual := cmdCtx.Stdout.(*bytes.Buffer).String()
	c.Assert(actual, gc.Equals, expectedOutput)

}
