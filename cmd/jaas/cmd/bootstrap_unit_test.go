// Copyright 2025 Canonical.

package cmd

import (
	"context"
	"errors"
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/gnuflag"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	jujuparams "github.com/juju/juju/rpc/params"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd/mocks"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

type bootstrapCmdMocks struct {
	client *mocks.MockJIMMAPI
	writer *mocks.MockWriter
	store  *mocks.MockClientStore
}

func setupBootstrapMocks(t *testing.T) *bootstrapCmdMocks {
	t.Helper()
	ctrl := gomock.NewController(t)
	h := &bootstrapCmdMocks{
		client: mocks.NewMockJIMMAPI(ctrl),
		writer: mocks.NewMockWriter(ctrl),
		store:  mocks.NewMockClientStore(ctrl),
	}
	t.Cleanup(ctrl.Finish)
	return h
}

func TestBootstrapArgParsing(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		checkFlags func(*qt.C, *bootstrapCommand)
		errMatch   string
	}{
		{
			name: "cloud-no-region",
			args: []string{"test-cloud", "controller-name", "controller-version"},
			checkFlags: func(c *qt.C, command *bootstrapCommand) {
				c.Check(command.cloud, qt.Equals, "test-cloud")
				c.Check(command.region, qt.Equals, "")
				c.Check(command.controllerName, qt.Equals, "controller-name")
				c.Check(command.controllerVersion, qt.Equals, "controller-version")
			},
		},
		{
			name: "cloud-with-region",
			args: []string{"test-cloud/region", "controller-name", "controller-version"},
			checkFlags: func(c *qt.C, command *bootstrapCommand) {
				c.Check(command.cloud, qt.Equals, "test-cloud")
				c.Check(command.region, qt.Equals, "region")
				c.Check(command.controllerName, qt.Equals, "controller-name")
				c.Check(command.controllerVersion, qt.Equals, "controller-version")
			},
		}, {
			name: "cloud-with-region-detach",
			args: []string{"test-cloud/region", "controller-name", "controller-version", "--detach"},
			checkFlags: func(c *qt.C, command *bootstrapCommand) {
				c.Check(command.cloud, qt.Equals, "test-cloud")
				c.Check(command.region, qt.Equals, "region")
				c.Check(command.controllerName, qt.Equals, "controller-name")
				c.Check(command.controllerVersion, qt.Equals, "controller-version")
				c.Check(command.detach, qt.Equals, true)
			},
		}, {
			name:     "too-few-args",
			args:     []string{"test-cloud/region"},
			errMatch: "expected at least 3 arguments, got 1",
		},
	}
	for i, test := range tests {
		test := test
		t.Run(fmt.Sprintf("%02d-%s", i, test.name), func(t *testing.T) {
			c := qt.New(t)
			command := &bootstrapCommand{}
			command.SetClientStore(jujuclienttesting.MinimalStore())
			err := cmdtesting.InitCommand(command, test.args)
			if test.errMatch == "" {
				c.Assert(err, qt.IsNil)
				test.checkFlags(c, command)
				return
			}
			c.Assert(err, qt.ErrorMatches, test.errMatch)
		})
	}
}

