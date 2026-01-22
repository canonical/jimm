// Copyright 2025 Canonical.

package cmd

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/gnuflag"
	"go.uber.org/mock/gomock"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func TestImportModelInit_ArgValidation(t *testing.T) {
	c := qt.New(t)

	validModelUUID := "ac30d6ae-0bed-4398-bba7-75d49e39f189"

	cases := []struct {
		about    string
		args     []string
		errMatch string
	}{
		{about: "0 args", args: nil, errMatch: "controller not specified"},
		{about: "1 arg", args: []string{"controller-1"}, errMatch: "model uuid not specified"},
		{about: "too many args", args: []string{"controller-1", validModelUUID, "extra"}, errMatch: "too many args"},
		{about: "invalid uuid", args: []string{"controller-1", "not-a-uuid"}, errMatch: "invalid model uuid"},
		{about: "valid uuid", args: []string{"controller-1", validModelUUID}, errMatch: ""},
	}

	for _, tc := range cases {
		c.Run(tc.about, func(c *qt.C) {
			cmd := &importModelCommand{}
			err := cmd.Init(tc.args)
			if tc.errMatch != "" {
				c.Assert(err, qt.ErrorMatches, tc.errMatch)
				return
			}
			c.Assert(err, qt.IsNil)
			c.Assert(cmd.req.Controller, qt.Equals, "controller-1")
			c.Assert(cmd.req.ModelTag, qt.Equals, "model-"+validModelUUID)
		})
	}
}

func TestImportModelRun_PassesRequestToAPI(t *testing.T) {
	c := qt.New(t)

	cmdMocks := setupCmdMocks(c)

	modelUUID := "ac30d6ae-0bed-4398-bba7-75d49e39f189"

	cmdMocks.client.EXPECT().
		ImportModel(gomock.Any()).
		DoAndReturn(func(req *apiparams.ImportModelRequest) error {
			c.Assert(req.Controller, qt.Equals, "controller-1")
			c.Assert(req.ModelTag, qt.Equals, "model-"+modelUUID)
			c.Assert(req.Owner, qt.Equals, "alice@canonical.com")
			return nil
		}).
		Times(1)

	cmdMocks.client.EXPECT().Close().Times(1)

	command := &importModelCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}

	err := cmdtesting.InitCommand(command, []string{"controller-1", modelUUID, "--owner", "alice@canonical.com"})
	c.Assert(err, qt.IsNil)

	err = command.Run(newTestContext(c))
	c.Assert(err, qt.IsNil)
}

func TestImportModelRun_WithoutOwnerFlag(t *testing.T) {
	c := qt.New(t)

	cmdMocks := setupCmdMocks(c)

	modelUUID := "ac30d6ae-0bed-4398-bba7-75d49e39f189"

	cmdMocks.client.EXPECT().
		ImportModel(gomock.Any()).
		DoAndReturn(func(req *apiparams.ImportModelRequest) error {
			c.Assert(req.Controller, qt.Equals, "controller-1")
			c.Assert(req.ModelTag, qt.Equals, "model-"+modelUUID)
			c.Assert(req.Owner, qt.Equals, "")
			return nil
		}).
		Times(1)

	cmdMocks.client.EXPECT().Close().Times(1)

	command := &importModelCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}

	fs := gnuflag.NewFlagSet("test", gnuflag.ContinueOnError)
	command.SetFlags(fs)

	err := cmdtesting.InitCommand(command, []string{"controller-1", modelUUID})
	c.Assert(err, qt.IsNil)

	err = command.Run(newTestContext(c))
	c.Assert(err, qt.IsNil)
}
