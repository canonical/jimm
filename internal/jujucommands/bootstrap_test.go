// Copyright 2025 Canonical.

package jujucommands_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	qt "github.com/frankban/quicktest"
	jujucloud "github.com/juju/juju/cloud"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/jujucommands"
	"github.com/canonical/jimm/v3/internal/jujucommands/mocks"
)

func (s *jujucommandsSuite) TestBootstrapCmdParams_Validate(c *qt.C) {
	p := jujucommands.BootstrapCmdParams{}
	c.Assert(p.Validate(), qt.ErrorMatches, ".*cloud \\[and region\\] name cannot be empty.*")

	p.CloudNameAndRegion = "testregion/testcloud"
	c.Assert(p.Validate(), qt.ErrorMatches, ".*controller name name cannot be empty.*")

	p.ControllerName = "my-controller"
	c.Assert(p.Validate(), qt.ErrorMatches, "missing login token refresh URL, this value should be automatically set by JIMM")

	p.DefaultLoginTokenURL = "myurl.com"
	c.Assert(p.Validate(), qt.IsNil)

	p.AgentVersion = "bad version"
	c.Assert(p.Validate(), qt.ErrorMatches, "invalid version \"bad version\"")

	p.AgentVersion = "1.1.1"
	c.Assert(p.Validate(), qt.IsNil)

	p.BootstrapOptions.StoragePool = &jujucommands.StoragePool{Name: "pool-only-name"}
	c.Assert(p.Validate(), qt.ErrorMatches, "storage pool requires both name and type")
}

func (s *jujucommandsSuite) TestBootstrapCmdParams_LoginTokenRefreshURLOverrideFromBootstrapConfig(c *qt.C) {
	p := jujucommands.BootstrapCmdParams{
		CloudNameAndRegion:   "testregion/testcloud",
		ControllerName:       "my-controller",
		DefaultLoginTokenURL: "https://default.example/.well-known/jwks.json",
		BootstrapOptions: jujucommands.BootstrapOptions{
			BootstrapConfig: map[string]string{
				"bootstrap-timeout":       "1000",
				"login-token-refresh-url": "https://override.example/.well-known/jwks.json",
			},
		},
	}

	args := p.BuildBootstrapCmdArgs()
	c.Assert(args, qt.DeepEquals, []string{
		"bootstrap",
		"--bootstrap-constraints",
		fmt.Sprintf("arch=%s", runtime.GOARCH),
		"--config",
		"login-token-refresh-url=https://override.example/.well-known/jwks.json",
		"--config",
		"bootstrap-timeout=1000",
		"testregion/testcloud",
		"my-controller",
	})
}

func (s *jujucommandsSuite) TestBootstrapCmdParams_BuildBootstrapCmdArgs(c *qt.C) {
	tests := []struct {
		name   string
		params jujucommands.BootstrapCmdParams
		expect []string
	}{
		{
			name: "all fields set",
			params: jujucommands.BootstrapCmdParams{
				CloudNameAndRegion:   "testregion/testcloud",
				ControllerName:       "my-controller",
				AgentVersion:         "1.1.1",
				DefaultLoginTokenURL: "myurl.com",
				BootstrapOptions: jujucommands.BootstrapOptions{
					BootstrapBase:        "ubuntu@24.04",
					BootstrapConstraints: map[string]string{"mem": "8G"},
					ModelConstraints:     map[string]string{"arch": "amd64"},
					ModelDefault:         map[string]string{"logging-config": "<root>=INFO"},
					StoragePool: &jujucommands.StoragePool{
						Name:       "controller-pool",
						Type:       "ebs",
						Attributes: map[string]string{"volume-type": "gp3"},
					},
					BootstrapConfig:       map[string]string{"bootstrap-timeout": "1000"},
					ControllerConfig:      map[string]string{"audit-log-enabled": "true"},
					ControllerModelConfig: map[string]string{"image-stream": "released"},
				},
			},
			expect: []string{
				"bootstrap",
				"--agent-version=1.1.1",
				"--bootstrap-base=ubuntu@24.04",
				"--bootstrap-constraints",
				fmt.Sprintf("arch=%s", runtime.GOARCH),
				"--bootstrap-constraints",
				"mem=8G",
				"--constraints",
				"arch=amd64",
				"--model-default",
				"logging-config=<root>=INFO",
				"--storage-pool",
				"name=controller-pool",
				"--storage-pool",
				"type=ebs",
				"--storage-pool",
				"volume-type=gp3",
				"--config",
				"login-token-refresh-url=myurl.com",
				"--config",
				"audit-log-enabled=true",
				"--config",
				"bootstrap-timeout=1000",
				"--config",
				"image-stream=released",
				"testregion/testcloud",
				"my-controller",
			},
		},
		{
			name: "no agent version",
			params: jujucommands.BootstrapCmdParams{
				CloudNameAndRegion:   "testregion/testcloud",
				ControllerName:       "my-controller",
				AgentVersion:         "",
				DefaultLoginTokenURL: "myurl.com",
				BootstrapOptions: jujucommands.BootstrapOptions{
					BootstrapConfig: map[string]string{
						"bootstrap-timeout": "1000",
					},
				},
			},
			expect: []string{
				"bootstrap",
				"--bootstrap-constraints",
				fmt.Sprintf("arch=%s", runtime.GOARCH),
				"--config",
				"login-token-refresh-url=myurl.com",
				"--config",
				"bootstrap-timeout=1000",
				"testregion/testcloud",
				"my-controller",
			},
		},
		{
			name: "no agent version and no bootstrap timeout",
			params: jujucommands.BootstrapCmdParams{
				CloudNameAndRegion:   "testregion/testcloud",
				ControllerName:       "my-controller",
				AgentVersion:         "",
				DefaultLoginTokenURL: "myurl.com",
			},
			expect: []string{
				"bootstrap",
				"--bootstrap-constraints",
				fmt.Sprintf("arch=%s", runtime.GOARCH),
				"--config",
				"login-token-refresh-url=myurl.com",
				"testregion/testcloud",
				"my-controller",
			},
		},
	}

	for _, tt := range tests {
		c.Run(tt.name, func(c *qt.C) {
			args := tt.params.BuildBootstrapCmdArgs()
			c.Assert(args, qt.DeepEquals, tt.expect)
		})
	}
}

