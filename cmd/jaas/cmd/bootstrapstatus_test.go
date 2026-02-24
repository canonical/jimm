// Copyright 2026 Canonical.

package cmd

import (
	"testing"
	"testing/synctest"

	qt "github.com/frankban/quicktest"
	"github.com/juju/cmd/v3/cmdtesting"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/pkg/api/params"
)

func TestBootstrapStatus(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	s.client.EXPECT().BootstrapInfo(gomock.Any()).Return(params.GetBootstrapInfoResponse{
		Status: params.StatusSuccessful,
	}, nil)
	s.client.EXPECT().Close().Return(nil)

	command := &bootstrapStatusCommand{}
	command.setJIMMAPI(s.client)
	initCommand(c, command, "test-job-id", "-f")

	ctx := newTestContext(c)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	stdout := cmdtesting.Stdout(ctx)
	c.Assert(stdout, qt.Equals, "Job completed successfully.\n")
}

func TestBootstrapStatus_Failed(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	s.client.EXPECT().BootstrapInfo(gomock.Any()).Return(params.GetBootstrapInfoResponse{
		Status: params.StatusFailed,
		Error:  "Job failed",
	}, nil)
	s.client.EXPECT().Close().Return(nil)

	command := &bootstrapStatusCommand{}
	command.setJIMMAPI(s.client)
	initCommand(c, command, "test-job-id", "-f")

	ctx := newTestContext(c)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	stdout := cmdtesting.Stdout(ctx)
	c.Assert(stdout, qt.Equals, "Job failed: Job failed\n")
}

func TestBootstrapStatus_Running(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	s.client.EXPECT().BootstrapInfo(gomock.Any()).Return(params.GetBootstrapInfoResponse{
		Status:    params.StatusRunning,
		Logs:      []string{"log1", "log2"},
		Watermark: 2,
	}, nil)
	s.client.EXPECT().Close().Return(nil)

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

	s.client.EXPECT().
		BootstrapInfo(&params.GetBootstrapInfoRequest{
			JobID:     "test-job-id",
			Watermark: 3,
		}).
		Return(params.GetBootstrapInfoResponse{
			Status: params.StatusSuccessful,
		}, nil)

	command := &bootstrapStatusCommand{
		sleepBetweenGetLogs: 0,
	}
	command.setJIMMAPI(s.client)
	initCommand(c, command, "test-job-id", "-f")

	ctx := newTestContext(c)
	// Skip the sleep time by using synctest.
	synctest.Test(t, func(t *testing.T) {
		err := command.Run(ctx)
		c.Assert(err, qt.IsNil)
	})

	stdout := cmdtesting.Stdout(ctx)
	c.Assert(stdout, qt.Equals, "log1\nlog2\nlog3\nJob completed successfully.\n")
}

func TestBootstrapStatus_NoFollow(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	s.client.EXPECT().BootstrapInfo(gomock.Any()).Return(params.GetBootstrapInfoResponse{
		Status:    params.StatusRunning,
		Logs:      []string{"log1", "log2"},
		Watermark: 2,
	}, nil)
	s.client.EXPECT().Close().Return(nil)

	command := &bootstrapStatusCommand{
		sleepBetweenGetLogs: 0,
	}
	command.setJIMMAPI(s.client)
	initCommand(c, command, "test-job-id")

	// Test that it does not wait for further logs.
	// It should return after the first status check.
	ctx := newTestContext(c)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	stdout := cmdtesting.Stdout(ctx)
	c.Assert(stdout, qt.Equals, "log1\nlog2\n")
}

func TestBootstrapStatus_AfterCompletion(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	s.client.EXPECT().BootstrapInfo(gomock.Any()).Return(params.GetBootstrapInfoResponse{
		Status:    params.StatusSuccessful,
		Logs:      []string{"log1", "log2"},
		Watermark: 2,
	}, nil)
	s.client.EXPECT().Close().Return(nil)

	command := &bootstrapStatusCommand{
		sleepBetweenGetLogs: 0,
	}
	command.setJIMMAPI(s.client)
	initCommand(c, command, "test-job-id", "-f")

	ctx := newTestContext(c)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	stdout := cmdtesting.Stdout(ctx)
	c.Assert(stdout, qt.Equals, "log1\nlog2\nJob completed successfully.\n")
}
