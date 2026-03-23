// Copyright 2026 Canonical.

package cmd

import (
	"errors"
	"fmt"
	"os"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/cmd/v3/cmdtesting"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	jujuparams "github.com/juju/juju/rpc/params"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/pkg/api/params"
)

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

func TestBootstrapWithPublicCloud(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	cloudName := "aws"

	s.store.EXPECT().CredentialForCloud(cloudName).Return(&jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"cred-1": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{}),
		},
	}, nil)

	s.client.EXPECT().StartBootstrap(gomock.Any()).DoAndReturn(func(bsp *params.BootstrapParams) (*params.StartBootstrapResponse, error) {
		// If the cloud is public, the Cloud field should be empty.
		c.Check(bsp.Cloud, qt.DeepEquals, jujuparams.Cloud{})
		return &params.StartBootstrapResponse{JobID: "test-job-id"}, nil
	})
	s.client.EXPECT().Close().Return(nil)

	command := &bootstrapCommand{}
	command.setJIMMAPI(s.client)
	command.SetClientStore(s.store)

	initCommand(c, command,
		fmt.Sprintf("%s/region", cloudName),
		"controller-name",
		"controller-version",
		"--detach",
	)

	ctx := newTestContext(c)
	mcmd := modelcmd.WrapBase(command)
	err := mcmd.Run(ctx)
	c.Assert(err, qt.IsNil)
}

// TestBootstrapApiParams verifies that the parameters passed to the
// Bootstrap API are correctly constructed from the command line arguments.
// It also uses a personal cloud to verify that the cloud params are set correctly.
func TestBootstrapApiParams(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	// Override the Juju data dir to a temp dir. The cloud-related operations
	// are not available on the store interface.
	tmpJujuDir := t.TempDir()
	defer os.RemoveAll(tmpJujuDir)
	oldJujuDir := osenv.SetJujuXDGDataHome(tmpJujuDir)
	defer osenv.SetJujuXDGDataHome(oldJujuDir)

	cloudName := "my-cloud"
	personalCloud := jujucloud.Cloud{
		Name:      cloudName,
		Type:      "openstack",
		AuthTypes: jujucloud.AuthTypes{jujucloud.UserPassAuthType},
		Endpoint:  "some-endpoint",
	}
	err := jujucloud.WritePersonalCloudMetadata(map[string]jujucloud.Cloud{
		cloudName: personalCloud,
	})
	c.Assert(err, qt.IsNil)

	s.store.EXPECT().CredentialForCloud(cloudName).Return(&jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"cred-1": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{}),
		},
	}, nil)
	s.client.EXPECT().StartBootstrap(gomock.Any()).DoAndReturn(func(bsp *params.BootstrapParams) (*params.StartBootstrapResponse, error) {
		expected := &params.BootstrapParams{
			ControllerName: "controller-name",
			CloudName:      cloudName,
			RegionName:     "region",
			Cloud: jujuparams.Cloud{
				Type:      "openstack",
				AuthTypes: []string{string(jujucloud.UserPassAuthType)},
				Endpoint:  "some-endpoint",
				Regions:   []jujuparams.CloudRegion{},
			},
			Credential: jujuparams.CloudCredential{
				AuthType:   string(jujucloud.UserPassAuthType),
				Attributes: map[string]string{},
			},
			BootstrapOptions: params.BootstrapOptions{
				BootstrapConfig: map[string]string{
					"bootstrap-timeout": "60",
					"string-option":     "value",
				},
			},
			ControllerVersion: "controller-version",
		}
		c.Check(bsp.ControllerName, qt.Equals, expected.ControllerName)
		c.Check(bsp.CloudName, qt.Equals, expected.CloudName)
		c.Check(bsp.RegionName, qt.Equals, expected.RegionName)
		c.Check(bsp.Cloud, qt.DeepEquals, expected.Cloud)
		c.Check(bsp.Credential, qt.DeepEquals, expected.Credential)
		c.Check(bsp.BootstrapOptions, qt.DeepEquals, expected.BootstrapOptions)
		c.Check(bsp.ControllerVersion, qt.Equals, expected.ControllerVersion)

		return &params.StartBootstrapResponse{
			JobID: "test-job-id",
		}, nil
	})
	s.client.EXPECT().Close().Return(nil)

	command := &bootstrapCommand{}
	command.setJIMMAPI(s.client)
	command.SetClientStore(s.store)

	initCommand(c, command,
		fmt.Sprintf("%s/region", cloudName),
		"controller-name",
		"controller-version",
		"--detach",
		"--config", "bootstrap-timeout=60",
		"--config", "string-option=value",
	)

	ctx := newTestContext(c)
	mcmd := modelcmd.WrapBase(command)
	err = mcmd.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestBootstrapRunDetached(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	cloudName := "aws"

	s.store.EXPECT().CredentialForCloud(cloudName).Return(&jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"cred-1": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{}),
		},
	}, nil)
	s.client.EXPECT().StartBootstrap(gomock.Any()).Return(&params.StartBootstrapResponse{
		JobID: "test-job-id",
	}, nil)
	s.client.EXPECT().Close().Return(nil)

	command := &bootstrapCommand{}
	command.setJIMMAPI(s.client)
	command.SetClientStore(s.store)

	initCommand(c, command,
		fmt.Sprintf("%s/region", cloudName),
		"controller-name",
		"controller-version",
		"--detach",
		"--config", "bootstrap-timeout=60",
		"--config", "string-option=value",
	)

	ctx := newTestContext(c)
	mcmd := modelcmd.WrapBase(command)
	err := mcmd.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestBootstrapWatchLogs(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	s.store.EXPECT().CredentialForCloud("aws").Return(&jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"cred-1": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{}),
		},
	}, nil)
	s.client.EXPECT().StartBootstrap(gomock.Any()).Return(&params.StartBootstrapResponse{
		JobID: "test-job-id",
	}, nil)
	s.client.EXPECT().Close().Return(nil)

	s.client.EXPECT().BootstrapInfo(gomock.Any()).Return(params.GetBootstrapInfoResponse{
		Status:    params.StatusSuccessful,
		Logs:      []string{"log-line", "log-line"},
		Watermark: 2,
	}, nil)

	command := &bootstrapCommand{}
	command.setJIMMAPI(s.client)
	command.SetClientStore(s.store)

	initCommand(c, command,
		"aws",
		"controller-name",
		"controller-version",
	)

	ctx := newTestContext(c)
	mcmd := modelcmd.WrapBase(command)
	err := mcmd.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestBootstrapFailsToGetCredential(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	s.store.EXPECT().CredentialForCloud("aws").Return(nil, errors.New("credential not found"))

	command := &bootstrapCommand{}
	command.setJIMMAPI(s.client)
	command.SetClientStore(s.store)

	initCommand(c, command,
		"aws/region",
		"controller-name",
		"controller-version",
	)

	ctx := newTestContext(c)
	mcmd := modelcmd.WrapBase(command)
	err := mcmd.Run(ctx)
	c.Assert(err, qt.ErrorMatches, `failed to get credential for cloud "aws": credential not found`)
}

