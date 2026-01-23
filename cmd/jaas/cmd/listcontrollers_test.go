// Copyright 2025 Canonical.

package cmd

import (
	"bytes"
	"testing"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/rpc/params"
	"gopkg.in/yaml.v2"

	"github.com/canonical/jimm/v3/pkg/api/params"
)

func TestListControllersSuperuser(t *testing.T) {
	c := qt.New(t)
	cmdMocks := setupCmdMocks(t)

	expectedControllers := []params.ControllerInfo{
		{
			Name:          "controller-1",
			UUID:          "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			PublicAddress: "",
			APIAddresses:  []string{"localhost:17070"},
			CACertificate: "-----BEGIN CERTIFICATE-----\ntest-cert\n-----END CERTIFICATE-----",
			CloudTag:      "cloud-test-cloud",
			CloudRegion:   "test-region",
			AgentVersion:  "3.0.0",
			Status: jujuparams.EntityStatus{
				Status: "available",
				Info:   "",
				Data:   map[string]interface{}{},
			},
		},
		{
			Name:          "controller-2",
			UUID:          "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			PublicAddress: "",
			APIAddresses:  []string{"localhost:46539"},
			CACertificate: "-----BEGIN CERTIFICATE-----\ntest-cert-2\n-----END CERTIFICATE-----",
			CloudTag:      "cloud-test-cloud",
			CloudRegion:   "test-region",
			AgentVersion:  "3.0.0",
			Status: jujuparams.EntityStatus{
				Status: "available",
				Info:   "",
				Data:   map[string]interface{}{},
			},
		},
	}

	cmdMocks.client.EXPECT().ListControllers().Return(expectedControllers, nil)
	cmdMocks.client.EXPECT().Close().Return(nil)

	command := &listControllersCommand{
		store: cmdMocks.store,
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}

	initCommand(c, command)

	ctx := newTestContext(t)

	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	output := ctx.Stdout.(*bytes.Buffer).String()
	var actual []params.ControllerInfo
	err = yaml.Unmarshal([]byte(output), &actual)
	c.Assert(err, qt.IsNil)
	c.Assert(actual, qt.DeepEquals, expectedControllers)
}

func TestListControllersEmpty(t *testing.T) {
	c := qt.New(t)
	cmdMocks := setupCmdMocks(t)

	cmdMocks.client.EXPECT().ListControllers().Return([]params.ControllerInfo{}, nil)
	cmdMocks.client.EXPECT().Close().Return(nil)

	command := &listControllersCommand{
		store: cmdMocks.store,
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}

	initCommand(c, command)

	ctx := newTestContext(t)

	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	output := ctx.Stdout.(*bytes.Buffer).String()
	var actual []params.ControllerInfo
	err = yaml.Unmarshal([]byte(output), &actual)
	c.Assert(err, qt.IsNil)
	c.Assert(actual, qt.DeepEquals, []params.ControllerInfo{})
}
