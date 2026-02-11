// Copyright 2026 Canonical.

package cmd

import (
	"fmt"
	"io"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	jobStatusCommandDoc = `
Displays logs for a job.
`
	jobStatusCommandExample = `
    juju job-status 2cb433a6-04eb-4ec4-9567-90426d20a004
`
)

// sleepBetweenGetLogs is the duration to wait between successive calls to get logs for a job.
const sleepBetweenGetLogs = 1 * time.Second

// NewJobStatusCommand returns a command to display logs for a job.
func NewJobStatusCommand() cmd.Command {
	cmd := &jobStatusCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// jobStatusCommand displays logs for a job.
type jobStatusCommand struct {
	jaasCommandBase

	jobId string

	sleepBetweenGetLogs time.Duration
	follow              bool
}

// Info implements cmd.Info interface.
func (c *jobStatusCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "job-status",
		Args:     "<job uuid>",
		Purpose:  "Displays logs for a job",
		Doc:      jobStatusCommandDoc,
		Examples: jobStatusCommandExample,
	})
}

// SetFlags implements cmd.SetFlags interface.
func (c *jobStatusCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.BoolVar(&c.follow, "f", false, "follow the logs of the job")
}

// Init implements the cmd.Command interface.
func (c *jobStatusCommand) Init(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("missing job id")
	}
	c.jobId, args = args[0], args[1:]
	if len(args) > 0 {
		return fmt.Errorf("unknown arguments")
	}

	c.sleepBetweenGetLogs = sleepBetweenGetLogs
	return nil
}

// Run implements cmd.Command.Run interface.
func (c *jobStatusCommand) Run(ctxt *cmd.Context) error {
	client, err := c.getJIMMAPI()
	if err != nil {
		return fmt.Errorf("failed to create JIMM client: %v", err)
	}
	defer client.Close()

	poller := logPoller{
		client:              client,
		jobId:               c.jobId,
		sleepBetweenGetLogs: c.sleepBetweenGetLogs,
		out:                 ctxt.Stdout,
		follow:              c.follow,
	}
	return poller.watchJobLogs()
}

type logPoller struct {
	client              JIMMAPI
	jobId               string
	sleepBetweenGetLogs time.Duration
	out                 io.Writer
	follow              bool
}

func (p logPoller) watchJobLogs() error {
	watermark := 0

	for {
		response, err := p.client.GetJobInfo(&params.GetJobInfoRequest{
			JobID:     p.jobId,
			Watermark: watermark,
		})
		if err != nil {
			return fmt.Errorf("failed to get job info: %w", err)
		}
		for _, log := range response.Logs {
			_, err = p.out.Write([]byte(log + "\n"))
			if err != nil {
				return fmt.Errorf("failed to write job log: %w", err)
			}
		}
		watermark = response.Watermark
		switch response.Status {
		case params.StatusRunning:
			// If the job is still running, we just continue to the next iteration.
		case params.StatusSuccessful:
			_, err = p.out.Write([]byte("Job completed successfully.\n"))
			if err != nil {
				return fmt.Errorf("failed to write job success message: %w", err)
			}
			return nil
		case params.StatusFailed:
			_, err = p.out.Write([]byte("Job failed: " + response.Error + "\n"))
			if err != nil {
				return fmt.Errorf("failed to write job error: %w", err)
			}
			return nil
		case params.StatusPending:
			_, err := p.out.Write([]byte("Job is pending...\n"))
			if err != nil {
				return fmt.Errorf("failed to write job pending message: %w", err)
			}
		default:
			return fmt.Errorf("unknown job status: %s", response.Status)
		}
		if !p.follow {
			return nil
		}
		time.Sleep(p.sleepBetweenGetLogs)
	}
}
