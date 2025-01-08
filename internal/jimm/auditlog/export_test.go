// Copyright 2025 Canonical.

package auditlog

import "context"

// AuditLogManager is a type alias to export auditLogManager for use in tests.
type AuditLogManager = auditLogManager
type PollTimeOfDay = pollTimeOfDay

var (
	CalculateNextPollDuration = calculateNextPollDuration
)

func (j *auditLogManager) Cleanup(ctx context.Context) {
	j.cleanup(ctx)
}
