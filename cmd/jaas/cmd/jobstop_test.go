// Copyright 2025 Canonical.

package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	"go.uber.org/mock/gomock"
)

func TestJobStop(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(t)

	jobId := "test-job-id"
	s.client.EXPECT().StopJob(gomock.Any()).Return(nil)

	command := &jobStopCommand{
		jobAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}

	initCommand(c, command, jobId)

	ctx := newTestContext(t)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	res := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(res, qt.Equals, fmt.Sprintf("Job %s has been stopped.\n", jobId))
}

func TestJobStopError(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(t)

	jobId := "test-job-id"
	s.client.EXPECT().StopJob(gomock.Any()).Return(errors.New("an error"))

	command := &jobStopCommand{
		jobAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
		jobId: jobId,
	}

	initCommand(c, command, jobId)

	ctx := newTestContext(t)
	err := command.Run(ctx)
	c.Assert(err, qt.ErrorMatches, "failed to stop job: an error")
}
