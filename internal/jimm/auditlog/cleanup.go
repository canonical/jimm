// Copyright 2025 Canonical.

package auditlog

import (
	"context"
	"time"

	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

// auditLogCleanupTime indicates that we poll at 9 AM.
var auditLogCleanupTime = pollTimeOfDay{
	Hours: 9,
}

// StartCleanup loop forever and checks daily for any logs
// that need to be cleaned up. This method should be run
// in a separate Go routine to avoid blocking, it will terminate
// when the provided context is cancelled.
func (j *auditLogManager) StartCleanup(ctx context.Context) {
	if j.retentionPeriodInDays == 0 {
		return
	}
	for {
		select {
		case <-time.After(calculateNextPollDuration(auditLogCleanupTime, time.Now().UTC())):
			j.cleanup(ctx)
		case <-ctx.Done():
			zapctx.Debug(ctx, "exiting audit log cleanup polling")
			return
		}
	}
}

// pollTimeOfDay holds the time hour, minutes and seconds to poll for cleanup.
type pollTimeOfDay struct {
	Hours   int
	Minutes int
	Seconds int
}

func (j *auditLogManager) cleanup(ctx context.Context) {
	retentionDate := time.Now().AddDate(0, 0, -(j.retentionPeriodInDays))
	deleted, err := j.store.DeleteAuditLogsBefore(ctx, retentionDate)
	if err != nil {
		zapctx.Error(ctx, "failed to cleanup audit logs", zap.Error(err))
	}
	zapctx.Debug(ctx, "audit log cleanup run successfully", zap.Int64("count", deleted))
}

// calculateNextPollDuration returns the next duration to poll on.
// We recalculate each time and not rely on running every 24 hours
// for absolute consistency within ns apart.
func calculateNextPollDuration(pollTime pollTimeOfDay, startingTime time.Time) time.Duration {
	now := startingTime
	pollTimeToday := time.Date(now.Year(), now.Month(), now.Day(), pollTime.Hours, pollTime.Minutes, pollTime.Seconds, 0, time.UTC)
	tillNextPoll := pollTimeToday.Sub(now)
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
