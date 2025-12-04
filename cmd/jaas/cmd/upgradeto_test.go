// Copyright 2025 Canonical.

// Note that this file is not an integration test
// because of limitations with the JujuConnSuite
// so it is placed under the cmd package.

package cmd

import (
	"context"
	"errors"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/gnuflag"
	jjclient "github.com/juju/juju/jujuclient"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd/mocks"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

// upgradeToSuite is a test suite for the upgrade-to command.
type upgradeToSuite struct {
	jimmClient *mocks.MockJIMMAPI
	writer     *mocks.MockWriter
	store      *mocks.MockClientStore
}

var _ = gc.Suite(&upgradeToSuite{})

func (s *upgradeToSuite) SetupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.jimmClient = mocks.NewMockJIMMAPI(ctrl)
	s.writer = mocks.NewMockWriter(ctrl)
	s.store = mocks.NewMockClientStore(ctrl)

	return ctrl
}

func (s *upgradeToSuite) TestUpgradeTo(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	testModelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	testModelTag := "model-93608db4-f1cb-4da5-9926-8233981aef0a"
	testTargetVersion := "3.5.0"

	upgradeToParams := &apiparams.UpgradeToRequest{
		TargetControllerVersion: testTargetVersion,
		ModelTag:                testModelTag,
	}

	s.jimmClient.EXPECT().UpgradeTo(upgradeToParams).Return(apiparams.UpgradeToResponse{
		Success: true,
	}, nil)
	s.jimmClient.EXPECT().Close().Return(nil)

	upgradeToCmd := &upgradeToCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return s.jimmClient, nil
		},
		store: s.store,
	}
	f := gnuflag.NewFlagSet("test", gnuflag.ExitOnError)
	f.SetOutput(s.writer)
	upgradeToCmd.SetFlags(f)

	// Set args after setting flags to avoid resetting them.
	upgradeToCmd.version = testTargetVersion
	upgradeToCmd.modelUUID = testModelUUID

	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}
	err := upgradeToCmd.Run(ctx)
	c.Assert(err, gc.IsNil)
}

func (s *upgradeToSuite) TestUpgradeToWithFailureResponse(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	testModelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	testModelTag := "model-93608db4-f1cb-4da5-9926-8233981aef0a"
	testTargetVersion := "3.5.0"
	testErrorMessage := "upgrade failed: controller not ready"

	upgradeToParams := &apiparams.UpgradeToRequest{
		TargetControllerVersion: testTargetVersion,
		ModelTag:                testModelTag,
	}

	// Now the error is returned directly by UpgradeTo instead of embedded in the response.
	s.jimmClient.EXPECT().UpgradeTo(upgradeToParams).Return(apiparams.UpgradeToResponse{}, errors.New(testErrorMessage))
	s.jimmClient.EXPECT().Close().Return(nil)

	upgradeToCmd := &upgradeToCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return s.jimmClient, nil
		},
		store: s.store,
	}
	f := gnuflag.NewFlagSet("test", gnuflag.ExitOnError)
	f.SetOutput(s.writer)
	upgradeToCmd.SetFlags(f)

	upgradeToCmd.version = testTargetVersion
	upgradeToCmd.modelUUID = testModelUUID

	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}
	err := upgradeToCmd.Run(ctx)
	c.Assert(err, gc.ErrorMatches, ".*"+testErrorMessage+".*")
}

func (s *upgradeToSuite) TestUpgradeToWithError(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	testModelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	testModelTag := "model-93608db4-f1cb-4da5-9926-8233981aef0a"
	testTargetVersion := "3.5.0"

	upgradeToParams := &apiparams.UpgradeToRequest{
		TargetControllerVersion: testTargetVersion,
		ModelTag:                testModelTag,
	}
	errorToReturn := errors.New("failed to initiate upgrade")
	s.jimmClient.EXPECT().UpgradeTo(upgradeToParams).Return(apiparams.UpgradeToResponse{}, errorToReturn)
	s.jimmClient.EXPECT().Close().Return(nil)

	upgradeToCmd := &upgradeToCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return s.jimmClient, nil
		},
		store: s.store,
	}
	f := gnuflag.NewFlagSet("test", gnuflag.ExitOnError)
	f.SetOutput(s.writer)
	upgradeToCmd.SetFlags(f)

	// Set args after setting flags to avoid resetting them.
	upgradeToCmd.version = testTargetVersion
	upgradeToCmd.modelUUID = testModelUUID

	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}
	err := upgradeToCmd.Run(ctx)
	c.Assert(err, gc.ErrorMatches, ".*failed to initiate upgrade.*")
}

func (s *upgradeToSuite) TestCommandsFailsWithMissingArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, NewUpgradeToCommandForTesting(jjclient.NewMemStore(), nil))
	c.Assert(err, gc.ErrorMatches, "missing required arguments: version and model UUID")
}

func (s *upgradeToSuite) TestCommandsFailsWithOnlyOneArg(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, NewUpgradeToCommandForTesting(jjclient.NewMemStore(), nil), "3.5.0")
	c.Assert(err, gc.ErrorMatches, "missing required arguments: version and model UUID")
}

func (s *upgradeToSuite) TestCommandsFailsWithInvalidVersion(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, NewUpgradeToCommandForTesting(jjclient.NewMemStore(), nil), "invalid-version", "93608db4-f1cb-4da5-9926-8233981aef0a")
	c.Assert(err, gc.ErrorMatches, "invalid version format: invalid-version")
}

func (s *upgradeToSuite) TestCommandsFailsWithInvalidModelUUID(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, NewUpgradeToCommandForTesting(jjclient.NewMemStore(), nil), "3.5.0", "invalid-uuid")
	c.Assert(err, gc.ErrorMatches, "invalid model UUID: invalid-uuid")
}

func (s *upgradeToSuite) TestCommandWithPositionalArgs(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	testModelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	testModelTag := "model-93608db4-f1cb-4da5-9926-8233981aef0a"
	testTargetVersion := "3.5.0"

	upgradeToParams := &apiparams.UpgradeToRequest{
		TargetControllerVersion: testTargetVersion,
		ModelTag:                testModelTag,
	}

	s.jimmClient.EXPECT().UpgradeTo(upgradeToParams).Return(apiparams.UpgradeToResponse{
		Success: true,
	}, nil)
	s.jimmClient.EXPECT().Close().Return(nil)

	upgradeToCmd := &upgradeToCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return s.jimmClient, nil
		},
		store: s.store,
	}
	f := gnuflag.NewFlagSet("test", gnuflag.ExitOnError)
	f.SetOutput(s.writer)
	upgradeToCmd.SetFlags(f)

	err := upgradeToCmd.Init([]string{testTargetVersion, testModelUUID})
	c.Assert(err, gc.IsNil)

	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}
	err = upgradeToCmd.Run(ctx)
	c.Assert(err, gc.IsNil)
}