func (s *jujucommandsSuite) TestBootstrapCmdParams_RunBootstrapCmd_PersonalCloudWritten(c *qt.C) {
	testCtx := c.Context()

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockRunner(ctrl)

	dir := c.TempDir()

	mockRunner.EXPECT().JujuDataDir().Return(dir).AnyTimes()
	mockRunner.EXPECT().RunJujuCmd(testCtx, gomock.Any()).DoAndReturn(func(ctx context.Context, args []string) (<-chan jujucommands.OutputLine, error) {
		outputCh := make(chan jujucommands.OutputLine, 1)
		close(outputCh)
		return outputCh, nil
	}).AnyTimes()

	cloudCred := jujucloud.NewNamedCredential("bootstrap-credential", jujucloud.CertificateAuthType, map[string]string{
		"server-cert": "server-cert",
		"client-cert": "client-cert",
		"client-key":  "client-key",
	}, false)

	p := jujucommands.BootstrapCmdParams{
		CloudNameAndRegion:   "testcloud/testregion",
		ControllerName:       "my-controller",
		AgentVersion:         "1.1.1",
		DefaultLoginTokenURL: "myurl.com",

		Cloud: jujucloud.Cloud{
			Type: "lxd",
			AuthTypes: jujucloud.AuthTypes{
				jujucloud.CertificateAuthType,
			},
			// Some fake addr.
			Endpoint: "https://127.0.0.1:8443",
			Regions: []jujucloud.Region{
				{
					Name: "default",
					// Some fake addr.
					Endpoint: "https://127.0.0.1:8443",
				},
			},
		},
		CloudCred: cloudCred,
	}

	cmd := jujucommands.NewBootstrapCmd(mockRunner)

	_, store, cleanup, err := cmd.Run(testCtx, p)
	c.Cleanup(func() {
		cleanup()
	})

	c.Assert(err, qt.IsNil)

	// Now we check the cred is added
	personalCloudCred, err := store.CredentialForCloud("testcloud")
	c.Assert(err, qt.IsNil)

	// Check just attributes, if the populated in memory credential is set for the "testcloud",
	// then we're sure the credential to be used is the one we provided for our provided cloud.
	// (Given it also exists in the temp directory)
	c.Assert(
		personalCloudCred.AuthCredentials["testcloud"].Attributes(),
		qt.DeepEquals,
		cloudCred.Attributes(),
	)

	// This was set to a temp dir until cleanup runs. We can use it to check the file exists.
	_, err = os.Stat(filepath.Join(dir, "clouds.yaml"))
	c.Assert(err, qt.IsNil)
}

func (s *jujucommandsSuite) TestBootstrapCmdParams_RunBootstrapCmd_PublicCloudWritten(c *qt.C) {
	testCtx := c.Context()

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	mockRunner := mocks.NewMockRunner(ctrl)
	dir := c.TempDir()
	mockRunner.EXPECT().JujuDataDir().Return(dir).AnyTimes()

	callCounter := 0
	mockRunner.EXPECT().RunJujuCmd(testCtx, gomock.Any()).DoAndReturn(func(ctx context.Context, args []string) (<-chan jujucommands.OutputLine, error) {
		callCounter++
		if callCounter == 1 {
			// This is only called once within bootstrap, so we can be pretty sure the public-clouds.yaml was written.
			c.Assert(args, qt.DeepEquals, []string{"update-public-clouds", "--client"})
		}

		outputCh := make(chan jujucommands.OutputLine, 1)
		close(outputCh)
		return outputCh, nil
	}).AnyTimes()

	cloudCred := jujucloud.NewNamedCredential("bootstrap-credential", jujucloud.AccessKeyAuthType, map[string]string{
		"aws-secret": "my-secret",
	}, false)
	p := jujucommands.BootstrapCmdParams{
		CloudNameAndRegion:   "aws/us-east-1",
		ControllerName:       "my-controller",
		AgentVersion:         "1.1.1",
		DefaultLoginTokenURL: "myurl.com",

		CloudCred: cloudCred,
	}

	cmd := jujucommands.NewBootstrapCmd(mockRunner)

	_, store, cleanup, err := cmd.Run(
		testCtx,
		p,
	)
	c.Cleanup(func() {
		cleanup()
	})

	c.Assert(err, qt.IsNil)

	// Now we check the cred is added for the cloud (excluding region)
	personalCloudCred, err := store.CredentialForCloud("aws")
	c.Assert(err, qt.IsNil)
	c.Assert(
		personalCloudCred.AuthCredentials["aws"].Attributes(),
		qt.DeepEquals,
		cloudCred.Attributes(),
	)

	// Make sure no personal cloud was written.
	_, err = os.Stat(filepath.Join(dir, "clouds.yaml"))
	c.Assert(err, qt.ErrorMatches, ".*no such file or directory.*")
}
