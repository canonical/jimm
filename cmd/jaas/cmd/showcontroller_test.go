// Copyright 2026 Canonical.

package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/cmd/v3/cmdtesting"
	jujuparams "github.com/juju/juju/rpc/params"
	"gopkg.in/yaml.v2"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func runShowControllerCommand(c *qt.C, mocks *cmdMocks, args ...string) (string, error) {
	showCmd := showControllerCommand{}
	showCmd.SetClientStore(mocks.store)
	showCmd.setJIMMAPI(mocks.client)

	ctx := newTestContext(c)
	err := initCommandWithError(&showCmd, args...)
	if err != nil {
		return "", err
	}

	err = showCmd.Run(ctx)
	if err != nil {
		return "", err
	}

	return cmdtesting.Stdout(ctx), nil
}

func TestShowControllerOutput(t *testing.T) {
	c := qt.New(t)

	attemptedAt := time.Date(2026, time.May, 29, 10, 45, 0, 0, time.UTC)
	errAt := attemptedAt.Add(-2 * time.Minute)
	controllerInfo := &apiparams.ControllerInfo{
		Name:          "test-controller",
		UUID:          "87654321-4321-4321-4321-cba987654321",
		PublicAddress: "test-controller.example.com:443",
		APIAddresses:  []string{"10.0.0.1:17070", "10.0.0.2:17070"},
		CACertificate: "-----BEGIN CERTIFICATE-----\ntest-cert\n-----END CERTIFICATE-----",
		CloudTag:      "cloud-test-cloud",
		CloudRegion:   "test-region",
		AgentVersion:  "3.6.8",
		Status: jujuparams.EntityStatus{
			Status: "available",
			Info:   "controller is healthy",
		},
		BootstrapJobStatus: &apiparams.BootstrapJobStatus{
			Bootstrap: apiparams.JobDetail{
				State:       "running",
				Attempt:     2,
				MaxAttempts: 5,
				AttemptedAt: &attemptedAt,
				Errors: []apiparams.JobAttemptError{{
					Attempt: 1,
					At:      errAt,
					Error:   "temporary network issue",
				}},
			},
		},
	}

	mocks := setupCmdMocks(c)
	mocks.client.EXPECT().Close().AnyTimes()
	mocks.client.EXPECT().ShowController("test-controller").Return(controllerInfo, nil).AnyTimes()

	expectedJSON := *controllerInfo
	expectedJSON.Status.Data = nil
	expectedYAML := *controllerInfo
	expectedYAML.Status.Data = map[string]any{}

	tests := []struct {
		name   string
		args   []string
		want   apiparams.ControllerInfo
		decode func(string) (apiparams.ControllerInfo, error)
	}{
		{
			name: "json",
			args: []string{"test-controller", "--format", "json"},
			want: expectedJSON,
			decode: func(output string) (apiparams.ControllerInfo, error) {
				var actual apiparams.ControllerInfo
				err := json.Unmarshal([]byte(strings.TrimSpace(output)), &actual)
				return actual, err
			},
		},
		{
			name: "yaml",
			args: []string{"test-controller", "--format", "yaml"},
			want: expectedYAML,
			decode: func(output string) (apiparams.ControllerInfo, error) {
				var actual apiparams.ControllerInfo
				err := yaml.Unmarshal([]byte(output), &actual)
				return actual, err
			},
		},
	}

	for _, test := range tests {
		c.Run(test.name, func(c *qt.C) {
			output, err := runShowControllerCommand(c, mocks, test.args...)
			c.Assert(err, qt.IsNil)

			actual, err := test.decode(output)
			c.Assert(err, qt.IsNil)
			c.Assert(actual, qt.DeepEquals, test.want)
		})
	}
}

func TestShowControllerError(t *testing.T) {
	c := qt.New(t)

	mocks := setupCmdMocks(c)
	mocks.client.EXPECT().Close().AnyTimes()
	mocks.client.EXPECT().ShowController("test-controller").Return(nil, errors.New("not found")).AnyTimes()

	_, err := runShowControllerCommand(c, mocks, "test-controller")
	c.Assert(err, qt.ErrorMatches, "not found")
}

func TestShowControllerArgsError(t *testing.T) {
	c := qt.New(t)

	mocks := setupCmdMocks(c)

	_, err := runShowControllerCommand(c, mocks)
	c.Assert(err, qt.ErrorMatches, "missing controller name")

	_, err = runShowControllerCommand(c, mocks, "test-controller", "extra-arg")
	c.Assert(err, qt.ErrorMatches, `unknown arguments: \[extra-arg\]`)
}
