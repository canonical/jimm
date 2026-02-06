// Copyright 2025 Canonical.

// Note that this file is not an integration test
// because of limitations with the JujuConnSuite
// so it is placed under the cmd package.

package cmd

import (
	"errors"
	"testing"

	qt "github.com/frankban/quicktest"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func TestUpgradeTo(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	testModelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	testModelTag := "model-93608db4-f1cb-4da5-9926-8233981aef0a"
	testTargetVersion := "3.5.0"

	upgradeToParams := &apiparams.UpgradeToRequest{
		TargetControllerVersion: testTargetVersion,
		ModelTag:                testModelTag,
	}

	s.client.EXPECT().UpgradeTo(upgradeToParams).Return(apiparams.UpgradeToResponse{
		Success: true,
	}, nil)
	s.client.EXPECT().Close().Return(nil)

	upgradeToCmd := &upgradeToCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}
	upgradeToCmd.SetClientStore(s.store)
	initCommand(c, upgradeToCmd, testTargetVersion, testModelUUID)

	ctx := newTestContext(c)
	err := upgradeToCmd.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestUpgradeToWithFailureResponse(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	testModelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	testModelTag := "model-93608db4-f1cb-4da5-9926-8233981aef0a"
	testTargetVersion := "3.5.0"
	testErrorMessage := "upgrade failed: controller not ready"

	upgradeToParams := &apiparams.UpgradeToRequest{
		TargetControllerVersion: testTargetVersion,
		ModelTag:                testModelTag,
	}

	// Now the error is returned directly by UpgradeTo instead of embedded in the response.
	s.client.EXPECT().UpgradeTo(upgradeToParams).Return(apiparams.UpgradeToResponse{}, errors.New(testErrorMessage))
	s.client.EXPECT().Close().Return(nil)

	upgradeToCmd := &upgradeToCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}
	initCommand(c, upgradeToCmd, testTargetVersion, testModelUUID)

	ctx := newTestContext(c)
	err := upgradeToCmd.Run(ctx)
	c.Assert(err, qt.ErrorMatches, ".*"+testErrorMessage+".*")
}

func TestUpgradeToWithError(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	testModelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	testModelTag := "model-93608db4-f1cb-4da5-9926-8233981aef0a"
	testTargetVersion := "3.5.0"

	upgradeToParams := &apiparams.UpgradeToRequest{
		TargetControllerVersion: testTargetVersion,
		ModelTag:                testModelTag,
	}
	errorToReturn := errors.New("failed to initiate upgrade")
	s.client.EXPECT().UpgradeTo(upgradeToParams).Return(apiparams.UpgradeToResponse{}, errorToReturn)
	s.client.EXPECT().Close().Return(nil)

	upgradeToCmd := &upgradeToCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}
	initCommand(c, upgradeToCmd, testTargetVersion, testModelUUID)

	ctx := newTestContext(c)
	err := upgradeToCmd.Run(ctx)
	c.Assert(err, qt.ErrorMatches, ".*failed to initiate upgrade.*")
}

func TestUpgradeToFailsWithMissingArgs(t *testing.T) {
	c := qt.New(t)
	upgradeToCmd := &upgradeToCommand{}
	err := initCommandWithError(upgradeToCmd)
	c.Assert(err, qt.ErrorMatches, "missing required arguments: version and model UUID")
}

func TestUpgradeToFailsWithOnlyOneArg(t *testing.T) {
	c := qt.New(t)
	upgradeToCmd := &upgradeToCommand{}
	err := initCommandWithError(upgradeToCmd, "3.5.0")
	c.Assert(err, qt.ErrorMatches, "missing required arguments: version and model UUID")
}

func TestUpgradeToFailsWithInvalidVersion(t *testing.T) {
	c := qt.New(t)
	upgradeToCmd := &upgradeToCommand{}
	err := initCommandWithError(upgradeToCmd, "invalid-version", "93608db4-f1cb-4da5-9926-8233981aef0a")
	c.Assert(err, qt.ErrorMatches, "invalid version format: invalid-version")
}

func TestUpgradeToFailsWithInvalidModelUUID(t *testing.T) {
	c := qt.New(t)
	upgradeToCmd := &upgradeToCommand{}
	err := initCommandWithError(upgradeToCmd, "3.5.0", "invalid-uuid")
	c.Assert(err, qt.ErrorMatches, "invalid model UUID: invalid-uuid")
}

func TestUpgradeToWithPositionalArgs(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	testModelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	testModelTag := "model-93608db4-f1cb-4da5-9926-8233981aef0a"
	testTargetVersion := "3.5.0"

	upgradeToParams := &apiparams.UpgradeToRequest{
		TargetControllerVersion: testTargetVersion,
		ModelTag:                testModelTag,
	}

	s.client.EXPECT().UpgradeTo(upgradeToParams).Return(apiparams.UpgradeToResponse{
		Success: true,
	}, nil)
	s.client.EXPECT().Close().Return(nil)

	upgradeToCmd := &upgradeToCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}
	upgradeToCmd.SetClientStore(s.store)
	initCommand(c, upgradeToCmd, testTargetVersion, testModelUUID)

	ctx := newTestContext(c)
	err := upgradeToCmd.Run(ctx)
	c.Assert(err, qt.IsNil)
}
