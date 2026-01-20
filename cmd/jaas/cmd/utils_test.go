package cmd

import (
	"bytes"
	"io"
	"testing"

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

func setupCmdMocks(t *testing.T) *cmdMocks {
	t.Helper()
	ctrl := gomock.NewController(t)
	h := &cmdMocks{
		client: mocks.NewMockJIMMAPI(ctrl),
		store:  mocks.NewMockClientStore(ctrl),
	}
	t.Cleanup(ctrl.Finish)
	return h
}

func newTestContext(t *testing.T) *jujucmd.Context {
	return &jujucmd.Context{
		Context: t.Context(),
		Dir:     t.TempDir(),
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
