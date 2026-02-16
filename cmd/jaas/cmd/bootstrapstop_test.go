// Copyright 2026 Canonical.

package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	"go.uber.org/mock/gomock"
)

func TestBootstrapStop(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	jobId := "test-job-id"
	s.client.EXPECT().StopBootstrap(gomock.Any()).Return(nil)

	command := &bootstrapStopCommand{}
	command.setJIMMAPI(s.client)

	initCommand(c, command, jobId)

	ctx := newTestContext(c)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	res := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(res, qt.Equals, fmt.Sprintf("Bootstrap job with ID %q has been stopped.\n", jobId))
}

func TestBootstrapStopError(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	jobId := "test-job-id"
	s.client.EXPECT().StopBootstrap(gomock.Any()).Return(errors.New("an error"))

	command := &bootstrapStopCommand{
		jobId: jobId,
	}
	command.setJIMMAPI(s.client)

	initCommand(c, command, jobId)

	ctx := newTestContext(c)
	err := command.Run(ctx)
	c.Assert(err, qt.ErrorMatches, "failed to stop bootstrap: an error")
}
