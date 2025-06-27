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

// // TestMigrateModelCommandSuperuser tests that the CLI command parses the user mapping and
// // a request is made to the source controller.
// // Because our test suite only spins up 1 controller we cannot fully test the migration.
// func (s *moveModelSuite) TestMigrateModelCommandSuperuser(c *gc.C) {
// 	s.AddController(c, "controller-attached-to-jimm", s.APIInfo(c))
// 	// For the sake of this test, we will simulate some aspects of the migration.
// 	// Besides the JIMM controller, we will create a fake "source-controller" that
// 	// will act the controller where a model currently exists. This controller is not reachable.
// 	originalClientStore := s.ClientStore
// 	fakeClientStore := func() *jjclient.MemStore {
// 		store := originalClientStore()
// 		sourceController := store.Controllers["JIMM"]
// 		sourceController.APIEndpoints = []string{"invalid:1234"}
// 		sourceController.ControllerUUID = "bbf7f816-5164-429c-9d61-d203078491f9"
// 		accountDetails, err := store.AccountDetails("JIMM")
// 		c.Check(err, gc.IsNil)
// 		store.Accounts["source-controller"] = *accountDetails
// 		err = store.AddController("source-controller", sourceController)
// 		c.Check(err, gc.IsNil)
// 		err = store.SetCurrentController("source-controller")
// 		c.Check(err, gc.IsNil)
// 		return store
// 	}
// 	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@canonical.com/cred")
// 	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
// 	mt := s.AddModel(c, names.NewUserTag("charlie@canonical.com"), "model-1", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)

// 	userMappingFile, err := os.CreateTemp(c.MkDir(), "")
// 	c.Assert(err, gc.IsNil)
// 	userMapping := `
// 	# This is a comment
// 	alice:alice@canonical.com`
// 	_, err = userMappingFile.WriteString(userMapping)
// 	c.Assert(err, gc.IsNil)

// 	bClient := s.SetupCLIAccess(c, "alice")
// 	// In this command we are migrating the model "model-1" from the source controller
// 	// (the user's current controller) to the JIMM controller, specifying the
// 	// "controller-attached-to-jimm" as the backing controller.
// 	_, err = cmdtesting.RunCommand(
// 		c, cmd.NewMigrateModelCommandForTesting(fakeClientStore(), bClient),
// 		mt.Id(),
// 		"JIMM",
// 		"--backing-controller=controller-attached-to-jimm",
// 		"--user-mapping",
// 		userMappingFile.Name(),
// 	)
// 	c.Assert(err, gc.IsNil)
// }

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
	mapping, err := migrateCmd.readUserMappingFile()
	c.Assert(err, gc.IsNil)
	c.Assert(mapping, gc.DeepEquals, map[string]string{
		"alice": "alice@canonical.com",
		"bob":   "bob@canonical.com",
	})
}

func (s *migrateModelSuite) TestCommandsFailsWithMissingArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, NewMigrateModelCommandForTesting(jjclient.NewMemStore(), nil), "myController")
	c.Assert(err, gc.ErrorMatches, "Missing controller name and model target arguments")
}
