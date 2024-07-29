// Copyright 2024 Canonical Ltd.

package dbmodel

import (
	"errors"

	"gorm.io/gorm"
)

// NewUserSSHKey returns a model for creating and retrieving a users ssh key.
func NewUserSSHKey(identityName string, key string) (*UserSSHKey, error) {
	if identityName == "" {
		return nil, errors.New("identity name cannot be empty")
	}
	if key == "" {
		return nil, errors.New("key cannot be empty")
	}
	return &UserSSHKey{
		IdentityName: identityName,
		SSHKey:       key,
	}, nil
}

// UserSSHKey holds an SSH key for a user.
type UserSSHKey struct {
	gorm.Model

	// IdentityName is the unique name (email or client id) of this entity.
	IdentityName string   `gorm:"uniqueIndex:unique_identity_ssh_key"`
	Identity     Identity `gorm:"foreignKey:IdentityName;references:Name"`

	// SSHKey holds the users SSH key.
	SSHKey string `gorm:"uniqueIndex:unique_identity_ssh_key;type:text"`
}
