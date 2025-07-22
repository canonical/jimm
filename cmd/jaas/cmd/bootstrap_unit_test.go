// Copyright 2025 Canonical.

package cmd

import (
	"context"
	"encoding/json"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd/mocks"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

type bootstrapCmdSuite struct {
	client *mocks.MockJIMMClient
	writer *mocks.MockWriter
}

var _ = gc.Suite(&bootstrapCmdSuite{})

func (s *bootstrapCmdSuite) SetupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.client = mocks.NewMockJIMMClient(ctrl)
	s.writer = mocks.NewMockWriter(ctrl)

	return ctrl
}

func (s *bootstrapCmdSuite) TestArgParsing(c *gc.C) {
	tests := []struct {
		args       []string
		checkFlags func(*gc.C, *bootstrapCommand)
		errMatch   string
	}{
		{
			args: []string{"test-cloud", "controller-name"},
			checkFlags: func(c *gc.C, command *bootstrapCommand) {
				c.Check(command.cloud, gc.Equals, "test-cloud")
				c.Check(command.region, gc.Equals, "")
				c.Check(command.controllerName, gc.Equals, "controller-name")
			},
		},
		{
			args: []string{"test-cloud/region", "controller-name"},
			checkFlags: func(c *gc.C, command *bootstrapCommand) {
				c.Check(command.cloud, gc.Equals, "test-cloud")
				c.Check(command.region, gc.Equals, "region")
				c.Check(command.controllerName, gc.Equals, "controller-name")
			},
		}, {
			args: []string{"test-cloud/region", "controller-name", "--agent-version=3.6.8", "--timeout=60", "--detach"},
			checkFlags: func(c *gc.C, command *bootstrapCommand) {
				c.Check(command.cloud, gc.Equals, "test-cloud")
				c.Check(command.region, gc.Equals, "region")
				c.Check(command.controllerName, gc.Equals, "controller-name")
				c.Check(command.agentVersion, gc.Equals, "3.6.8")
				c.Check(command.timeout, gc.Equals, 60)
				c.Check(command.detach, gc.Equals, true)
			},
		}, {
			args:     []string{"test-cloud/region"},
			errMatch: "expected at least 2 arguments, got 1",
		},
	}
	for i, test := range tests {
		c.Log("Test ", i)
		command := &bootstrapCommand{}
		command.SetClientStore(jujuclienttesting.MinimalStore())
		err := cmdtesting.InitCommand(command, test.args)
		if test.errMatch == "" {
			c.Check(err, gc.IsNil)
			test.checkFlags(c, command)
		} else {
			c.Check(err, gc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *bootstrapCmdSuite) TestBootstrapRunDetached(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.client.EXPECT().Bootstrap(gomock.Any()).DoAndReturn(func(bsp *params.BootstrapStartParams) (*params.BootstrapStartResponse, error) {
		c.Assert(bsp.ControllerName, gc.Equals, "controller-name")
		c.Assert(bsp.CloudName, gc.Equals, "test-cloud")
		c.Assert(bsp.RegionName, gc.Equals, "region")
		c.Assert(bsp.Flags.AgentVersion, gc.Equals, "3.6.8")
		c.Assert(bsp.Flags.Timeout, gc.Equals, 60)
		return &params.BootstrapStartResponse{
			JobID: "test-job-id",
		}, nil
	})
	s.writer.EXPECT().Write(gomock.Any()).DoAndReturn(func(b []byte) (int, error) {
		resp := &params.BootstrapStartResponse{}
		err := json.Unmarshal(b, resp)
		c.Check(err, gc.IsNil)
		c.Check(resp.JobID, gc.Equals, "test-job-id")
		return len(b), nil
	})

	command := &bootstrapCommand{
		bootstrapAPIFunc: func() (JIMMClient, error) {
			return s.client, nil
		},
	}
	f := gnuflag.NewFlagSet("test", gnuflag.ExitOnError)
	f.SetOutput(s.writer)
	command.SetFlags(f)
	command.controllerName = "controller-name"
	command.cloud = "test-cloud"
	command.region = "region"
	command.agentVersion = "3.6.8"
	command.timeout = 60
	command.detach = true

	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}

	err := command.Run(ctx)
	c.Assert(err, gc.IsNil)
}

func (s *bootstrapCmdSuite) TestBootstrapWatchLogs(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.client.EXPECT().Bootstrap(gomock.Any()).Return(&params.BootstrapStartResponse{
		JobID: "test-job-id",
	}, nil)
	s.writer.EXPECT().Write(gomock.Any()).DoAndReturn(func(b []byte) (int, error) {
		resp := &params.BootstrapStartResponse{}
		err := json.Unmarshal(b, resp)
		c.Check(err, gc.IsNil)
		c.Check(resp.JobID, gc.Equals, "test-job-id")
		return len(b), nil
	})

	s.client.EXPECT().BootstrapStatus(gomock.Any()).Return(params.BootstrapStatusResponse{
		Status:    params.StatusSuccessful,
		Logs:      []string{"log-line", "log-line"},
		Watermark: 2,
	}, nil)

	s.writer.EXPECT().Write(gomock.Any()).DoAndReturn(func(b []byte) (int, error) {
		c.Check(string(b), gc.Equals, "log-line\n")
		return len(b), nil
	}).Times(2)

	s.writer.EXPECT().Write(gomock.Any()).DoAndReturn(func(b []byte) (int, error) {
		c.Check(string(b), gc.Equals, "Bootstrap job completed successfully.\n")
		return len(b), nil
	})

	command := &bootstrapCommand{
		bootstrapAPIFunc: func() (JIMMClient, error) {
			return s.client, nil
		},
	}
	f := gnuflag.NewFlagSet("test", gnuflag.ExitOnError)
	f.SetOutput(s.writer)
	command.SetFlags(f)

	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}

	err := command.Run(ctx)
	c.Assert(err, gc.IsNil)
}
