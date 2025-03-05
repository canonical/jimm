// Copyright 2025 Canonical.

package dbmodel

import "time"

// SSHKey holds a user's public SSH key.
type SSHKey struct {
	// Note this doesn't use the standard gorm.Model to avoid soft-deletes.

	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// IdentityName is the unique name (email or client-id) of this entity.
	IdentityName string   `gorm:"uniqueIndex:unique_identity_ssh_key"`
	Identity     Identity `gorm:"foreignKey:IdentityName;references:Name"`

	// IdentityName is the unique name (email or client-id) of this entity.
	ModelUUID string `gorm:"uniqueIndex:unique_identity_ssh_key"`
	Model     Model  `gorm:"foreignKey:ModelUUID;references:uuid"`

	// PublicKey holds the user's public SSH key.
	PublicKey []byte `gorm:"uniqueIndex:unique_identity_ssh_key"`
	// MD5Fingerprint is the MD5 fingerprint of the public key.
	MD5Fingerprint string
	// KeyComment holds a user provided comment.
	KeyComment string
}
