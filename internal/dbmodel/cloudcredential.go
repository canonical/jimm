// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"database/sql"
	"fmt"

	"github.com/juju/names/v4"
	"gorm.io/gorm"
)

// A CloudCredential is a credential that is used to access a cloud.
type CloudCredential struct {
	gorm.Model

	// Name is the name of the credential.
	Name string

	// Cloud is the cloud this credential is for.
	CloudName string
	Cloud     Cloud `gorm:"foreignKey:CloudName;references:Name"`

	// Owner is the user that owns this credential.
	OwnerUsername string
	Owner         User `gorm:"foreignKey:OwnerUsername;references:Username"`

	// AuthType is the type of the credential.
	AuthType string

	// Label is an optional label for the credential.
	Label string

	// AttributesInVault indicates whether the attributes are stored in
	// the configured vault key-value store, rather than this database.
	AttributesInVault bool

	// Attributes contains the attributes of the credential.
	Attributes StringMap

	// Valid stores whether the cloud-credential is known to be valid.
	Valid sql.NullBool
}

// Tag returns a names.Tag for the cloud-credential.
func (c CloudCredential) Tag() names.Tag {
	return names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s", c.CloudName, c.OwnerUsername, c.Name))
}

// SetTag sets the Name, CloudName and Username fields from the given tag.
func (c *CloudCredential) SetTag(t names.CloudCredentialTag) {
	c.CloudName = t.Cloud().Id()
	c.Name = t.Name()
	c.OwnerUsername = t.Owner().Id()
}

// Path returns a juju style cloud credential path.
func (c CloudCredential) Path() string {
	return fmt.Sprintf("%s/%s/%s", c.CloudName, c.OwnerUsername, c.Name)
}
