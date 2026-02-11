// Copyright 2026 Canonical.

package jujucommands_test

import (
	"context"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/jujuclient"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/jujucommands"
	"github.com/canonical/jimm/v3/internal/jujucommands/mocks"
)

func (s *jujucommandsSuite) TestDestroyControllerCmdParams_Validate(c *qt.C) {
	p := jujucommands.DestroyControllerCmdParams{}
	c.Assert(p.Validate(), qt.ErrorMatches, ".*controller name cannot be empty.*")

	p.ControllerName = "foo"
	c.Assert(p.Validate(), qt.IsNil)
}

func (s *jujucommandsSuite) TestDestroyControllerCmdParams_Args(c *qt.C) {
	params := jujucommands.DestroyControllerCmdParams{
		ControllerName: "my-controller",
	}
	expect := []string{
		"destroy-controller",
		"my-controller",
		"--no-prompt",
	}

	args := params.BuildCmdArgs()
	c.Assert(args, qt.DeepEquals, expect)
}

func (s *jujucommandsSuite) TestDestroyControllerCmdParams_RunWithStore(c *qt.C) {
	testCtx := c.Context()

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	mockRunner := mocks.NewMockRunner(ctrl)
	dir := c.TempDir()
	mockRunner.EXPECT().JujuDataDir().Return(dir).AnyTimes()

	params := jujucommands.DestroyControllerCmdParams{
		ControllerName: "my-controller",
		ControllerDetails: jujuclient.ControllerDetails{
			ControllerUUID: "not-quite-a-uuid",
		},
		AccountDetails: jujuclient.AccountDetails{
			User: "my-user",
		},
	}

	mockRunner.EXPECT().RunJujuCmd(testCtx, gomock.Any()).DoAndReturn(func(ctx context.Context, args []string) (<-chan jujucommands.OutputLine, error) {
		outputCh := make(chan jujucommands.OutputLine, 1)
		close(outputCh)
		return outputCh, nil
	}).AnyTimes()

	cmd := jujucommands.NewDestroyControllerCmd(mockRunner)
	_, err := cmd.Run(testCtx, params)
	c.Assert(err, qt.IsNil)

	store := jujuclient.NewFileClientStore()

	ctrlDetails, err := store.ControllerByName(params.ControllerName)
	c.Assert(err, qt.IsNil)
	c.Assert(ctrlDetails.ControllerUUID, qt.Equals, "not-quite-a-uuid")

	acctDetails, err := store.AccountDetails(params.ControllerName)
	c.Assert(err, qt.IsNil)
	c.Assert(acctDetails.User, qt.Equals, "my-user")
}
