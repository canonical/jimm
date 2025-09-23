// Copyright 2025 Canonical.

package cmd

import (
	"context"
	"errors"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/gnuflag"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	jujuparams "github.com/juju/juju/rpc/params"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd/mocks"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

type bootstrapCmdSuite struct {
	client *mocks.MockJIMMAPI
	writer *mocks.MockWriter
	store  *mocks.MockClientStore
}

var _ = gc.Suite(&bootstrapCmdSuite{})

func (s *bootstrapCmdSuite) SetupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.client = mocks.NewMockJIMMAPI(ctrl)
	s.writer = mocks.NewMockWriter(ctrl)
	s.store = mocks.NewMockClientStore(ctrl)

	return ctrl
}

func (s *bootstrapCmdSuite) TestArgParsing(c *gc.C) {
	tests := []struct {
		args       []string
		checkFlags func(*gc.C, *bootstrapCommand)
		errMatch   string
	}{
		{
			args: []string{"test-cloud", "controller-name", "controller-version"},
			checkFlags: func(c *gc.C, command *bootstrapCommand) {
				c.Check(command.cloud, gc.Equals, "test-cloud")
				c.Check(command.region, gc.Equals, "")
				c.Check(command.controllerName, gc.Equals, "controller-name")
				c.Check(command.controllerVersion, gc.Equals, "controller-version")
			},
		},
		{
			args: []string{"test-cloud/region", "controller-name", "controller-version"},
			checkFlags: func(c *gc.C, command *bootstrapCommand) {
				c.Check(command.cloud, gc.Equals, "test-cloud")
				c.Check(command.region, gc.Equals, "region")
				c.Check(command.controllerName, gc.Equals, "controller-name")
				c.Check(command.controllerVersion, gc.Equals, "controller-version")
			},
		}, {
			args: []string{"test-cloud/region", "controller-name", "controller-version", "--timeout=60", "--detach"},
			checkFlags: func(c *gc.C, command *bootstrapCommand) {
				c.Check(command.cloud, gc.Equals, "test-cloud")
				c.Check(command.region, gc.Equals, "region")
				c.Check(command.controllerName, gc.Equals, "controller-name")
				c.Check(command.controllerVersion, gc.Equals, "controller-version")
				c.Check(command.timeout, gc.Equals, 60)
				c.Check(command.detach, gc.Equals, true)
			},
		}, {
			args:     []string{"test-cloud/region"},
			errMatch: "expected at least 3 arguments, got 1",
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

	cloudName := "aws"

	s.store.EXPECT().CredentialForCloud(cloudName).Return(&jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"cred-1": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{}),
		},
	}, nil)
	s.client.EXPECT().Bootstrap(gomock.Any()).DoAndReturn(func(bsp *params.BootstrapStartParams) (*params.BootstrapStartResponse, error) {
		expected := &params.BootstrapStartParams{
			ControllerName: "controller-name",
			CloudName:      cloudName,
			RegionName:     "region",
			Cloud:          jujuparams.Cloud{},
			Credential: jujuparams.CloudCredential{
				AuthType:   string(jujucloud.UserPassAuthType),
				Attributes: map[string]string{},
			},
			Flags: params.BootstrapFlags{
				Timeout: 60,
			},
			ControllerVersion: "controller-version",
		}
		c.Assert(bsp.ControllerName, gc.Equals, expected.ControllerName)
		c.Assert(bsp.CloudName, gc.Equals, expected.CloudName)
		c.Assert(bsp.RegionName, gc.Equals, expected.RegionName)
		// AWS is dynamically populated, i.e., 32 regions.
		// So we expect just ec2 and it should be ok.
		c.Assert(bsp.Cloud.Type, gc.DeepEquals, "ec2")
		c.Assert(bsp.Credential, gc.DeepEquals, expected.Credential)
		c.Assert(bsp.Flags.Timeout, gc.Equals, expected.Flags.Timeout)
		c.Assert(bsp.ControllerVersion, gc.Equals, expected.ControllerVersion)

		return &params.BootstrapStartResponse{
			JobID: "test-job-id",
		}, nil
	})
	s.client.EXPECT().Close().Return(nil)

	command := &bootstrapCommand{
		store: s.store,
		bootstrapAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}
	f := gnuflag.NewFlagSet("test", gnuflag.ExitOnError)
	f.SetOutput(s.writer)
	command.SetFlags(f)
	command.controllerName = "controller-name"
	command.cloud = cloudName
	command.region = "region"
	command.controllerVersion = "controller-version"
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

	s.store.EXPECT().CredentialForCloud("aws").Return(&jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"cred-1": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{}),
		},
	}, nil)
	s.client.EXPECT().Bootstrap(gomock.Any()).Return(&params.BootstrapStartResponse{
		JobID: "test-job-id",
	}, nil)
	s.client.EXPECT().Close().Return(nil)

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
		store: s.store,
		bootstrapAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}
	f := gnuflag.NewFlagSet("test", gnuflag.ExitOnError)
	f.SetOutput(s.writer)
	command.SetFlags(f)
	command.cloud = "aws"

	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}

	err := command.Run(ctx)
	c.Assert(err, gc.IsNil)
}

