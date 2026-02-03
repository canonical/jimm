package cmd

import (
	"bytes"
	"io"

	qt "github.com/frankban/quicktest"
	jujucmd "github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd/mocks"
)

// This file contains common test helpers for command tests.
// They closely mirror those in github.com/juju/cmd/v3/cmdtesting
// but have the benefit of avoiding gocheck.

type cmdMocks struct {
	client *mocks.MockJIMMAPI
	store  *mocks.MockClientStore
}

func setupCmdMocks(c *qt.C) *cmdMocks {
	c.Helper()
	ctrl := gomock.NewController(c)
	h := &cmdMocks{
		client: mocks.NewMockJIMMAPI(ctrl),
		store:  mocks.NewMockClientStore(ctrl),
	}
	c.Cleanup(ctrl.Finish)
	return h
}

func newTestContext(c *qt.C) *jujucmd.Context {
	return &jujucmd.Context{
		Context: c.Context(),
		Dir:     c.TempDir(),
		Stdin:   &bytes.Buffer{},
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	}
}

// initCommand initializes the command with the given args.
// This is important for commands that expect default values
// to be set during init e.g. for formatters, etc.
func initCommand(c *qt.C, command jujucmd.Command, args ...string) {
	f := gnuflag.NewFlagSetWithFlagKnownAs(command.Info().Name, gnuflag.ContinueOnError, jujucmd.FlagAlias(command, "flag"))
	f.SetOutput(io.Discard)
	command.SetFlags(f)
	err := f.Parse(command.AllowInterspersedFlags(), args)
	c.Assert(err, qt.IsNil)
	err = command.Init(f.Args())
	c.Assert(err, qt.IsNil)
}

// initCommandWithError initializes the command with the given args
// and returns any error encountered.
func initCommandWithError(command jujucmd.Command, args ...string) error {
	f := gnuflag.NewFlagSetWithFlagKnownAs(command.Info().Name, gnuflag.ContinueOnError, jujucmd.FlagAlias(command, "flag"))
	f.SetOutput(io.Discard)
	command.SetFlags(f)
	err := f.Parse(command.AllowInterspersedFlags(), args)
	if err != nil {
		return err
	}
	return command.Init(f.Args())
}
