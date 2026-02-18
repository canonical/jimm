// Copyright 2025 Canonical.

package auditlog

import "context"

type PollTimeOfDay = pollTimeOfDay

var (
	CalculateNextPollDuration = calculateNextPollDuration
	RedactSensitiveParams     = redactSensitiveParams
	RedactJSON                = redactJSON
	SensitiveMethods          = &sensitiveMethods
)

func (j *AuditLogManager) Cleanup(ctx context.Context) {
	j.cleanup(ctx)
}
