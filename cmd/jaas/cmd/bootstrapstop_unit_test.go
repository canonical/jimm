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

type bootstrapStopSuite struct {
	client *mocks.MockJIMMAPI
	writer *mocks.MockWriter
}

var _ = gc.Suite(&bootstrapStopSuite{})

func (s *bootstrapStopSuite) SetupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.client = mocks.NewMockJIMMAPI(ctrl)
	s.writer = mocks.NewMockWriter(ctrl)

	return ctrl
}

func (s *bootstrapStopSuite) TestBootstrapStop(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()
	jobId := "test-job-id"
	s.client.EXPECT().BootstrapStop(gomock.Any()).Return(nil)
	s.writer.EXPECT().Write(fmt.Appendf(nil, "Bootstrap job %s has been stopped.\n", jobId))

	command := &bootstrapStopCommand{
		bootstrapAPIFunc: func() (JIMMAPI, error) {
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

func (s *bootstrapStopSuite) TestBootstrapStopError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()
	jobId := "test-job-id"
	s.client.EXPECT().BootstrapStop(gomock.Any()).Return(errors.New("an error"))

	command := &bootstrapStopCommand{
		bootstrapAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
		jobId: jobId,
	}
	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}
	err := command.Run(ctx)
	c.Assert(err, gc.ErrorMatches, "failed to stop bootstrap job: an error")
}
