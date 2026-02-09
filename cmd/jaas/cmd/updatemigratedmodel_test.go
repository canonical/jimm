// Copyright 2025 Canonical.

package cmd

import (
	"testing"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
	qt "github.com/frankban/quicktest"
)

func TestUpdateMigratedModel(t *testing.T) {
	c := qt.New(t)
	cmdMocks := setupCmdMocks(c)

	cmdMocks.client.EXPECT().UpdateMigratedModel(&apiparams.UpdateMigratedModelRequest{
		ModelTag:         "model-2f54eaf8-0608-42e7-9f69-d85d6e1369b0",
		TargetController: "mycontroller",
	}).Return(nil)
	cmdMocks.client.EXPECT().Close().Return(nil)

	command := &updateMigratedModelCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}
	command.SetClientStore(cmdMocks.store)

	initCommand(c, command, "mycontroller", "2f54eaf8-0608-42e7-9f69-d85d6e1369b0")

	ctx := newTestContext(c)

	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestUpdateMigratedModelNoController(t *testing.T) {
	c := qt.New(t)

	command := &updateMigratedModelCommand{}

	err := initCommandWithError(command)
	c.Assert(err, qt.ErrorMatches, `controller not specified`)
}

func TestUpdateMigratedModelNoModelUUID(t *testing.T) {
	c := qt.New(t)

	command := &updateMigratedModelCommand{}

	err := initCommandWithError(command, "mycontroller")
	c.Assert(err, qt.ErrorMatches, `model uuid not specified`)
}

func TestUpdateMigratedModelInvalidModelUUID(t *testing.T) {
	c := qt.New(t)

	command := &updateMigratedModelCommand{}

	err := initCommandWithError(command, "mycontroller", "not-a-uuid")
	c.Assert(err, qt.ErrorMatches, `invalid model uuid`)
}

func TestUpdateMigratedModelTooManyArgs(t *testing.T) {
	c := qt.New(t)

	command := &updateMigratedModelCommand{}

	err := initCommandWithError(command, "controller-id", "not-a-uuid", "spare-argument")
	c.Assert(err, qt.ErrorMatches, `too many args`)
}
