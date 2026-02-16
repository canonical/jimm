// Copyright 2026 Canonical.

package cmd

import (
	"bytes"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/pkg/api/params"
)

func TestArgParsing(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		args       []string
		checkFlags func(*qt.C, *destroyControllerCommand)
		errMatch   string
	}{
		{
			args: []string{"controller-name"},
			checkFlags: func(c *qt.C, command *destroyControllerCommand) {
				c.Check(command.controllerName, qt.Equals, "controller-name")
			},
		},
		{
			args: []string{"controller-name"},
			checkFlags: func(c *qt.C, command *destroyControllerCommand) {
				c.Check(command.controllerName, qt.Equals, "controller-name")
			},
		}, {
			args: []string{"controller-name", "--detach"},
			checkFlags: func(c *qt.C, command *destroyControllerCommand) {
				c.Check(command.controllerName, qt.Equals, "controller-name")
				c.Check(command.detach, qt.Equals, true)
			},
		}, {
			args:     []string{},
			errMatch: "missing controller name",
		},
	}
	for i, test := range tests {
		c.Log("Test ", i)
		command := &destroyControllerCommand{}
		command.SetClientStore(jujuclienttesting.MinimalStore())
		command.noPrompt = true
		err := cmdtesting.InitCommand(command, test.args)
		if test.errMatch == "" {
			c.Check(err, qt.IsNil)
			test.checkFlags(c, command)
		} else {
			c.Check(err, qt.ErrorMatches, test.errMatch)
		}
	}
}

func TestRunDetached(t *testing.T) {
	c := qt.New(t)

	s := setupCmdMocks(c)

	s.client.EXPECT().StartDestroyController(gomock.Any()).DoAndReturn(func(bsp *params.DestroyControllerRequest) (*params.StartBootstrapResponse, error) {
		expected := &params.DestroyControllerRequest{
			ControllerName: "controller-name",
		}
		c.Assert(bsp.ControllerName, qt.Equals, expected.ControllerName)

		return &params.StartBootstrapResponse{
			JobID: "test-job-id",
		}, nil
	})
	s.client.EXPECT().Close().Return(nil)

	command := &destroyControllerCommand{}
	command.setJIMMAPI(s.client)
	command.SetClientStore(s.store)

	initCommand(c, command, "controller-name", "--detach", "--no-prompt")

	ctx := newTestContext(c)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestWatchLogs(t *testing.T) {
	c := qt.New(t)

	s := setupCmdMocks(c)

	s.client.EXPECT().StartDestroyController(gomock.Any()).Return(&params.StartBootstrapResponse{
		JobID: "test-job-id",
	}, nil)
	s.client.EXPECT().Close().Return(nil)

	s.client.EXPECT().BootstrapInfo(gomock.Any()).Return(params.GetBootstrapInfoResponse{
		Status:    params.StatusSuccessful,
		Logs:      []string{"log-line", "log-line"},
		Watermark: 2,
	}, nil)

	command := &destroyControllerCommand{}
	command.setJIMMAPI(s.client)
	command.SetClientStore(s.store)

	initCommand(c, command, "controller-name", "--no-prompt")

	ctx := newTestContext(c)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	out := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(out, qt.Equals, "log-line\nlog-line\nJob completed successfully.\n")
}
