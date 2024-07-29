// Copyright 2024 Canonical Ltd.

package dbmodel

import (
	"errors"

	"gorm.io/gorm"
)

// NewUserSSHKeys returns an empty slice of userSSHKey's to be populated
// from the database.
func NewUserSSHKeys() []userSSHKey {
	return []userSSHKey{}
}

// NewUserSSHKey returns a model for creating and retrieving a users ssh key.
func NewUserSSHKey(identityName string, key string) (*userSSHKey, error) {
	if identityName == "" {
		return nil, errors.New("identity name cannot be empty")
	}
	return &userSSHKey{
		IdentityName: identityName,
		SSHKey:       key,
	}, nil
}

// userSSHKey holds an SSH key for a user.
type userSSHKey struct {
	gorm.Model

	// IdentityName is the unique name (email or client id) of this entity.
	IdentityName string
	Identity     Identity `gorm:"foreignKey:IdentityName;references:Name"`

	// Keys holds the users SSH keys.
	SSHKey string `gorm:"type:text"`
}
