package workers

import (
	"time"

	"github.com/riverqueue/river/rivertype"
)

const (
	AddModelTimeout   = 5 * time.Minute
	AddModelDelayStep = 5
)

// WaitConfig is used to set the waiting duration for river jobs.
type WaitConfig struct {
	Duration time.Duration
}

// LinearRetryPolicy delays subsequent retries by DelayStep seconds for each time
// For DelayStep 5s, the retry delays will be (5s, 10s, 15s, etc.).
type LinearRetryPolicy struct {
	// DelayStep configures the number of additional seconds to add before each retry
	DelayStep int
}

// NewLinearRetryPolicy returns the a new ClientRetryPolicy that uses a linear schedule
// First retry waits 1 * DelayStep, then 2 * DelayStep, and so on.
func NewLinearRetryPolicy(DelayStep int) *LinearRetryPolicy {
	return &LinearRetryPolicy{DelayStep: DelayStep}
}

// NextAt returns the next retry time based on the non-generic JobRow
// which includes an up-to-date Errors list.
func (policy *LinearRetryPolicy) NextAt(job *rivertype.JobRow) time.Time {
	return time.Now().Add(time.Duration(len(job.Errors)*policy.DelayStep) * time.Second)
}
