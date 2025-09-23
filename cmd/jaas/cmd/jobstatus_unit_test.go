// Copyright 2025 Canonical.

package cmd

import (
	"context"

	"github.com/juju/cmd/v3"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd/mocks"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

type jobStatusSuite struct {
	client *mocks.MockJIMMAPI
	writer *mocks.MockWriter
}

var _ = gc.Suite(&jobStatusSuite{})

func (s *jobStatusSuite) SetupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.client = mocks.NewMockJIMMAPI(ctrl)
	s.writer = mocks.NewMockWriter(ctrl)

	return ctrl
}

func (s *jobStatusSuite) TestJobStatus(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.client.EXPECT().GetJobInfo(gomock.Any()).Return(params.GetJobInfoResponse{
		Status: params.StatusSuccessful,
	}, nil)
	s.client.EXPECT().Close().Return(nil)
	s.writer.EXPECT().Write([]byte("Job completed successfully.\n"))

	command := &jobStatusCommand{
		jobAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
		jobId:               "test-job-id",
		sleepBetweenGetLogs: 0,
		follow:              true,
	}
	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}
	err := command.Run(ctx)
	c.Assert(err, gc.IsNil)
}

func (s *jobStatusSuite) TestJobStatus_Failed(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.client.EXPECT().GetJobInfo(gomock.Any()).Return(params.GetJobInfoResponse{
		Status: params.StatusFailed,
		Error:  "Job failed",
	}, nil)
	s.client.EXPECT().Close().Return(nil)
	s.writer.EXPECT().Write([]byte("Job failed: Job failed\n"))

	command := &jobStatusCommand{
		jobAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
		jobId:               "test-job-id",
		sleepBetweenGetLogs: 0,
		follow:              true,
	}
	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}
	err := command.Run(ctx)
	c.Assert(err, gc.IsNil)
}

func (s *jobStatusSuite) TestJobStatus_Running(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.client.EXPECT().GetJobInfo(gomock.Any()).Return(params.GetJobInfoResponse{
		Status:    params.StatusRunning,
		Logs:      []string{"log1", "log2"},
		Watermark: 2,
	}, nil)
	s.client.EXPECT().Close().Return(nil)
	s.writer.EXPECT().Write([]byte("log1\n"))
	s.writer.EXPECT().Write([]byte("log2\n"))

	s.client.EXPECT().
		GetJobInfo(&params.GetJobInfoRequest{
			JobID:     "test-job-id",
			Watermark: 2,
		}).
		Return(params.GetJobInfoResponse{
			Status:    params.StatusRunning,
			Logs:      []string{"log3"},
			Watermark: 3,
		}, nil)
	s.writer.EXPECT().Write([]byte("log3\n"))

	s.client.EXPECT().
		GetJobInfo(&params.GetJobInfoRequest{
			JobID:     "test-job-id",
			Watermark: 3,
		}).
		Return(params.GetJobInfoResponse{
			Status: params.StatusSuccessful,
		}, nil)
	s.writer.EXPECT().Write([]byte("Job completed successfully.\n"))

	command := &jobStatusCommand{
		jobAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
		jobId:               "test-job-id",
		sleepBetweenGetLogs: 0,
		follow:              true,
	}
	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}
	err := command.Run(ctx)
	c.Assert(err, gc.IsNil)
}

func (s *jobStatusSuite) TestJobStatus_NoFollow(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.client.EXPECT().GetJobInfo(gomock.Any()).Return(params.GetJobInfoResponse{
		Status:    params.StatusRunning,
		Logs:      []string{"log1", "log2"},
		Watermark: 2,
	}, nil)
	s.client.EXPECT().Close().Return(nil)
	s.writer.EXPECT().Write([]byte("log1\n"))
	s.writer.EXPECT().Write([]byte("log2\n"))

	command := &jobStatusCommand{
		jobAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
		jobId:               "test-job-id",
		sleepBetweenGetLogs: 0,
		follow:              false,
	}
	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}
	// Test that it does not wait for further logs.
	// It should return after the first status check.
	err := command.Run(ctx)
	c.Assert(err, gc.IsNil)
}

func (s *jobStatusSuite) TestJobStatus_AfterCompletion(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.client.EXPECT().GetJobInfo(gomock.Any()).Return(params.GetJobInfoResponse{
		Status:    params.StatusSuccessful,
		Logs:      []string{"log1", "log2"},
		Watermark: 2,
	}, nil)
	s.client.EXPECT().Close().Return(nil)
	s.writer.EXPECT().Write([]byte("log1\n"))
	s.writer.EXPECT().Write([]byte("log2\n"))
	s.writer.EXPECT().Write([]byte("Job completed successfully.\n"))

	command := &jobStatusCommand{
		jobAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
		jobId:               "test-job-id",
		sleepBetweenGetLogs: 0,
		follow:              true,
	}
	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}
	err := command.Run(ctx)
	c.Assert(err, gc.IsNil)
}
