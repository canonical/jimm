package cmd_test

import (
	"bytes"

	"github.com/juju/cmd/v3/cmdtesting"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/cmd/jimmctl/cmd"
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
	expected := "Deleted 0 logs\n"
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
