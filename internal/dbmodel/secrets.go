package dbmodel

import "time"

// A Secret is a generic secret.
type Secret struct {
	// ID contains the ID of the entry.
	ID uint `gorm:"primarykey"`

	// Time contains the time the secret was created/updated.
	Time time.Time

	// Type contains the secret type, i.e. controller, cloudCredential, jwks, etc.
	Type string `gorm:"index:idx_secret_name,unique"`

	// Contains an identifier for the secret that is unique for a specific secret type.
	Tag string `gorm:"index:idx_secret_name,unique"`

	// Contains the secret data.
	Data JSON
}