func (s *bootstrapCmdSuite) TestBootstrapFailsToGetCredential(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.store.EXPECT().CredentialForCloud("aws").Return(nil, errors.New("credential not found"))

	command := &bootstrapCommand{
		store: s.store,
		bootstrapAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}
	f := gnuflag.NewFlagSet("test", gnuflag.ExitOnError)
	f.SetOutput(s.writer)
	command.SetFlags(f)
	command.controllerName = "controller-name"
	command.cloud = "aws" // Need a valid cloud to reach credential error.
	command.region = "region"
	command.controllerVersion = "controller-version"

	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}

	err := command.Run(ctx)
	c.Assert(err, gc.ErrorMatches, `failed to get credential for cloud "aws": credential not found`)
}

func (s *bootstrapCmdSuite) TestBootstrapMultipleCredentials(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.store.EXPECT().CredentialForCloud("aws").Return(&jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"cred-1": {Label: "cred-1"},
			"cred-2": {Label: "cred-2"},
		},
	}, nil).Times(2)

	command := &bootstrapCommand{
		store: s.store,
		bootstrapAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}
	f := gnuflag.NewFlagSet("test", gnuflag.ExitOnError)
	f.SetOutput(s.writer)
	command.SetFlags(f)
	command.controllerName = "controller-name"
	command.cloud = "aws" // Need a valid cloud to reach credential error.
	command.region = "region"
	command.controllerVersion = "controller-version"

	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}

	err := command.Run(ctx)
	c.Assert(err, gc.ErrorMatches, `multiple credentials found for cloud "aws", please set a default or specify one using --credential`)

	// Now specify a credential and verify the command works.
	command.credentialName = "cred-2"

	s.client.EXPECT().Bootstrap(gomock.Any()).Return(&params.BootstrapStartResponse{
		JobID: "test-job-id",
	}, nil)
	s.client.EXPECT().Close().Return(nil)

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

	err = command.Run(ctx)
	c.Assert(err, gc.IsNil)
}

func (s *bootstrapCmdSuite) TestBootstrapWithDefaultCredential(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	cloudName := "aws"

	s.store.EXPECT().CredentialForCloud(cloudName).Return(&jujucloud.CloudCredential{
		DefaultCredential: "cred-1",
		AuthCredentials: map[string]jujucloud.Credential{
			"cred-1": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{}),
			"cred-2": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{}),
		},
	}, nil)

	s.client.EXPECT().Bootstrap(gomock.Any()).Return(&params.BootstrapStartResponse{JobID: "test-job-id"}, nil)
	s.client.EXPECT().Close().Return(nil)

	command := &bootstrapCommand{
		store: s.store,
		bootstrapAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}
	f := gnuflag.NewFlagSet("test", gnuflag.ExitOnError)
	f.SetOutput(s.writer)
	command.SetFlags(f)
	command.controllerName = "controller-name"
	command.cloud = cloudName
	command.region = "region"
	command.controllerVersion = "controller-version"
	command.timeout = 60
	command.detach = true

	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}

	err := command.Run(ctx)
	c.Assert(err, gc.IsNil)
}

func (s *bootstrapCmdSuite) TestBootstrapSpecifiedCredentialWithDefault(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	cloudName := "aws"

	s.store.EXPECT().CredentialForCloud(cloudName).Return(&jujucloud.CloudCredential{
		DefaultCredential: "cred-1",
		AuthCredentials: map[string]jujucloud.Credential{
			"cred-1": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{}),
			"cred-2": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{}),
		},
	}, nil)

	command := &bootstrapCommand{
		store: s.store,
		bootstrapAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}
	f := gnuflag.NewFlagSet("test", gnuflag.ExitOnError)
	f.SetOutput(s.writer)
	command.SetFlags(f)
	command.controllerName = "controller-name"
	command.cloud = cloudName
	command.region = "region"
	command.controllerVersion = "controller-version"
	command.timeout = 60
	command.detach = true
	command.credentialName = "cred-3" // Use a different credential than the default.

	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}

	err := command.Run(ctx)
	c.Assert(err, gc.ErrorMatches, `no credential found with name "cred-3"`)
}
