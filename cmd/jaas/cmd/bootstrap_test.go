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
			name: "bootstrap-option-flags",
			args: []string{
				"test-cloud/region",
				"controller-name",
				"controller-version",
				"--profile", "aws-prod",
				"--bootstrap-base", "ubuntu@24.04",
				"--bootstrap-constraints", "mem=8G",
				"--constraints", "arch=amd64",
				"--model-default", "logging-config=<root>=INFO",
				"--storage-pool", "name=controller-pool",
				"--storage-pool", "type=ebs",
				"--config", "audit-log-enabled=true",
				"--config", "image-stream=released",
			},
			checkFlags: func(c *qt.C, command *bootstrapCommand) {
				c.Check(command.cloud, qt.Equals, "test-cloud")
				c.Check(command.region, qt.Equals, "region")
				c.Check(command.profileName, qt.Equals, "aws-prod")
				c.Check(command.bootstrapBase, qt.Equals, "ubuntu@24.04")
				c.Check([]string(command.bootstrapCons), qt.DeepEquals, []string{"mem=8G"})
				c.Check([]string(command.constraints), qt.DeepEquals, []string{"arch=amd64"})
			},
		}, {
			name:     "invalid-bootstrap-base",
			args:     []string{"test-cloud/region", "controller-name", "controller-version", "--bootstrap-base", "not-a-base"},
			errMatch: `invalid bootstrap base "not-a-base": .*`,
		}, {
			name:     "too-few-args",
			args:     []string{"test-cloud/region"},
			errMatch: "expected at least 3 arguments, got 1",
		},
	}
	for i, test := range tests {
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
		c.Check(bsp.Cloud, qt.DeepEquals, params.BootstrapCloud{
			Name: cloudName,
			Region: params.BootstrapCloudRegion{
				Name: "region",
			},
		})
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
			Cloud: params.BootstrapCloud{
				Name:      cloudName,
				Type:      "openstack",
				AuthTypes: []string{string(jujucloud.UserPassAuthType)},
				Endpoint:  "some-endpoint",
				Region: params.BootstrapCloudRegion{
					Name: "region",
				},
			},
			Credential: jujuparams.CloudCredential{
				AuthType:   string(jujucloud.UserPassAuthType),
				Attributes: map[string]string{},
			},
			BootstrapOptions: params.BootstrapOptions{
				BootstrapBase: "ubuntu@24.04",
				BootstrapConstraints: map[string]string{
					"mem":       "8G",
					"cores":     "2",
					"root-disk": "10G",
				},
				ModelConstraints: map[string]string{
					"arch": "amd64",
				},
				ModelDefault: map[string]string{
					"logging-config": "<root>=INFO",
				},
				StoragePool: &params.BootstrapStoragePool{
					Name: "controller-pool",
					Type: "ebs",
					Attributes: map[string]string{
						"volume-type": "gp3",
					},
				},
				BootstrapConfig: map[string]string{
					"audit-log-enabled": "true",
					"bootstrap-timeout": "60",
					"image-stream":      "released",
					"string-option":     "value",
				},
			},
			ControllerVersion: "controller-version",
		}
		c.Check(bsp.ControllerName, qt.Equals, expected.ControllerName)
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
		"--bootstrap-base", "ubuntu@24.04",
		"--bootstrap-constraints", "mem=8G",
		"--bootstrap-constraints", "cores=2 root-disk=10G",
		"--constraints", "arch=amd64",
		"--model-default", "logging-config=<root>=INFO",
		"--storage-pool", "name=controller-pool",
		"--storage-pool", "type=ebs",
		"--storage-pool", "volume-type=gp3",
		"--config", "audit-log-enabled=true",
		"--config", "bootstrap-timeout=60",
		"--config", "image-stream=released",
		"--config", "string-option=value",
	)

	ctx := newTestContext(c)
	mcmd := modelcmd.WrapBase(command)
	err = mcmd.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestBootstrapProfileMergesWithExplicitFlags(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	cloudName := "aws"

	s.store.EXPECT().CredentialForCloud(cloudName).Return(&jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"cred-1": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{}),
		},
	}, nil)
	s.client.EXPECT().GetControllerProfile(&params.GetControllerProfileRequest{Name: "aws-prod"}).Return(
		params.GetControllerProfileResponse{ControllerProfile: params.ControllerProfile{
			Name:        "aws-prod",
			JujuVersion: "3.6",
			Cloud: params.BootstrapCloud{
				Name:      cloudName,
				Type:      "ec2",
				AuthTypes: []string{string(jujucloud.UserPassAuthType)},
				Region: params.BootstrapCloudRegion{
					Name: "eu-west-1",
				},
			},
			BootstrapOptions: params.BootstrapOptions{
				BootstrapBase: "ubuntu@22.04",
				BootstrapConstraints: map[string]string{
					"mem": "4G",
				},
				ModelDefault: map[string]string{
					"logging-config": "<root>=WARNING",
				},
				BootstrapConfig: map[string]string{
					"bootstrap-timeout":       "30",
					"controller-service-type": "loadbalancer",
				},
				ControllerConfig: map[string]string{
					"audit-log-enabled": "true",
				},
			},
		}},
		nil,
	)
	s.client.EXPECT().StartBootstrap(gomock.Any()).DoAndReturn(func(bsp *params.BootstrapParams) (*params.StartBootstrapResponse, error) {
		c.Check(bsp.Cloud, qt.DeepEquals, params.BootstrapCloud{
			Name:      cloudName,
			Type:      "ec2",
			AuthTypes: []string{string(jujucloud.UserPassAuthType)},
			Region: params.BootstrapCloudRegion{
				Name: "region",
			},
		})
		c.Check(bsp.BootstrapOptions, qt.DeepEquals, params.BootstrapOptions{
			BootstrapBase: "ubuntu@24.04",
			BootstrapConstraints: map[string]string{
				"mem":   "4G",
				"cores": "2",
			},
			ModelDefault: map[string]string{
				"logging-config": "<root>=INFO",
			},
			BootstrapConfig: map[string]string{
				"bootstrap-timeout":       "30",
				"controller-service-type": "loadbalancer",
				"image-stream":            "released",
			},
			ControllerConfig: map[string]string{
				"audit-log-enabled": "true",
			},
		})
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
		"--profile", "aws-prod",
		"--bootstrap-base", "ubuntu@24.04",
		"--bootstrap-constraints", "cores=2",
		"--model-default", "logging-config=<root>=INFO",
		"--config", "image-stream=released",
	)

	ctx := newTestContext(c)
	mcmd := modelcmd.WrapBase(command)
	err := mcmd.Run(ctx)
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

func TestSplitEscapedFields(t *testing.T) {
	c := qt.New(t)
	tests := []struct {
		name     string
		value    string
		expected []string
	}{
		{
			name:     "empty",
			value:    "",
			expected: nil,
		},
		{
			name:     "plain fields",
			value:    "mem=8G arch=amd64",
			expected: []string{"mem=8G", "arch=amd64"},
		},
		{
			name:     "escaped spaces",
			value:    `tags=prod\ blue zones=us-east-1a`,
			expected: []string{"tags=prod blue", "zones=us-east-1a"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c.Check(splitEscapedFields(test.value), qt.DeepEquals, test.expected)
		})
	}
}

func TestStringifyScalar(t *testing.T) {
	c := qt.New(t)
	tests := []struct {
		name     string
		value    any
		expected string
		errMatch string
	}{
		{
			name:     "string",
			value:    "value",
			expected: "value",
		},
		{
			name:     "bool",
			value:    true,
			expected: "true",
		},
		{
			name:     "integer",
			value:    42,
			expected: "42",
		},
		{
			name:     "float",
			value:    3.5,
			expected: "3.5",
		},
		{
			name:     "nil",
			value:    nil,
			errMatch: "nil value",
		},
		{
			name:     "non scalar",
			value:    []string{"nope"},
			errMatch: `unsupported type \[\]string`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := stringifyScalar(test.value)
			if test.errMatch != "" {
				c.Assert(err, qt.ErrorMatches, test.errMatch)
				return
			}
			c.Assert(err, qt.IsNil)
			c.Check(value, qt.Equals, test.expected)
		})
	}
}
