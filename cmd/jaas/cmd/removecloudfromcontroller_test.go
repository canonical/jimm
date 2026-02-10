// Copyright 2026 Canonical.

package cmd

import (
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/cmd/v3/cmdtesting"
	"go.uber.org/mock/gomock"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func TestRemoveCloudFromController(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	s.client.EXPECT().RemoveCloudFromController(gomock.Any()).DoAndReturn(
		func(rcfcr *apiparams.RemoveCloudFromControllerRequest) error {
			c.Check(rcfcr.ControllerName, qt.Equals, "controller-1")
			c.Check(rcfcr.CloudTag, qt.Equals, "cloud-test-cloud")
			return nil
		})
	s.client.EXPECT().Close()

	command := &removeCloudFromControllerCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}

	initCommand(c, command, "controller-1", "test-cloud")
	ctx := newTestContext(c)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(cmdtesting.Stderr(ctx), qt.Equals, "Cloud \"test-cloud\" removed from controller \"controller-1\".\n")
}

func TestRemoveCloudFromControllerWrongArguments(t *testing.T) {
	c := qt.New(t)

	command := &removeCloudFromControllerCommand{}

	err := initCommandWithError(command, "controller-1")
	c.Assert(err, qt.ErrorMatches, "missing arguments")

	err = initCommandWithError(command, "controller-1", "cloud", "fake-arg")
	c.Assert(err, qt.ErrorMatches, "too many arguments")
}

func TestRemoveCloudFromControllerCloudNotFound(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	s.client.EXPECT().RemoveCloudFromController(gomock.Any()).Return(fmt.Errorf("cloud \"test-cloud\" not found"))
	command := &removeCloudFromControllerCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}
	s.client.EXPECT().Close()

	initCommand(c, command, "controller-1", "test-cloud")
	ctx := newTestContext(c)
	err := command.Run(ctx)
	c.Assert(err, qt.ErrorMatches, ".*cloud \"test-cloud\" not found")
}
