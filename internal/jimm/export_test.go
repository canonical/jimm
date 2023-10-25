// Copyright 2020 Canonical Ltd.

package jimm

import "github.com/canonical/jimm/internal/dbmodel"

var (
	DetermineAccessLevelAfterRevoke = determineAccessLevelAfterRevoke
	DetermineAccessLevelAfterGrant  = determineAccessLevelAfterGrant
	FilterApplicationOfferDetail    = filterApplicationOfferDetail
)

func (j *JIMM) AddAuditLogEntry(ale *dbmodel.AuditLogEntry) {
	j.addAuditLogEntry(ale)
}
