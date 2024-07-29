// Copyright 2024 Canonical Ltd.

package dbmodel

import (
	"errors"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// NewUserSSHKeys returns a model for creating and retrieving users ssh keys.
func NewUserSSHKeys(identityName string, keys []string) (*userSSHKeys, error) {
	if identityName == "" {
		return nil, errors.New("identity name cannot be empty")
	}
	return &userSSHKeys{
		IdentityName: identityName,
		Keys:         keys,
	}, nil
}

// userSSHKey holds the SSH key for a user.
type userSSHKeys struct {
	gorm.Model

	// IdentityName is the unique name (email or client id) of this entity.
	IdentityName string
	Identity     Identity `gorm:"foreignKey:IdentityName;references:Name"` // Association setup

	// Key holds the users SSH keys.
	Keys pq.StringArray `gorm:"type:text[]"`
}
