// Copyright 2024 Canonical Ltd.

package dbmodel

import (
	"database/sql"

	"gorm.io/gorm"
)

// ServiceAccount represents a service account, an OIDC/OAUTH concept.
type ServiceAccount struct {
	gorm.Model

	// ClientID is the service account ClientID.
	ClientID string `gorm:"not null;uniqueIndex"`

	// DisplayName is the name that should be displayed for the service account.
	DisplayName string `gorm:"not null"`

	// LastLogin is the time the service account last authenticated to the JIMM
	// server. LastLogin will only be a valid time if the account has
	// authenticated at least once.
	LastLogin sql.NullTime

	// CloudCredentials are the cloud credentials owned by this service account.
	CloudCredentials []CloudCredential `gorm:"foreignKey:OwnerClientID;references:ClientID"`
}
