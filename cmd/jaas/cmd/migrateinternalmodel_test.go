// Copyright 2025 Canonical.

package cmd

import (
	"bytes"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/gnuflag"
	jujuparams "github.com/juju/juju/rpc/params"
	"go.uber.org/mock/gomock"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func TestMigrateInternalModelCommand_BuildsRequestAndWritesOutput(t *testing.T) {
	c := qt.New(t)

	cmdMocks := setupCmdMocks(t)

	cmdMocks.client.EXPECT().
		MigrateModel(gomock.Any()).
		DoAndReturn(func(req *apiparams.MigrateModelRequest) (*jujuparams.InitiateMigrationResults, error) {
			c.Assert(req, qt.Not(qt.IsNil))
			c.Assert(req.Specs, qt.HasLen, 2)
			c.Assert(req.Specs[0].TargetController, qt.Equals, "controller-1")
			c.Assert(req.Specs[0].TargetModelNameOrUUID, qt.Equals, "model-uuid-1")
			c.Assert(req.Specs[1].TargetController, qt.Equals, "controller-1")
			c.Assert(req.Specs[1].TargetModelNameOrUUID, qt.Equals, "owner/model-2")

			// Return an empty successful result set.
			return &jujuparams.InitiateMigrationResults{Results: []jujuparams.InitiateMigrationResult{}}, nil
		}).
		Times(1)
	cmdMocks.client.EXPECT().Close().Times(1)

	command := &migrateInternalModelCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}

	fs := gnuflag.NewFlagSet("test", gnuflag.ContinueOnError)
	command.SetFlags(fs)

	err := cmdtesting.InitCommand(command, []string{"controller-1", "model-uuid-1", "owner/model-2"})
	c.Assert(err, qt.IsNil)

	ctx := newTestContext(t)
	err = command.Run(ctx)
	c.Assert(err, qt.IsNil)

	out := ctx.Stdout.(*bytes.Buffer).String()
	// YAML/JSON output should contain the root key from InitiateMigrationResults formatting.
	c.Assert(out, qt.Contains, "results")
}

func TestMigrateInternalModelCommandFailsWithMissingArgs(t *testing.T) {
	c := qt.New(t)

	var command migrateInternalModelCommand
	err := command.Init([]string{"myController"})
	c.Assert(err, qt.ErrorMatches, "missing controller name and model target arguments")
}