func TestBootstrapMultipleCredentials(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	s.store.EXPECT().CredentialForCloud("aws").Return(&jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"cred-1": {Label: "cred-1"},
			"cred-2": {Label: "cred-2"},
		},
	}, nil).Times(2)

	command := &bootstrapCommand{}
	command.setJIMMAPI(s.client)
	command.SetClientStore(s.store)

	initCommand(c, command,
		"aws/region",
		"controller-name",
		"controller-version",
	)

	ctx := newTestContext(c)
	mcmd := modelcmd.WrapBase(command)
	err := mcmd.Run(ctx)
	c.Assert(err, qt.ErrorMatches, `multiple credentials found for cloud "aws", please set a default or specify one using --credential`)

	// Now specify a credential and verify the command works.
	initCommand(c, command,
		"aws/region",
		"controller-name",
		"controller-version",
		"--credential", "cred-2",
	)

	s.client.EXPECT().StartBootstrap(gomock.Any()).Return(&params.StartBootstrapResponse{
		JobID: "test-job-id",
	}, nil)
	s.client.EXPECT().Close().Return(nil)

	s.client.EXPECT().BootstrapInfo(gomock.Any()).Return(params.GetBootstrapInfoResponse{
		Status:    params.StatusSuccessful,
		Logs:      []string{"log-line", "log-line"},
		Watermark: 2,
	}, nil)

	err = mcmd.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestBootstrapWithDefaultCredential(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	cloudName := "aws"

	s.store.EXPECT().CredentialForCloud(cloudName).Return(&jujucloud.CloudCredential{
		DefaultCredential: "cred-1",
		AuthCredentials: map[string]jujucloud.Credential{
			"cred-1": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{}),
			"cred-2": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{}),
		},
	}, nil)

	s.client.EXPECT().StartBootstrap(gomock.Any()).Return(&params.StartBootstrapResponse{JobID: "test-job-id"}, nil)
	s.client.EXPECT().Close().Return(nil)

	command := &bootstrapCommand{}
	command.setJIMMAPI(s.client)
	command.SetClientStore(s.store)

	initCommand(c, command,
		fmt.Sprintf("%s/region", cloudName),
		"controller-name",
		"controller-version",
		"--detach",
	)

	ctx := newTestContext(c)
	mcmd := modelcmd.WrapBase(command)
	err := mcmd.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestBootstrapSpecifiedCredentialWithDefault(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	cloudName := "aws"

	s.store.EXPECT().CredentialForCloud(cloudName).Return(&jujucloud.CloudCredential{
		DefaultCredential: "cred-1",
		AuthCredentials: map[string]jujucloud.Credential{
			"cred-1": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{}),
			"cred-2": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{}),
		},
	}, nil)

	command := &bootstrapCommand{}
	command.setJIMMAPI(s.client)
	command.SetClientStore(s.store)

	initCommand(c, command,
		fmt.Sprintf("%s/region", cloudName),
		"controller-name",
		"controller-version",
		"--detach",
		"--credential", "cred-3",
	)

	ctx := newTestContext(c)
	mcmd := modelcmd.WrapBase(command)
	err := mcmd.Run(ctx)
	c.Assert(err, qt.ErrorMatches, `no credential found with name "cred-3"`)
}