func TestBootstrapRunDetached(t *testing.T) {
	c := qt.New(t)
	s := setupBootstrapMocks(t)

	cloudName := "aws"

	s.store.EXPECT().CredentialForCloud(cloudName).Return(&jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"cred-1": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{}),
		},
	}, nil)
	s.client.EXPECT().StartBootstrapJob(gomock.Any()).DoAndReturn(func(bsp *params.BootstrapParams) (*params.StartJobResponse, error) {
		expected := &params.BootstrapParams{
			ControllerName: "controller-name",
			CloudName:      cloudName,
			RegionName:     "region",
			Cloud:          jujuparams.Cloud{},
			Credential: jujuparams.CloudCredential{
				AuthType:   string(jujucloud.UserPassAuthType),
				Attributes: map[string]string{},
			},
			Config: map[string]string{
				"bootstrap-timeout": "60",
				"string-option":     "value",
			},
			ControllerVersion: "controller-version",
		}
		c.Assert(bsp.ControllerName, qt.Equals, expected.ControllerName)
		c.Assert(bsp.CloudName, qt.Equals, expected.CloudName)
		c.Assert(bsp.RegionName, qt.Equals, expected.RegionName)
		// AWS is dynamically populated, i.e., 32 regions.
		// So we expect just ec2 and it should be ok.
		c.Assert(bsp.Cloud.Type, qt.Equals, "ec2")
		c.Assert(bsp.Credential, qt.DeepEquals, expected.Credential)
		c.Assert(bsp.Config, qt.DeepEquals, expected.Config)
		c.Assert(bsp.ControllerVersion, qt.Equals, expected.ControllerVersion)

		return &params.StartJobResponse{
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

	configOpts := common.ConfigFlag{}
	err := configOpts.Set("bootstrap-timeout=60")
	c.Assert(err, qt.IsNil)
	err = configOpts.Set("string-option=value")
	c.Assert(err, qt.IsNil)

	f := gnuflag.NewFlagSet("test", gnuflag.ExitOnError)
	f.SetOutput(s.writer)
	command.SetFlags(f)
	command.controllerName = "controller-name"
	command.cloud = cloudName
	command.region = "region"
	command.controllerVersion = "controller-version"
	command.config = configOpts
	command.detach = true

	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}

	err = command.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestBootstrapWatchLogs(t *testing.T) {
	c := qt.New(t)
	s := setupBootstrapMocks(t)

	s.store.EXPECT().CredentialForCloud("aws").Return(&jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"cred-1": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{}),
		},
	}, nil)
	s.client.EXPECT().StartBootstrapJob(gomock.Any()).Return(&params.StartJobResponse{
		JobID: "test-job-id",
	}, nil)
	s.client.EXPECT().Close().Return(nil)

	s.client.EXPECT().GetJobInfo(gomock.Any()).Return(params.GetJobInfoResponse{
		Status:    params.StatusSuccessful,
		Logs:      []string{"log-line", "log-line"},
		Watermark: 2,
	}, nil)

	s.writer.EXPECT().Write(gomock.Any()).DoAndReturn(func(b []byte) (int, error) {
		c.Check(string(b), qt.Equals, "log-line\n")
		return len(b), nil
	}).Times(2)

	s.writer.EXPECT().Write(gomock.Any()).DoAndReturn(func(b []byte) (int, error) {
		c.Check(string(b), qt.Equals, "Job completed successfully.\n")
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
	c.Assert(err, qt.IsNil)
}

func TestBootstrapFailsToGetCredential(t *testing.T) {
	c := qt.New(t)
	s := setupBootstrapMocks(t)

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
	c.Assert(err, qt.ErrorMatches, `failed to get credential for cloud "aws": credential not found`)
}

func TestBootstrapMultipleCredentials(t *testing.T) {
	c := qt.New(t)
	s := setupBootstrapMocks(t)

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
	c.Assert(err, qt.ErrorMatches, `multiple credentials found for cloud "aws", please set a default or specify one using --credential`)

	// Now specify a credential and verify the command works.
	command.credentialName = "cred-2"

	s.client.EXPECT().StartBootstrapJob(gomock.Any()).Return(&params.StartJobResponse{
		JobID: "test-job-id",
	}, nil)
	s.client.EXPECT().Close().Return(nil)

	s.client.EXPECT().GetJobInfo(gomock.Any()).Return(params.GetJobInfoResponse{
		Status:    params.StatusSuccessful,
		Logs:      []string{"log-line", "log-line"},
		Watermark: 2,
	}, nil)

	s.writer.EXPECT().Write(gomock.Any()).DoAndReturn(func(b []byte) (int, error) {
		c.Check(string(b), qt.Equals, "log-line\n")
		return len(b), nil
	}).Times(2)

	s.writer.EXPECT().Write(gomock.Any()).DoAndReturn(func(b []byte) (int, error) {
		c.Check(string(b), qt.Equals, "Job completed successfully.\n")
		return len(b), nil
	})

	err = command.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestBootstrapWithDefaultCredential(t *testing.T) {
	c := qt.New(t)
	s := setupBootstrapMocks(t)

	cloudName := "aws"

	s.store.EXPECT().CredentialForCloud(cloudName).Return(&jujucloud.CloudCredential{
		DefaultCredential: "cred-1",
		AuthCredentials: map[string]jujucloud.Credential{
			"cred-1": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{}),
			"cred-2": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{}),
		},
	}, nil)

	s.client.EXPECT().StartBootstrapJob(gomock.Any()).Return(&params.StartJobResponse{JobID: "test-job-id"}, nil)
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
	command.detach = true

	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}

	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestBootstrapSpecifiedCredentialWithDefault(t *testing.T) {
	c := qt.New(t)
	s := setupBootstrapMocks(t)

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
	command.detach = true
	command.credentialName = "cred-3" // Use a different credential than the default.

	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}

	err := command.Run(ctx)
	c.Assert(err, qt.ErrorMatches, `no credential found with name "cred-3"`)
}
