// Copyright 2025 Canonical.

package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/juju/cmd/v3"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd/mocks"
)

type jobStopSuite struct {
	client *mocks.MockJIMMAPI
	writer *mocks.MockWriter
}

var _ = gc.Suite(&jobStopSuite{})

func (s *jobStopSuite) SetupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.client = mocks.NewMockJIMMAPI(ctrl)
	s.writer = mocks.NewMockWriter(ctrl)

	return ctrl
}

func (s *jobStopSuite) TestJobStop(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()
	jobId := "test-job-id"
	s.client.EXPECT().StopJob(gomock.Any()).Return(nil)
	s.writer.EXPECT().Write(fmt.Appendf(nil, "Job %s has been stopped.\n", jobId))

	command := &jobStopCommand{
		jobAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
		jobId: jobId,
	}
	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}
	err := command.Run(ctx)
	c.Assert(err, gc.IsNil)
}

func (s *jobStopSuite) TestJobStopError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()
	jobId := "test-job-id"
	s.client.EXPECT().StopJob(gomock.Any()).Return(errors.New("an error"))

	command := &jobStopCommand{
		jobAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
		jobId: jobId,
	}
	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}
	err := command.Run(ctx)
	c.Assert(err, gc.ErrorMatches, "failed to stop job: an error")
}
