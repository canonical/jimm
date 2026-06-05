// Copyright 2026 Canonical.

package river

import (
	"time"

	riverqueue "github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/canonical/jimm/v3/internal/rivertypes"
)

const (
	upgradeFlowRetryAfterFirstFailure  = 30 * time.Second
	upgradeFlowRetryAfterSecondFailure = 2 * time.Minute
)

type upgradeFlowRetryPolicy struct {
	defaultPolicy riverqueue.ClientRetryPolicy
	timeNow       func() time.Time
}

// newUpgradeFlowRetryPolicy creates a new upgradeFlowRetryPolicy
// which retries upgrade-to style jobs with a fixed delay, and falls back
// to the default retry policy for other jobs.
func newUpgradeFlowRetryPolicy() *upgradeFlowRetryPolicy {
	return &upgradeFlowRetryPolicy{
		defaultPolicy: &riverqueue.DefaultClientRetryPolicy{},
		timeNow:       time.Now,
	}
}

func (p *upgradeFlowRetryPolicy) NextRetry(job *rivertype.JobRow) time.Time {
	if !isUpgradeFlowJobKind(job.Kind) {
		return p.defaultRetryPolicy().NextRetry(job)
	}

	delay, ok := upgradeFlowRetryDelay(job.Attempt)
	if !ok {
		return p.defaultRetryPolicy().NextRetry(job)
	}

	return p.timeNowUTC().Add(delay)
}

func (p *upgradeFlowRetryPolicy) defaultRetryPolicy() riverqueue.ClientRetryPolicy {
	if p.defaultPolicy != nil {
		return p.defaultPolicy
	}
	return &riverqueue.DefaultClientRetryPolicy{}
}

func (p *upgradeFlowRetryPolicy) timeNowUTC() time.Time {
	if p.timeNow != nil {
		return p.timeNow().UTC()
	}
	return time.Now().UTC()
}

func isUpgradeFlowJobKind(kind string) bool {
	switch kind {
	case rivertypes.UpgradeToJobKind, migrationWorkerJobKind, upgradeWorkerJobKind:
		return true
	default:
		return false
	}
}

func upgradeFlowRetryDelay(attempt int) (time.Duration, bool) {
	switch attempt {
	case 1:
		return upgradeFlowRetryAfterFirstFailure, true
	case 2:
		return upgradeFlowRetryAfterSecondFailure, true
	default:
		return upgradeFlowRetryAfterSecondFailure, false
	}
}
