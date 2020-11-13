// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"fmt"

	"github.com/juju/names/v4"
	"gorm.io/gorm"
)

// A CloudCredential is a credential that is used to access a cloud.
type CloudCredential struct {
	gorm.Model

	// Name is the name of the credential.
	Name string `gorm:"not null;uniqueIndex:idx_cloud_owner_name"`

	// Cloud is the cloud this credential is for.
	CloudName string `gorm:"not null;uniqueIndex:idx_cloud_owner_name"`
	Cloud     Cloud  `gorm:"foreignKey:CloudName;references:Name"`

	// User is the user that owns this credential.
	OwnerID string `gorm:"not null;uniqueIndex:idx_cloud_owner_name"`
	Owner   User   `gorm:"foreignKey:OwnerID;references:Username"`

	// AuthType is the type of the credential.
	AuthType string `gorm:"not null"`

	// Label is an optional label for the credential.
	Label string

	// AttributesInVault indicates whether the attributes are stored in
	// the configured vault key-value store, rather than this database.
	AttributesInVault bool `gorm:"not null"`

	// Attributes contains the attributes of the credential.
	Attributes StringMap
}

// Tag returns a names.Tag for the cloud-credential.
func (c CloudCredential) Tag() names.Tag {
	return names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s", c.CloudName, c.OwnerID, c.Name))
}

// SetTag sets the Name, CloudName and Username fields from the given tag.
func (c *CloudCredential) SetTag(t names.CloudCredentialTag) {
	c.CloudName = t.Cloud().Id()
	c.Name = t.Name()
	c.OwnerID = t.Owner().Id()
}
