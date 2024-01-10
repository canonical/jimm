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
	Cloud     Cloud `gorm:"foreignKey:CloudName;references:Name;constraint:OnDelete:CASCADE"`

	// Owner is the user that owns this credential.
	Owner         *User `gorm:"foreignKey:OwnerUsername;references:Username"`
	OwnerUsername *string

	// OwnerServiceAccount is the service account that owns this credential.
	OwnerServiceAccount *ServiceAccount `gorm:"foreignKey:OwnerClientID;references:ClientID"`
	OwnerClientID       *string

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

	// Models contains the models using this credential.
	Models []Model
}

// Tag returns a names.Tag for the cloud-credential.
func (c CloudCredential) Tag() names.Tag {
	return c.ResourceTag()
}

// ResourceTag returns a tag for the cloud-credential.  This method
// is intended to be used in places where we expect to see
// a concrete type names.CloudCredentialTag instead of the
// names.Tag interface.
func (c CloudCredential) ResourceTag() names.CloudCredentialTag {
	// TODO (babakks): we should use the correct owner (user or service account).
	// For now we just assume the owner is a User (not a service account).
	return names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s", c.CloudName, *c.OwnerUsername, c.Name))
}

// SetTag sets the Name, CloudName and Username fields from the given tag.
func (c *CloudCredential) SetTag(t names.CloudCredentialTag) {
	c.CloudName = t.Cloud().Id()
	c.Name = t.Name()
	// TODO (babakks): we should set the Owner based on the tag's Owner field; which can be a user or service account.
	// For now we just assume the owner is a User (not a service account).
	owner := t.Owner().Id()
	c.OwnerUsername = &owner
}

// Path returns a juju style cloud credential path.
func (c CloudCredential) Path() string {
	// TODO (babakks): we should use the correct owner (user or service account).
	// For now we just assume the owner is a User (not a service account).
	return fmt.Sprintf("%s/%s/%s", c.CloudName, *c.OwnerUsername, c.Name)
}
