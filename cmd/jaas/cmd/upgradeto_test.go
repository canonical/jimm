// Copyright 2025 Canonical.

package cmd

import (
	"bytes"
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
	testTargetController := "test-controller"

	upgradeToParams := &apiparams.UpgradeToRequest{
		TargetControllerName: testTargetController,
		ModelTag:             testModelTag,
	}

	s.client.EXPECT().UpgradeTo(upgradeToParams).Return(apiparams.UpgradeToResponse{
		Success: true,
		JobID:   12345,
	}, nil)
	s.client.EXPECT().Close().Return(nil)

	upgradeToCmd := &upgradeToCommand{}
	upgradeToCmd.setJIMMAPI(s.client)
	upgradeToCmd.SetClientStore(s.store)
	initCommand(c, upgradeToCmd, testTargetController, testModelUUID)

	ctx := newTestContext(c)
	err := upgradeToCmd.Run(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), qt.Matches, "success: true\njob-id: 12345\n")
}

func TestUpgradeToWithFailureResponse(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	testModelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	testModelTag := "model-93608db4-f1cb-4da5-9926-8233981aef0a"
	testTargetController := "test-controller"
	testErrorMessage := "upgrade failed: controller not ready"

	upgradeToParams := &apiparams.UpgradeToRequest{
		TargetControllerName: testTargetController,
		ModelTag:             testModelTag,
	}

	// Now the error is returned directly by UpgradeTo instead of embedded in the response.
	s.client.EXPECT().UpgradeTo(upgradeToParams).Return(apiparams.UpgradeToResponse{}, errors.New(testErrorMessage))
	s.client.EXPECT().Close().Return(nil)

	upgradeToCmd := &upgradeToCommand{}
	upgradeToCmd.setJIMMAPI(s.client)
	initCommand(c, upgradeToCmd, testTargetController, testModelUUID)

	ctx := newTestContext(c)
	err := upgradeToCmd.Run(ctx)
	c.Assert(err, qt.ErrorMatches, ".*"+testErrorMessage+".*")
}

func TestUpgradeToWithError(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	testModelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	testModelTag := "model-93608db4-f1cb-4da5-9926-8233981aef0a"
	testTargetController := "test-controller"

	upgradeToParams := &apiparams.UpgradeToRequest{
		TargetControllerName: testTargetController,
		ModelTag:             testModelTag,
	}
	errorToReturn := errors.New("failed to initiate upgrade")
	s.client.EXPECT().UpgradeTo(upgradeToParams).Return(apiparams.UpgradeToResponse{}, errorToReturn)
	s.client.EXPECT().Close().Return(nil)

	upgradeToCmd := &upgradeToCommand{}
	upgradeToCmd.setJIMMAPI(s.client)
	initCommand(c, upgradeToCmd, testTargetController, testModelUUID)

	ctx := newTestContext(c)
	err := upgradeToCmd.Run(ctx)
	c.Assert(err, qt.ErrorMatches, ".*failed to initiate upgrade.*")
}

func TestUpgradeToFailsWithMissingArgs(t *testing.T) {
	c := qt.New(t)
	upgradeToCmd := &upgradeToCommand{}
	err := initCommandWithError(upgradeToCmd)
	c.Assert(err, qt.ErrorMatches, "missing required arguments: controller name and model UUID")
}

func TestUpgradeToFailsWithOnlyOneArg(t *testing.T) {
	c := qt.New(t)
	upgradeToCmd := &upgradeToCommand{}
	err := initCommandWithError(upgradeToCmd, "3.5.0")
	c.Assert(err, qt.ErrorMatches, "missing required arguments: controller name and model UUID")
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
	testTargetController := "test-controller"

	upgradeToParams := &apiparams.UpgradeToRequest{
		TargetControllerName: testTargetController,
		ModelTag:             testModelTag,
	}

	s.client.EXPECT().UpgradeTo(upgradeToParams).Return(apiparams.UpgradeToResponse{
		Success: true,
	}, nil)
	s.client.EXPECT().Close().Return(nil)

	upgradeToCmd := &upgradeToCommand{}
	upgradeToCmd.setJIMMAPI(s.client)
	upgradeToCmd.SetClientStore(s.store)
	initCommand(c, upgradeToCmd, testTargetController, testModelUUID)

	ctx := newTestContext(c)
	err := upgradeToCmd.Run(ctx)
	c.Assert(err, qt.IsNil)
}
