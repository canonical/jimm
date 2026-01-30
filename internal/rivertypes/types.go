package rivertypes

import (
	"github.com/juju/version/v2"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// UpgradeToArgs are the arguments for the upgrade-to worker.
type UpgradeToArgs struct {
	ModelUUID            string         `json:"model-uuid" river:"unique"`
	TargetVersion        version.Number `json:"target-version"`
	Username             string         `json:"username"`
	TargetControllerName string         `json:"target_controller_name"`
}

// Kind implements the [river.JobArgs] interface.
func (UpgradeToArgs) Kind() string { return "upgrade-to" }

// InsertOpts implements the [river.JobArgsWithInsertOpts] interface.
func (UpgradeToArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRunning,
				rivertype.JobStateRetryable,
				rivertype.JobStateScheduled,
			},
		},
	}
}
