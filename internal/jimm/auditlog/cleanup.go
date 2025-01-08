// Copyright 2025 Canonical.
package auditlog

import (
	"context"
	"time"

	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

// pollTimeOfDay holds the time hour, minutes and seconds to poll at.
type pollTimeOfDay struct {
	Hours   int
	Minutes int
	Seconds int
}

var pollDuration = pollTimeOfDay{
	Hours: 9,
}

// StartCleanup starts a routine which checks daily for any logs
// needed to be cleaned up.
func (j *auditLogManager) StartCleanup(ctx context.Context) {
	if j.retentionPeriodInDays == 0 {
		return
	}
	go j.poll(ctx)
}

// poll is designed to be run in a routine where it can be cancelled safely
// from the service's context. It calculates the poll duration at 9am each day
// UTC.
func (j *auditLogManager) poll(ctx context.Context) {

	for {
		select {
		case <-time.After(calculateNextPollDuration(time.Now().UTC())):
			retentionDate := time.Now().AddDate(0, 0, -(j.retentionPeriodInDays))
			deleted, err := j.store.DeleteAuditLogsBefore(ctx, retentionDate)
			if err != nil {
				zapctx.Error(ctx, "failed to cleanup audit logs", zap.Error(err))
				continue
			}
			zapctx.Debug(ctx, "audit log cleanup run successfully", zap.Int64("count", deleted))
		case <-ctx.Done():
			zapctx.Debug(ctx, "exiting audit log cleanup polling")
			return
		}
	}
}

// calculateNextPollDuration returns the next duration to poll on.
// We recalculate each time and not rely on running every 24 hours
// for absolute consistency within ns apart.
func calculateNextPollDuration(startingTime time.Time) time.Duration {
	now := startingTime
	pollTime := time.Date(now.Year(), now.Month(), now.Day(), pollDuration.Hours, pollDuration.Minutes, pollDuration.Seconds, 0, time.UTC)
	tillNextPoll := pollTime.Sub(now)
	var d time.Duration
	// If the next poll time is behind the current time
	if tillNextPoll < 0 {
		// Add 24 hours, flip it to an absolute duration, i.e., -10h == 10h
		// and subtract it from 24 hours to calculate the poll time for tomorrow
		d = time.Hour*24 - tillNextPoll.Abs()
	} else {
		d = tillNextPoll.Abs()
	}
	return d
}
