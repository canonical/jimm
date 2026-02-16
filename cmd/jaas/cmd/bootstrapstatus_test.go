// Copyright 2026 Canonical.

package cmd

import (
	"context"

	"github.com/juju/cmd/v3"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd/mocks"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

type bootstrapStatusSuite struct {
	client *mocks.MockJIMMAPI
	writer *mocks.MockWriter
}

var _ = gc.Suite(&bootstrapStatusSuite{})

func (s *bootstrapStatusSuite) SetupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.client = mocks.NewMockJIMMAPI(ctrl)
	s.writer = mocks.NewMockWriter(ctrl)

	return ctrl
}

func (s *bootstrapStatusSuite) TestBootstrapStatus(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.client.EXPECT().BootstrapInfo(gomock.Any()).Return(params.GetBootstrapInfoResponse{
		Status: params.StatusSuccessful,
	}, nil)
	s.client.EXPECT().Close().Return(nil)
	s.writer.EXPECT().Write([]byte("Job completed successfully.\n"))

	command := &bootstrapStatusCommand{
		jobId:               "test-job-id",
		sleepBetweenGetLogs: 0,
		follow:              true,
	}
	command.setJIMMAPI(s.client)
	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}
	err := command.Run(ctx)
	c.Assert(err, gc.IsNil)
}

func (s *bootstrapStatusSuite) TestBootstrapStatus_Failed(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.client.EXPECT().BootstrapInfo(gomock.Any()).Return(params.GetBootstrapInfoResponse{
		Status: params.StatusFailed,
		Error:  "Job failed",
	}, nil)
	s.client.EXPECT().Close().Return(nil)
	s.writer.EXPECT().Write([]byte("Job failed: Job failed\n"))

	command := &bootstrapStatusCommand{
		jobId:               "test-job-id",
		sleepBetweenGetLogs: 0,
		follow:              true,
	}
	command.setJIMMAPI(s.client)
	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}
	err := command.Run(ctx)
	c.Assert(err, gc.IsNil)
}

func (s *bootstrapStatusSuite) TestBootstrapStatus_Running(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.client.EXPECT().BootstrapInfo(gomock.Any()).Return(params.GetBootstrapInfoResponse{
		Status:    params.StatusRunning,
		Logs:      []string{"log1", "log2"},
		Watermark: 2,
	}, nil)
	s.client.EXPECT().Close().Return(nil)
	s.writer.EXPECT().Write([]byte("log1\n"))
	s.writer.EXPECT().Write([]byte("log2\n"))

	s.client.EXPECT().
		BootstrapInfo(&params.GetBootstrapInfoRequest{
			JobID:     "test-job-id",
			Watermark: 2,
		}).
		Return(params.GetBootstrapInfoResponse{
			Status:    params.StatusRunning,
			Logs:      []string{"log3"},
			Watermark: 3,
		}, nil)
	s.writer.EXPECT().Write([]byte("log3\n"))

	s.client.EXPECT().
		BootstrapInfo(&params.GetBootstrapInfoRequest{
			JobID:     "test-job-id",
			Watermark: 3,
		}).
		Return(params.GetBootstrapInfoResponse{
			Status: params.StatusSuccessful,
		}, nil)
	s.writer.EXPECT().Write([]byte("Job completed successfully.\n"))

	command := &bootstrapStatusCommand{
		jobId:               "test-job-id",
		sleepBetweenGetLogs: 0,
		follow:              true,
	}
	command.setJIMMAPI(s.client)
	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}
	err := command.Run(ctx)
	c.Assert(err, gc.IsNil)
}

func (s *bootstrapStatusSuite) TestBootstrapStatus_NoFollow(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.client.EXPECT().BootstrapInfo(gomock.Any()).Return(params.GetBootstrapInfoResponse{
		Status:    params.StatusRunning,
		Logs:      []string{"log1", "log2"},
		Watermark: 2,
	}, nil)
	s.client.EXPECT().Close().Return(nil)
	s.writer.EXPECT().Write([]byte("log1\n"))
	s.writer.EXPECT().Write([]byte("log2\n"))

	command := &bootstrapStatusCommand{
		jobId:               "test-job-id",
		sleepBetweenGetLogs: 0,
		follow:              false,
	}
	command.setJIMMAPI(s.client)
	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}
	// Test that it does not wait for further logs.
	// It should return after the first status check.
	err := command.Run(ctx)
	c.Assert(err, gc.IsNil)
}

func (s *bootstrapStatusSuite) TestBootstrapStatus_AfterCompletion(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.client.EXPECT().BootstrapInfo(gomock.Any()).Return(params.GetBootstrapInfoResponse{
		Status:    params.StatusSuccessful,
		Logs:      []string{"log1", "log2"},
		Watermark: 2,
	}, nil)
	s.client.EXPECT().Close().Return(nil)
	s.writer.EXPECT().Write([]byte("log1\n"))
	s.writer.EXPECT().Write([]byte("log2\n"))
	s.writer.EXPECT().Write([]byte("Job completed successfully.\n"))

	command := &bootstrapStatusCommand{
		jobId:               "test-job-id",
		sleepBetweenGetLogs: 0,
		follow:              true,
	}
	command.setJIMMAPI(s.client)
	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}
	err := command.Run(ctx)
	c.Assert(err, gc.IsNil)
}
