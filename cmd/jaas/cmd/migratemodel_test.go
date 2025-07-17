// Copyright 2025 Canonical.

// Note that this file is not an integration test
// because of limitations with the JujuConnSuite
// so it is placed under the cmd package.

package cmd

import (
	"os"

	"github.com/juju/cmd/v3/cmdtesting"
	jjclient "github.com/juju/juju/jujuclient"
	gc "gopkg.in/check.v1"
)

// migrateModelSuite is a test suite for the migrate model command.
// It does not perform integration tests like other suites because
// our test suite doesn't support spinning up multiple controllers
// so this behaviour is tested elsewhere instead.
type migrateModelSuite struct{}

var _ = gc.Suite(&migrateModelSuite{})

func (s *migrateModelSuite) TestReadUserMapping(c *gc.C) {
	userMappingFile, err := os.CreateTemp(c.MkDir(), "")
	c.Assert(err, gc.IsNil)

	userMapping := `
# This is a comment
alice: alice@canonical.com
bob: bob@canonical.com
`
	_, err = userMappingFile.WriteString(userMapping)
	c.Assert(err, gc.IsNil)

	migrateCmd := NewMigrateModelCommandForTesting(jjclient.NewMemStore(), nil)
	migrateCmd.userMappingFile = userMappingFile.Name()
	mapping, err := migrateCmd.parseUserMappingFile()
	c.Assert(err, gc.IsNil)
	c.Assert(mapping, gc.DeepEquals, map[string]string{
		"alice": "alice@canonical.com",
		"bob":   "bob@canonical.com",
	})
}

func (s *migrateModelSuite) TestReadUserMappingFailsWithEmptyYaml(c *gc.C) {
	userMappingFile, err := os.CreateTemp(c.MkDir(), "")
	c.Assert(err, gc.IsNil)

	// Invalid YAML content
	_, err = userMappingFile.WriteString("")
	c.Assert(err, gc.IsNil)

	migrateCmd := NewMigrateModelCommandForTesting(jjclient.NewMemStore(), nil)
	migrateCmd.userMappingFile = userMappingFile.Name()
	_, err = migrateCmd.parseUserMappingFile()
	c.Assert(err, gc.ErrorMatches, "user mapping file is empty or not properly formatted")
}

func (s *migrateModelSuite) TestCommandsFailsWithMissingArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, NewMigrateModelCommandForTesting(jjclient.NewMemStore(), nil), "myController")
	c.Assert(err, gc.ErrorMatches, "Missing controller name and model target arguments")
}
